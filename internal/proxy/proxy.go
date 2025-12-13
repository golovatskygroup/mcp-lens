package proxy

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

// Proxy manages connection to upstream MCP server
type Proxy struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser

	pending   map[int64]chan *mcp.Response
	pendingMu sync.Mutex
	nextID    atomic.Int64

	initialized bool
	initMu      sync.Mutex
}

// Config holds upstream server configuration
type Config struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	Env     map[string]string `yaml:"env"`
}

// New creates a new proxy to upstream MCP server
func New(cfg Config) *Proxy {
	cmd := exec.Command(cfg.Command, cfg.Args...)

	// Set environment
	cmd.Env = os.Environ()
	for k, v := range cfg.Env {
		// Expand environment variables in values
		expanded := os.ExpandEnv(v)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, expanded))
	}

	return &Proxy{
		cmd:     cmd,
		pending: make(map[int64]chan *mcp.Response),
	}
}

// Start starts the upstream MCP server process
func (p *Proxy) Start(ctx context.Context) error {
	var err error

	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := p.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	p.stdout = bufio.NewReader(stdout)

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start upstream: %w", err)
	}

	// Start response reader
	go p.readResponses()

	// Start stderr reader (for debugging)
	go p.readStderr()

	return nil
}

// Initialize sends initialize request to upstream
func (p *Proxy) Initialize(ctx context.Context) error {
	p.initMu.Lock()
	defer p.initMu.Unlock()

	if p.initialized {
		return nil
	}

	params := mcp.InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities: mcp.ClientCapability{
			Roots: &mcp.RootsCapability{ListChanged: true},
		},
		ClientInfo: mcp.ClientInfo{
			Name:    "mcp-proxy",
			Version: "1.0.0",
		},
	}

	resp, err := p.call(ctx, "initialize", params)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	// Send initialized notification
	if err := p.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("initialized notification failed: %w", err)
	}

	p.initialized = true
	return nil
}

// ListTools fetches all tools from upstream
func (p *Proxy) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if err := p.Initialize(ctx); err != nil {
		return nil, err
	}

	resp, err := p.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("list tools error: %s", resp.Error.Message)
	}

	var result mcp.ListToolsResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tools: %w", err)
	}

	return result.Tools, nil
}

// CallTool executes a tool on the upstream server
func (p *Proxy) CallTool(ctx context.Context, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	if err := p.Initialize(ctx); err != nil {
		return nil, err
	}

	params := mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	}

	resp, err := p.call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return &mcp.CallToolResult{
			Content: []mcp.ContentBlock{
				{Type: "text", Text: fmt.Sprintf("Error: %s", resp.Error.Message)},
			},
			IsError: true,
		}, nil
	}

	var result mcp.CallToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	return &result, nil
}

// Stop stops the upstream process
func (p *Proxy) Stop() error {
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.cmd.Process != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

func (p *Proxy) call(ctx context.Context, method string, params any) (*mcp.Response, error) {
	id := p.nextID.Add(1)

	var paramsData json.RawMessage
	if params != nil {
		var err error
		paramsData, err = json.Marshal(params)
		if err != nil {
			return nil, err
		}
	}

	req := mcp.Request{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsData,
	}

	// Create response channel
	respCh := make(chan *mcp.Response, 1)
	p.pendingMu.Lock()
	p.pending[id] = respCh
	p.pendingMu.Unlock()

	defer func() {
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
	}()

	// Send request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if _, err := fmt.Fprintf(p.stdin, "%s\n", data); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *Proxy) notify(method string, params any) error {
	var paramsData json.RawMessage
	if params != nil {
		var err error
		paramsData, err = json.Marshal(params)
		if err != nil {
			return err
		}
	}

	notif := mcp.Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsData,
	}

	data, err := json.Marshal(notif)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintf(p.stdin, "%s\n", data)
	return err
}

func (p *Proxy) readResponses() {
	for {
		line, err := p.stdout.ReadBytes('\n')
		if err != nil {
			return
		}

		var resp mcp.Response
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		// Match response to pending request
		if resp.ID != nil {
			var id int64
			switch v := resp.ID.(type) {
			case float64:
				id = int64(v)
			case int64:
				id = v
			case int:
				id = int64(v)
			default:
				continue
			}

			p.pendingMu.Lock()
			ch, ok := p.pending[id]
			p.pendingMu.Unlock()

			if ok {
				ch <- &resp
			}
		}
	}
}

func (p *Proxy) readStderr() {
	buf := make([]byte, 1024)
	for {
		n, err := p.stderr.Read(buf)
		if err != nil {
			return
		}
		// Log stderr to our stderr for debugging
		os.Stderr.Write(buf[:n])
	}
}
