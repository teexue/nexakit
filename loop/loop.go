package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/teexue/nexakit/compaction"
	"github.com/teexue/nexakit/event"
	"github.com/teexue/nexakit/hook"
	"github.com/teexue/nexakit/permission"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/tool"
)

// pendingResult holds a tool execution result for ordered collection.
type pendingResult struct {
	idx      int
	callID   string
	toolName string
	output   json.RawMessage
	args     json.RawMessage
	parts    []provider.ContentPart
}

// newPendingResult records a finished tool call at its original call index so
// parallel results can be reordered and both execution paths stay consistent.
func newPendingResult(idx int, call provider.ToolCall, res tool.Result) pendingResult {
	return pendingResult{
		idx: idx, callID: call.ID, toolName: call.Name,
		args: call.Arguments, output: res.Output, parts: res.ContentParts,
	}
}

// runEnv is the context-free dependency set shared by every turn of a run:
// the run's resolved defaults (logger, policy, hooks, approver, context
// window) plus its event sink and tool definitions. It is built once so that
// turns pass a context.Context as a parameter instead of storing it in a
// struct, and so a new dependency is added in one place rather than at every
// call site.
type runEnv struct {
	cfg      Config
	toolDefs []provider.ToolDefinition
	out      chan<- event.Event
	log      *slog.Logger
	pol      permission.Policy
	hooks    *hook.Chain
	approver Approver
	window   int
}

// Run executes the agent loop and streams events.
func Run(ctx context.Context, cfg Config) (<-chan event.Event, error) {
	if err := validateRunConfig(cfg); err != nil {
		return nil, err
	}
	cfg, err := prepareSession(cfg)
	if err != nil {
		return nil, err
	}
	toolDefs, err := cfg.Registry.Definitions(ExpandImpliedTools(cfg.Agent.Tools))
	if err != nil {
		return nil, err
	}
	seedMessages(cfg)
	cfg.imageKeepFrom = imageKeepFromAfterSeed(cfg)
	cfg.failStreak = &toolFailStreak{}
	cfg.failStreak.seed(cfg.Session.GetMessages())
	ctx = attachRunContext(ctx, cfg)

	out := make(chan event.Event)
	go func() {
		defer close(out)
		runLoop(ctx, cfg, toolDefs, out)

		if cfg.Store != nil {
			if err := cfg.Store.Save(cfg.Session); err != nil {
				slog.Warn("log.session.persist_failed", "session_id", cfg.Session.ID, "error", err)
			}
		}
	}()
	return out, nil
}

// newRunEnv resolves the run's defaults once: nil logger/policy/approver fall
// back to their defaults, and the context window is resolved here so it is not
// re-derived (e.g. via /api/show) on every turn.
func newRunEnv(ctx context.Context, cfg Config, toolDefs []provider.ToolDefinition, out chan<- event.Event) runEnv {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	pol := cfg.Policy
	if pol == nil {
		pol = permission.AllowAllPolicy{}
	}
	approver := cfg.Approver
	if approver == nil {
		approver = DenyAllApprover{}
	}
	return runEnv{
		cfg:      cfg,
		toolDefs: toolDefs,
		out:      out,
		log:      log,
		pol:      pol,
		hooks:    cfg.Hooks,
		approver: approver,
		window:   resolveContextWindow(ctx, cfg),
	}
}

