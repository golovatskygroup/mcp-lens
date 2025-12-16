package tool

import (
	"context"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDynamicExecutor(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	assert.NotNil(t, executor)
	assert.NotNil(t, executor.registry)
}

func TestDynamicExecutor_Execute(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	// Register a tool
	tool := &ToolDefinition{
		Name:        "test_executor_tool",
		Description: "Test tool for executor",
		Language:    "go",
		Code:        "func main() { return 42 }",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Execute tool (will return error as Go execution is not implemented)
	params := map[string]interface{}{"input": "test"}
	_, err = executor.Execute(ctx, "test_executor_tool", params)

	// Since execution is not implemented, we expect an error
	assert.Error(t, err)

	// Verify execution was logged
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tool_executions WHERE tool_id = ?", tool.ID).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestDynamicExecutor_ExecuteNonExistent(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	// Try to execute non-existent tool
	params := map[string]interface{}{}
	result, err := executor.Execute(ctx, "non_existent_tool", params)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestDynamicExecutor_ExecuteDisabledTool(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	// Register and disable a tool
	tool := &ToolDefinition{
		Name:        "disabled_tool",
		Description: "Tool to be disabled",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	err = registry.Delete(ctx, "disabled_tool")
	require.NoError(t, err)

	// Try to execute disabled tool
	result, err := executor.Execute(ctx, "disabled_tool", map[string]interface{}{})

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestDynamicExecutor_ExecuteByLanguage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	tests := []struct {
		name     string
		language string
		wantErr  bool
	}{
		{"go language", "go", true}, // Not implemented yet, expect error
		{"javascript language", "javascript", true},
		{"python language", "python", true},
		{"ruby language", "ruby", true},
		{"rust language", "rust", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &ToolDefinition{
				Name:        "tool_" + tt.language,
				Description: "Test",
				Language:    tt.language,
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			}

			result, err := executor.executeByLanguage(ctx, tool, map[string]interface{}{})

			if tt.wantErr {
				assert.Error(t, err)
			}
			assert.NotNil(t, result)
		})
	}
}

func TestDynamicExecutor_UnsupportedLanguage(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	tool := &ToolDefinition{
		Name:        "unsupported_tool",
		Description: "Test",
		Language:    "cobol",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	result, err := executor.executeByLanguage(ctx, tool, map[string]interface{}{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported language")
	assert.Nil(t, result)
}

func TestDynamicExecutor_ValidateParams(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	// Register a tool with required params
	tool := &ToolDefinition{
		Name:        "validate_test",
		Description: "Test",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"required_field": map[string]interface{}{
					"type": "string",
				},
				"optional_field": map[string]interface{}{
					"type": "string",
				},
			},
			"required": []interface{}{"required_field"},
		},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Test with valid params
	params := map[string]interface{}{
		"required_field": "value",
	}
	err = executor.ValidateParams(ctx, "validate_test", params)
	assert.NoError(t, err)

	// Test with missing required param
	invalidParams := map[string]interface{}{
		"optional_field": "value",
	}
	err = executor.ValidateParams(ctx, "validate_test", invalidParams)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required parameter")
}

func TestDynamicExecutor_IsDynamicTool(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	// Register a tool
	tool := &ToolDefinition{
		Name:        "dynamic_test",
		Description: "Test",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Test existing tool
	assert.True(t, executor.IsDynamicTool(ctx, "dynamic_test"))

	// Test non-existent tool
	assert.False(t, executor.IsDynamicTool(ctx, "non_existent"))
}

func TestDynamicExecutor_ExecuteGo(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	tool := &ToolDefinition{
		Name:        "go_test",
		Description: "Test",
		Language:    "go",
		Code:        "func main() { return 42 }",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	result, err := executor.executeGo(ctx, tool, map[string]interface{}{})

	// Since Go execution is not implemented, expect error
	assert.Error(t, err)
	assert.NotNil(t, result)

	resultMap := result.(map[string]interface{})
	assert.Equal(t, "error", resultMap["status"])
	assert.Contains(t, resultMap["message"], "not yet implemented")
}

func TestDynamicExecutor_ExecuteJavaScript(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	tool := &ToolDefinition{
		Name:        "js_test",
		Description: "Test",
		Language:    "javascript",
		Code:        "function main() { return 42; }",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	result, err := executor.executeJavaScript(ctx, tool, map[string]interface{}{})

	assert.Error(t, err)
	assert.NotNil(t, result)
}

func TestDynamicExecutor_ExecutePython(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	tool := &ToolDefinition{
		Name:        "py_test",
		Description: "Test",
		Language:    "python",
		Code:        "def main(): return 42",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	result, err := executor.executePython(ctx, tool, map[string]interface{}{})

	assert.Error(t, err)
	assert.NotNil(t, result)
}

func TestDynamicExecutor_ExecuteDocker(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	executor := NewDynamicExecutor(registry)
	ctx := context.Background()

	tool := &ToolDefinition{
		Name:        "rust_test",
		Description: "Test",
		Language:    "rust",
		Code:        "fn main() { 42 }",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	result, err := executor.executeDocker(ctx, tool, map[string]interface{}{})

	assert.Error(t, err)
	assert.NotNil(t, result)

	resultMap := result.(map[string]interface{})
	assert.Equal(t, "container_starting", resultMap["status"])
}
