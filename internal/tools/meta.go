package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

// Handler processes meta-tool calls
type Handler struct {
	registry *registry.Registry
	executor func(name string, args json.RawMessage) (*mcp.CallToolResult, error)
}

// NewHandler creates a new meta-tool handler
func NewHandler(reg *registry.Registry, executor func(string, json.RawMessage) (*mcp.CallToolResult, error)) *Handler {
	return &Handler{
		registry: reg,
		executor: executor,
	}
}

// MetaTools returns the 3 meta-tools that replace all upstream tools
func (h *Handler) MetaTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "search_tools",
			Description: "Search available GitHub tools by keyword or category. Returns tool names and short descriptions. Use this to discover tools before executing them.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {
						"type": "string",
						"description": "Search query (e.g., 'pull request', 'issues', 'code search')"
					},
					"category": {
						"type": "string",
						"description": "Filter by category: repository, issues, pull_requests, reviews, code_search, branches, releases, users, copilot",
						"enum": ["repository", "issues", "pull_requests", "reviews", "code_search", "branches", "releases", "users", "copilot", "sub_issues"]
					},
					"limit": {
						"type": "integer",
						"description": "Max results to return (default: 10)",
						"default": 10
					}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "describe_tool",
			Description: "Get the full schema and description of a specific tool. Use this after search_tools to understand a tool's parameters before executing it.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {
						"type": "string",
						"description": "Exact tool name (from search_tools results)"
					}
				},
				"required": ["name"]
			}`),
		},
		{
			Name:        "execute_tool",
			Description: "Execute a GitHub tool with the given parameters. The tool will be auto-activated for this session.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {
						"type": "string",
						"description": "Tool name to execute"
					},
					"params": {
						"type": "object",
						"description": "Tool-specific parameters (see describe_tool for schema)"
					}
				},
				"required": ["name", "params"]
			}`),
		},
	}
}

// SearchToolsInput represents input for search_tools
type SearchToolsInput struct {
	Query    string `json:"query"`
	Category string `json:"category,omitempty"`
	Limit    int    `json:"limit,omitempty"`
}

// DescribeToolInput represents input for describe_tool
type DescribeToolInput struct {
	Name string `json:"name"`
}

// ExecuteToolInput represents input for execute_tool
type ExecuteToolInput struct {
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params"`
}

// Handle processes a meta-tool call
func (h *Handler) Handle(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	switch name {
	case "search_tools":
		return h.handleSearch(args)
	case "describe_tool":
		return h.handleDescribe(args)
	case "execute_tool":
		return h.handleExecute(args)
	default:
		return nil, fmt.Errorf("unknown meta-tool: %s", name)
	}
}

// IsMetaTool checks if a tool name is a meta-tool
func (h *Handler) IsMetaTool(name string) bool {
	return name == "search_tools" || name == "describe_tool" || name == "execute_tool"
}

func (h *Handler) handleSearch(args json.RawMessage) (*mcp.CallToolResult, error) {
	var input SearchToolsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	if input.Query == "" {
		return errorResult("Query is required"), nil
	}

	results := h.registry.Search(input.Query, input.Category, input.Limit)

	// Format results
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d tools matching '%s':\n\n", len(results), input.Query))

	for i, r := range results {
		sb.WriteString(fmt.Sprintf("%d. **%s** [%s]\n   %s\n\n", i+1, r.Name, r.Category, r.Description))
	}

	if len(results) == 0 {
		sb.WriteString("No tools found. Try a different query or browse categories:\n")
		for _, cat := range h.registry.ListCategories() {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", cat.Name, cat.Description))
		}
	}

	return textResult(sb.String()), nil
}

func (h *Handler) handleDescribe(args json.RawMessage) (*mcp.CallToolResult, error) {
	var input DescribeToolInput
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	if input.Name == "" {
		return errorResult("Tool name is required"), nil
	}

	tool, ok := h.registry.GetTool(input.Name)
	if !ok {
		return errorResult(fmt.Sprintf("Tool '%s' not found. Use search_tools to find available tools.", input.Name)), nil
	}

	// Format tool description
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s\n\n", tool.Name))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", tool.Description))
	sb.WriteString("**Input Schema:**\n```json\n")

	// Pretty print the schema
	var prettySchema map[string]any
	json.Unmarshal(tool.InputSchema, &prettySchema)
	schemaBytes, _ := json.MarshalIndent(prettySchema, "", "  ")
	sb.Write(schemaBytes)
	sb.WriteString("\n```\n")

	return textResult(sb.String()), nil
}

func (h *Handler) handleExecute(args json.RawMessage) (*mcp.CallToolResult, error) {
	var input ExecuteToolInput
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	if input.Name == "" {
		return errorResult("Tool name is required"), nil
	}

	// Check if tool exists
	_, ok := h.registry.GetTool(input.Name)
	if !ok {
		return errorResult(fmt.Sprintf("Tool '%s' not found. Use search_tools to find available tools.", input.Name)), nil
	}

	// Activate tool for this session
	h.registry.Activate(input.Name)

	// Execute via upstream proxy
	return h.executor(input.Name, input.Params)
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.ContentBlock{
			{Type: "text", Text: text},
		},
	}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.ContentBlock{
			{Type: "text", Text: "Error: " + msg},
		},
		IsError: true,
	}
}
