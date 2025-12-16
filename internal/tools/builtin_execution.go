package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/discovery"
	"github.com/golovatskygroup/mcp-lens/internal/executor"
	"github.com/golovatskygroup/mcp-lens/internal/executor/docker"
	"github.com/golovatskygroup/mcp-lens/internal/tool"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

// ExecutionDependencies holds dependencies for code execution tools
type ExecutionDependencies struct {
	DockerManager  *docker.Manager
	NativeExecutor *executor.NativeExecutor
	ToolRegistry   tool.ToolRegistry
	Discovery      *discovery.Client
}

// ===== list_adapters =====

// ListAdaptersParams defines parameters for list_adapters tool
type ListAdaptersParams struct {
	Format string `json:"format,omitempty"` // text|json (default: json)
}

// AdapterInfo represents information about an available API adapter
type AdapterInfo struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Category    string                 `json:"category"`
	Operations  []AdapterOperation     `json:"operations,omitempty"`
	Schema      map[string]interface{} `json:"schema,omitempty"`
}

// AdapterOperation represents a single operation within an adapter
type AdapterOperation struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Method      string                 `json:"method,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema,omitempty"`
	Examples    []string               `json:"examples,omitempty"`
}

func (h *Handler) listAdapters(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params ListAdaptersParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid parameters: " + err.Error()), nil
	}

	if params.Format == "" {
		params.Format = "json"
	}

	adapters := h.buildAdapterCatalog()

	if strings.EqualFold(params.Format, "text") {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Available API Adapters (%d):\n\n", len(adapters)))
		for _, adapter := range adapters {
			sb.WriteString(fmt.Sprintf("## %s\n", adapter.Name))
			sb.WriteString(fmt.Sprintf("Category: %s\n", adapter.Category))
			sb.WriteString(fmt.Sprintf("Description: %s\n", adapter.Description))
			sb.WriteString(fmt.Sprintf("Operations: %d\n\n", len(adapter.Operations)))
		}
		return textResult(sb.String()), nil
	}

	return jsonResult(map[string]interface{}{
		"adapters": adapters,
		"count":    len(adapters),
	}), nil
}

func (h *Handler) buildAdapterCatalog() []AdapterInfo {
	domains := map[string]*AdapterInfo{
		"jira": {
			Name:        "jira",
			Description: "Atlassian Jira API adapter for issue tracking and project management",
			Category:    "project_management",
			Operations:  []AdapterOperation{},
		},
		"confluence": {
			Name:        "confluence",
			Description: "Atlassian Confluence API adapter for documentation and knowledge base",
			Category:    "documentation",
			Operations:  []AdapterOperation{},
		},
		"grafana": {
			Name:        "grafana",
			Description: "Grafana API adapter for monitoring dashboards and metrics",
			Category:    "monitoring",
			Operations:  []AdapterOperation{},
		},
		"github": {
			Name:        "github",
			Description: "GitHub API adapter for code repositories and pull requests",
			Category:    "development",
			Operations:  []AdapterOperation{},
		},
	}

	for _, t := range h.BuiltinTools() {
		for prefix, adapter := range domains {
			if strings.HasPrefix(t.Name, prefix+"_") ||
				strings.HasPrefix(t.Name, "get_pull_request") ||
				strings.HasPrefix(t.Name, "list_pull_request") {
				op := AdapterOperation{
					Name:        t.Name,
					Description: t.Description,
				}
				if t.InputSchema != nil {
					var schema map[string]interface{}
					json.Unmarshal(t.InputSchema, &schema)
					op.InputSchema = schema
				}
				adapter.Operations = append(adapter.Operations, op)
				break
			}
		}
	}

	result := []AdapterInfo{}
	for _, adapter := range domains {
		if len(adapter.Operations) > 0 {
			result = append(result, *adapter)
		}
	}

	return result
}

// ===== execute_code =====

