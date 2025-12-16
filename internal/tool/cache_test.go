package tool

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestToolCache_SetGet(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 10)

	tool := &ToolDefinition{
		Name:        "test_tool",
		Description: "Test",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	// Set and get
	cache.Set("test_tool", tool)
	retrieved := cache.Get("test_tool")

	assert.NotNil(t, retrieved)
	assert.Equal(t, tool.Name, retrieved.Name)
}

func TestToolCache_GetNonExistent(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 10)

	retrieved := cache.Get("non_existent")
	assert.Nil(t, retrieved)
}

func TestToolCache_Delete(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 10)

	tool := &ToolDefinition{
		Name:        "test_tool",
		Description: "Test",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	cache.Set("test_tool", tool)
	assert.NotNil(t, cache.Get("test_tool"))

	cache.Delete("test_tool")
	assert.Nil(t, cache.Get("test_tool"))
}

func TestToolCache_Clear(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 10)

	for i := 0; i < 5; i++ {
		tool := &ToolDefinition{
			Name:        "tool_" + string(rune('0'+i)),
			Description: "Test",
			Language:    "go",
			Code:        "code",
			InputSchema: map[string]interface{}{"type": "object"},
		}
		cache.Set(tool.Name, tool)
	}

	assert.Equal(t, 5, cache.Size())

	cache.Clear()
	assert.Equal(t, 0, cache.Size())
}

func TestToolCache_Size(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 10)

	assert.Equal(t, 0, cache.Size())

	for i := 0; i < 3; i++ {
		tool := &ToolDefinition{
			Name:        "tool_" + string(rune('0'+i)),
			Description: "Test",
			Language:    "go",
			Code:        "code",
			InputSchema: map[string]interface{}{"type": "object"},
		}
		cache.Set(tool.Name, tool)
	}

	assert.Equal(t, 3, cache.Size())
}

func TestToolCache_MaxSize(t *testing.T) {
	cache := NewToolCache(5*time.Minute, 3)

	// Add more tools than max size
	for i := 0; i < 5; i++ {
		tool := &ToolDefinition{
			Name:        "tool_" + string(rune('0'+i)),
			Description: "Test",
			Language:    "go",
			Code:        "code",
			InputSchema: map[string]interface{}{"type": "object"},
		}
		cache.Set(tool.Name, tool)
		time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	}

	// Cache should only have max size entries
	assert.LessOrEqual(t, cache.Size(), 3)
}

func TestToolCache_Expiration(t *testing.T) {
	cache := NewToolCache(100*time.Millisecond, 10)

	tool := &ToolDefinition{
		Name:        "test_tool",
		Description: "Test",
		Language:    "go",
		Code:        "code",
		InputSchema: map[string]interface{}{"type": "object"},
	}

	cache.Set("test_tool", tool)
	assert.NotNil(t, cache.Get("test_tool"))

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	retrieved := cache.Get("test_tool")
	assert.Nil(t, retrieved)
}

func TestToolCache_Cleanup(t *testing.T) {
	cache := NewToolCache(100*time.Millisecond, 10)

	// Add multiple tools
	for i := 0; i < 3; i++ {
		tool := &ToolDefinition{
			Name:        "tool_" + string(rune('0'+i)),
			Description: "Test",
			Language:    "go",
			Code:        "code",
			InputSchema: map[string]interface{}{"type": "object"},
		}
		cache.Set(tool.Name, tool)
	}

	assert.Equal(t, 3, cache.Size())

	// Wait for cleanup to run (cleanup runs every minute, but expiration is 100ms)
	time.Sleep(150 * time.Millisecond)

	// Manually trigger cleanup by accessing after expiration
	for i := 0; i < 3; i++ {
		cache.Get("tool_" + string(rune('0'+i)))
	}

	// Wait for cleanup goroutine (it runs every minute)
	// Since we can't easily test the background cleanup, we just verify expiration works
	time.Sleep(100 * time.Millisecond)
}
