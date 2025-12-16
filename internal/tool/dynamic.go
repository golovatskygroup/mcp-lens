package tool

import (
	"context"
	"fmt"
	"time"
)

// DynamicExecutor executes dynamic tools loaded from the registry
type DynamicExecutor struct {
	registry ToolRegistry
}

// NewDynamicExecutor creates a new dynamic tool executor
func NewDynamicExecutor(registry ToolRegistry) *DynamicExecutor {
	return &DynamicExecutor{
		registry: registry,
	}
}

// Execute executes a dynamic tool by name with given parameters
func (e *DynamicExecutor) Execute(ctx context.Context, name string, params map[string]interface{}) (interface{}, error) {
	startTime := time.Now()

	// Get tool from registry
	tool, err := e.registry.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool: %w", err)
	}

	// Validate tool status
	if tool.Status != "active" {
		return nil, fmt.Errorf("tool is not active: %s", name)
	}

	// Execute based on language
	result, err := e.executeByLanguage(ctx, tool, params)

	// Calculate execution time
	executionTimeMs := time.Since(startTime).Milliseconds()

	// Log execution
	status := "success"
	errorMsg := ""
	if err != nil {
		status = "error"
		errorMsg = err.Error()
	}

	// Log execution (ignore logging errors)
	if sqlReg, ok := e.registry.(*SQLiteRegistry); ok {
		_ = sqlReg.LogExecution(ctx, tool.ID, status, params, result, errorMsg, executionTimeMs)
	}

	if err != nil {
		return nil, err
	}

	return result, nil
}

// executeByLanguage executes code based on language type
func (e *DynamicExecutor) executeByLanguage(ctx context.Context, tool *ToolDefinition, params map[string]interface{}) (interface{}, error) {
	switch tool.Language {
	case "go":
		return e.executeGo(ctx, tool, params)
	case "javascript":
		return e.executeJavaScript(ctx, tool, params)
	case "python":
		return e.executePython(ctx, tool, params)
	case "ruby", "rust", "java", "php", "bash", "typescript":
		return e.executeDocker(ctx, tool, params)
	default:
		return nil, fmt.Errorf("unsupported language: %s", tool.Language)
	}
}

// executeGo executes Go code (placeholder for Yaegi integration)
func (e *DynamicExecutor) executeGo(ctx context.Context, tool *ToolDefinition, params map[string]interface{}) (interface{}, error) {
	// TODO: Implement Yaegi-based Go execution
	return map[string]interface{}{
		"status":  "error",
		"message": "Go execution not yet implemented",
		"tool":    tool.Name,
	}, fmt.Errorf("Go execution not yet implemented")
}

// executeJavaScript executes JavaScript code (placeholder for goja integration)
func (e *DynamicExecutor) executeJavaScript(ctx context.Context, tool *ToolDefinition, params map[string]interface{}) (interface{}, error) {
	// TODO: Implement goja-based JavaScript execution
	return map[string]interface{}{
		"status":  "error",
		"message": "JavaScript execution not yet implemented",
		"tool":    tool.Name,
	}, fmt.Errorf("JavaScript execution not yet implemented")
}

// executePython executes Python code (placeholder for Python integration)
func (e *DynamicExecutor) executePython(ctx context.Context, tool *ToolDefinition, params map[string]interface{}) (interface{}, error) {
	// TODO: Implement Python execution
	return map[string]interface{}{
		"status":  "error",
		"message": "Python execution not yet implemented",
		"tool":    tool.Name,
	}, fmt.Errorf("Python execution not yet implemented")
}

// executeDocker executes code in Docker container (placeholder for Docker integration)
func (e *DynamicExecutor) executeDocker(ctx context.Context, tool *ToolDefinition, params map[string]interface{}) (interface{}, error) {
	// TODO: Implement Docker-based execution
	return map[string]interface{}{
		"status":  "container_starting",
		"message": fmt.Sprintf("Docker execution for %s not yet implemented", tool.Language),
		"tool":    tool.Name,
		"retry_after_seconds": 5,
	}, fmt.Errorf("Docker execution not yet implemented")
}

// ValidateParams validates tool parameters against input schema
func (e *DynamicExecutor) ValidateParams(ctx context.Context, name string, params map[string]interface{}) error {
	tool, err := e.registry.Get(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get tool: %w", err)
	}

	// TODO: Implement JSON Schema validation
	// For now, just check required fields if they exist in schema
	if required, ok := tool.InputSchema["required"].([]interface{}); ok {
		for _, req := range required {
			reqField := req.(string)
			if _, exists := params[reqField]; !exists {
				return fmt.Errorf("missing required parameter: %s", reqField)
			}
		}
	}

	return nil
}

// IsDynamicTool checks if a tool is a dynamic tool (exists in registry)
func (e *DynamicExecutor) IsDynamicTool(ctx context.Context, name string) bool {
	_, err := e.registry.Get(ctx, name)
	return err == nil
}
