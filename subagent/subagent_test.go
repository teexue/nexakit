package subagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/teexue/nexakit/agent"
	"github.com/teexue/nexakit/event"
	"github.com/teexue/nexakit/loop"
	"github.com/teexue/nexakit/permission"
	"github.com/teexue/nexakit/provider"
	"github.com/teexue/nexakit/provider/mock"
	"github.com/teexue/nexakit/session"
	"github.com/teexue/nexakit/tool"
)

type testTool struct {
	name string
}

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string { return "test tool" }
func (t *testTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *testTool) Execute(_ context.Context, _ json.RawMessage) (tool.Result, error) {
	return tool.Result{Output: json.RawMessage(`"ok"`)}, nil
}

// stubRegistry is a tiny tool.Registry implementation. The real registry
// package imports subagent (to register the delegate_task tool), so using it
// here would create an import cycle; these tests only need Get/Definitions.
type stubRegistry struct {
	tools map[string]tool.Tool
}

func newStubRegistry() *stubRegistry {
	return &stubRegistry{tools: map[string]tool.Tool{}}
}

func (r *stubRegistry) register(t tool.Tool) { r.tools[t.Name()] = t }

func (r *stubRegistry) Get(name string) (tool.Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *stubRegistry) Definitions(names []string) ([]provider.ToolDefinition, error) {
	defs := make([]provider.ToolDefinition, 0, len(names))
	for _, n := range names {
		if t, ok := r.tools[n]; ok {
			defs = append(defs, provider.ToolDefinition{Name: t.Name(), Description: t.Description(), Parameters: t.InputSchema()})
		}
	}
	return defs, nil
}

func setupDeps() Deps {
	reg := newStubRegistry()
	reg.register(&testTool{name: "echo"})

	return Deps{
		AgentsDir: "/tmp/nonexistent-agents",
		Registry:  reg,
		NewProvider: func(a *agent.Agent) (provider.Provider, error) {
			return &mock.MockProvider{
				Calls: [][]mock.MockStep{
					{{Text: "sub-agent response"}},
				},
			}, nil
		},
		Policy: permission.AllowAllPolicy{},
	}
}

func TestRun_BasicExecution(t *testing.T) {
	deps := setupDeps()
	ctx := context.Background()
	out := make(chan event.Event, 100)

	result, err := Run(ctx, Config{
		Task:  "do something",
		Depth: 0,
	}, deps, out)
	if err != nil {
		t.Fatal(err)
	}

	if result.Response != "sub-agent response" {
		t.Errorf("expected 'sub-agent response', got %q", result.Response)
	}
	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %q", result.Status)
	}
}

func TestChildLoopConfig_IncludesImages(t *testing.T) {
	imgs := []provider.ContentPart{{
		Type:     "image_url",
		ImageURL: &provider.ImageURL{URL: "data:image/png;base64,x"},
	}}
	cfg := childLoopConfig(
		Config{Task: "look", Images: imgs},
		Deps{},
		&agent.Agent{Name: "a"},
		nil,
		session.New("child"),
	)
	if len(cfg.Images) != 1 {
		t.Fatalf("Images = %d, want 1", len(cfg.Images))
	}
	if cfg.Images[0].Type != "image_url" {
		t.Fatalf("Images[0].Type = %q", cfg.Images[0].Type)
	}
}

func TestRun_EmitsSubAgentEvents(t *testing.T) {
	deps := setupDeps()
	ctx := context.Background()
	out := make(chan event.Event, 100)

	_, err := Run(ctx, Config{
		Task:  "do something",
		Depth: 0,
	}, deps, out)
	if err != nil {
		t.Fatal(err)
	}

	close(out)
	var hasStart, hasEnd bool
	for ev := range out {
		if ev.Type == event.TypeSubAgentStart && ev.SessionID != "" {
			hasStart = true
			if ev.Status != StatusRunning {
				t.Errorf("expected status %q, got %q", StatusRunning, ev.Status)
			}
		}
		if ev.Type == event.TypeSubAgentEnd {
			hasEnd = true
		}
	}

	if !hasStart {
		t.Error("expected TypeSubAgentStart event")
	}
	if !hasEnd {
		t.Error("expected TypeSubAgentEnd event")
	}
}

func TestRun_FirstChildAllowedAtMaxDepth(t *testing.T) {
	deps := setupDeps()
	_, err := Run(context.Background(), Config{
		Task:   "do something",
		Depth:  1,
		Limits: loop.SubagentLimits{Enabled: true, MaxTurns: 5, MaxDepth: 1},
	}, deps, make(chan event.Event, 100))
	if err != nil {
		t.Fatalf("first child at max depth 1 should run: %v", err)
	}
}