func runLoop(ctx context.Context, cfg Config, toolDefs []provider.ToolDefinition, out chan<- event.Event) {
	maxTurns := cfg.Agent.MaxTurns // 0 = unlimited until model returns without tool calls
	env := newRunEnv(ctx, cfg, toolDefs, out)

	var totalInputTokens, totalOutputTokens, totalCacheRead, totalCacheCreation int
	var lastTurn tokenDelta // most recent completed turn's usage, for done events

	for turn := 1; maxTurns <= 0 || turn <= maxTurns; turn++ {
		select {
		case <-ctx.Done():
			emitCancelled(env.out, doneStats{
				sessionID: cfg.Session.ID, turn: turn,
				input: lastTurn.input, output: lastTurn.output,
				cacheRead: lastTurn.cacheRead, cacheCreation: lastTurn.cacheCreation,
				window: env.window, totalInput: totalInputTokens, totalOutput: totalOutputTokens,
			})
			cfg.Session.AddUsage(totalInputTokens, totalOutputTokens, totalCacheRead, totalCacheCreation, env.window)
			persistSession(cfg)
			return
		default:
		}

		tokens, done := executeTurn(ctx, env, turn, totalInputTokens, totalOutputTokens)
		lastTurn = tokens
		totalInputTokens += tokens.input
		totalOutputTokens += tokens.output
		totalCacheRead += tokens.cacheRead
		totalCacheCreation += tokens.cacheCreation
		// Persist after each completed turn so a page refresh mid-run
		// recovers the conversation up to the last finished turn via
		// GET /sessions/:id.
		persistSession(cfg)
		if done {
			cfg.Session.AddUsage(totalInputTokens, totalOutputTokens, totalCacheRead, totalCacheCreation, env.window)
			persistSession(cfg)
			return
		}
	}

	forceEmit(env.out, event.Event{Type: event.TypeError, Code: "max_turns", Message: fmt.Sprintf("exceeded max turns %d", maxTurns)})
	forceEmit(env.out, event.Event{Type: event.TypeDone, Status: "failed", Turns: maxTurns, InputTokens: lastTurn.input, OutputTokens: lastTurn.output, ContextWindow: env.window, SessionID: cfg.Session.ID, CacheReadInputTokens: lastTurn.cacheRead, CacheCreationInputTokens: lastTurn.cacheCreation, TotalInputTokens: totalInputTokens, TotalOutputTokens: totalOutputTokens})
	cfg.Session.AddUsage(totalInputTokens, totalOutputTokens, totalCacheRead, totalCacheCreation, env.window)
	persistSession(cfg)
}

type tokenDelta struct {
	input, output int
	cacheRead     int
	cacheCreation int
}

// doneInputs bundles the values a done event needs, keeping
// completedDoneEvent within the parameter limit.
type doneInputs struct {
	turn         int
	totalInput   int
	totalOutput  int
	tokens       tokenDelta
	finishReason string
}

// executeTurn executes a single turn of the agent loop. Returns token deltas and
// whether the loop should terminate (text-only response, cancellation, or error).
func executeTurn(ctx context.Context, env runEnv, turn, totalInput, totalOutput int) (tokenDelta, bool) {
	fireOnTurnStart(env.hooks, turn, env.log)

	// Message count at request time; used to record real usage so the next
	// compaction can project the delta appended after this response.
	reqMsgCount := len(env.cfg.Session.GetMessages())
	chunks, err := env.cfg.Provider.Stream(ctx, provider.Request{
		Model: env.cfg.Agent.Model,
		Messages: provider.DropImagesBefore(
			env.cfg.Session.GetMessages(), env.cfg.imageKeepFrom,
		),
		Tools: env.toolDefs, MaxTokens: env.cfg.Agent.MaxTokens,
		ContextWindow: env.window,
	})
	if err != nil {
		forceEmit(env.out, event.Event{Type: event.TypeError, Code: "provider_error", Message: err.Error()})
		forceEmit(env.out, event.Event{Type: event.TypeDone, Status: "failed", Turns: turn, ContextWindow: env.window, SessionID: env.cfg.Session.ID})
		return tokenDelta{}, true
	}

	text, reasoning, toolCalls, tokens, finishReason, cancelled := consumeStream(ctx, chunks, env.out)
	// Persist the real prompt token count reported by the provider. This is
	// the authoritative usage of the request that just completed.
	if tokens.input > 0 {
		env.cfg.Session.SetLastUsage(tokens.input, tokens.output, tokens.cacheRead, tokens.cacheCreation, reqMsgCount)
	}
	if cancelled {
		emitCancelled(env.out, doneStats{
			sessionID: env.cfg.Session.ID, turn: turn,
			input: tokens.input, output: tokens.output,
			cacheRead: tokens.cacheRead, cacheCreation: tokens.cacheCreation,
			window: env.window, totalInput: totalInput, totalOutput: totalOutput,
		})
		return tokens, true
	}

	if len(toolCalls) == 0 {
		env.cfg.Session.AddMessages(provider.Message{
			Role: provider.RoleAssistant, Content: text, ReasoningContent: reasoning,
		})
		forceEmit(env.out, completedDoneEvent(env, doneInputs{
			turn: turn, totalInput: totalInput, totalOutput: totalOutput,
			tokens: tokens, finishReason: finishReason,
		}))
		// Compact even for plain text turns — otherwise pure chat sessions
		// (no tool calls) never trigger context management.
		compactIfNeeded(ctx, env.cfg, env.out, compactHint{turn: turn, log: env.log, window: env.window})
		return tokens, true
	}

	env.cfg.Session.AddMessages(provider.Message{
		Role: provider.RoleAssistant, Content: text, ReasoningContent: reasoning, ToolCalls: toolCalls,
	})

	results := collectToolResults(ctx, env, toolCalls)
	fireOnTurnEnd(env.hooks, ctx, turn, env.log)
	recordToolResults(env.cfg, results, env.window)
	compactIfNeeded(ctx, env.cfg, env.out, compactHint{turn: turn, log: env.log, window: env.window})

	return tokens, false
}

