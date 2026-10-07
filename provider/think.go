package provider

import (
	"net/url"
	"strings"
)

// ThinkWire is the vendor field set for one unified effort value.
// Handled is false when the effort should not be sent (unknown vendor,
// unsupported level, or empty effort).
type ThinkWire struct {
	Handled          bool
	ThinkingType     string
	ThinkingKeep     string
	ReasoningEffort  string
	EnableThinking   *bool
	ThinkingBudget   int
	OutputEffort     string
	OpenRouterEffort string
	OllamaSet        bool
	OllamaValue      any
}

// ThinkChoices lists the effort ids the model actually accepts.
// Empty means this model has no intensity control.
func ThinkChoices(style APIStyle, baseURL, providerName, model string) []string {
	switch thinkFamily(style, baseURL, providerName) {
	case "ollama":
		return ollamaChoices(model)
	case "moonshot":
		return moonshotChoices(model)
	case "deepseek":
		return []string{"off", "low", "high", "max"}
	case "zhipu":
		return zhipuChoices(model)
	case "qwen":
		return []string{"off", "low", "medium", "high", "max"}
	case "groq":
		return groqChoices(model)
	case "openrouter", "openai", "anthropic":
		return []string{"off", "low", "medium", "high", "max"}
	default:
		return nil
	}
}

// MapThink converts a unified effort into the vendor's documented fields.
func MapThink(style APIStyle, baseURL, providerName, model, effort string) ThinkWire {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return ThinkWire{}
	}
	family := thinkFamily(style, baseURL, providerName)
	if !choiceAllowed(ThinkChoices(style, baseURL, providerName, model), effort) {
		return compatThink(family, style, model, effort)
	}
	return mapThinkFamily(family, style, model, effort)
}

func choiceAllowed(choices []string, effort string) bool {
	for _, c := range choices {
		if c == effort {
			return true
		}
	}
	return false
}

// compatThink applies documented aliases for values the UI does not offer,
// such as DeepSeek mapping medium onto high.
func compatThink(family string, style APIStyle, model, effort string) ThinkWire {
	switch family {
	case "deepseek":
		mapped := deepseekEffort(effort)
		if mapped == "" {
			return ThinkWire{}
		}
		return deepseekWire(style, effort == "off", mapped)
	case "zhipu":
		return zhipuCompat(model, effort)
	default:
		return ThinkWire{}
	}
}

func mapThinkFamily(family string, style APIStyle, model, effort string) ThinkWire {
	switch family {
	case "ollama":
		return ollamaWire(model, effort)
	case "moonshot":
		return moonshotWire(model, effort)
	case "deepseek":
		return deepseekWire(style, effort == "off", deepseekEffort(effort))
	case "zhipu":
		return zhipuWire(model, effort)
	case "qwen":
		return qwenWire(effort)
	case "groq":
		return groqWire(model, effort)
	case "openrouter":
		return ThinkWire{Handled: true, OpenRouterEffort: openRouterEffort(effort)}
	case "openai":
		return ThinkWire{Handled: true, ReasoningEffort: openAIEffort(effort)}
	case "anthropic":
		return anthropicWire(effort)
	default:
		return ThinkWire{}
	}
}

func thinkFamily(style APIStyle, baseURL, providerName string) string {
	if style == StyleOllama {
		return "ollama"
	}
	host := requestHost(baseURL)
	name := strings.ToLower(strings.TrimSpace(providerName))
	blob := host + " " + name
	switch {
	case strings.Contains(blob, "moonshot"):
		return "moonshot"
	case strings.Contains(blob, "deepseek"):
		return "deepseek"
	case strings.Contains(blob, "bigmodel") || strings.Contains(blob, "z.ai") || name == "zhipu":
		return "zhipu"
	case strings.Contains(blob, "dashscope") || strings.Contains(blob, "aliyuncs") || name == "qwen":
		return "qwen"
	case strings.Contains(blob, "groq"):
		return "groq"
	case strings.Contains(blob, "openrouter"):
		return "openrouter"
	case strings.Contains(blob, "siliconflow"):
		return "siliconflow"
	case strings.Contains(host, "anthropic.com") || name == "anthropic":
		return "anthropic"
	case strings.Contains(host, "openai.com") || name == "openai":
		return "openai"
	case strings.Contains(blob, "ollama") || strings.Contains(host, ":11434"):
		return "ollama"
	default:
		return ""
	}
}

func requestHost(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return strings.ToLower(baseURL)
	}
	return strings.ToLower(parsed.Host)
}

func modelHas(model string, parts ...string) bool {
	m := strings.ToLower(model)
	for _, p := range parts {
		if strings.Contains(m, p) {
			return true
		}
	}
	return false
}

func ollamaChoices(model string) []string {
	if modelHas(model, "gpt-oss") {
		return []string{"low", "medium", "high"}
	}
	return []string{"off", "low", "medium", "high", "max"}
}