func TestRun_DepthLimitExceeded(t *testing.T) {
	deps := setupDeps()
	_, err := Run(context.Background(), Config{
		Task:  "do something",
		Depth: DefaultMaxDepth + 1,
	}, deps, make(chan event.Event, 100))
	if err == nil {
		t.Fatal("expected depth limit error")
	}
}

func TestRun_WithContext(t *testing.T) {
	deps := setupDeps()
	ctx := context.Background()
	out := make(chan event.Event, 100)

	result, err := Run(ctx, Config{
		Task:    "summarize",
		Context: "Here is the data to summarize",
		Depth:   0,
	}, deps, out)
	if err != nil {
		t.Fatal(err)
	}

	if result.Response == "" {
		t.Error("expected non-empty response")
	}
}

func TestRun_WithMaxTurns(t *testing.T) {
	deps := setupDeps()
	ctx := context.Background()
	out := make(chan event.Event, 100)

	result, err := Run(ctx, Config{
		Task:   "do something",
		Depth:  0,
		Limits: loop.SubagentLimits{Enabled: true, MaxTurns: 3, MaxDepth: 1},
	}, deps, out)
	if err != nil {
		t.Fatal(err)
	}

	if result.Turns > 3 {
		t.Errorf("expected at most 3 turns, got %d", result.Turns)
	}
}

func TestRun_WithAgentName_NotFound(t *testing.T) {
	deps := setupDeps()
	ctx := context.Background()
	out := make(chan event.Event, 100)

	_, err := Run(ctx, Config{
		AgentName: "nonexistent",
		Task:      "do something",
		Depth:     0,
	}, deps, out)
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestRun_CancelledContext(t *testing.T) {
	deps := setupDeps()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	out := make(chan event.Event, 100)

	// Should still complete (mock provider doesn't respect context cancellation).
	_, err := Run(ctx, Config{
		Task:  "do something",
		Depth: 0,
	}, deps, out)
	// May or may not error depending on timing, but should not panic.
	_ = err
}

type memStore struct {
	sessions map[string]*session.Session
}

func (m *memStore) Save(sess *session.Session) error {
	if m.sessions == nil {
		m.sessions = map[string]*session.Session{}
	}
	m.sessions[sess.ID] = sess
	return nil
}

func (m *memStore) Load(id string) (*session.Session, error) {
	sess, ok := m.sessions[id]
	if !ok {
		return nil, session.ErrNotFound
	}
	return sess, nil
}

func (m *memStore) List() ([]session.Meta, error) { return nil, nil }
func (m *memStore) Delete(id string) error        { return nil }

func TestRun_PersistsChildSession(t *testing.T) {
	store := &memStore{}
	deps := setupDeps()
	deps.Store = store
	deps.ParentSessionID = "parent-1"
	deps.UserID = "usr"
	out := make(chan event.Event, 100)

	result, err := Run(context.Background(), Config{Task: "nested work", Depth: 0}, deps, out)
	if err != nil {
		t.Fatal(err)
	}
	if result.SessionID == "" {
		t.Fatal("expected child session id")
	}
	saved, err := store.Load(result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetMetadata()[session.MetadataKeySource] != session.SourceSubagent {
		t.Errorf("source = %q", saved.GetMetadata()[session.MetadataKeySource])
	}
	if saved.GetMetadata()[session.MetadataKeyParentSession] != "parent-1" {
		t.Errorf("parent = %q", saved.GetMetadata()[session.MetadataKeyParentSession])
	}
}

func TestLoadSubAgent_StripsDelegateAtMaxDepth(t *testing.T) {
	deps := setupDeps()
	reg := deps.Registry.(*stubRegistry)
	reg.register(&testTool{name: ToolName})
	deps.ParentAgent = &agent.Agent{Name: "p", Tools: []string{"echo"}}
	a, err := loadSubAgent(deps, Config{Depth: 1}, loop.SubagentLimits{
		Enabled: true, MaxTurns: 5, MaxDepth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range a.Tools {
		if n == ToolName {
			t.Fatal("child at max depth must not receive delegate_task")
		}
	}
}

func TestLoadSubAgent_KeepsDelegateBelowMaxDepth(t *testing.T) {
	deps := setupDeps()
	reg := deps.Registry.(*stubRegistry)
	reg.register(&testTool{name: ToolName})
	deps.ParentAgent = &agent.Agent{Name: "p", Tools: []string{"echo"}}
	a, err := loadSubAgent(deps, Config{Depth: 1}, loop.SubagentLimits{
		Enabled: true, MaxTurns: 5, MaxDepth: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range a.Tools {
		if n == ToolName {
			found = true
		}
	}
	if !found {
		t.Fatal("expected delegate_task when another nesting level remains")
	}
}
