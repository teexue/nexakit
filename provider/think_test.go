package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestThinkChoices(t *testing.T) {
	cases := []struct {
		name, style, base, vendor, model string
		want                              []string
	}{
		{"kimi k3", "openai", "https://api.moonshot.cn/v1", "moonshot", "kimi-k3", []string{"low", "high", "max"}},
		{"kimi code", "anthropic", "https://api.moonshot.cn/anthropic", "moonshot", "kimi-k2.7-code", nil},
		{"kimi 2.6", "openai", "https://api.moonshot.cn/v1", "moonshot", "kimi-k2.6", []string{"off", "on"}},
		{"deepseek", "openai", "https://api.deepseek.com", "deepseek", "deepseek-v4-flash", []string{"off", "low", "high", "max"}},
		{"glm 5.3", "openai", "https://open.bigmodel.cn/api/paas/v4", "zhipu", "glm-5.3", []string{"low", "high", "max"}},
		{"glm 5.2", "openai", "https://open.bigmodel.cn/api/paas/v4", "zhipu", "glm-5.2", []string{"off", "high", "max"}},
		{"qwen", "openai", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen", "qwen-plus", []string{"off", "low", "medium", "high", "max"}},
		{"gpt-oss ollama", "ollama", "https://ollama.com", "ollama_cloud", "gpt-oss:20b", []string{"low", "medium", "high"}},
		{"ollama glm", "ollama", "https://ollama.com", "ollama_cloud", "glm-5.3", []string{"off", "low", "medium", "high", "max"}},
		{"groq oss", "openai", "https://api.groq.com/openai/v1", "groq", "openai/gpt-oss-20b", []string{"low", "medium", "high"}},
		{"openai", "openai", "https://api.openai.com/v1", "openai", "gpt-5.2", []string{"off", "low", "medium", "high", "max"}},
		{"claude", "anthropic", "https://api.anthropic.com", "anthropic", "claude-sonnet-4-6", []string{"off", "low", "medium", "high", "max"}},
		{"openrouter", "openai", "https://openrouter.ai/api/v1", "openrouter", "openai/gpt-5", []string{"off", "low", "medium", "high", "max"}},
		{"silicon", "openai", "https://api.siliconflow.cn/v1", "siliconflow", "Qwen/Qwen2.5", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ThinkChoices(APIStyle(tc.style), tc.base, tc.vendor, tc.model)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestMapThinkWire(t *testing.T) {
	deepseek := "https://api.deepseek.com"
	wire := MapThink(StyleOpenAI, deepseek, "deepseek", "deepseek-v4-flash", "low")
	assert.True(t, wire.Handled)
	assert.Equal(t, "enabled", wire.ThinkingType)
	assert.Equal(t, "low", wire.ReasoningEffort)

	off := MapThink(StyleOpenAI, deepseek, "deepseek", "deepseek-v4-flash", "off")
	assert.Equal(t, "disabled", off.ThinkingType)
	assert.Empty(t, off.ReasoningEffort)

	medium := MapThink(StyleOpenAI, deepseek, "deepseek", "deepseek-v4-flash", "medium")
	assert.Equal(t, "high", medium.ReasoningEffort)

	anth := MapThink(StyleAnthropic, deepseek, "deepseek", "deepseek-v4-flash", "max")
	assert.Equal(t, "max", anth.OutputEffort)
	assert.Empty(t, anth.ReasoningEffort)

	k3 := MapThink(StyleOpenAI, "https://api.moonshot.cn/v1", "moonshot", "kimi-k3", "high")
	assert.Equal(t, "high", k3.ReasoningEffort)
	assert.Empty(t, k3.ThinkingType)

	k26 := MapThink(StyleAnthropic, "https://api.moonshot.cn/anthropic", "moonshot", "kimi-k2.6", "off")
	assert.Equal(t, "disabled", k26.ThinkingType)

	glm := MapThink(StyleOpenAI, "https://open.bigmodel.cn/api/paas/v4", "zhipu", "glm-5.3", "max")
	assert.Equal(t, "enabled", glm.ThinkingType)
	assert.Equal(t, "max", glm.ReasoningEffort)
	assert.False(t, MapThink(StyleOpenAI, "https://open.bigmodel.cn/api/paas/v4", "zhipu", "glm-5.3", "off").Handled)

	collapsed := MapThink(StyleOpenAI, "https://open.bigmodel.cn/api/paas/v4", "zhipu", "glm-5.2", "medium")
	assert.Equal(t, "high", collapsed.ReasoningEffort)

	qwen := MapThink(StyleOpenAI, "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen", "qwen-plus", "medium")
	assert.NotNil(t, qwen.EnableThinking)
	assert.True(t, *qwen.EnableThinking)
	assert.Equal(t, 4000, qwen.ThinkingBudget)

	oss := MapThink(StyleOllama, "https://ollama.com", "ollama_cloud", "gpt-oss:20b", "low")
	assert.Equal(t, "low", oss.OllamaValue)
	assert.False(t, MapThink(StyleOllama, "https://ollama.com", "ollama_cloud", "gpt-oss:20b", "off").Handled)

	claude := MapThink(StyleAnthropic, "https://api.anthropic.com", "anthropic", "claude-opus-4-7", "medium")
	assert.Equal(t, "adaptive", claude.ThinkingType)
	assert.Equal(t, "medium", claude.OutputEffort)

	router := MapThink(StyleOpenAI, "https://openrouter.ai/api/v1", "openrouter", "openai/gpt-5", "off")
	assert.Equal(t, "none", router.OpenRouterEffort)

	assert.False(t, MapThink(StyleOpenAI, "https://api.openai.com/v1", "openai", "gpt-5.2", "").Handled)
}
