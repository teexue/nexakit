package tool

import "context"

type ctxKeyWorkDir struct{}
type ctxKeyShell struct{}

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
