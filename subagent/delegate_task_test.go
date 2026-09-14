package subagent_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/teexue/nexakit/agent"
	"github.com/teexue/nexakit/event"
	"github.com/teexue/nexakit/loop"
	"github.com/teexue/nexakit/permission"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/session"
	"github.com/teexue/nexakit/subagent"
	"github.com/teexue/nexakit/tool"
)

// tinyPNG is a 1x1 PNG used to exercise image loading.
var tinyPNG = mustDecode("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")

func mustDecode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// stubRegistry is a minimal loop.ToolRegistry for these tests. It avoids
// importing the registry package, which now imports subagent (registering the
// delegate_task tool), so the dependency would otherwise cycle.
type stubRegistry struct{}

// Get reports no tool registered.
func (stubRegistry) Get(string) (tool.Tool, bool) { return nil, false }

// Definitions resolves no tool definitions.
func (stubRegistry) Definitions([]string) ([]provider.ToolDefinition, error) { return nil, nil }

func TestDelegateTask_RequiresSpawn(t *testing.T) {
	res, err := subagent.DelegateTask{}.Execute(context.Background(), json.RawMessage(`{"task":"hi"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
	assert.Equal(t, tool.Result{}, res)
}

func TestDelegateTask_RunsSubAgent(t *testing.T) {
	reg := stubRegistry{}
	parent := &agent.Agent{
		ID: "agt", Name: "parent", Provider: "mock", Model: "m",
		SystemPrompt: "parent", MaxTurns: 3, MaxTokens: 128,
	}
	sess := session.NewForUser("agt", "usr")
	ctx := loop.WithSpawn(context.Background(), loop.Spawn{
		Registry: reg,
		NewProvider: func(*agent.Agent) (provider.Provider, error) {
			return &provider.MockProvider{
				Calls: [][]provider.MockStep{{{Text: "ok"}}},
			}, nil
		},
		Policy:    permission.AllowAllPolicy{},
		Agent:     parent,
		UserID:    "usr",
		SessionID: sess.ID,
	})
	ctx = loop.WithParentEventChan(ctx, make(chan event.Event, 8))

	res, err := subagent.DelegateTask{}.Execute(ctx, json.RawMessage(`{"task":"summarize this"}`))
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(res.Output, &out))
	assert.Equal(t, "ok", out["response"])
	assert.Equal(t, "completed", out["status"])
	assert.NotEmpty(t, out["session_id"])
}

func TestDelegateTask_ChildCannotNest(t *testing.T) {
	reg := stubRegistry{}
	parent := &agent.Agent{
		ID: "agt", Name: "parent", Provider: "mock", Model: "m",
		SystemPrompt: "parent", MaxTurns: 3, MaxTokens: 128,
	}
	ctx := loop.WithSpawn(context.Background(), loop.Spawn{
		Registry: reg,
		NewProvider: func(*agent.Agent) (provider.Provider, error) {
			return &provider.MockProvider{
				Calls: [][]provider.MockStep{{{Text: "ok"}}},
			}, nil
		},
		Policy: permission.AllowAllPolicy{},
		Agent:  parent,
		Depth:  1,
		Subagent: loop.SubagentLimits{
			Enabled: true, MaxTurns: 5, MaxDepth: 1,
		},
	})
	ctx = loop.WithParentEventChan(ctx, make(chan event.Event, 8))
	_, err := subagent.DelegateTask{}.Execute(ctx, json.RawMessage(`{"task":"nested"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depth limit")
}

func TestDelegateTask_LoadsImages(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dot.png"), tinyPNG, 0o644))

	reg := stubRegistry{}
	parent := &agent.Agent{
		ID: "agt", Name: "parent", Provider: "mock", Model: "m",
		SystemPrompt: "parent", MaxTurns: 3, MaxTokens: 128,
	}
	ctx := loop.WithSpawn(context.Background(), loop.Spawn{
		Registry: reg,
		NewProvider: func(*agent.Agent) (provider.Provider, error) {
			return &provider.MockProvider{
				Calls: [][]provider.MockStep{{{Text: "saw image"}}},
			}, nil
		},
		Policy:  permission.AllowAllPolicy{},
		Agent:   parent,
		WorkDir: dir,
	})
	ctx = loop.WithParentEventChan(ctx, make(chan event.Event, 8))

	res, err := subagent.DelegateTask{}.Execute(ctx, json.RawMessage(`{"task":"describe","images":["dot.png"]}`))
	require.NoError(t, err)
	var out map[string]any
	require.NoError(t, json.Unmarshal(res.Output, &out))
	assert.Equal(t, "saw image", out["response"])
	assert.EqualValues(t, 1, out["images"])
}

func TestDelegateTask_BadImage(t *testing.T) {
	dir := t.TempDir()
	reg := stubRegistry{}
	parent := &agent.Agent{
		ID: "agt", Name: "parent", Provider: "mock", Model: "m",
		SystemPrompt: "parent", MaxTurns: 3, MaxTokens: 128,
	}
	ctx := loop.WithSpawn(context.Background(), loop.Spawn{
		Registry: reg,
		NewProvider: func(*agent.Agent) (provider.Provider, error) {
			return &provider.MockProvider{Calls: [][]provider.MockStep{{{Text: "ok"}}}}, nil
		},
		Policy:  permission.AllowAllPolicy{},
		Agent:   parent,
		WorkDir: dir,
	})
	ctx = loop.WithParentEventChan(ctx, make(chan event.Event, 8))
	_, err := subagent.DelegateTask{}.Execute(ctx, json.RawMessage(`{"task":"x","images":["missing.png"]}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load image")
}
