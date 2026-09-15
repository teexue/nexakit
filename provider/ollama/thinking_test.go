package ollama

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teexue/nexakit/provider"
)

// TestThinkingPerRequestOverride verifies the agent → request thinking chain:
// a request-level think config wins over the constructor-level one, and nil
// keeps the constructor-level behavior.
func TestThinkingPerRequestOverride(t *testing.T) {
	profile, err := New(Config{
		Thinking: &provider.ThinkingConfig{Type: "enabled"},
	})
	require.NoError(t, err)

	body := profile.buildRequest(provider.Request{
		Model:    "qwen3",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		Thinking: &provider.ThinkingConfig{Type: "disabled"},
	})
	assert.Equal(t, false, body.Think, "request-level must override constructor level")

	body = profile.buildRequest(provider.Request{
		Model:    "qwen3",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	assert.Equal(t, true, body.Think, "nil request thinking keeps constructor level")

	plain, err := New(Config{})
	require.NoError(t, err)
	body = plain.buildRequest(provider.Request{
		Model:    "qwen3",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
	})
	assert.Nil(t, body.Think, "unconfigured = model default (field omitted)")
}
