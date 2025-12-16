package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Language represents supported programming languages
type Language string

const (
	LanguageGo         Language = "go"
	LanguageJavaScript Language = "javascript"
	LanguagePython     Language = "python"
)

// ExecuteRequest represents a code execution request
type ExecuteRequest struct {
	Language    Language               `json:"language"`
	Code        string                 `json:"code"`
	Input       map[string]interface{} `json:"input,omitempty"`
	Timeout     time.Duration          `json:"timeout,omitempty"`
	Environment map[string]string      `json:"environment,omitempty"`
}

// ExecuteResponse represents a code execution response
type ExecuteResponse struct {
	Output      interface{}            `json:"output,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Stdout      string                 `json:"stdout,omitempty"`
	Stderr      string                 `json:"stderr,omitempty"`
	ExecutionMs int64                  `json:"execution_ms"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// RuntimeStatus represents the status of a language runtime
type RuntimeStatus struct {
	Language    Language  `json:"language"`
	Status      string    `json:"status"` // "running", "stopped", "error"
	StartedAt   time.Time `json:"started_at,omitempty"`
	LastUsed    time.Time `json:"last_used,omitempty"`
	ExecuteCount int      `json:"execute_count"`
	Error       string    `json:"error,omitempty"`
}

// CodeExecutor defines the interface for code execution
type CodeExecutor interface {
	Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error)
	StartRuntime(ctx context.Context, language Language) (*RuntimeStatus, error)
	StopRuntime(ctx context.Context, language Language) error
	ListRuntimes(ctx context.Context) ([]RuntimeStatus, error)
}

// NativeExecutor is the main executor that routes to language-specific executors
type NativeExecutor struct {
	goExecutor     CodeExecutor
	jsExecutor     CodeExecutor
	pythonExecutor CodeExecutor
	validator      *Validator
	sandbox        *SandboxConfig
}

// NewNativeExecutor creates a new native code executor
func NewNativeExecutor(sandbox *SandboxConfig, validator *Validator) *NativeExecutor {
	return &NativeExecutor{
		validator: validator,
		sandbox:   sandbox,
	}
}

// Execute executes code in the appropriate language runtime
func (e *NativeExecutor) Execute(ctx context.Context, req ExecuteRequest) (*ExecuteResponse, error) {
	start := time.Now()

	// Validate code before execution
	if e.validator != nil {
		if err := e.validator.Validate(req.Language, req.Code); err != nil {
			return &ExecuteResponse{
				Error:       fmt.Sprintf("validation failed: %v", err),
				ExecutionMs: time.Since(start).Milliseconds(),
			}, nil
		}
	}

	// Set default timeout
	if req.Timeout == 0 {
		req.Timeout = 30 * time.Second
	}

	// Create timeout context
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	// Route to appropriate executor
	var executor CodeExecutor
	switch req.Language {
	case LanguageGo:
		if e.goExecutor == nil {
			return nil, fmt.Errorf("Go executor not initialized")
		}
		executor = e.goExecutor
	case LanguageJavaScript:
		if e.jsExecutor == nil {
			return nil, fmt.Errorf("JavaScript executor not initialized")
		}
		executor = e.jsExecutor
	case LanguagePython:
		if e.pythonExecutor == nil {
			return nil, fmt.Errorf("Python executor not initialized")
		}
		executor = e.pythonExecutor
	default:
		return nil, fmt.Errorf("unsupported language: %s", req.Language)
	}

	// Execute code
	resp, err := executor.Execute(execCtx, req)
	if err != nil {
		return &ExecuteResponse{
			Error:       err.Error(),
			ExecutionMs: time.Since(start).Milliseconds(),
		}, nil
	}

	// Update execution time if not set
	if resp.ExecutionMs == 0 {
		resp.ExecutionMs = time.Since(start).Milliseconds()
	}

	return resp, nil
}

// StartRuntime starts a language runtime
func (e *NativeExecutor) StartRuntime(ctx context.Context, language Language) (*RuntimeStatus, error) {
	var executor CodeExecutor
	switch language {
	case LanguageGo:
		executor = e.goExecutor
	case LanguageJavaScript:
		executor = e.jsExecutor
	case LanguagePython:
		executor = e.pythonExecutor
	default:
		return nil, fmt.Errorf("unsupported language: %s", language)
	}

	if executor == nil {
		return nil, fmt.Errorf("%s executor not initialized", language)
	}

	return executor.StartRuntime(ctx, language)
}

// StopRuntime stops a language runtime
func (e *NativeExecutor) StopRuntime(ctx context.Context, language Language) error {
	var executor CodeExecutor
	switch language {
	case LanguageGo:
		executor = e.goExecutor
	case LanguageJavaScript:
		executor = e.jsExecutor
	case LanguagePython:
		executor = e.pythonExecutor
	default:
		return fmt.Errorf("unsupported language: %s", language)
	}

	if executor == nil {
		return fmt.Errorf("%s executor not initialized", language)
	}

	return executor.StopRuntime(ctx, language)
}

// ListRuntimes lists all runtime statuses
func (e *NativeExecutor) ListRuntimes(ctx context.Context) ([]RuntimeStatus, error) {
	var statuses []RuntimeStatus

	executors := []struct {
		executor CodeExecutor
		language Language
	}{
		{e.goExecutor, LanguageGo},
		{e.jsExecutor, LanguageJavaScript},
		{e.pythonExecutor, LanguagePython},
	}

	for _, exec := range executors {
		if exec.executor != nil {
			runtimes, err := exec.executor.ListRuntimes(ctx)
			if err != nil {
				// Add error status
				statuses = append(statuses, RuntimeStatus{
					Language: exec.language,
					Status:   "error",
					Error:    err.Error(),
				})
			} else {
				statuses = append(statuses, runtimes...)
			}
		}
	}

	return statuses, nil
}

// SetGoExecutor sets the Go executor
func (e *NativeExecutor) SetGoExecutor(executor CodeExecutor) {
	e.goExecutor = executor
}

// SetJavaScriptExecutor sets the JavaScript executor
func (e *NativeExecutor) SetJavaScriptExecutor(executor CodeExecutor) {
	e.jsExecutor = executor
}

// SetPythonExecutor sets the Python executor
func (e *NativeExecutor) SetPythonExecutor(executor CodeExecutor) {
	e.pythonExecutor = executor
}

// MarshalInput converts input map to JSON string
func MarshalInput(input map[string]interface{}) (string, error) {
	if input == nil {
		return "{}", nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal input: %w", err)
	}
	return string(data), nil
}

// UnmarshalOutput converts JSON string to output value
func UnmarshalOutput(output string) (interface{}, error) {
	var result interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output: %w", err)
	}
	return result, nil
}