// consumeStream reads provider chunks and collects text, reasoning, and tool calls.
func consumeStream(
	ctx context.Context,
	chunks <-chan provider.Chunk,
	out chan<- event.Event,
) (string, string, []provider.ToolCall, tokenDelta, string, bool) {
	var text, reasoning, finishReason string
	var toolCalls []provider.ToolCall
	var tokens tokenDelta

	for chunk := range chunks {
		tokens.input += chunk.InputTokens
		tokens.output += chunk.OutputTokens
		tokens.cacheRead += chunk.CacheReadInputTokens
		tokens.cacheCreation += chunk.CacheCreationInputTokens
		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}

		if chunk.ReasoningDelta != "" {
			reasoning += chunk.ReasoningDelta
			emit(ctx, out, event.Event{Type: event.TypeReasoningDelta, Content: chunk.ReasoningDelta})
		}
		if chunk.TextDelta != "" {
			text += chunk.TextDelta
			emit(ctx, out, event.Event{Type: event.TypeTextDelta, Content: chunk.TextDelta})
		}
		toolCalls = append(toolCalls, chunk.ToolCalls...)
	}

	select {
	case <-ctx.Done():
		return text, reasoning, toolCalls, tokens, finishReason, true
	default:
	}

	return text, reasoning, toolCalls, tokens, finishReason, false
}

// collectToolResults gathers tool results in index order.
func collectToolResults(ctx context.Context, env runEnv, toolCalls []provider.ToolCall) []pendingResult {
	if env.cfg.Agent.ToolExecMode() == "parallel" {
		return collectParallelResults(ctx, env, toolCalls)
	}
	return collectSerialResults(ctx, env, toolCalls)
}

func collectParallelResults(ctx context.Context, env runEnv, toolCalls []provider.ToolCall) []pendingResult {
	maxParallel := env.cfg.Agent.ToolMaxParallel()
	type indexedResult struct {
		idx    int
		result pendingResult
	}
	resultCh := make(chan indexedResult, len(toolCalls))
	var wg sync.WaitGroup
	toolSem := make(chan struct{}, maxParallel)

	for i, call := range toolCalls {
		wg.Add(1)
		go func(call provider.ToolCall, i int) {
			defer wg.Done()
			release, ok := acquireToolSlot(ctx, toolSem, call.Name)
			if !ok {
				return
			}
			defer release()
			res := executeOneTool(ctx, env, call)
			select {
			case resultCh <- indexedResult{i, newPendingResult(i, call, res)}:
			case <-ctx.Done():
			}
		}(call, i)
	}

	go func() { wg.Wait(); close(resultCh) }()

	byIndex := make(map[int]pendingResult)
	for r := range resultCh {
		byIndex[r.idx] = r.result
	}
	results := make([]pendingResult, 0, len(toolCalls))
	for i := 0; i < len(toolCalls); i++ {
		if r, ok := byIndex[i]; ok {
			results = append(results, r)
		}
	}
	return results
}

// acquireToolSlot gates ordinary tools with toolSem. Delegate tools skip it —
// their concurrency is enforced inside subagent.Run.
func acquireToolSlot(ctx context.Context, toolSem chan struct{}, toolName string) (func(), bool) {
	if IsDelegateTool(toolName) {
		return func() {}, true
	}
	select {
	case <-ctx.Done():
		return nil, false
	case toolSem <- struct{}{}:
		return func() { <-toolSem }, true
	}
}

func collectSerialResults(ctx context.Context, env runEnv, toolCalls []provider.ToolCall) []pendingResult {
	results := make([]pendingResult, 0, len(toolCalls))
	for i, call := range toolCalls {
		res := executeOneTool(ctx, env, call)
		results = append(results, newPendingResult(i, call, res))
	}
	return results
}

