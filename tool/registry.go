package tool

import "github.com/teexue/nexakit/provider"

// Registry resolves tools by name and their LLM definitions. It is the single
// definition of the "tool registry" capability; registry.Registry is the
// default implementation. The loop depends on this interface rather than a
// concrete registry so the two never form an import cycle.
type Registry interface {
	// Get returns a tool by name.
	Get(name string) (Tool, bool)
	// Definitions resolves the given names to LLM tool definitions.
	Definitions(names []string) ([]provider.ToolDefinition, error)
}
