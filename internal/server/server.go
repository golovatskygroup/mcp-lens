package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/proxy"
	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/internal/tools"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
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

	// Make tool discovery include local (proxy-provided) tools as well.
	reg.LoadTools(s.handler.BuiltinTools())
	logf("Loaded %d local tools", len(s.handler.BuiltinTools()))

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
	// Expose only the `query` tool (user-facing entrypoint).
	// `router` is hidden from tools/list but still callable for backwards compatibility.
	var queryTool *mcp.Tool
	for _, t := range s.handler.BuiltinTools() {
		if t.Name == "query" {
			tt := t
			queryTool = &tt
			break
		}
	}

	tools := []mcp.Tool{}
	if queryTool != nil {
		tools = append(tools, *queryTool)
	}

	result := mcp.ListToolsResult{
		Tools: tools,
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

	// `query` is the primary entrypoint; `router` kept for backwards compatibility.
	if params.Name == "query" || params.Name == "router" {
		result, err = s.handler.Handle(s.ctx, params.Name, params.Arguments)
	} else {
		result = &mcp.CallToolResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: "Tool is not exposed directly. Use the 'query' tool to ask a free-form request, and the proxy will plan and execute appropriate tools for you."}},
			IsError: true,
		}
	}

	// Note: upstream tools are only callable through the router.

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
	sb.WriteString("Instead of loading all 50+ GitHub tools, this proxy provides a small set of local tools for discovery + PR review, plus on-demand upstream tools.\n\n")

	sb.WriteString("## How to use this proxy\n\n")
	sb.WriteString("Call the `query` tool with a free-form request. The proxy will:\n")
	sb.WriteString("1. Plan which tools to call (up to max_steps).\n")
	sb.WriteString("2. Validate the plan against a read-only policy.\n")
	sb.WriteString("3. Execute the plan and return results.\n\n")
	sb.WriteString("Example:\n")
	sb.WriteString("  {\"name\": \"query\", \"arguments\": {\"input\": \"Show open PRs in owner/repo\"}}\n\n")

	sb.WriteString("Local tools (used internally by the router):\n")
	sb.WriteString("- search_tools: Find tools by keyword or category (format=text|json)\n")
	sb.WriteString("- describe_tool: Get full schema of a specific tool\n")
	sb.WriteString("- execute_tool: Run an upstream tool (and activate it for the session)\n")
	sb.WriteString("- get_pull_request_details / list_pull_request_files / get_pull_request_diff / list_pull_request_commits / get_pull_request_checks\n")
	sb.WriteString("- prepare_pull_request_review_bundle\n\n")
	sb.WriteString("Jira local tools (read-only by default policy):\n")
	sb.WriteString("- jira_get_myself / jira_list_projects / jira_search_issues / jira_get_issue / jira_get_issue_comments / jira_get_issue_transitions\n\n")

	sb.WriteString("Notes:\n")
	sb.WriteString("- GitHub 404 often means private repo or missing access. Set GITHUB_TOKEN for API access.\n\n")
	sb.WriteString("- Jira auth (local tools):\n")
	sb.WriteString("  - Cloud scripts: set JIRA_BASE_URL + JIRA_EMAIL + JIRA_API_TOKEN\n")
	sb.WriteString("  - Data Center/Server: set JIRA_BASE_URL + JIRA_PAT (Bearer)\n")
	sb.WriteString("  - OAuth 2.0 (3LO): set JIRA_OAUTH_ACCESS_TOKEN + JIRA_CLOUD_ID (uses api.atlassian.com)\n\n")
	sb.WriteString("- Multi-Jira setup:\n")
	sb.WriteString("  - Set JIRA_CLIENTS_JSON (map of client aliases -> config) and optionally JIRA_DEFAULT_CLIENT.\n")
	sb.WriteString("  - In queries, prefix input with `jira <client>` to route Jira calls to that client (e.g., `jira webpower show GO-27`).\n\n")
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
