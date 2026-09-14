package agent

import (
	"fmt"

	"github.com/teexue/nexakit/compaction"
)

func (a *Agent) validate() error {
	if err := a.validateCore(); err != nil {
		return err
	}
	if err := a.validateToolExecution(); err != nil {
		return err
	}
	if err := a.validateMCPServers(); err != nil {
		return err
	}
	if err := a.validateCompaction(); err != nil {
		return err
	}
	return a.validateKnowledge()
}

func (a *Agent) validateCore() error {
	if a.Version == 0 {
		a.Version = 1
	}
	if a.Version != 1 {
		return fmt.Errorf("unsupported agent version %d", a.Version)
	}
	if a.Name == "" {
		return fmt.Errorf("name is required")
	}
	if a.Provider == "" {
		return fmt.Errorf("provider is required")
	}
	if a.SystemPrompt == "" {
		return fmt.Errorf("system_prompt is required")
	}
	if a.Model == "" {
		return fmt.Errorf("model is required")
	}
	if len(a.Tools) == 0 {
		return fmt.Errorf("tools must not be empty")
	}
	return nil
}

func (a *Agent) validateToolExecution() error {
	if a.ToolExecution == nil {
		a.ToolExecution = &ToolExecution{Mode: ToolExecParallel, MaxParallel: defaultMaxParallel}
		return nil
	}
	switch a.ToolExecution.Mode {
	case "", ToolExecParallel:
		a.ToolExecution.Mode = ToolExecParallel
	case ToolExecSerial:
	default:
		return fmt.Errorf("tool_execution.mode must be %q or %q, got %q", ToolExecParallel, ToolExecSerial, a.ToolExecution.Mode)
	}
	if a.ToolExecution.MaxParallel <= 0 {
		a.ToolExecution.MaxParallel = defaultMaxParallel
	}
	return nil
}

func (a *Agent) validateMCPServers() error {
	for i, mcp := range a.MCPServers {
		if mcp.Name == "" {
			return fmt.Errorf("mcp_servers[%d]: name is required", i)
		}
		switch mcp.Type {
		case "stdio":
			if mcp.Command == "" {
				return fmt.Errorf("mcp_servers[%d] (%s): command is required for stdio type", i, mcp.Name)
			}
		case "sse":
			if mcp.URL == "" {
				return fmt.Errorf("mcp_servers[%d] (%s): url is required for sse type", i, mcp.Name)
			}
		default:
			return fmt.Errorf("mcp_servers[%d] (%s): type must be 'stdio' or 'sse', got %q", i, mcp.Name, mcp.Type)
		}
	}
	return nil
}

func (a *Agent) validateCompaction() error {
	if a.Compaction == nil {
		return nil
	}
	switch a.Compaction.Strategy {
	case "":
		a.Compaction.Strategy = compaction.StrategyCascade
	case compaction.StrategyCascade, compaction.StrategyTruncation,
		compaction.StrategySliding, compaction.StrategySummarize:
	default:
		return fmt.Errorf("compaction.strategy must be one of %q, %q, %q, %q, got %q",
			compaction.StrategyCascade, compaction.StrategyTruncation,
			compaction.StrategySliding, compaction.StrategySummarize, a.Compaction.Strategy)
	}
	if a.Compaction.KeepRecent <= 0 {
		a.Compaction.KeepRecent = 20
	}
	if a.Compaction.KeepHead < 0 {
		return fmt.Errorf("compaction.keep_head must be >= 0")
	}
	if a.Compaction.TriggerRatio != 0 && (a.Compaction.TriggerRatio < 0 || a.Compaction.TriggerRatio > 1) {
		return fmt.Errorf("compaction.trigger_ratio must be in (0, 1]")
	}
	if a.Compaction.TargetRatio != 0 && (a.Compaction.TargetRatio < 0 || a.Compaction.TargetRatio >= 1) {
		return fmt.Errorf("compaction.target_ratio must be in (0, 1)")
	}
	if a.Compaction.TargetRatio != 0 && a.Compaction.TriggerRatio != 0 &&
		a.Compaction.TargetRatio >= a.Compaction.TriggerRatio {
		return fmt.Errorf("compaction.target_ratio must be below trigger_ratio")
	}
	return nil
}

func (a *Agent) validateKnowledge() error {
	if a.Knowledge == nil {
		return nil
	}
	if a.Knowledge.TopK < 0 {
		return fmt.Errorf("knowledge.top_k must be >= 0")
	}
	if a.Knowledge.TopK == 0 {
		a.Knowledge.TopK = 5
	}
	return nil
}
