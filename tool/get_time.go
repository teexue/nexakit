package tool

import (
	"context"
	"encoding/json"
	"time"
)

// GetTime returns the current UTC time.
type GetTime struct{}

// Name returns the tool name.
func (GetTime) Name() string { return GetTimeName }

// Description returns a human-readable description.
func (GetTime) Description() string { return "Return the current UTC time in RFC3339 format." }

// InputSchema returns the JSON Schema for the tool's input.
func (GetTime) InputSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Execute runs the tool.
func (GetTime) Execute(_ context.Context, _ json.RawMessage) (Result, error) {
	out, _ := json.Marshal(map[string]string{"time": time.Now().UTC().Format(time.RFC3339)})
	return Result{Output: out}, nil
}
