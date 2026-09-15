package loop_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teexue/nexakit/agent"
	"github.com/teexue/nexakit/loop"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/registry"
	"github.com/teexue/nexakit/session"
)

// captureProvider records the provider.Request it receives, then replays a
// trivial text response so the loop terminates after one turn.
type captureProvider struct {
	got chan provider.Request
}

func newCaptureProvider() *captureProvider {
	return &captureProvider{got: make(chan provider.Request, 1)}
}

func (p *captureProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	select {
	case p.got <- req:
	default:
	}
	// The loop consumes the channel until it is closed, so close it after
	// the terminal chunk (matching the real provider contract).
	ch := make(chan provider.Chunk, 2)
	ch <- provider.Chunk{TextDelta: "ok", Done: true}
	close(ch)
	return ch, nil
}

// TestRunForwardsAgentThinking verifies the agent.Thinking → Request.Thinking
// leg of the chain end to end.
func TestRunForwardsAgentThinking(t *testing.T) {
	reg := registry.New()
	registry.RegisterBuiltin(reg, t.TempDir())

	capture := newCaptureProvider()
	sc := &agent.Agent{
		Name:     "test",
		Provider: "mock",
		Model:    "some-model",
		MaxTurns: 3,
		Thinking: &provider.ThinkingConfig{Type: "disabled"},
	}

	events, err := loop.Run(context.Background(), loop.Config{
		Provider: capture,
		Registry: reg,
		Agent:    sc,
		Session:  session.New(sc.Name),
		Prompt:   "hello",
	})
	require.NoError(t, err)
	for range events {
	}

	select {
	case req := <-capture.got:
		require.NotNil(t, req.Thinking, "agent.Thinking must reach provider.Request")
		assert.Equal(t, "disabled", req.Thinking.Type)
	default:
		t.Fatal("provider was never called")
	}
}
