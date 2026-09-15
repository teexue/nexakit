package provider

// Default base URLs for each wire protocol family. They live in the root
// package so both the implementation subpackages and the catalog can resolve
// a base URL without importing each other.

const (
	// DefaultOpenAIBaseURL is the OpenAI-compatible default API base URL.
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"
	// DefaultAnthropicBaseURL is the Anthropic Messages API default base URL.
	DefaultAnthropicBaseURL = "https://api.anthropic.com"
	// DefaultAnthropicVersion is the default Anthropic API version header.
	DefaultAnthropicVersion = "2023-06-01"
	// DefaultOllamaBaseURL is the native Ollama default host.
	DefaultOllamaBaseURL = "http://localhost:11434"
	// DefaultContextWindow is the conservative context size assumed when neither
	// the caller nor the provider reports one. It keeps compaction active for any
	// model so long histories get trimmed instead of growing unboundedly.
	DefaultContextWindow = 128_000 // 128K tokens
)

// DefaultBaseURLFor returns the default API base URL for an API style.
func DefaultBaseURLFor(style APIStyle) string {
	switch style {
	case StyleAnthropic:
		return DefaultAnthropicBaseURL
	case StyleOllama:
		return DefaultOllamaBaseURL
	default:
		return DefaultOpenAIBaseURL
	}
}

// EffectiveMaxOutput resolves the per-request output cap from an explicit
// configuration, falling back to DefaultMaxTokens. Provider-reported limits
// (e.g. model introspection) are the caller's responsibility to pass in.
func EffectiveMaxOutput(configured int) int {
	if configured > 0 {
		return configured
	}
	return DefaultMaxTokens
}

// EffectiveContextWindow resolves the model context window from an explicit
// configuration, falling back to DefaultContextWindow so compaction never
// silently disables itself. A provider that can introspect the model's real
// runtime limit (see ContextResolver) supplies the configured value.
func EffectiveContextWindow(configured int) int {
	if configured > 0 {
		return configured
	}
	return DefaultContextWindow
}