// ExecuteCodeParams defines parameters for execute_code tool
type ExecuteCodeParams struct {
	Language string                 `json:"language"`
	Code     string                 `json:"code"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Timeout  string                 `json:"timeout,omitempty"` // default: 30s
}

// ExecuteCodeResult represents the result of code execution
type ExecuteCodeResult struct {
	Status            string      `json:"status"` // success|error|container_starting
	Result            interface{} `json:"result,omitempty"`
	Error             string      `json:"error,omitempty"`
	Stdout            string      `json:"stdout,omitempty"`
	Stderr            string      `json:"stderr,omitempty"`
	ExecutionTimeMs   int64       `json:"execution_time_ms,omitempty"`
	RetryAfterSeconds int         `json:"retry_after_seconds,omitempty"`
}

func (h *Handler) executeCode(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params ExecuteCodeParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid parameters: " + err.Error()), nil
	}

	if params.Language == "" {
		return errorResult("language is required"), nil
	}
	if params.Code == "" {
		return errorResult("code is required"), nil
	}
	if params.Timeout == "" {
		params.Timeout = "30s"
	}

	timeout, err := time.ParseDuration(params.Timeout)
	if err != nil {
		return errorResult("Invalid timeout format: " + err.Error()), nil
	}

	// Limit timeout to 5 minutes
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Normalize language name
	lang := strings.ToLower(params.Language)

	// Check if native runtime is available
	if h.execDeps != nil && h.execDeps.NativeExecutor != nil {
		switch lang {
		case "go", "golang":
			return h.executeNative(execCtx, executor.LanguageGo, params)
		case "javascript", "js", "node":
			return h.executeNative(execCtx, executor.LanguageJavaScript, params)
		case "python", "py":
			return h.executeNative(execCtx, executor.LanguagePython, params)
		}
	}

	// Use Docker for other languages
	if h.execDeps != nil && h.execDeps.DockerManager != nil {
		return h.executeDocker(execCtx, lang, params)
	}

	return jsonResult(ExecuteCodeResult{
		Status: "error",
		Error:  "Code execution not available: execution dependencies not configured",
	}), nil
}

func (h *Handler) executeNative(ctx context.Context, lang executor.Language, params ExecuteCodeParams) (*mcp.CallToolResult, error) {
	timeout, _ := time.ParseDuration(params.Timeout)
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	req := executor.ExecuteRequest{
		Language: lang,
		Code:     params.Code,
		Input:    params.Input,
		Timeout:  timeout,
	}

	resp, err := h.execDeps.NativeExecutor.Execute(ctx, req)
	if err != nil {
		return jsonResult(ExecuteCodeResult{
			Status: "error",
			Error:  err.Error(),
		}), nil
	}

	result := ExecuteCodeResult{
		Status:          "success",
		Stdout:          resp.Stdout,
		Stderr:          resp.Stderr,
		ExecutionTimeMs: resp.ExecutionMs,
	}

	if resp.Error != "" {
		result.Status = "error"
		result.Error = resp.Error
	} else {
		result.Result = resp.Output
	}

	return jsonResult(result), nil
}

func (h *Handler) executeDocker(ctx context.Context, lang string, params ExecuteCodeParams) (*mcp.CallToolResult, error) {
	// Map string to docker.Language
	var dockerLang docker.Language
	switch lang {
	case "ruby":
		dockerLang = docker.LangRuby
	case "rust":
		dockerLang = docker.LangRust
	case "java":
		dockerLang = docker.LangJava
	case "php":
		dockerLang = docker.LangPHP
	case "bash":
		dockerLang = docker.LangBash
	case "typescript", "ts":
		dockerLang = docker.LangTypeScript
	default:
		return jsonResult(ExecuteCodeResult{
			Status: "error",
			Error:  fmt.Sprintf("Unsupported language for Docker execution: %s", lang),
		}), nil
	}

	// Check runtime status
	status, err := h.execDeps.DockerManager.Status(ctx, dockerLang)
	if err != nil {
		// Try to start the container
		status, err = h.execDeps.DockerManager.Start(ctx, dockerLang)
		if err != nil {
			return jsonResult(ExecuteCodeResult{
				Status: "error",
				Error:  fmt.Sprintf("Failed to start runtime: %v", err),
			}), nil
		}
	}

	if status.Status == docker.StatusStarting {
		return jsonResult(ExecuteCodeResult{
			Status:            "container_starting",
			RetryAfterSeconds: docker.DefaultRetryAfterSeconds,
		}), nil
	}

	if status.Status != docker.StatusReady {
		return jsonResult(ExecuteCodeResult{
			Status: "error",
			Error:  fmt.Sprintf("Container not ready: %s", status.Message),
		}), nil
	}

	// Execute code in container
	output, err := h.execDeps.DockerManager.Execute(ctx, status.ContainerID, params.Code, params.Input)
	if err != nil {
		return jsonResult(ExecuteCodeResult{
			Status: "error",
			Error:  err.Error(),
		}), nil
	}

	return jsonResult(ExecuteCodeResult{
		Status: "success",
		Result: output,
	}), nil
}

// ===== start_runtime =====

// StartRuntimeParams defines parameters for start_runtime tool
type StartRuntimeParams struct {
	Language string `json:"language"`
}

func (h *Handler) startRuntime(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params StartRuntimeParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid parameters: " + err.Error()), nil
	}

	if params.Language == "" {
		return errorResult("language is required"), nil
	}

	if h.execDeps == nil || h.execDeps.DockerManager == nil {
		return errorResult("Docker runtime manager not configured"), nil
	}

	// Map string to docker.Language
	var dockerLang docker.Language
	lang := strings.ToLower(params.Language)
	switch lang {
	case "ruby":
		dockerLang = docker.LangRuby
	case "rust":
		dockerLang = docker.LangRust
	case "java":
		dockerLang = docker.LangJava
	case "php":
		dockerLang = docker.LangPHP
	case "bash":
		dockerLang = docker.LangBash
	case "typescript", "ts":
		dockerLang = docker.LangTypeScript
	default:
		supported := []string{"ruby", "rust", "java", "php", "bash", "typescript"}
		return errorResult(fmt.Sprintf("Unsupported language: %s (supported: %v)", lang, supported)), nil
	}

	status, err := h.execDeps.DockerManager.Start(ctx, dockerLang)
	if err != nil {
		return jsonResult(map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
		}), nil
	}

	return jsonResult(map[string]interface{}{
		"status":       status.Status,
		"language":     status.Language,
		"container_id": status.ContainerID,
		"started_at":   status.StartedAt,
		"message":      status.Message,
	}), nil
}

// ===== register_tool =====

// RegisterToolParams defines parameters for register_tool tool
type RegisterToolParams struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Language    string                 `json:"language"`
	Code        string                 `json:"code"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

func (h *Handler) registerTool(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params RegisterToolParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid parameters: " + err.Error()), nil
	}

	// Validate parameters
	if params.Name == "" {
		return errorResult("name is required"), nil
	}
	if params.Description == "" {
		return errorResult("description is required"), nil
	}
	if params.Language == "" {
		return errorResult("language is required"), nil
	}
	if params.Code == "" {
		return errorResult("code is required"), nil
	}

	// Check for conflicts with built-in tools
	builtinNames := []string{"query", "router", "search_tools", "describe_tool", "execute_tool",
		"list_adapters", "execute_code", "start_runtime", "register_tool", "rollback_tool", "discover_api"}
	for _, name := range builtinNames {
		if params.Name == name {
			return errorResult(fmt.Sprintf("tool name conflicts with built-in tool: %s", name)), nil
		}
	}

	// Check for mutating operations
	mutatingPrefixes := []string{"create", "update", "delete", "remove", "destroy", "modify", "set"}
	for _, prefix := range mutatingPrefixes {
		if strings.HasPrefix(strings.ToLower(params.Name), prefix) {
			return errorResult(fmt.Sprintf("tool name suggests mutating operation (not allowed): %s", params.Name)), nil
		}
	}

	if h.execDeps == nil || h.execDeps.ToolRegistry == nil {
		return errorResult("Tool registry not configured"), nil
	}

	toolDef := &tool.ToolDefinition{
		Name:        params.Name,
		Description: params.Description,
		Language:    params.Language,
		Code:        params.Code,
		InputSchema: params.InputSchema,
	}

	if err := h.execDeps.ToolRegistry.Register(ctx, toolDef); err != nil {
		return errorResult("Failed to register tool: " + err.Error()), nil
	}

	return jsonResult(map[string]interface{}{
		"status":  "success",
		"name":    toolDef.Name,
		"version": toolDef.Version,
		"message": fmt.Sprintf("Tool '%s' registered successfully (version %d)", toolDef.Name, toolDef.Version),
	}), nil
}