func ollamaWire(model, effort string) ThinkWire {
	if modelHas(model, "gpt-oss") {
		if effort != "low" && effort != "medium" && effort != "high" {
			return ThinkWire{}
		}
		return ThinkWire{Handled: true, OllamaSet: true, OllamaValue: effort}
	}
	wire := ThinkWire{Handled: true, OllamaSet: true}
	if effort == "off" {
		wire.OllamaValue = false
		return wire
	}
	wire.OllamaValue = effort
	return wire
}

func moonshotChoices(model string) []string {
	switch {
	case modelHas(model, "kimi-k3"):
		return []string{"low", "high", "max"}
	case modelHas(model, "kimi-k2.7-code"):
		return nil
	case modelHas(model, "kimi-k2.6", "kimi-k2.5"):
		return []string{"off", "on"}
	default:
		return nil
	}
}

func moonshotWire(model, effort string) ThinkWire {
	if modelHas(model, "kimi-k3") {
		return ThinkWire{Handled: true, ReasoningEffort: effort}
	}
	if !modelHas(model, "kimi-k2.6", "kimi-k2.5") {
		return ThinkWire{}
	}
	kind := "enabled"
	if effort == "off" {
		kind = "disabled"
	}
	return ThinkWire{Handled: true, ThinkingType: kind}
}

func deepseekEffort(effort string) string {
	switch effort {
	case "off":
		return ""
	case "low":
		return "low"
	case "medium", "high", "xhigh":
		return "high"
	case "max", "ultra":
		return "max"
	default:
		return ""
	}
}

func deepseekWire(style APIStyle, off bool, effort string) ThinkWire {
	wire := ThinkWire{Handled: true}
	if off {
		wire.ThinkingType = "disabled"
		return wire
	}
	wire.ThinkingType = "enabled"
	if style == StyleAnthropic {
		wire.OutputEffort = effort
		return wire
	}
	wire.ReasoningEffort = effort
	return wire
}

func zhipuChoices(model string) []string {
	switch {
	case modelHas(model, "glm-5.3"):
		return []string{"low", "high", "max"}
	case modelHas(model, "glm-5.2", "glm-5.1"):
		return []string{"off", "high", "max"}
	case modelHas(model, "glm"):
		return []string{"off", "on"}
	default:
		return nil
	}
}

func zhipuWire(model, effort string) ThinkWire {
	if modelHas(model, "glm-5.3") {
		return ThinkWire{Handled: true, ThinkingType: "enabled", ReasoningEffort: effort}
	}
	if modelHas(model, "glm-5.2", "glm-5.1") {
		if effort == "off" {
			return ThinkWire{Handled: true, ThinkingType: "disabled", ReasoningEffort: "none"}
		}
		return ThinkWire{Handled: true, ThinkingType: "enabled", ReasoningEffort: effort}
	}
	kind := "enabled"
	if effort == "off" {
		kind = "disabled"
	}
	return ThinkWire{Handled: true, ThinkingType: kind}
}

func zhipuCompat(model, effort string) ThinkWire {
	if !modelHas(model, "glm-5.2", "glm-5.1") {
		return ThinkWire{}
	}
	switch effort {
	case "none", "minimal":
		return zhipuWire(model, "off")
	case "low", "medium":
		return zhipuWire(model, "high")
	case "xhigh":
		return zhipuWire(model, "max")
	default:
		return ThinkWire{}
	}
}

func qwenWire(effort string) ThinkWire {
	on := true
	budget := 0
	switch effort {
	case "off":
		on = false
	case "low":
		budget = 1024
	case "medium":
		budget = 4000
	case "high":
		budget = 16384
	case "max":
		budget = 32768
	default:
		return ThinkWire{}
	}
	return ThinkWire{Handled: true, EnableThinking: &on, ThinkingBudget: budget}
}

func groqChoices(model string) []string {
	switch {
	case modelHas(model, "gpt-oss"):
		return []string{"low", "medium", "high"}
	case modelHas(model, "qwen"):
		return []string{"off", "on"}
	default:
		return nil
	}
}

func groqWire(model, effort string) ThinkWire {
	if modelHas(model, "gpt-oss") {
		return ThinkWire{Handled: true, ReasoningEffort: effort}
	}
	value := "default"
	if effort == "off" {
		value = "none"
	}
	return ThinkWire{Handled: true, ReasoningEffort: value}
}

func openAIEffort(effort string) string {
	if effort == "off" {
		return "none"
	}
	return effort
}

func openRouterEffort(effort string) string {
	if effort == "off" {
		return "none"
	}
	return effort
}

func anthropicWire(effort string) ThinkWire {
	if effort == "off" {
		return ThinkWire{Handled: true, ThinkingType: "disabled"}
	}
	return ThinkWire{Handled: true, ThinkingType: "adaptive", OutputEffort: effort}
}
