package loop

import (
	"context"
	"log/slog"

	"github.com/teexue/nexakit/agent"
	"github.com/teexue/nexakit/compaction"
	"github.com/teexue/nexakit/event"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/session"
)

// resolveContextWindow resolves the model context window. An explicit agent
// value wins, then the model's official spec, then a conservative default.
// When the provider knows the model's real runtime limit (e.g. Ollama via
// /api/show), it overrides the static default so compaction and the server's
// actual context size agree.
func resolveContextWindow(ctx context.Context, cfg Config) int {
	configured := cfg.ContextWindow
	if cfg.Agent.Compaction != nil && cfg.Agent.Compaction.ContextWindow > 0 {
		configured = cfg.Agent.Compaction.ContextWindow
	}
	if r, ok := cfg.Provider.(provider.ContextResolver); ok {
		return r.ResolveContextWindow(ctx, cfg.Agent.Model, configured)
	}
	return provider.EffectiveContextWindow(cfg.Agent.Model, configured)
}

type compactHint struct {
	turn   int
	log    *slog.Logger
	window int
}

// compactSettings holds the compaction knobs resolved from agent config with
// their defaults applied, so compactIfNeeded does not repeat the nil fallbacks.
type compactSettings struct {
	strategy     compaction.Strategy
	ratio        float64
	targetRatio  float64
	keepRecent   int
	keepHead     int
	maxMessages  int
	summaryModel string
}

// resolveCompactionSettings applies the agent's compaction config (or its
// defaults when unset) to produce a settings value.
func resolveCompactionSettings(comp *agent.CompactionConfig, defaultModel string) compactSettings {
	s := compactSettings{strategy: compaction.StrategyTruncation, summaryModel: defaultModel}
	if comp == nil {
		return s
	}
	s.ratio = comp.TriggerRatio
	s.targetRatio = comp.TargetRatio
	s.keepRecent = comp.KeepRecent
	s.keepHead = comp.KeepHead
	s.maxMessages = comp.MaxMessages
	s.strategy = compaction.Strategy(comp.Strategy)
	if comp.SummaryModel != "" {
		s.summaryModel = comp.SummaryModel
	}
	return s
}

// projectTokens estimates the prompt size the next request would hit: the
// provider's last real input_tokens plus an estimate of the messages appended
// since. This avoids relying on a raw estimate for the whole (often CJK-heavy)
// history and lets compaction fire before the context window fills.
func projectTokens(sess *session.Session) int {
	lastInput, _, lastMsgCount := sess.LastUsage()
	msgs := sess.GetMessages()
	if lastInput > 0 && lastMsgCount > 0 && lastMsgCount < len(msgs) {
		return lastInput + compaction.EstimateTokens(msgs[lastMsgCount:])
	}
	return compaction.EstimateTokens(msgs)
}

// compactIfNeeded runs the configured compaction strategy after a turn when
// the projected prompt usage exceeds the context window budget. hint.window is
// the already-resolved effective context window (passed in so /api/show is not
// re-fetched per turn).
func compactIfNeeded(ctx context.Context, cfg Config, out chan<- event.Event, hint compactHint) {
	s := resolveCompactionSettings(cfg.Agent.Compaction, cfg.Agent.Model)
	// Reserve tokens for the compaction summary output and the next
	// completion: min(maxOutput, 20K), aligning with Claude Code's budget.
	maxOut := provider.EffectiveMaxOutput(cfg.Agent.Model, cfg.Agent.MaxTokens)
	reserve := compaction.SummaryBudget(maxOut)
	tokenLimit := compaction.ResolveTokenLimit(hint.window, reserve, s.ratio)
	if tokenLimit <= 0 && s.maxMessages <= 0 {
		return // no context window and no legacy message trigger
	}
	// Compress down to a target below the trigger line so several new turns
	// can accrue before compaction fires again (avoids re-firing every turn).
	targetLimit := compaction.ResolveTargetLimit(hint.window, reserve, s.targetRatio)
	currentTokens := projectTokens(cfg.Session)

	cmp := compaction.NewCompactor(compaction.Config{
		Strategy:      s.strategy,
		TokenLimit:    tokenLimit,
		MaxMessages:   s.maxMessages,
		KeepRecent:    s.keepRecent,
		KeepHead:      s.keepHead,
		CurrentTokens: currentTokens,
		TargetTokens:  targetLimit,
		ContextWindow: hint.window,
		Provider:      cfg.Provider,
		Model:         s.summaryModel,
		MaxOutput:     reserve,
	})
	result, err := cmp.Compact(ctx, cfg.Session.GetMessages())
	if err != nil {
		hint.log.Warn("log.compaction.error", "turn", hint.turn, "error", err)
		return
	}
	if result == nil {
		return
	}
	cfg.Session.SetMessages(result.Compacted)
	// The recorded real usage no longer describes the message list; drop it so
	// the next projection starts from the compacted state.
	cfg.Session.ClearUsage()
	emit(ctx, out, event.Event{Type: event.TypeCompaction, Content: result.Summary})
	hint.log.Info("log.compaction.compacted",
		"turn", hint.turn,
		"old_messages", result.OldCount,
		"new_messages", result.NewCount,
		"token_limit", tokenLimit,
		"current_tokens", currentTokens,
		"est_tokens", compaction.EstimateTokens(result.Compacted),
	)
}

// compactMessages applies the default truncation strategy and returns the
// compacted message list, or nil when no compaction is needed. Used by tests.
func compactMessages(ctx context.Context, messages []provider.Message, tokenLimit, maxMessages, keepRecent int) []provider.Message {
	cmp := compaction.NewCompactor(compaction.Config{
		Strategy:    compaction.StrategyTruncation,
		TokenLimit:  tokenLimit,
		MaxMessages: maxMessages,
		KeepRecent:  keepRecent,
	})
	result, err := cmp.Compact(ctx, messages)
	if err != nil || result == nil {
		return nil
	}
	return result.Compacted
}

// compactionTokenLimit derives the soft compaction threshold from a model
// window with default trigger ratio, mirroring compactIfNeeded.
func compactionTokenLimit(window, reserve int, _ float64) int {
	return compaction.ResolveTokenLimit(window, reserve, 0)
}
