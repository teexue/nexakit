package anthropic

import "github.com/teexue/nexakit/provider"

func applyAnthropicEffort(baseURL, vendor, model string, thinking *provider.ThinkingConfig, out *anthropicRequest) {
	if thinking == nil || thinking.Effort == "" {
		return
	}
	wire := provider.MapThink(provider.StyleAnthropic, baseURL, vendor, model, thinking.Effort)
	if !wire.Handled {
		return
	}
	if wire.ThinkingType != "" {
		keep := wire.ThinkingKeep
		if keep == "" {
			keep = thinking.Keep
		}
		out.Thinking = &anthropicThinking{Type: wire.ThinkingType, Keep: keep}
	}
	if wire.OutputEffort != "" {
		out.OutputConfig = &anthropicOutputConfig{Effort: wire.OutputEffort}
	}
	out.ReasoningEffort = wire.ReasoningEffort
}