// ===== rollback_tool =====

// RollbackToolParams defines parameters for rollback_tool tool
type RollbackToolParams struct {
	Name    string `json:"name"`
	Version int    `json:"version,omitempty"`
}

func (h *Handler) rollbackTool(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params RollbackToolParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid parameters: " + err.Error()), nil
	}

	if params.Name == "" {
		return errorResult("name is required"), nil
	}

	if h.execDeps == nil || h.execDeps.ToolRegistry == nil {
		return errorResult("Tool registry not configured"), nil
	}

	if params.Version <= 0 {
		// Get available versions
		versions, err := h.execDeps.ToolRegistry.GetVersions(ctx, params.Name)
		if err != nil {
			return errorResult("Failed to get tool versions: " + err.Error()), nil
		}

		if len(versions) < 2 {
			return errorResult("No previous version available for rollback"), nil
		}

		// Rollback to previous version
		params.Version = versions[1].Version
	}

	if err := h.execDeps.ToolRegistry.Rollback(ctx, params.Name, params.Version); err != nil {
		return errorResult("Failed to rollback tool: " + err.Error()), nil
	}

	return jsonResult(map[string]interface{}{
		"status":  "success",
		"name":    params.Name,
		"version": params.Version,
		"message": fmt.Sprintf("Tool '%s' rolled back to version %d", params.Name, params.Version),
	}), nil
}

