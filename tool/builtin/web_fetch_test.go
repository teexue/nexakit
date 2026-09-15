package builtin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/teexue/nexakit/tool/builtin"
)

// TestWebFetchTruncationBoundary pins the truncation semantics: a body that is
// exactly max_bytes long must NOT be reported as truncated. The read-ahead fix
// reads one extra byte to distinguish "exactly the cap" from "more follows".
func TestWebFetchTruncationBoundary(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		maxBytes      int
		wantTruncated bool
	}{
		{"exact size not truncated", "abcdefgh", 8, false},
		{"longer than cap", "abcdefghi", 8, true},
		{"shorter than cap", "abc", 8, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			wf := builtin.WebFetch{}
			input, err := json.Marshal(map[string]any{
				"url":       srv.URL,
				"max_bytes": tt.maxBytes,
			})
			if err != nil {
				t.Fatal(err)
			}
			res, err := wf.Execute(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}

			var out struct {
				Body      string `json:"body"`
				BodyBytes int    `json:"body_bytes"`
				Truncated bool   `json:"truncated"`
			}
			if err := json.Unmarshal(res.Output, &out); err != nil {
				t.Fatal(err)
			}
			if out.Truncated != tt.wantTruncated {
				t.Fatalf("truncated = %v, want %v", out.Truncated, tt.wantTruncated)
			}
			if out.BodyBytes != min(tt.maxBytes, len(tt.body)) {
				t.Fatalf("body_bytes = %d, want %d", out.BodyBytes, min(tt.maxBytes, len(tt.body)))
			}
		})
	}
}
