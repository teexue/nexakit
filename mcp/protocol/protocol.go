package protocol

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/teexue/nexakit/provider"
)

// Transport is the request/notification surface every MCP transport implements.
// Expressing the handshake and tool RPCs once against it keeps stdio and SSE
// from duplicating protocol logic.
type Transport interface {
	SendRequest(ctx context.Context, method string, params json.RawMessage) (*Response, error)
	SendNotification(ctx context.Context, method string, params json.RawMessage) error
}

// NewInitializeParams builds the initialize handshake parameters for a caller
// identity, declaring tool support.
func NewInitializeParams(id ClientInfo) json.RawMessage {
	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: MCPProtocolVersion,
		Capabilities:    ClientCapabilities{Tools: &ToolsCapability{}},
		ClientInfo:      id,
	})
	return params
}

// PerformHandshake runs the MCP initialize request followed by the initialized
// notification, shared by every transport.
func PerformHandshake(ctx context.Context, t Transport, id ClientInfo) error {
	resp, err := t.SendRequest(ctx, MethodInitialize, NewInitializeParams(id))
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}
	if err := t.SendNotification(ctx, MethodInitialized, nil); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	return nil
}

// ListTools performs tools/list and decodes the result, shared by every transport.
func ListTools(ctx context.Context, t Transport) ([]provider.ToolDefinition, error) {
	resp, err := t.SendRequest(ctx, MethodToolsList, nil)
	if err != nil {
		return nil, fmt.Errorf("tools/list: %w", err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/list error: %s", resp.Error.Message)
	}
	var result ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tools/list result: %w", err)
	}
	return toProviderTools(result.Tools), nil
}

// CallTool performs tools/call and decodes the result, shared by every transport.
func CallTool(ctx context.Context, t Transport, name string, args map[string]any) (*CallToolResult, error) {
	params, _ := json.Marshal(CallToolParams{Name: name, Arguments: args})
	resp, err := t.SendRequest(ctx, MethodToolsCall, params)
	if err != nil {
		return nil, fmt.Errorf("tools/call %s: %w", name, err)
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("tools/call %s error: %s", name, resp.Error.Message)
	}
	var result CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("unmarshal tools/call result: %w", err)
	}
	return &result, nil
}

// toProviderTools maps wire tool definitions onto the shared provider type.
func toProviderTools(defs []ToolDef) []provider.ToolDefinition {
	out := make([]provider.ToolDefinition, len(defs))
	for i, d := range defs {
		out[i] = provider.ToolDefinition{Name: d.Name, Description: d.Description, Parameters: d.InputSchema}
	}
	return out
}