// ===== discover_api =====

// DiscoverAPIParams defines parameters for discover_api tool
type DiscoverAPIParams struct {
	ServiceName    string   `json:"service_name"`
	SearchStrategy string   `json:"search_strategy,omitempty"` // openapi_first, full_discovery, endpoints_only
	FocusAreas     []string `json:"focus_areas,omitempty"`
}

func (h *Handler) discoverAPI(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var params DiscoverAPIParams
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid parameters: " + err.Error()), nil
	}

	if params.ServiceName == "" {
		return errorResult("service_name is required"), nil
	}

	if h.execDeps == nil || h.execDeps.Discovery == nil {
		return errorResult("Discovery client not configured"), nil
	}

	// Map strategy string to discovery.DiscoveryStrategy
	strategy := discovery.StrategyOpenAPIFirst
	switch strings.ToLower(params.SearchStrategy) {
	case "full_discovery":
		strategy = discovery.StrategyFullDiscovery
	case "endpoints_only":
		strategy = discovery.StrategyEndpointsOnly
	}

	req := discovery.DiscoveryRequest{
		ServiceName: params.ServiceName,
		Strategy:    strategy,
		FocusAreas:  params.FocusAreas,
	}

	result, err := h.execDeps.Discovery.DiscoverAPI(ctx, req)
	if err != nil {
		return errorResult("Discovery failed: " + err.Error()), nil
	}

	return jsonResult(result), nil
}

// ===== Tool Definitions =====

// ExecutionToolDefinitions returns the MCP tool definitions for execution tools
func ExecutionToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "list_adapters",
			Description: "List available API adapters (Jira, Confluence, Grafana, GitHub) and their operations.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"format": {"type": "string", "description": "Output format: text or json (default: json)", "enum": ["text", "json"], "default": "json"}
				}
			}`),
		},
		{
			Name:        "execute_code",
			Description: "Execute code in a sandboxed environment. Native support for Go, JavaScript, Python. Docker-based for Ruby, Rust, Java, PHP, Bash, TypeScript.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"language": {"type": "string", "description": "Programming language", "enum": ["go", "javascript", "python", "ruby", "rust", "java", "php", "bash", "typescript"]},
					"code": {"type": "string", "description": "Code to execute"},
					"input": {"type": "object", "description": "Input data passed to the code (available as JSON)"},
					"timeout": {"type": "string", "description": "Execution timeout (e.g., '30s', '1m')", "default": "30s"}
				},
				"required": ["language", "code"]
			}`),
		},
		{
			Name:        "start_runtime",
			Description: "Pre-start a Docker container for a language runtime (warm pool). Reduces latency for first execute_code call.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"language": {"type": "string", "description": "Language runtime to start", "enum": ["ruby", "rust", "java", "php", "bash", "typescript"]}
				},
				"required": ["language"]
			}`),
		},
		{
			Name:        "register_tool",
			Description: "Register a new dynamic tool with code. The tool will be persisted and available for future sessions.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Tool name (snake_case, no mutating prefixes like create_, delete_)"},
					"description": {"type": "string", "description": "Human-readable description"},
					"language": {"type": "string", "description": "Implementation language", "enum": ["go", "javascript", "python"]},
					"code": {"type": "string", "description": "Tool implementation code"},
					"input_schema": {"type": "object", "description": "JSON Schema for tool parameters"}
				},
				"required": ["name", "description", "language", "code", "input_schema"]
			}`),
		},
		{
			Name:        "rollback_tool",
			Description: "Rollback a dynamic tool to a previous version.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Tool name to rollback"},
					"version": {"type": "integer", "description": "Target version (omit to rollback to previous)"}
				},
				"required": ["name"]
			}`),
		},
		{
			Name:        "discover_api",
			Description: "Discover API documentation for a service using Perplexity search. Finds OpenAPI specs, endpoints, authentication methods.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"service_name": {"type": "string", "description": "Service/API name to discover (e.g., 'Stripe API', 'Slack API')"},
					"search_strategy": {"type": "string", "description": "Discovery strategy", "enum": ["openapi_first", "full_discovery", "endpoints_only"], "default": "openapi_first"},
					"focus_areas": {"type": "array", "items": {"type": "string"}, "description": "Specific areas to focus on (e.g., ['authentication', 'webhooks'])"}
				},
				"required": ["service_name"]
			}`),
		},
	}
}
