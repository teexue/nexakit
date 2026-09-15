// Package stdio implements the MCP transport over a subprocess's stdin/stdout.
package stdio

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/teexue/nexakit/mcp/protocol"
	"github.com/teexue/nexakit/provider"
)

// Client communicates with an MCP server over stdin/stdout.
type Client struct {
	command       string
	args          []string
	env           []string
	name          string
	clientName    string
	clientVersion string
	logger        *slog.Logger

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu      sync.Mutex
	nextID  atomic.Int64
	pending map[int64]chan *protocol.Response
	closed  bool
}

// Config configures a Client.
type Config struct {
	Name          string            // display name for the server
	Command       string            // executable path
	Args          []string          // command arguments
	Env           map[string]string // additional environment variables
	Logger        *slog.Logger      // optional logger
	ClientName    string            // handshake client name; empty uses the kit default
	ClientVersion string            // handshake client version; empty uses the kit default
}

// New creates a new stdio Client.
func New(cfg Config) *Client {
	var envSlice []string
	for k, v := range cfg.Env {
		envSlice = append(envSlice, k+"="+v)
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		command:       cfg.Command,
		args:          cfg.Args,
		env:           envSlice,
		name:          cfg.Name,
		clientName:    cfg.ClientName,
		clientVersion: cfg.ClientVersion,
		logger:        logger,
		pending:       make(map[int64]chan *protocol.Response),
	}
}

// Name returns the configured name of this MCP server.
func (c *Client) Name() string { return c.name }

// Connect starts the subprocess and performs the MCP initialize handshake.
func (c *Client) Connect(ctx context.Context) error {
	c.cmd = exec.CommandContext(ctx, c.command, c.args...)
	c.cmd.Env = append(c.cmd.Environ(), c.env...)

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	c.stdout = bufio.NewReader(stdoutPipe)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}

	// Start response reader goroutine.
	go c.readLoop()

	if err := protocol.PerformHandshake(ctx, c, protocol.HandshakeIdentity(c.clientName, c.clientVersion)); err != nil {
		return err
	}

	c.logger.Info("log.mcp.stdio.connected", "name", c.name)
	return nil
}

// ListTools returns the tools provided by the server.
func (c *Client) ListTools(ctx context.Context) ([]provider.ToolDefinition, error) {
	return protocol.ListTools(ctx, c)
}

// CallTool invokes a tool on the server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*protocol.CallToolResult, error) {
	return protocol.CallTool(ctx, c, name, args)
}

// Close shuts down the connection gracefully.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Close stdin to signal the subprocess to exit.
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Wait()
	}
	c.logger.Info("log.mcp.stdio.disconnected", "name", c.name)
	return nil
}

// SendRequest sends a JSON-RPC request and waits for the response.
func (c *Client) SendRequest(ctx context.Context, method string, params json.RawMessage) (*protocol.Response, error) {
	id := c.nextID.Add(1)
	ch := make(chan *protocol.Response, 1)

	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := protocol.Request{
		JSONRPC: protocol.JSONRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}

	if err := c.writeMessage(req); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

// SendNotification sends a JSON-RPC notification (no response expected).
func (c *Client) SendNotification(_ context.Context, method string, params json.RawMessage) error {
	notif := protocol.Notification{
		JSONRPC: protocol.JSONRPCVersion,
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(notif)
}

// writeMessage writes a JSON message followed by a newline to stdin.
func (c *Client) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.stdin.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// readLoop reads JSON-RPC messages from stdout and dispatches responses.
func (c *Client) readLoop() {
	for {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				c.logger.Debug("log.mcp.stdio.read_error", "name", c.name, "error", err)
			}
			return
		}

		// Try to parse as a response (has "id" field).
		var resp protocol.Response
		if err := json.Unmarshal(line, &resp); err != nil {
			c.logger.Debug("log.mcp.stdio.parse_error", "name", c.name, "error", err)
			continue
		}

		// Dispatch to pending request.
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()

		if ok {
			select {
			case ch <- &resp:
			default:
			}
		}
	}
}