func recordToolResults(cfg Config, results []pendingResult, window int) {
	budget := maxToolResultBytes
	if window > 0 {
		budget = toolResultBudget(currentPressure(cfg, window))
	}
	for _, tr := range results {
		content := truncateToolOutputBudget(string(tr.output), budget)
		if cfg.failStreak != nil {
			content = cfg.failStreak.annotate(tr.toolName, tr.args, tr.output, content)
		}
		cfg.Session.AddMessages(provider.Message{
			Role: provider.RoleTool, ToolCallID: tr.callID, Name: tr.toolName, Content: content,
		})
		// Vision APIs require image blocks on user/assistant turns, not as
		// JSON inside RoleTool. Attach them as a follow-up user message
		// (hidden from chat UIs via provider.IsToolImageUserMessage).
		if imgs := imageContentParts(tr.parts); len(imgs) > 0 {
			label := provider.ToolImageUserContent(tr.toolName)
			cfg.Session.AddMessages(provider.Message{
				Role:    provider.RoleUser,
				Content: label,
				ContentParts: append([]provider.ContentPart{{
					Type: "text", Text: label,
				}}, imgs...),
			})
		}
	}
}

// currentPressure estimates current context usage as a fraction of the
// effective context window, preferring the provider's last real input_tokens
// over a raw estimate (more accurate for CJK-heavy history).
func currentPressure(cfg Config, window int) float64 {
	if window <= 0 {
		return 0
	}
	lastInput, _, lastMsgCount := cfg.Session.LastUsage()
	msgs := cfg.Session.GetMessages()
	used := compaction.EstimateTokens(msgs)
	if lastInput > 0 && lastMsgCount > 0 && lastMsgCount < len(msgs) {
		used = lastInput + compaction.EstimateTokens(msgs[lastMsgCount:])
	}
	return float64(used) / float64(window)
}

func fireOnTurnStart(hooks *hook.Chain, turn int, log *slog.Logger) {
	if hooks != nil {
		if err := hooks.OnTurnStart(context.Background(), hook.TurnInfo{TurnNumber: turn}); err != nil {
			log.Warn("log.hook.turn_start_error", "turn", turn, "error", err)
		}
	}
}

func fireOnTurnEnd(hooks *hook.Chain, ctx context.Context, turn int, log *slog.Logger) {
	if hooks != nil {
		if err := hooks.OnTurnEnd(ctx, hook.TurnInfo{TurnNumber: turn}); err != nil {
			log.Warn("log.hook.turn_end_error", "turn", turn, "error", err)
		}
	}
}

// persistSession saves the current session state to the store, if configured.
// Failures are non-fatal: a stale on-disk copy is better than aborting a run.
func persistSession(cfg Config) {
	if cfg.Store == nil {
		return
	}
	if err := cfg.Store.Save(cfg.Session); err != nil {
		slog.Warn("log.session.persist_failed", "session_id", cfg.Session.ID, "error", err)
	}
}

type doneStats struct {
	sessionID     string
	turn          int
	input         int
	output        int
	cacheRead     int
	cacheCreation int
	window        int
	totalInput    int
	totalOutput   int
}

func completedDoneEvent(env runEnv, in doneInputs) event.Event {
	maxOut := provider.EffectiveMaxOutput(env.cfg.Agent.Model, env.cfg.Agent.MaxTokens)
	return event.Event{
		Type: event.TypeDone, Status: "completed", Turns: in.turn,
		InputTokens: in.tokens.input, OutputTokens: in.tokens.output,
		CacheReadInputTokens: in.tokens.cacheRead, CacheCreationInputTokens: in.tokens.cacheCreation,
		ContextWindow: env.window, SessionID: env.cfg.Session.ID,
		TotalInputTokens: in.totalInput, TotalOutputTokens: in.totalOutput,
		Truncated: provider.IsOutputTruncated(in.finishReason, in.tokens.output, maxOut),
	}
}

func emitCancelled(out chan<- event.Event, s doneStats) {
	forceEmit(out, event.Event{Type: event.TypeError, Code: "cancelled", Message: "context cancelled"})
	forceEmit(out, event.Event{
		Type: event.TypeDone, Status: "cancelled", Turns: s.turn,
		InputTokens: s.input, OutputTokens: s.output,
		CacheReadInputTokens: s.cacheRead, CacheCreationInputTokens: s.cacheCreation,
		ContextWindow: s.window, SessionID: s.sessionID,
		TotalInputTokens: s.totalInput, TotalOutputTokens: s.totalOutput,
	})
}
