package tool

import (
	"context"
	"fmt"
)

// BuiltinTool represents a built-in tool with its handler
type BuiltinTool struct {
	Definition *ToolDefinition
	Handler    func(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// GetBuiltinTools returns all built-in tools
func GetBuiltinTools() []*BuiltinTool {
	return []*BuiltinTool{
		listAdaptersTool(),
		executeCodeTool(),
		startRuntimeTool(),
		registerToolTool(),
		rollbackToolTool(),
		discoverAPITool(),
	}
}

// listAdaptersTool returns the list_adapters tool
func listAdaptersTool() *BuiltinTool {
	return &BuiltinTool{
		Definition: &ToolDefinition{
			Name:        "list_adapters",
			Description: "List all available API adapters with their schemas",
			Language:    "builtin",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"filter": map[string]interface{}{
						"type":        "string",
						"description": "Filter adapters by type (rest, graphql, openapi)",
					},
				},
			},
			Status: "active",
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// TODO: Implement adapter listing
			return map[string]interface{}{
				"adapters": []interface{}{},
				"message":  "Adapter listing not yet implemented",
			}, nil
		},
	}
}

// executeCodeTool returns the execute_code tool
func executeCodeTool() *BuiltinTool {
	return &BuiltinTool{
		Definition: &ToolDefinition{
			Name:        "execute_code",
			Description: "Execute code in any supported language. Native runtimes: go, javascript, python. Others via Docker.",
			Language:    "builtin",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"language": map[string]interface{}{
						"type": "string",
						"enum": []string{"go", "javascript", "python", "ruby", "rust", "java", "php", "bash"},
						"description": "Programming language to execute code in",
					},
					"code": map[string]interface{}{
						"type":        "string",
						"description": "Code to execute",
					},
					"input": map[string]interface{}{
						"type":        "object",
						"description": "Input parameters available to the code",
					},
					"timeout": map[string]interface{}{
						"type":        "string",
						"default":     "30s",
						"description": "Execution timeout",
					},
				},
				"required": []string{"language", "code"},
			},
			Status: "active",
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// TODO: Implement code execution
			return map[string]interface{}{
				"status":  "error",
				"message": "Code execution not yet implemented",
			}, nil
		},
	}
}

// startRuntimeTool returns the start_runtime tool
func startRuntimeTool() *BuiltinTool {
	return &BuiltinTool{
		Definition: &ToolDefinition{
			Name:        "start_runtime",
			Description: "Pre-start a Docker container for a language runtime. Use before execute_code for non-native languages.",
			Language:    "builtin",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"language": map[string]interface{}{
						"type": "string",
						"enum": []string{"ruby", "rust", "java", "php", "bash", "typescript"},
						"description": "Language to start runtime for",
					},
				},
				"required": []string{"language"},
			},
			Status: "active",
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// TODO: Implement runtime startup
			return map[string]interface{}{
				"status":  "error",
				"message": "Runtime startup not yet implemented",
			}, nil
		},
	}
}

// registerToolTool returns the register_tool tool
func registerToolTool() *BuiltinTool {
	return &BuiltinTool{
		Definition: &ToolDefinition{
			Name:        "register_tool",
			Description: "Register executed code as a new reusable tool",
			Language:    "builtin",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Tool name (snake_case)",
					},
					"description": map[string]interface{}{
						"type":        "string",
						"description": "Tool description",
					},
					"language": map[string]interface{}{
						"type":        "string",
						"description": "Programming language",
					},
					"code": map[string]interface{}{
						"type":        "string",
						"description": "Tool code",
					},
					"input_schema": map[string]interface{}{
						"type":        "object",
						"description": "JSON Schema for tool input",
					},
				},
				"required": []string{"name", "description", "language", "code", "input_schema"},
			},
			Status: "active",
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// TODO: Implement tool registration
			return map[string]interface{}{
				"status":  "error",
				"message": "Tool registration not yet implemented",
			}, nil
		},
	}
}

// rollbackToolTool returns the rollback_tool tool
func rollbackToolTool() *BuiltinTool {
	return &BuiltinTool{
		Definition: &ToolDefinition{
			Name:        "rollback_tool",
			Description: "Rollback a tool to a previous version",
			Language:    "builtin",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name": map[string]interface{}{
						"type":        "string",
						"description": "Tool name",
					},
					"version": map[string]interface{}{
						"type":        "integer",
						"description": "Version number to rollback to",
					},
				},
				"required": []string{"name", "version"},
			},
			Status: "active",
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// TODO: Implement tool rollback
			return map[string]interface{}{
				"status":  "error",
				"message": "Tool rollback not yet implemented",
			}, nil
		},
	}
}

// discoverAPITool returns the discover_api tool
func discoverAPITool() *BuiltinTool {
	return &BuiltinTool{
		Definition: &ToolDefinition{
			Name:        "discover_api",
			Description: "Discover API documentation and OpenAPI specs for any service using Perplexity AI",
			Language:    "builtin",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"service_name": map[string]interface{}{
						"type":        "string",
						"description": "Name of the service (e.g., 'Stripe', 'GitHub', 'Slack')",
					},
					"search_strategy": map[string]interface{}{
						"type":        "string",
						"enum":        []string{"openapi_first", "full_discovery", "endpoints_only"},
						"default":     "openapi_first",
						"description": "Discovery strategy to use",
					},
					"focus_areas": map[string]interface{}{
						"type": "array",
						"items": map[string]interface{}{
							"type": "string",
						},
						"description": "Specific areas to focus on (e.g., 'authentication', 'rate limits')",
					},
				},
				"required": []string{"service_name"},
			},
			Status: "active",
		},
		Handler: func(ctx context.Context, params map[string]interface{}) (interface{}, error) {
			// TODO: Implement API discovery
			return map[string]interface{}{
				"status":  "error",
				"message": "API discovery not yet implemented",
			}, nil
		},
	}
}

// ExecuteBuiltinTool executes a built-in tool by name
func ExecuteBuiltinTool(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	builtins := GetBuiltinTools()
	for _, tool := range builtins {
		if tool.Definition.Name == name {
			return tool.Handler(ctx, params)
		}
	}

	return nil, fmt.Errorf("builtin tool not found: %s", name)
}

// IsBuiltinTool checks if a tool name is a built-in tool
func IsBuiltinTool(name string) bool {
	builtins := GetBuiltinTools()
	for _, tool := range builtins {
		if tool.Definition.Name == name {
			return true
		}
	}
	return false
}
