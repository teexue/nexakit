package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// rpcTransport is the shared request/notification surface implemented by both
// StdioClient and SSEClient. Expressing the MCP handshake and tool RPCs once
// against it keeps the two transports from duplicating protocol logic.
type rpcTransport interface {
	sendRequest(ctx context.Context, method string, params json.RawMessage) (*Response, error)
	sendNotification(ctx context.Context, method string, params json.RawMessage) error
}

// newInitializeParams builds the initialize handshake parameters for a caller
// identity, declaring tool support.
func newInitializeParams(id ClientInfo) json.RawMessage {
	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: mcpProtocolVersion,
		Capabilities:    ClientCapabilities{Tools: &ToolsCapability{}},
		ClientInfo:      id,
	})
	return params
}

// performHandshake runs the MCP initialize request followed by the initialized
// notification, shared by every transport.
func performHandshake(ctx context.Context, t rpcTransport, id ClientInfo) error {
	resp, err := t.sendRequest(ctx, methodInitialize, newInitializeParams(id))
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}
	if err := t.sendNotification(ctx, methodInitialized, nil); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	return nil
}

// listTools performs tools/list and decodes the result, shared by every transport.
func listTools(ctx context.Context, t rpcTransport) ([]ToolDefinition, error) {
	resp, err := t.sendRequest(ctx, methodToolsList, nil)
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
	return result.Tools, nil
}

// callTool performs tools/call and decodes the result, shared by every transport.
func callTool(ctx context.Context, t rpcTransport, name string, args map[string]any) (*CallToolResult, error) {
	params, _ := json.Marshal(CallToolParams{Name: name, Arguments: args})
	resp, err := t.sendRequest(ctx, methodToolsCall, params)
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
