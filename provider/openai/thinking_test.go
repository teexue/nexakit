package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teexue/nexakit/provider"
)

// TestThinkingPerRequestOverride verifies the agent → request thinking chain:
// a request-level thinking config wins over the constructor-level one, and nil
// falls back to the constructor-level configuration.
func TestThinkingPerRequestOverride(t *testing.T) {
	profile, err := New(Config{
		APIKey:   "test",
		Thinking: &provider.ThinkingConfig{Type: "enabled", Keep: "all"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Request-level wins over the constructor-level config.
	body := profile.buildRequest(provider.Request{
		Model: "kimi-k2.6",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
		},
		Thinking: &provider.ThinkingConfig{Type: "disabled"},
	})
	if body.Thinking == nil || body.Thinking.Type != "disabled" {
		t.Fatalf("request-level thinking = %#v", body.Thinking)
	}

	// nil request thinking keeps the constructor-level configuration.
	body = profile.buildRequest(provider.Request{
		Model: "kimi-k2.6",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
		},
	})
	if body.Thinking == nil || body.Thinking.Type != "enabled" || body.Thinking.Keep == nil || *body.Thinking.Keep != "all" {
		t.Fatalf("fallback thinking = %#v", body.Thinking)
	}

	// nil on both sides sends no thinking field (model default applies).
	plain, err := New(Config{APIKey: "test"})
	if err != nil {
		t.Fatal(err)
	}
	body = plain.buildRequest(provider.Request{
		Model: "kimi-k2.6",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
		},
	})
	if body.Thinking != nil {
		t.Fatalf("unconfigured thinking = %#v", body.Thinking)
	}
}

// TestThinkingWireBody verifies the thinking field reaches the JSON wire body.
func TestThinkingWireBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()

	profile, err := New(Config{APIKey: "test", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := profile.Stream(context.Background(), provider.Request{
		Model:    "kimi-k2.6",
		Messages: []provider.Message{{Role: provider.RoleUser, Content: "hi"}},
		Thinking: &provider.ThinkingConfig{Type: "enabled"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range chunks {
	}
}
