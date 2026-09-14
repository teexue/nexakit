package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

// CreateDirectory creates a directory (and any necessary parents).
type CreateDirectory struct {
	WorkDir string // sandbox root for path resolution
}

// Name returns the tool name.
func (CreateDirectory) Name() string { return CreateDirectoryName }

// Description returns a human-readable description.
func (CreateDirectory) Description() string {
	return "Create a directory and any necessary parent directories. Returns the created path."
}

// InputSchema returns the JSON Schema for the tool's input.
func (CreateDirectory) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path of the directory to create (relative to work directory or absolute)",
			},
		},
		"required": []string{"path"},
	}
}

// Execute runs the tool.
func (cd CreateDirectory) Execute(ctx context.Context, input json.RawMessage) (Result, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return Result{}, fmt.Errorf("parse create_directory input: %w", err)
	}

	workDir := resolveWorkDir(ctx, cd.WorkDir)
	safePath, err := SafePath(workDir, args.Path)
	if err != nil {
		return Result{}, err
	}

	if err := os.MkdirAll(safePath, 0o755); err != nil {
		return Result{}, fmt.Errorf("create directory: %w", err)
	}

	out, _ := json.Marshal(map[string]any{
		"path": args.Path,
	})
	return Result{Output: out}, nil
}
