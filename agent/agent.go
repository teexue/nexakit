// Package agent holds runtime agent configuration and its validation.
// It describes behavior (prompt, tools, model, limits) but reads no files
// and defines no on-disk format.
package agent

import (
	"github.com/teexue/nexakit/compaction"
	"github.com/teexue/nexakit/permission"
)

// ToolExecution configures tool execution strategy.
type ToolExecution struct {
	Mode        string // ToolExecParallel | ToolExecSerial
	MaxParallel int    // max concurrent tools, default 4
}

// MCPServerConfig describes an MCP server connection declared on an agent.
type MCPServerConfig struct {
	Name    string
	Type    string // "stdio" | "sse"
	Command string
	Args    []string
	Env     map[string]string
	URL     string
}

// CompactionConfig configures context window compaction.
// Compaction is driven by estimated token usage vs the model context window,
// not by raw message count.
type CompactionConfig struct {
	Strategy      compaction.Strategy // defaults to cascade
	ContextWindow int                 // model context size in tokens; 0 = use runtime/provider
	TriggerRatio  float64             // compact when usage exceeds window*ratio (default 1.0 = window - reserve)
	TargetRatio   float64             // compress down to window*ratio (default 0.6); must be below trigger
	KeepRecent    int                 // recent conversation messages to preserve when truncating
	KeepHead      int                 // oldest conversation messages preserved verbatim (stable cache prefix); default 2
	MaxMessages   int                 // optional legacy secondary trigger; 0 = disabled
	SummaryModel  string              // model used for summarize strategy; empty = agent model
}

// KnowledgeConfig scopes RAG retrieval for an agent.
type KnowledgeConfig struct {
	Bases []string // empty = all bases
	TopK  int
}

// OptimizeConfig enables in-pipeline prompt optimization for an agent.
// Optimization runs at assembly time (before loop.Run) via an extra LLM call.
// System prompt optimization is a separate, editor-triggered action whose
// result the user reviews before saving.
type OptimizeConfig struct {
	UserPrompt bool // optimize each user prompt before the run
}

// Agent configures agent behavior for a production use case.
type Agent struct {
	Version       int
	ID            string
	Name          string
	Provider      string
	SystemPrompt  string
	Tools         []string
	Skills        []string
	Model         string
	MaxTurns      int // 0 = unlimited until model stops
	MaxTokens     int
	ToolExecution *ToolExecution
	Permissions   *permission.Permissions
	MCPServers    []MCPServerConfig
	Compaction    *CompactionConfig
	Knowledge     *KnowledgeConfig
	Optimize      *OptimizeConfig

	// Runtime-only context parts. Kept separate for prompt caching.
	ProjectContext string
	SkillsContext  string
}

const (
	defaultMaxParallel = 4
)

// Tool execution modes. These are the canonical string values for
// ToolExecution.Mode; the loop and validation both reference them so the
// wire/config values live in exactly one place.
const (
	// ToolExecParallel runs independent tool calls concurrently (the default).
	ToolExecParallel = "parallel"
	// ToolExecSerial runs tool calls one at a time.
	ToolExecSerial = "serial"
)

// ToolExecMode returns the configured tool execution mode with defaults.
func (a *Agent) ToolExecMode() string {
	if a.ToolExecution == nil || a.ToolExecution.Mode == "" {
		return ToolExecParallel
	}
	return a.ToolExecution.Mode
}

// ToolMaxParallel returns the max parallel tools with defaults.
func (a *Agent) ToolMaxParallel() int {
	if a.ToolExecution == nil || a.ToolExecution.MaxParallel <= 0 {
		return defaultMaxParallel
	}
	return a.ToolExecution.MaxParallel
}

// Validate checks the agent config the loop relies on.
func (a *Agent) Validate() error {
	return a.validate()
}
