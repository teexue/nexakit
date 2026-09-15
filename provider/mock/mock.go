// Package mock provides test doubles for the provider contract: a scripted
// streaming provider and an echo-then-reply preset. Import only from tests.
package mock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/teexue/nexakit/provider"
)

// MockStep defines one provider response in sequence within a single Stream call.
type MockStep struct {
	Text         string
	Reasoning    string
	ToolCalls    []provider.ToolCall
	OutputTokens int
	FinishReason string
}

// MockProvider returns predefined streaming steps for tests and offline demos.
// Each Stream invocation consumes the next entry in Calls.
// If BlockOnStream is true, Stream blocks until the context is cancelled
// when there are no more predefined calls (useful for testing cancellation).
type MockProvider struct {
	Calls         [][]MockStep
	BlockOnStream bool
	mu            sync.Mutex
	index         int
}

// Stream implements Provider.
func (m *MockProvider) Stream(ctx context.Context, req provider.Request) (<-chan provider.Chunk, error) {
	m.mu.Lock()
	var steps []MockStep
	if m.index < len(m.Calls) {
		steps = m.Calls[m.index]
	}
	m.index++
	m.mu.Unlock()

	ch := make(chan provider.Chunk)
	go func() {
		defer close(ch)
		m.runSteps(ctx, ch, steps)
	}()
	return ch, nil
}

// StreamCallCount returns how many times Stream has been invoked.
func (m *MockProvider) StreamCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.index
}

func (m *MockProvider) runSteps(ctx context.Context, ch chan<- provider.Chunk, steps []MockStep) {
	if len(steps) == 0 {
		m.handleEmptyResponse(ctx, ch)
		return
	}

	for i, step := range steps {
		if ctx.Err() != nil {
			return
		}
		m.emitStep(ctx, ch, step)
		if i == len(steps)-1 {
			sendDone(ctx, ch, step)
		}
	}
}

func (m *MockProvider) handleEmptyResponse(ctx context.Context, ch chan<- provider.Chunk) {
	if m.BlockOnStream {
		<-ctx.Done()
		return
	}
	select {
	case <-ctx.Done():
	case ch <- provider.Chunk{TextDelta: "mock empty response", Done: true}:
	}
}

func (m *MockProvider) emitStep(ctx context.Context, ch chan<- provider.Chunk, step MockStep) {
	if step.Reasoning != "" {
		emitRunes(ctx, ch, step.Reasoning, func(r string) provider.Chunk { return provider.Chunk{ReasoningDelta: r} })
	}
	if step.Text != "" {
		emitRunes(ctx, ch, step.Text, func(r string) provider.Chunk { return provider.Chunk{TextDelta: r} })
	}
	if len(step.ToolCalls) > 0 {
		select {
		case <-ctx.Done():
		case ch <- provider.Chunk{ToolCalls: step.ToolCalls}:
		}
	}
}

func emitRunes(ctx context.Context, ch chan<- provider.Chunk, s string, makeChunk func(string) provider.Chunk) {
	for _, r := range s {
		select {
		case <-ctx.Done():
			return
		case ch <- makeChunk(string(r)):
		}
	}
}

func sendDone(ctx context.Context, ch chan<- provider.Chunk, step MockStep) {
	select {
	case <-ctx.Done():
	case ch <- provider.Chunk{
		Done:         true,
		OutputTokens: step.OutputTokens,
		FinishReason: step.FinishReason,
	}:
	}
}

// EchoThenReply builds a mock that calls echo on first turn then replies on second.
func EchoThenReply(message string) *MockProvider {
	args, _ := json.Marshal(map[string]string{"message": message})
	return &MockProvider{
		Calls: [][]MockStep{
			{{
				ToolCalls: []provider.ToolCall{{
					ID:        "call_1",
					Name:      "echo",
					Arguments: args,
				}},
			}},
			{{Text: fmt.Sprintf("Echo result received for: %s", message)}},
		},
	}
}
