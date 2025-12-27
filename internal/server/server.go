package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/discovery"
	"github.com/golovatskygroup/mcp-lens/internal/envutil"
	"github.com/golovatskygroup/mcp-lens/internal/executor"
	"github.com/golovatskygroup/mcp-lens/internal/executor/docker"
	"github.com/golovatskygroup/mcp-lens/internal/executor/native"
	"github.com/golovatskygroup/mcp-lens/internal/proxy"
	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/internal/tool"
	"github.com/golovatskygroup/mcp-lens/internal/tools"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"

	_ "github.com/mattn/go-sqlite3" // SQLite driver for tool registry
)

// Server is the main MCP proxy server
type Server struct {
	transport     *mcp.Transport
	registry      *registry.Registry
	proxy         *proxy.Proxy
	handler       *tools.Handler
	ctx           context.Context
	dockerManager *docker.Manager
	toolRegistryDB *sql.DB
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

	// Initialize execution dependencies (Docker, native executor, tool registry, discovery)
	s.initExecutionDependencies(ctx)

	// Make tool discovery include local (proxy-provided) tools as well.
	localTools := s.handler.BuiltinTools()
	// Hide dev-only tools from discovery unless explicitly enabled.
	devOn := envutil.DevModeEnabled()
	if !devOn {
		filtered := make([]mcp.Tool, 0, len(localTools))
		for _, t := range localTools {
			if t.Name == "dev_scaffold_tool" {
				continue
			}
			filtered = append(filtered, t)
		}
		localTools = filtered
	}
	reg.LoadTools(localTools)
	logf("Loaded %d local tools", len(localTools))

	return s
}

