package provider

import "context"

// RunMeta describes the run an LLM request belongs to, carried via context
// so the provider layer can attribute requests without knowing the caller.
type RunMeta struct {
	Agent     string
	SessionID string
	Source    string // chat | http | kanban | cli | optimize
}

type runMetaKey struct{}

// WithRunMeta attaches run metadata to ctx.
func WithRunMeta(ctx context.Context, meta RunMeta) context.Context {
	return context.WithValue(ctx, runMetaKey{}, meta)
}

// RunMetaFrom extracts run metadata from ctx; zero value when absent.
func RunMetaFrom(ctx context.Context) RunMeta {
	if m, ok := ctx.Value(runMetaKey{}).(RunMeta); ok {
		return m
	}
	return RunMeta{}
}
