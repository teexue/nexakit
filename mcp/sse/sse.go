// Package sse implements the MCP transport over HTTP Server-Sent Events.
package sse

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/teexue/nexakit/mcp/protocol"
	"github.com/teexue/nexakit/provider"
)

// Client communicates with an MCP server over HTTP SSE.
type Client struct {
	url           string
	name          string
	clientName    string
	clientVersion string
	logger        *slog.Logger

	mu       sync.Mutex
	nextID   atomic.Int64
	pending  map[int64]chan *protocol.Response
	client   *http.Client
	cancelFn context.CancelFunc
	closed   bool
}

// Config configures a Client.
type Config struct {
	Name          string       // display name
	URL           string       // SSE endpoint URL
	Logger        *slog.Logger // optional logger
	HTTPClient    *http.Client // optional HTTP client
	ClientName    string       // handshake client name; empty uses the kit default
	ClientVersion string       // handshake client version; empty uses the kit default
}

// New creates a new SSE Client.
func New(cfg Config) *Client {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	client := cfg.HTTPClient
	if client == nil {
		client = provider.DefaultHTTPClient()
	}
	return &Client{
		url:           cfg.URL,
		name:          cfg.Name,
		clientName:    cfg.ClientName,
		clientVersion: cfg.ClientVersion,
		logger:        logger,
		pending:       make(map[int64]chan *protocol.Response),
		client:        client,
	}
}

// Name returns the configured name of this MCP server.
func (c *Client) Name() string { return c.name }

// Connect establishes the SSE connection and performs the MCP handshake.
func (c *Client) Connect(ctx context.Context) error {
	ctx, c.cancelFn = context.WithCancel(ctx)

	// Start SSE listener.
	go c.listenSSE(ctx)

	if err := protocol.PerformHandshake(ctx, c, protocol.HandshakeIdentity(c.clientName, c.clientVersion)); err != nil {
		return err
	}

	c.logger.Info("log.mcp.sse.connected", "name", c.name)
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

// Close shuts down the connection.
func (c *Client) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	if c.cancelFn != nil {
		c.cancelFn()
	}
	c.logger.Info("log.mcp.sse.disconnected", "name", c.name)
	return nil
}

// SendRequest sends a JSON-RPC request via HTTP POST and waits for the response.
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

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	// The RPC result is already read from ch; the body is drained implicitly.
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r, nil
	}
}

// SendNotification sends a JSON-RPC notification via HTTP POST.
func (c *Client) SendNotification(ctx context.Context, method string, params json.RawMessage) error {
	notif := protocol.Notification{
		JSONRPC: protocol.JSONRPCVersion,
		Method:  method,
		Params:  params,
	}
	body, _ := json.Marshal(notif)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return err
	}
	// Fire-and-forget notification: the response body carries no useful data.
	defer func() { _ = resp.Body.Close() }()
	return nil
}

// listenSSE reads the SSE stream and dispatches responses.
func (c *Client) listenSSE(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.url, nil)
	if err != nil {
		c.logger.Error("log.mcp.sse.connect_failed", "name", c.name, "error", err)
		return
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		c.logger.Error("log.mcp.sse.connect_failed", "name", c.name, "error", err)
		return
	}
	// Closing the long-lived event stream cannot fail meaningfully here.
	defer func() { _ = resp.Body.Close() }()

	reader := bufio.NewReader(resp.Body)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				c.logger.Debug("log.mcp.sse.read_error", "name", c.name, "error", err)
			}
			return
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var resp protocol.Response
		if err := json.Unmarshal([]byte(data), &resp); err != nil {
			continue
		}

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