// Run starts the server main loop
func (s *Server) Run() error {
	// Cleanup execution dependencies on shutdown
	defer s.Cleanup(s.ctx)

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
	case "resources/list":
		return s.handleListResources(req)
	case "resources/read":
		return s.handleReadResource(req)
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
			Resources: &mcp.ResourcesCapability{
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
	// Expose core tools for code execution + query as main entrypoint.
	// Adapter tools (jira_*, confluence_*, grafana_*, github_*) are accessed via query.
	exposedTools := map[string]bool{
		"query":         true, // Main entrypoint for routing
		"execute_code":  true, // Code execution (native + Docker)
		"start_runtime": true, // Pre-start Docker container
		"register_tool": true, // Register dynamic tool
		"rollback_tool": true, // Rollback tool version
		"discover_api":  true, // API discovery via Perplexity
		"list_adapters": true, // List available API adapters
		"search_tools":  true, // Search available tools
		"describe_tool": true, // Get tool schema
	}

	tools := []mcp.Tool{}
	for _, t := range s.handler.BuiltinTools() {
		if exposedTools[t.Name] {
			tools = append(tools, t)
		}
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

	// Core tools that can be called directly
	directTools := map[string]bool{
		"query":         true,
		"execute_code":  true,
		"start_runtime": true,
		"register_tool": true,
		"rollback_tool": true,
		"discover_api":  true,
		"list_adapters": true,
		"search_tools":  true,
		"describe_tool": true,
		"execute_tool":  true, // For executing discovered tools
	}

	if directTools[params.Name] {
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
	lines := []string{
		"mcp-lens is a read-only MCP proxy that routes requests to internal and upstream tools.",
		"Use `query` for most tasks: provide a free-form request and the proxy will plan, validate (read-only), and execute the necessary steps.",
		"Example: {\"name\":\"query\",\"arguments\":{\"input\":\"Show open PRs in owner/repo\",\"output\":{\"view\":\"summary\"}}}",
		"To discover capabilities, use `search_tools` then `describe_tool` (or just ask via `query`).",
		"For noisy outputs, set `output.view` to `summary` or `errors_only` in `query` arguments.",
		"Large outputs may be returned as artifacts (`artifact_uri` / `artifact_path`); fetch them via `resources/read`.",
		"Never attempt mutations (create/update/delete/merge/push); ask the user for confirmation and return guidance instead.",
	}

	if envutil.DevModeEnabled() {
		lines = append(lines, "Dev mode is enabled: `query` may use dev-only scaffolding tools (e.g. `dev_scaffold_tool`) for generating new local tool code.")
	}

	return strings.Join(lines, "\n")
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[mcp-proxy] "+format+"\n", args...)
}

// initExecutionDependencies initializes the code execution infrastructure.
// Configuration via environment variables:
//   - MCP_ENABLE_DOCKER=1: Enable Docker runtime for code execution (requires Docker)
//   - MCP_ENABLE_NATIVE_EXEC=1: Enable native code execution (Go, JS, Python)
//   - MCP_ENABLE_TOOL_REGISTRY=1: Enable dynamic tool registration
//   - MCP_TOOL_REGISTRY_PATH: Path to SQLite database for tool registry (default: ~/.mcp-lens/tools.db)
//   - PERPLEXITY_API_KEY: API key for Perplexity discovery client
func (s *Server) initExecutionDependencies(ctx context.Context) {
	deps := &tools.ExecutionDependencies{}
	componentsEnabled := 0

	// Initialize Docker manager if enabled
	if os.Getenv("MCP_ENABLE_DOCKER") == "1" {
		dockerMgr, err := docker.NewManager(docker.DefaultConfig())
		if err != nil {
			logf("Warning: Failed to initialize Docker manager: %v", err)
		} else {
			deps.DockerManager = dockerMgr
			s.dockerManager = dockerMgr
			logf("Docker runtime manager initialized")
			componentsEnabled++
		}
	}

	// Initialize native executor if enabled
	if os.Getenv("MCP_ENABLE_NATIVE_EXEC") == "1" {
		// Create sandbox and validator for native execution
		sandbox := executor.DefaultSandboxConfig()
		validator := executor.NewValidator(sandbox)

		// Create native executor
		nativeExec := executor.NewNativeExecutor(sandbox, validator)

		// Create and attach language-specific executors
		goExecutor := native.NewGoExecutor(sandbox)
		jsExecutor := native.NewJSExecutor(sandbox)
		pythonExecutor := native.NewPythonExecutor(sandbox)

		nativeExec.SetGoExecutor(goExecutor)
		nativeExec.SetJavaScriptExecutor(jsExecutor)
		nativeExec.SetPythonExecutor(pythonExecutor)

		deps.NativeExecutor = nativeExec
		logf("Native code executor initialized (Go, JavaScript, Python)")
		componentsEnabled++
	}

	// Initialize tool registry if enabled
	if os.Getenv("MCP_ENABLE_TOOL_REGISTRY") == "1" {
		dbPath := os.Getenv("MCP_TOOL_REGISTRY_PATH")
		if dbPath == "" {
			homeDir, err := os.UserHomeDir()
			if err == nil {
				dbPath = homeDir + "/.mcp-lens/tools.db"
			} else {
				dbPath = "/tmp/mcp-lens-tools.db"
			}
		}

		// Ensure directory exists
		if dir := dbPath[:strings.LastIndex(dbPath, "/")]; dir != "" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				logf("Warning: Failed to create tool registry directory: %v", err)
			}
		}

		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			logf("Warning: Failed to open tool registry database: %v", err)
		} else {
			s.toolRegistryDB = db
			cache := tool.NewToolCache(time.Hour, 100)
			registry, err := tool.NewSQLiteRegistry(db, cache)
			if err != nil {
				logf("Warning: Failed to initialize tool registry: %v", err)
				db.Close()
				s.toolRegistryDB = nil
			} else {
				deps.ToolRegistry = registry
				logf("Tool registry initialized at %s", dbPath)
				componentsEnabled++
			}
		}
	}

	// Initialize discovery client (always available, uses Perplexity API key if set)
	perplexityKey := os.Getenv("PERPLEXITY_API_KEY")
	if perplexityKey != "" {
		discoveryClient := discovery.NewClient(discovery.WithPerplexityAPIKey(perplexityKey))
		deps.Discovery = discoveryClient
		logf("Discovery client initialized with Perplexity API")
		componentsEnabled++
	} else {
		// Initialize without API key - will fail gracefully when used
		discoveryClient := discovery.NewClient()
		deps.Discovery = discoveryClient
		logf("Discovery client initialized (no Perplexity API key - discovery will be limited)")
	}

	// Set dependencies on handler
	if componentsEnabled > 0 {
		s.handler.SetExecutionDependencies(deps)
		logf("Execution dependencies configured (%d components)", componentsEnabled)
	} else {
		logf("No execution dependencies enabled. Set environment variables to enable:")
		logf("  MCP_ENABLE_DOCKER=1      - Docker runtime for code execution")
		logf("  MCP_ENABLE_NATIVE_EXEC=1 - Native execution (Go, JS, Python)")
		logf("  MCP_ENABLE_TOOL_REGISTRY=1 - Dynamic tool registration")
		logf("  PERPLEXITY_API_KEY       - API discovery via Perplexity")
	}
}

// Cleanup releases resources held by the server
func (s *Server) Cleanup(ctx context.Context) {
	if s.dockerManager != nil {
		if err := s.dockerManager.Close(ctx); err != nil {
			logf("Warning: Failed to cleanup Docker manager: %v", err)
		}
	}
	if s.toolRegistryDB != nil {
		if err := s.toolRegistryDB.Close(); err != nil {
			logf("Warning: Failed to close tool registry database: %v", err)
		}
	}
}
