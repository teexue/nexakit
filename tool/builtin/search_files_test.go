package builtin_test

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/teexue/nexakit/tool/builtin"
)

// TestSearchFilesSkipsBinaryFiles verifies that binary content cannot produce
// garbled pseudo-matches: a NUL byte in the sniff window marks the file binary
// and it is skipped entirely.
func TestSearchFilesSkipsBinaryFiles(t *testing.T) {
	dir := t.TempDir()
	textPath := dir + "/note.txt"
	if err := os.WriteFile(textPath, []byte("needle in a text file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	binPath := dir + "/blob.bin"
	bin := append([]byte("PNG\x00\x01\x02"), make([]byte, 100)...)
	bin = append(bin, []byte("needle inside binary")...)
	if err := os.WriteFile(binPath, bin, 0o644); err != nil {
		t.Fatal(err)
	}

	sf := builtin.SearchFiles{WorkDir: dir}
	input, err := json.Marshal(map[string]any{"pattern": "needle"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := sf.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		Count   int `json:"count"`
		Matches []struct {
			File string `json:"file"`
			Text string `json:"text"`
		} `json:"matches"`
	}
	if err := json.Unmarshal(res.Output, &out); err != nil {
		t.Fatal(err)
	}
	if out.Count != 1 {
		t.Fatalf("count = %d, want 1 (text file only, binary skipped)", out.Count)
	}
	if !strings.HasSuffix(out.Matches[0].File, "note.txt") {
		t.Fatalf("match file = %q, want note.txt", out.Matches[0].File)
	}
}
