package tool

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetBuiltinTools(t *testing.T) {
	builtins := GetBuiltinTools()

	assert.Len(t, builtins, 6)

	// Verify all expected tools are present
	expectedTools := []string{
		"list_adapters",
		"execute_code",
		"start_runtime",
		"register_tool",
		"rollback_tool",
		"discover_api",
	}

	for _, expected := range expectedTools {
		found := false
		for _, tool := range builtins {
			if tool.Definition.Name == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected tool not found: %s", expected)
	}
}

func TestBuiltinTool_ListAdapters(t *testing.T) {
	tool := listAdaptersTool()

	assert.Equal(t, "list_adapters", tool.Definition.Name)
	assert.NotEmpty(t, tool.Definition.Description)
	assert.Equal(t, "builtin", tool.Definition.Language)
	assert.NotNil(t, tool.Definition.InputSchema)
	assert.NotNil(t, tool.Handler)

	// Test handler
	ctx := context.Background()
	params := map[string]interface{}{}
	result, err := tool.Handler(ctx, params)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBuiltinTool_ExecuteCode(t *testing.T) {
	tool := executeCodeTool()

	assert.Equal(t, "execute_code", tool.Definition.Name)
	assert.Contains(t, tool.Definition.Description, "Execute code")
	assert.Equal(t, "builtin", tool.Definition.Language)

	// Verify input schema has required fields
	schema := tool.Definition.InputSchema
	props := schema["properties"].(map[string]interface{})
	assert.Contains(t, props, "language")
	assert.Contains(t, props, "code")

	required := schema["required"].([]string)
	assert.Contains(t, required, "language")
	assert.Contains(t, required, "code")

	// Test handler
	ctx := context.Background()
	params := map[string]interface{}{
		"language": "go",
		"code":     "func main() {}",
	}
	result, err := tool.Handler(ctx, params)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBuiltinTool_StartRuntime(t *testing.T) {
	tool := startRuntimeTool()

	assert.Equal(t, "start_runtime", tool.Definition.Name)
	assert.Equal(t, "builtin", tool.Definition.Language)

	// Verify input schema
	schema := tool.Definition.InputSchema
	props := schema["properties"].(map[string]interface{})
	langProp := props["language"].(map[string]interface{})

	enum := langProp["enum"].([]string)
	assert.Contains(t, enum, "ruby")
	assert.Contains(t, enum, "rust")

	// Test handler
	ctx := context.Background()
	params := map[string]interface{}{
		"language": "rust",
	}
	result, err := tool.Handler(ctx, params)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBuiltinTool_RegisterTool(t *testing.T) {
	tool := registerToolTool()

	assert.Equal(t, "register_tool", tool.Definition.Name)
	assert.Equal(t, "builtin", tool.Definition.Language)

	// Verify required fields
	schema := tool.Definition.InputSchema
	required := schema["required"].([]string)
	assert.Contains(t, required, "name")
	assert.Contains(t, required, "description")
	assert.Contains(t, required, "language")
	assert.Contains(t, required, "code")
	assert.Contains(t, required, "input_schema")

	// Test handler
	ctx := context.Background()
	params := map[string]interface{}{
		"name":         "my_tool",
		"description":  "My tool",
		"language":     "go",
		"code":         "code",
		"input_schema": map[string]interface{}{"type": "object"},
	}
	result, err := tool.Handler(ctx, params)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBuiltinTool_RollbackTool(t *testing.T) {
	tool := rollbackToolTool()

	assert.Equal(t, "rollback_tool", tool.Definition.Name)
	assert.Equal(t, "builtin", tool.Definition.Language)

	// Verify required fields
	schema := tool.Definition.InputSchema
	required := schema["required"].([]string)
	assert.Contains(t, required, "name")
	assert.Contains(t, required, "version")

	// Test handler
	ctx := context.Background()
	params := map[string]interface{}{
		"name":    "my_tool",
		"version": 1,
	}
	result, err := tool.Handler(ctx, params)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestBuiltinTool_DiscoverAPI(t *testing.T) {
	tool := discoverAPITool()

	assert.Equal(t, "discover_api", tool.Definition.Name)
	assert.Contains(t, tool.Definition.Description, "Discover API")
	assert.Equal(t, "builtin", tool.Definition.Language)

	// Verify input schema
	schema := tool.Definition.InputSchema
	props := schema["properties"].(map[string]interface{})
	assert.Contains(t, props, "service_name")
	assert.Contains(t, props, "search_strategy")
	assert.Contains(t, props, "focus_areas")

	required := schema["required"].([]string)
	assert.Contains(t, required, "service_name")

	// Test handler
	ctx := context.Background()
	params := map[string]interface{}{
		"service_name": "GitHub",
	}
	result, err := tool.Handler(ctx, params)

	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestExecuteBuiltinTool(t *testing.T) {
	ctx := context.Background()

	// Test valid builtin tool
	result, err := ExecuteBuiltinTool(ctx, "list_adapters", map[string]interface{}{})
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Test non-existent builtin tool
	result, err = ExecuteBuiltinTool(ctx, "non_existent", map[string]interface{}{})
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestIsBuiltinTool(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		{"list_adapters is builtin", "list_adapters", true},
		{"execute_code is builtin", "execute_code", true},
		{"register_tool is builtin", "register_tool", true},
		{"custom tool is not builtin", "my_custom_tool", false},
		{"empty string is not builtin", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsBuiltinTool(tt.toolName)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBuiltinToolsHaveValidSchemas(t *testing.T) {
	builtins := GetBuiltinTools()

	for _, tool := range builtins {
		t.Run(tool.Definition.Name, func(t *testing.T) {
			// Check all required fields are present
			assert.NotEmpty(t, tool.Definition.Name)
			assert.NotEmpty(t, tool.Definition.Description)
			assert.Equal(t, "builtin", tool.Definition.Language)
			assert.NotNil(t, tool.Definition.InputSchema)
			assert.NotNil(t, tool.Handler)

			// Check input schema has type
			schema := tool.Definition.InputSchema
			assert.Contains(t, schema, "type")
			assert.Equal(t, "object", schema["type"])

			// Check input schema has properties
			assert.Contains(t, schema, "properties")
			props := schema["properties"]
			assert.NotNil(t, props)
		})
	}
}
