package loop

import (
	"context"
	"log/slog"

	"github.com/teexue/nexakit/agent"
	"github.com/teexue/nexakit/event"
	"github.com/teexue/nexakit/hook"
	"github.com/teexue/nexakit/permission"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/session"
	"github.com/teexue/nexakit/tool"
)

// ctxKeyParentEventChan is the unexported context key for the parent event channel.
type ctxKeyParentEventChan struct{}

// Config configures a single agent run.
type Config struct {
	Provider provider.Provider
	Registry tool.Registry
	Agent    *agent.Agent
	Session  *session.Session
	Prompt   string
	Logger   *slog.Logger

	// Store is an optional session persistence backend.
	// When set alongside SessionID, the loop loads the existing session
	// from the store at the start and saves it after each run.
	Store session.Store

	// SessionID, when set with Store, resumes an existing session.
	// When empty, a new session is created and its ID is stored after the run.
	SessionID string

	// Policy controls tool execution permissions.
	// When nil, all tools are allowed (AllowAllPolicy).
	Policy permission.Policy

	// Hooks are lifecycle callbacks invoked around tool execution and turns.
	// When nil, no hooks are called.
	Hooks *hook.Chain

	// Approver handles interactive tool approval when Policy returns Confirm.
	// When nil, DenyAllApprover is used (tools requiring approval are denied).
	Approver Approver

	// WorkDir overrides the working directory for file operation tools.
	// When empty, tools use their registered default.
	WorkDir string

	// Shell is the preferred command interpreter id for run_command
	// (bash, powershell, pwsh, cmd, sh). Empty means auto-detect
	// (Git Bash first on Windows).
	Shell string

	// Images are attached to the user prompt as multimodal content parts.
	Images []provider.ContentPart

	// Source attributes the run for request auditing (e.g. "http", "kanban",
	// "cli"). Optional; empty means unattributed.
	Source string

	// ContextWindow is the model context size in tokens used for compaction.
	// When 0, agent.Compaction.ContextWindow is used if set; otherwise compaction
	// is skipped unless a legacy max_messages trigger is configured.
	ContextWindow int

	// failStreak tracks consecutive identical tool failures within a run.
	failStreak *toolFailStreak

	// imageKeepFrom is the first session message index whose image payloads
	// may be sent to the model. Earlier images belong to prior user turns.
	imageKeepFrom int

	// AgentsDir and NewProvider let tools spawn nested loop.Run calls
	// (delegate_task). Optional; empty means sub-agent spawning is disabled.
	AgentsDir   string
	NewProvider func(a *agent.Agent) (provider.Provider, error)
	// LoadAgent loads a named agent for delegate_task. Nil disables loading a
	// different agent for a child run.
	LoadAgent func(dir, name string) (*agent.Agent, error)
	// EnrichContext may rewrite the context before the loop starts. Optional.
	EnrichContext func(context.Context, *agent.Agent) context.Context
	// Depth is the current sub-agent nesting level (0 = main agent).
	Depth int
	// Subagent holds process-wide nested-run limits from global settings.
	Subagent SubagentLimits
}

// GetParentEventChan returns the parent event channel from context, or nil.
func GetParentEventChan(ctx context.Context) chan<- event.Event {
	if ch, ok := ctx.Value(ctxKeyParentEventChan{}).(chan<- event.Event); ok {
		return ch
	}
	return nil
}

// WithParentEventChan returns a context with the parent event channel set.
func WithParentEventChan(ctx context.Context, ch chan<- event.Event) context.Context {
	return context.WithValue(ctx, ctxKeyParentEventChan{}, ch)
}
