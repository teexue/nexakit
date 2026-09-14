package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/teexue/nexakit/event"
	"github.com/teexue/nexakit/hook"
	"github.com/teexue/nexakit/permission"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/tool"
)

// executeOneTool runs a single tool call and emits tool_start / tool_result events.
func executeOneTool(ctx context.Context, env runEnv, call provider.ToolCall) tool.Result {
	return executeTool(ctx, env, call)
}

func executeTool(ctx context.Context, env runEnv, call provider.ToolCall) tool.Result {
	inputJSON := prepareInput(call.Arguments, env.log)
	emit(ctx, env.out, event.Event{Type: event.TypeToolStart, Tool: call.Name, Input: inputJSON, ToolCallID: call.ID})

	if result, denied := checkPermission(ctx, env.pol, env.approver, call, env.out); denied {
		return tool.Result{Output: result}
	}

	fireOnToolStartHook(env.hooks, call, env.log)

	t, ok := env.cfg.Registry.Get(call.Name)
	if !ok {
		return tool.Result{Output: emitToolNotFound(ctx, env.hooks, call, env.out)}
	}

	res, execErr := runTool(ctx, t, call, env.out)
	if execErr != nil {
		return tool.Result{Output: emitToolError(ctx, env.hooks, call, execErr, env.out)}
	}

	if env.hooks != nil {
		_ = env.hooks.OnToolResult(ctx, hook.ToolResultInfo{Name: call.Name, Output: res.Output})
	}
	emit(ctx, env.out, event.Event{Type: event.TypeToolResult, Tool: call.Name, Output: res.Output, ToolCallID: call.ID})
	return res
}

// prepareInput normalizes tool arguments for display.
func prepareInput(args json.RawMessage, log *slog.Logger) json.RawMessage {
	var input any
	if err := json.Unmarshal(args, &input); err != nil {
		log.Warn("log.tool.unmarshal_args", "error", err)
		input = string(args)
	}
	inputJSON, err := json.Marshal(input)
	if err != nil {
		log.Warn("log.tool.marshal_input", "error", err)
		return args
	}
	return inputJSON
}

// checkPermission evaluates the tool permission policy. Returns a result and
// true if the tool was denied, or zero value and false if allowed.
func checkPermission(ctx context.Context, pol permission.Policy, approver Approver, call provider.ToolCall, out chan<- event.Event) (json.RawMessage, bool) {
	decision := pol.Check(permission.ToolCall{Name: call.Name, Arguments: call.Arguments})

	if decision == permission.Deny {
		errJSON, _ := json.Marshal(map[string]string{"error": "permission denied", "tool": call.Name})
		emit(ctx, out, event.Event{Type: event.TypeToolResult, Tool: call.Name, Output: json.RawMessage(errJSON), ToolCallID: call.ID})
		return json.RawMessage(errJSON), true
	}

	if decision == permission.Confirm {
		emit(ctx, out, event.Event{
			Type: event.TypeToolApproval, Tool: call.Name,
			Input: call.Arguments, ToolCallID: call.ID, ApprovalID: call.ID,
		})
		approved := approver.Approve(ctx, ApprovalRequest{
			Tool: call.Name, Arguments: call.Arguments, ApprovalID: call.ID,
		})
		if !approved {
			outJSON := userRejectedOutput(call.Name)
			emit(ctx, out, event.Event{Type: event.TypeToolResult, Tool: call.Name, Output: outJSON, ToolCallID: call.ID})
			return outJSON, true
		}
	}

	return nil, false
}

func userRejectedOutput(tool string) json.RawMessage {
	body, _ := json.Marshal(map[string]string{
		"status":  "user_rejected",
		"tool":    tool,
		"message": "The user declined to execute this tool. This is not a tool execution failure. Do not retry the same call; ask the user or choose a different approach.",
	})
	return body
}

func fireOnToolStartHook(hooks *hook.Chain, call provider.ToolCall, log *slog.Logger) {
	if hooks != nil {
		if err := hooks.OnToolStart(context.Background(), hook.ToolStartInfo{Name: call.Name, Arguments: call.Arguments}); err != nil {
			log.Warn("log.hook.tool_start_error", "tool", call.Name, "error", err)
		}
	}
}

func emitToolNotFound(ctx context.Context, hooks *hook.Chain, call provider.ToolCall, out chan<- event.Event) json.RawMessage {
	errJSON, _ := json.Marshal(map[string]string{"error": "tool not found"})
	if hooks != nil {
		_ = hooks.OnToolResult(ctx, hook.ToolResultInfo{Name: call.Name, Output: errJSON, Error: fmt.Errorf("tool not found")})
	}
	emit(ctx, out, event.Event{Type: event.TypeToolResult, Tool: call.Name, Output: json.RawMessage(errJSON), ToolCallID: call.ID})
	return json.RawMessage(errJSON)
}

// runTool executes one tool call with parent-event and call-id context.
func runTool(ctx context.Context, t tool.Tool, call provider.ToolCall, out chan<- event.Event) (tool.Result, error) {
	toolCtx := WithToolCallID(WithParentEventChan(ctx, out), call.ID)
	return t.Execute(toolCtx, call.Arguments)
}

func emitToolError(ctx context.Context, hooks *hook.Chain, call provider.ToolCall, execErr error, out chan<- event.Event) json.RawMessage {
	outVal, _ := json.Marshal(map[string]string{"error": execErr.Error()})
	if hooks != nil {
		_ = hooks.OnToolResult(ctx, hook.ToolResultInfo{Name: call.Name, Output: outVal, Error: execErr})
	}
	emit(ctx, out, event.Event{Type: event.TypeToolResult, Tool: call.Name, Output: json.RawMessage(outVal), ToolCallID: call.ID})
	return json.RawMessage(outVal)
}

// imageContentParts keeps only image_url parts for multimodal follow-up messages.
func imageContentParts(parts []provider.ContentPart) []provider.ContentPart {
	if len(parts) == 0 {
		return nil
	}
	out := make([]provider.ContentPart, 0, len(parts))
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL != nil && p.ImageURL.URL != "" {
			out = append(out, p)
		}
	}
	return out
}
