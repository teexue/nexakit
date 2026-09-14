// Package tool defines the unified capability abstraction. Every capability
// the model can invoke is a Tool; the loop holds no tool-specific logic.
package tool

import (
	"context"
	"encoding/json"

	"github.com/teexue/nexakit/provider"
)

// Result is returned by a tool execution.
type Result struct {
	Output json.RawMessage `json:"output"`
	// ContentParts are multimodal blocks (typically images) that must be sent
	// to the model as separate content parts — not inlined as JSON text.
	ContentParts []provider.ContentPart `json:"content_parts,omitempty"`
}

// Tool is the unified capability abstraction.
type Tool interface {
	Name() string
	Description() string
	InputSchema() map[string]any
	Execute(ctx context.Context, input json.RawMessage) (Result, error)
}

// Canonical built-in tool names. Defined once here so the loop, permission
// policy, and built-in implementations agree on the string instead of each
// spelling a literal. Each constant is the wire name of the matching tool type.
const (
	// GetTimeName is the name of the GetTime tool ("get_time").
	GetTimeName = "get_time"
	// ReadFileName is the name of the ReadFile tool ("read_file").
	ReadFileName = "read_file"
	// ReadImageName is the name of the ReadImage tool ("read_image").
	ReadImageName = "read_image"
	// WriteFileName is the name of the WriteFile tool ("write_file").
	WriteFileName = "write_file"
	// EditFileName is the name of the EditFile tool ("edit_file").
	EditFileName = "edit_file"
	// DeleteFileName is the name of the DeleteFile tool ("delete_file").
	DeleteFileName = "delete_file"
	// ListDirectoryName is the name of the ListDirectory tool ("list_directory").
	ListDirectoryName = "list_directory"
	// CreateDirectoryName is the name of the CreateDirectory tool ("create_directory").
	CreateDirectoryName = "create_directory"
	// SearchFilesName is the name of the SearchFiles tool ("search_files").
	SearchFilesName = "search_files"
	// RunCommandName is the name of the RunCommand tool ("run_command").
	RunCommandName = "run_command"
	// WebFetchName is the name of the WebFetch tool ("web_fetch").
	WebFetchName = "web_fetch"
	// DelegateTaskName is the name of the DelegateTask tool ("delegate_task").
	// It is special: it spawns a nested run, so the loop routes it outside the
	// tool semaphore.
	DelegateTaskName = "delegate_task"
)
