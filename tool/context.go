package tool

import "context"

type ctxKeyWorkDir struct{}
type ctxKeyShell struct{}
type ctxKeyToolCallID struct{}

// GetWorkDir returns the working directory from context, or empty string.
// The loop injects it before executing tools so a tool's static WorkDir can be
// overridden per run.
func GetWorkDir(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyWorkDir{}).(string); ok {
		return v
	}
	return ""
}

// WithWorkDir returns a context with the working directory set.
func WithWorkDir(ctx context.Context, dir string) context.Context {
	return context.WithValue(ctx, ctxKeyWorkDir{}, dir)
}

// GetShell returns the preferred shell id from context, or empty for auto.
func GetShell(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyShell{}).(string); ok {
		return v
	}
	return ""
}

// WithShell returns a context with the preferred shell id set.
func WithShell(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyShell{}, id)
}

// WithToolCallID returns a context carrying the active tool call id.
func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyToolCallID{}, id)
}

// ToolCallIDFrom returns the active tool call id from ctx, or empty.
func ToolCallIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyToolCallID{}).(string); ok {
		return v
	}
	return ""
}
