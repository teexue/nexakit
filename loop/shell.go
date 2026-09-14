package loop

import (
	"context"

	"github.com/teexue/nexakit/tool"
)

// GetShell returns the preferred shell id from context, or empty for auto.
// It delegates to tool, which owns the run-scoped tool context keys.
func GetShell(ctx context.Context) string {
	return tool.GetShell(ctx)
}

// WithShell returns a context with the preferred shell id set.
func WithShell(ctx context.Context, id string) context.Context {
	return tool.WithShell(ctx, id)
}
