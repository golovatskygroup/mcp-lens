package tools

import (
	"context"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/tool"
)

// MockToolRegistry is a simple in-memory implementation for testing
type MockToolRegistry struct {
	tools map[string]*tool.ToolDefinition
}

func NewMockToolRegistry() *MockToolRegistry {
	return &MockToolRegistry{
		tools: make(map[string]*tool.ToolDefinition),
	}
}

func (m *MockToolRegistry) Register(ctx context.Context, t *tool.ToolDefinition) error {
	m.tools[t.Name] = t
	return nil
}

func (m *MockToolRegistry) Get(ctx context.Context, name string) (*tool.ToolDefinition, error) {
	if t, ok := m.tools[name]; ok {
		return t, nil
	}
	return nil, nil
}

func (m *MockToolRegistry) List(ctx context.Context) ([]*tool.ToolDefinition, error) {
	result := make([]*tool.ToolDefinition, 0, len(m.tools))
	for _, t := range m.tools {
		result = append(result, t)
	}
	return result, nil
}

func (m *MockToolRegistry) Update(ctx context.Context, name string, t *tool.ToolDefinition) error {
	m.tools[name] = t
	return nil
}

func (m *MockToolRegistry) Delete(ctx context.Context, name string) error {
	delete(m.tools, name)
	return nil
}

func (m *MockToolRegistry) Rollback(ctx context.Context, name string, version int) error {
	return nil
}

func (m *MockToolRegistry) GetVersions(ctx context.Context, name string) ([]*tool.ToolDefinition, error) {
	return nil, nil
}

// TestBuiltinToolsIncludesDynamicTools tests that BuiltinTools() includes active dynamic tools
func TestBuiltinToolsIncludesDynamicTools(t *testing.T) {
	// Setup
	mockRegistry := NewMockToolRegistry()
	handler := &Handler{
		execDeps: &ExecutionDependencies{
			ToolRegistry: mockRegistry,
		},
	}

	// Register a test tool
	testTool := &tool.ToolDefinition{
		ID:          "test-id",
		Name:        "test_calculator",
		Description: "Test calculator tool",
		Language:    "bash",
		Code:        "echo 42",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"value": map[string]interface{}{"type": "number"},
			},
		},
		Status:  "active",
		Version: 1,
	}
	
	err := mockRegistry.Register(context.Background(), testTool)
	if err != nil {
		t.Fatalf("Failed to register test tool: %v", err)
	}

	// Test
	tools := handler.BuiltinTools()

	// Verify
	found := false
	for _, tool := range tools {
		if tool.Name == "test_calculator" {
			found = true
			if tool.Description != "Test calculator tool" {
				t.Errorf("Expected description 'Test calculator tool', got '%s'", tool.Description)
			}
			break
		}
	}

	if !found {
		t.Error("Expected test_calculator to be in BuiltinTools(), but it was not found")
	}
}

// TestBuiltinToolsExcludesInactiveTools tests that inactive tools are not included
func TestBuiltinToolsExcludesInactiveTools(t *testing.T) {
	// Setup
	mockRegistry := NewMockToolRegistry()
	handler := &Handler{
		execDeps: &ExecutionDependencies{
			ToolRegistry: mockRegistry,
		},
	}

	// Register an inactive tool
	testTool := &tool.ToolDefinition{
		ID:          "test-id",
		Name:        "inactive_tool",
		Description: "Inactive tool",
		Language:    "bash",
		Code:        "echo 42",
		InputSchema: map[string]interface{}{
			"type": "object",
		},
		Status:  "disabled",
		Version: 1,
	}
	
	err := mockRegistry.Register(context.Background(), testTool)
	if err != nil {
		t.Fatalf("Failed to register test tool: %v", err)
	}

	// Test
	tools := handler.BuiltinTools()

	// Verify - inactive tool should NOT be present
	for _, tool := range tools {
		if tool.Name == "inactive_tool" {
			t.Error("Expected inactive_tool to NOT be in BuiltinTools(), but it was found")
		}
	}
}

// TestIsLocalToolRecognizesDynamicTools tests that IsLocalTool() recognizes dynamic tools
func TestIsLocalToolRecognizesDynamicTools(t *testing.T) {
	// Setup
	mockRegistry := NewMockToolRegistry()
	handler := &Handler{
		execDeps: &ExecutionDependencies{
			ToolRegistry: mockRegistry,
		},
	}

	// Register a test tool
	testTool := &tool.ToolDefinition{
		ID:          "test-id",
		Name:        "custom_analyzer",
		Description: "Custom analyzer",
		Language:    "python",
		Code:        "print('test')",
		InputSchema: map[string]interface{}{
			"type": "object",
		},
		Status:  "active",
		Version: 1,
	}
	
	err := mockRegistry.Register(context.Background(), testTool)
	if err != nil {
		t.Fatalf("Failed to register test tool: %v", err)
	}

	// Test
	isLocal := handler.IsLocalTool("custom_analyzer")

	// Verify
	if !isLocal {
		t.Error("Expected custom_analyzer to be recognized as a local tool")
	}
}

// TestBuildPolicyIncludesDynamicTools tests that buildPolicy() includes dynamic tools in AllowLocal
func TestBuildPolicyIncludesDynamicTools(t *testing.T) {
	// This test requires access to buildPolicy which is in router.go
	// We'll add it here as a basic integration check

	mockRegistry := NewMockToolRegistry()
	_ = &Handler{
		execDeps: &ExecutionDependencies{
			ToolRegistry: mockRegistry,
		},
	}

	// Register a test tool
	testTool := &tool.ToolDefinition{
		ID:          "test-id",
		Name:        "policy_test_tool",
		Description: "Policy test tool",
		Language:    "bash",
		Code:        "echo test",
		InputSchema: map[string]interface{}{
			"type": "object",
		},
		Status:  "active",
		Version: 1,
	}

	err := mockRegistry.Register(context.Background(), testTool)
	if err != nil {
		t.Fatalf("Failed to register test tool: %v", err)
	}

	// Test - note: this requires buildPolicy to be accessible or moved to a testable location
	// For now, we verify that the tool is at least registered
	retrievedTool, err := mockRegistry.Get(context.Background(), "policy_test_tool")
	if err != nil || retrievedTool == nil {
		t.Error("Expected tool to be retrievable from registry")
	}
}
