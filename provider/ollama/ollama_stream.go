package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	"github.com/teexue/nexakit/provider"
)

// ollamaStreamEvent is one NDJSON line from /api/chat.
type ollamaStreamEvent struct {
	Message ollamaStreamMessage `json:"message"`
	Done    bool                `json:"done"`
	// DoneReason is set on the terminal event (e.g. "stop", "length").
	DoneReason string `json:"done_reason"`
	// usage fields on the terminal event
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

type ollamaStreamMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	Thinking  string           `json:"thinking"`
	ToolCalls []ollamaToolCall `json:"tool_calls"`
}

// toolCallCounter generates stable IDs for Ollama tool calls, which do not
// carry an ID on the wire.
var toolCallCounter uint64

func (o *Ollama) readStream(ctx context.Context, r io.Reader, ch chan<- provider.Chunk) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev ollamaStreamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}

		if ev.Message.Thinking != "" {
			provider.SendChunk(ctx, ch, provider.Chunk{ReasoningDelta: ev.Message.Thinking})
		}
		if ev.Message.Content != "" {
			provider.SendChunk(ctx, ch, provider.Chunk{TextDelta: ev.Message.Content})
		}
		if len(ev.Message.ToolCalls) > 0 {
			calls := make([]provider.ToolCall, 0, len(ev.Message.ToolCalls))
			for _, tc := range ev.Message.ToolCalls {
				args := tc.Function.Arguments
				if len(args) == 0 {
					args = json.RawMessage("{}")
				}
				calls = append(calls, provider.ToolCall{
					ID:        fmt.Sprintf("call_%d", atomic.AddUint64(&toolCallCounter, 1)),
					Name:      tc.Function.Name,
					Arguments: args,
				})
			}
			provider.SendChunk(ctx, ch, provider.Chunk{ToolCalls: calls})
		}

		if ev.Done {
			provider.SendChunk(ctx, ch, provider.Chunk{
				Done:         true,
				FinishReason: ev.DoneReason,
				InputTokens:  ev.PromptEvalCount,
				OutputTokens: ev.EvalCount,
			})
			return
		}
	}
	// Stream ended without an explicit done event; signal completion so the
	// loop does not wait forever.
	provider.SendChunk(ctx, ch, provider.Chunk{Done: true})
}
