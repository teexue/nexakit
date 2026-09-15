package builtin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/teexue/nexakit/tool"
)

const defaultMaxReadBytes = 1024 * 1024 // 1MB

// ReadFile reads the contents of a file.
type ReadFile struct {
	WorkDir string // sandbox root for path resolution
}

// Name returns the tool name.
func (ReadFile) Name() string { return tool.ReadFileName }

// Description returns a human-readable description.
func (ReadFile) Description() string {
	return "Read the contents of a file. Returns the file content as text. " +
		"offset is always a 1-based line number from the first line of the file. " +
		"If truncated is true, call again with offset=next_offset (also from line 1)."
}

// InputSchema returns the JSON Schema for the tool's input.
func (ReadFile) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file (relative to work directory or absolute)",
			},
			"encoding": map[string]any{
				"type":        "string",
				"description": "File encoding: 'utf-8' (default) or 'base64'",
				"enum":        []string{"utf-8", "base64"},
			},
			"offset": map[string]any{
				"type":        "integer",
				"description": "1-based line number from the start of the file (default 1).",
			},
			"max_bytes": map[string]any{
				"type":        "integer",
				"description": "Maximum bytes to read in this call (default 1048576 = 1MB)",
			},
		},
		"required": []string{"path"},
	}
}

// readFileArgs is the parsed read_file input. Offset is a 1-based start
// line; 0 means line 1.
type readFileArgs struct {
	Path     string
	Encoding string
	Offset   int64
	MaxBytes int
}

// readMeta carries the read position metadata for the output body.
type readMeta struct {
	startLine, nextLine, totalSize int64
	truncated                      bool
}

// Execute runs the tool. offset is a 1-based line number from the first line.
func (r ReadFile) Execute(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	args, err := parseReadFileArgs(input)
	if err != nil {
		return tool.Result{}, fmt.Errorf("parse read_file input: %w", err)
	}

	workDir := resolveWorkDir(ctx, r.WorkDir)
	safePath, err := SafePath(workDir, args.Path)
	if err != nil {
		return tool.Result{}, err
	}

	info, err := os.Stat(safePath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("stat file: %w", err)
	}

	f, err := os.Open(safePath)
	if err != nil {
		return tool.Result{}, fmt.Errorf("read file: %w", err)
	}
	// A close error on a read-only handle cannot affect the returned bytes.
	defer func() { _ = f.Close() }()

	startLine := args.Offset
	if startLine < 1 {
		startLine = 1
	}
	data, nextLine, truncated, err := readFileRange(f, startLine, args.MaxBytes)
	if err != nil {
		return tool.Result{}, fmt.Errorf("read file: %w", err)
	}
	return encodeReadOutput(args, data, readMeta{
		startLine: startLine, nextLine: nextLine,
		truncated: truncated, totalSize: info.Size(),
	})
}

func encodeReadOutput(args readFileArgs, data []byte, meta readMeta) (tool.Result, error) {
	content := string(data)
	if args.Encoding == "base64" {
		content = encodeBase64(data)
	}
	body := map[string]any{
		"path":       args.Path,
		"total_size": meta.totalSize,
		"offset":     meta.startLine,
		"read":       len(data),
		"truncated":  meta.truncated,
		"encoding":   args.Encoding,
		"content":    content,
	}
	if meta.truncated {
		body["next_offset"] = meta.nextLine
	}
	out, _ := json.Marshal(body)
	return tool.Result{Output: out}, nil
}

func parseReadFileArgs(input json.RawMessage) (readFileArgs, error) {
	var raw struct {
		Path     string          `json:"path"`
		Encoding string          `json:"encoding"`
		Offset   json.RawMessage `json:"offset"`
		MaxBytes json.RawMessage `json:"max_bytes"`
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return readFileArgs{}, err
	}
	maxBytes := int(jsonInt(raw.MaxBytes, 0))
	if maxBytes <= 0 {
		maxBytes = defaultMaxReadBytes
	}
	offset := jsonInt(raw.Offset, 0)
	if offset < 0 {
		offset = 0
	}
	return readFileArgs{
		Path: raw.Path, Encoding: raw.Encoding,
		Offset: offset, MaxBytes: maxBytes,
	}, nil
}

// jsonInt extracts an integer from a JSON value leniently. The model sends
// tool arguments as JSON, but real transcripts show numbers also arriving as
// float64 (some providers decode integers to float) or as strings ("1024");
// rejecting those turns a recoverable request into a tool error. Anything
// unparsable falls back to def so the caller applies its own default.
func jsonInt(raw json.RawMessage, def int64) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return def
	}
	var n int64
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return int64(f)
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if v, err := strconv.ParseInt(s, 10, 64); err == nil {
			return v
		}
	}
	return def
}

// readFileRange reads from 1-based startLine until maxBytes. nextLine is the
// 1-based line to pass as offset to continue. truncated is true when more
// content remains after this chunk.
func readFileRange(r io.Reader, startLine int64, maxBytes int) ([]byte, int64, bool, error) {
	if startLine < 1 {
		startLine = 1
	}
	br := bufio.NewReaderSize(r, 32*1024)
	var lineNo int64
	var out []byte
	for {
		line, err := br.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if err == io.EOF {
				return out, startLine, false, nil
			}
			return nil, startLine, false, err
		}
		lineNo++
		if lineNo < startLine {
			if err == io.EOF {
				return out, startLine, false, nil
			}
			continue
		}
		chunk, next, truncated, done := appendLine(out, line, lineNo, maxBytes, err)
		if done {
			return chunk, next, truncated, nil
		}
		out = chunk
	}
}

func appendLine(out, line []byte, lineNo int64, maxBytes int, readErr error) ([]byte, int64, bool, bool) {
	if len(out)+len(line) > maxBytes {
		if len(out) == 0 {
			if maxBytes < len(line) {
				line = line[:maxBytes]
			}
			return append([]byte(nil), line...), lineNo + 1, true, true
		}
		return out, lineNo, true, true
	}
	out = append(out, line...)
	if readErr == io.EOF {
		return out, lineNo + 1, false, true
	}
	return out, 0, false, false
}
