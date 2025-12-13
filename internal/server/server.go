package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nyarum/mcp-proxy/internal/proxy"
	"github.com/nyarum/mcp-proxy/internal/registry"
	"github.com/nyarum/mcp-proxy/internal/tools"
	"github.com/nyarum/mcp-proxy/pkg/mcp"
)

// Server is the main MCP proxy server
type Server struct {
	transport *mcp.Transport
	registry  *registry.Registry
	proxy     *proxy.Proxy
	handler   *tools.Handler
	ctx       context.Context
}

// New creates a new MCP proxy server
func New(ctx context.Context, upstreamCfg proxy.Config) *Server {
	reg := registry.NewRegistry()
	prx := proxy.New(upstreamCfg)

	s := &Server{
		transport: mcp.NewTransport(os.Stdin, os.Stdout),
		registry:  reg,
		proxy:     prx,
		ctx:       ctx,
	}

	// Create handler with executor that calls upstream
	s.handler = tools.NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		return prx.CallTool(ctx, name, args)
	})

	return s
}

// Run starts the server main loop
func (s *Server) Run() error {
	// Start upstream proxy
	if err := s.proxy.Start(s.ctx); err != nil {
		return fmt.Errorf("failed to start upstream: %w", err)
	}
	defer s.proxy.Stop()

	// Fetch tools from upstream
	upstreamTools, err := s.proxy.ListTools(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to list upstream tools: %w", err)
	}

	// Load tools into registry
	s.registry.LoadTools(upstreamTools)
	logf("Loaded %d tools from upstream", len(upstreamTools))

	// Main message loop
	for {
		req, err := s.transport.ReadMessage()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			logf("Error reading message: %v", err)
			continue
		}

		resp := s.handleRequest(req)
		if resp != nil {
			if err := s.transport.WriteResponse(resp); err != nil {
				logf("Error writing response: %v", err)
			}
		}
	}
}

func (s *Server) handleRequest(req *mcp.Request) *mcp.Response {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req)
	case "notifications/initialized":
		// No response needed for notifications
		return nil
	case "tools/list":
		return s.handleListTools(req)
	case "tools/call":
		return s.handleCallTool(req)
	case "ping":
		return s.handlePing(req)
	default:
		return mcp.NewErrorResponse(req.ID, mcp.MethodNotFound, fmt.Sprintf("Method not found: %s", req.Method))
	}
}

func (s *Server) handleInitialize(req *mcp.Request) *mcp.Response {
	result := mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: mcp.ServerCapabilities{
			Tools: &mcp.ToolsCapability{
				ListChanged: true,
			},
		},
		ServerInfo: mcp.ServerInfo{
			Name:    "mcp-github-proxy",
			Version: "1.0.0",
		},
		Instructions: s.buildInstructions(),
	}

	resp, err := mcp.NewResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, err.Error())
	}
	return resp
}

func (s *Server) handleListTools(req *mcp.Request) *mcp.Response {
	// Return only meta-tools + any activated tools
	allTools := s.handler.MetaTools()
	allTools = append(allTools, s.registry.ListActive()...)

	result := mcp.ListToolsResult{
		Tools: allTools,
	}

	resp, err := mcp.NewResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, err.Error())
	}
	return resp
}

func (s *Server) handleCallTool(req *mcp.Request) *mcp.Response {
	var params mcp.CallToolParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InvalidParams, "Invalid params: "+err.Error())
	}

	var result *mcp.CallToolResult
	var err error

	if s.handler.IsMetaTool(params.Name) {
		// Handle meta-tool
		result, err = s.handler.Handle(params.Name, params.Arguments)
	} else {
		// Check if tool is activated or known
		_, known := s.registry.GetTool(params.Name)
		if !known {
			result = &mcp.CallToolResult{
				Content: []mcp.ContentBlock{
					{Type: "text", Text: fmt.Sprintf("Tool '%s' not found. Use search_tools to discover available tools.", params.Name)},
				},
				IsError: true,
			}
		} else {
			// Auto-activate and execute
			s.registry.Activate(params.Name)
			result, err = s.proxy.CallTool(s.ctx, params.Name, params.Arguments)
		}
	}

	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, err.Error())
	}

	resp, err := mcp.NewResponse(req.ID, result)
	if err != nil {
		return mcp.NewErrorResponse(req.ID, mcp.InternalError, err.Error())
	}
	return resp
}

func (s *Server) handlePing(req *mcp.Request) *mcp.Response {
	resp, _ := mcp.NewResponse(req.ID, map[string]any{})
	return resp
}

func (s *Server) buildInstructions() string {
	var sb strings.Builder
	sb.WriteString("GitHub MCP Proxy with Dynamic Tool Discovery.\n\n")
	sb.WriteString("Instead of loading all 50+ GitHub tools, this proxy provides 3 meta-tools:\n")
	sb.WriteString("- search_tools: Find tools by keyword or category\n")
	sb.WriteString("- describe_tool: Get full schema of a specific tool\n")
	sb.WriteString("- execute_tool: Run a discovered tool\n\n")
	sb.WriteString("Available categories:\n")

	for _, cat := range s.registry.ListCategories() {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", cat.Name, cat.Description))
	}

	sb.WriteString(fmt.Sprintf("\nTotal available tools: %d\n", s.registry.ToolCount()))
	return sb.String()
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[mcp-proxy] "+format+"\n", args...)
}
