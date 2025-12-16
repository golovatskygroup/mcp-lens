package tool

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) (*sql.DB, func()) {
	// Create temporary database
	tmpFile, err := os.CreateTemp("", "test_*.db")
	require.NoError(t, err)
	tmpFile.Close()

	db, err := sql.Open("sqlite3", tmpFile.Name())
	require.NoError(t, err)

	cleanup := func() {
		db.Close()
		os.Remove(tmpFile.Name())
	}

	return db, cleanup
}

func TestSQLiteRegistry_Register(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Test successful registration
	tool := &ToolDefinition{
		Name:        "test_tool",
		Description: "A test tool",
		Language:    "go",
		Code:        "func main() { return nil }",
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
	assert.NoError(t, err)
	assert.NotEmpty(t, tool.ID)
	assert.Equal(t, 1, tool.Version)
	assert.Equal(t, "active", tool.Status)

	// Test duplicate name
	duplicate := &ToolDefinition{
		Name:        "test_tool",
		Description: "Another test tool",
		Language:    "go",
		Code:        "func main() { return nil }",
		InputSchema: map[string]interface{}{
			"type": "object",
		},
	}

	err = registry.Register(ctx, duplicate)
	assert.Error(t, err)
}

func TestSQLiteRegistry_Get(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register a tool
	tool := &ToolDefinition{
		Name:        "get_test_tool",
		Description: "A test tool for get",
		Language:    "python",
		Code:        "def main(): pass",
		InputSchema: map[string]interface{}{
			"type": "object",
		},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Test get existing tool
	retrieved, err := registry.Get(ctx, "get_test_tool")
	assert.NoError(t, err)
	assert.Equal(t, tool.Name, retrieved.Name)
	assert.Equal(t, tool.Description, retrieved.Description)
	assert.Equal(t, tool.Language, retrieved.Language)

	// Test get non-existent tool
	_, err = registry.Get(ctx, "non_existent")
	assert.Error(t, err)
}

func TestSQLiteRegistry_List(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register multiple tools
	tools := []*ToolDefinition{
		{
			Name:        "tool_a",
			Description: "Tool A",
			Language:    "go",
			Code:        "code a",
			InputSchema: map[string]interface{}{"type": "object"},
		},
		{
			Name:        "tool_b",
			Description: "Tool B",
			Language:    "javascript",
			Code:        "code b",
			InputSchema: map[string]interface{}{"type": "object"},
		},
		{
			Name:        "tool_c",
			Description: "Tool C",
			Language:    "python",
			Code:        "code c",
			InputSchema: map[string]interface{}{"type": "object"},
		},
	}

	for _, tool := range tools {
		err := registry.Register(ctx, tool)
		require.NoError(t, err)
	}

	// List all tools
	list, err := registry.List(ctx)
	assert.NoError(t, err)
	assert.Len(t, list, 3)
	assert.Equal(t, "tool_a", list[0].Name)
	assert.Equal(t, "tool_b", list[1].Name)
	assert.Equal(t, "tool_c", list[2].Name)
}

func TestSQLiteRegistry_Update(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register initial tool
	tool := &ToolDefinition{
		Name:        "modify_test_tool",
		Description: "Version 1",
		Language:    "go",
		Code:        "version 1 code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)
	initialID := tool.ID

	// Update tool
	updatedTool := &ToolDefinition{
		Name:        "modify_test_tool",
		Description: "Version 2",
		Language:    "go",
		Code:        "version 2 code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	err = registry.Update(ctx, "modify_test_tool", updatedTool)
	assert.NoError(t, err)

	// Verify update
	retrieved, err := registry.Get(ctx, "modify_test_tool")
	assert.NoError(t, err)
	assert.Equal(t, initialID, retrieved.ID)
	assert.Equal(t, 2, retrieved.Version)
	assert.Equal(t, "Version 2", retrieved.Description)
	assert.Equal(t, "version 2 code", retrieved.Code)
}

func TestSQLiteRegistry_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register a tool
	tool := &ToolDefinition{
		Name:        "disable_test_tool",
		Description: "To be deleted",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Delete tool
	err = registry.Delete(ctx, "disable_test_tool")
	assert.NoError(t, err)

	// Verify deletion (soft delete)
	_, err = registry.Get(ctx, "disable_test_tool")
	assert.Error(t, err)

	// Verify tool still exists in database but disabled
	var status string
	err = db.QueryRow("SELECT status FROM tools WHERE name = ?", "disable_test_tool").Scan(&status)
	assert.NoError(t, err)
	assert.Equal(t, "disabled", status)
}

func TestSQLiteRegistry_Versioning(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register initial tool
	tool := &ToolDefinition{
		Name:        "version_test",
		Description: "V1",
		Language:    "go",
		Code:        "v1",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Create multiple versions
	for i := 2; i <= 5; i++ {
		updated := &ToolDefinition{
			Name:        "version_test",
			Description: "V" + string(rune('0'+i)),
			Language:    "go",
			Code:        "v" + string(rune('0'+i)),
			InputSchema: map[string]interface{}{"type": "object"},
		}
		err = registry.Update(ctx, "version_test", updated)
		require.NoError(t, err)
	}

	// Get versions
	versions, err := registry.GetVersions(ctx, "version_test")
	assert.NoError(t, err)
	assert.Len(t, versions, 5)

	// Verify versions are in descending order
	for i := 0; i < len(versions); i++ {
		assert.Equal(t, 5-i, versions[i].Version)
	}
}

func TestSQLiteRegistry_Rollback(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register initial tool
	tool := &ToolDefinition{
		Name:        "rollback_test",
		Description: "Version 1",
		Language:    "go",
		Code:        "v1 code",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Update to version 2
	v2 := &ToolDefinition{
		Name:        "rollback_test",
		Description: "Version 2",
		Language:    "go",
		Code:        "v2 code",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	err = registry.Update(ctx, "rollback_test", v2)
	require.NoError(t, err)

	// Update to version 3
	v3 := &ToolDefinition{
		Name:        "rollback_test",
		Description: "Version 3",
		Language:    "go",
		Code:        "v3 code",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	err = registry.Update(ctx, "rollback_test", v3)
	require.NoError(t, err)

	// Rollback to version 1
	err = registry.Rollback(ctx, "rollback_test", 1)
	assert.NoError(t, err)

	// Verify rollback
	retrieved, err := registry.Get(ctx, "rollback_test")
	assert.NoError(t, err)
	assert.Equal(t, "Version 1", retrieved.Description)
	assert.Equal(t, "v1 code", retrieved.Code)
}

func TestValidateTool(t *testing.T) {
	tests := []struct {
		name    string
		tool    *ToolDefinition
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid tool",
			tool: &ToolDefinition{
				Name:        "valid_tool",
				Description: "A valid tool",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: false,
		},
		{
			name:    "nil tool",
			tool:    nil,
			wantErr: true,
			errMsg:  "tool cannot be nil",
		},
		{
			name: "empty name",
			tool: &ToolDefinition{
				Name:        "",
				Description: "desc",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "name cannot be empty",
		},
		{
			name: "invalid name with spaces",
			tool: &ToolDefinition{
				Name:        "invalid name",
				Description: "desc",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "invalid tool name",
		},
		{
			name: "forbidden create prefix",
			tool: &ToolDefinition{
				Name:        "create_something",
				Description: "desc",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "mutating prefix",
		},
		{
			name: "forbidden update prefix",
			tool: &ToolDefinition{
				Name:        "update_something",
				Description: "desc",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "mutating prefix",
		},
		{
			name: "forbidden delete prefix",
			tool: &ToolDefinition{
				Name:        "delete_something",
				Description: "desc",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "mutating prefix",
		},
		{
			name: "empty description",
			tool: &ToolDefinition{
				Name:        "test",
				Description: "",
				Language:    "go",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "description cannot be empty",
		},
		{
			name: "empty language",
			tool: &ToolDefinition{
				Name:        "test",
				Description: "desc",
				Language:    "",
				Code:        "code",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "language cannot be empty",
		},
		{
			name: "empty code",
			tool: &ToolDefinition{
				Name:        "test",
				Description: "desc",
				Language:    "go",
				Code:        "",
				InputSchema: map[string]interface{}{"type": "object"},
			},
			wantErr: true,
			errMsg:  "code cannot be empty",
		},
		{
			name: "nil input schema",
			tool: &ToolDefinition{
				Name:        "test",
				Description: "desc",
				Language:    "go",
				Code:        "code",
				InputSchema: nil,
			},
			wantErr: true,
			errMsg:  "input schema cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTool(tt.tool)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsValidToolName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid snake_case", "valid_tool_name", true},
		{"valid with numbers", "tool_123", true},
		{"valid single word", "tool", true},
		{"invalid with space", "invalid tool", false},
		{"invalid with dash", "invalid-tool", false},
		{"invalid with dot", "invalid.tool", false},
		{"invalid uppercase in middle", "invalidTool", false},
		{"valid uppercase at start", "Tool_name", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidToolName(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSQLiteRegistry_LogExecution(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	cache := NewToolCache(5*time.Minute, 100)
	registry, err := NewSQLiteRegistry(db, cache)
	require.NoError(t, err)

	ctx := context.Background()

	// Register a tool
	tool := &ToolDefinition{
		Name:        "log_test_tool",
		Description: "Tool for logging test",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}
	err = registry.Register(ctx, tool)
	require.NoError(t, err)

	// Log execution
	params := map[string]interface{}{"input": "test"}
	result := map[string]interface{}{"output": "success"}
	err = registry.LogExecution(ctx, tool.ID, "success", params, result, "", 150)
	assert.NoError(t, err)

	// Verify log
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tool_executions WHERE tool_id = ?", tool.ID).Scan(&count)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
}
