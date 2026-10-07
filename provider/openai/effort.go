package openai

import "github.com/teexue/nexakit/provider"

func applyOpenAIEffort(baseURL, vendor, model string, thinking *provider.ThinkingConfig, out *openAIRequest) bool {
	if thinking == nil || thinking.Effort == "" {
		return false
	}
	wire := provider.MapThink(provider.StyleOpenAI, baseURL, vendor, model, thinking.Effort)
	if !wire.Handled {
		return false
	}
	if wire.ThinkingType != "" {
		th := openAIThinking{Type: wire.ThinkingType}
		keep := wire.ThinkingKeep
		if keep == "" {
			keep = thinking.Keep
		}
		if keep != "" {
			th.Keep = &keep
		}
		out.Thinking = &th
	}
	out.ReasoningEffort = wire.ReasoningEffort
	out.EnableThinking = wire.EnableThinking
	out.ThinkingBudget = wire.ThinkingBudget
	if wire.OpenRouterEffort != "" {
		out.Reasoning = &openAIReasoning{Effort: wire.OpenRouterEffort}
	}
	return true
}
