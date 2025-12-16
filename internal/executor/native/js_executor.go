package native

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/golovatskygroup/mcp-lens/internal/executor"
)

// JSExecutor executes JavaScript code using goja
type JSExecutor struct {
	sandbox   *executor.SandboxConfig
	runtime   *goja.Runtime
	mu        sync.RWMutex
	status    executor.RuntimeStatus
	execCount int
}

// NewJSExecutor creates a new JavaScript executor
func NewJSExecutor(sandbox *executor.SandboxConfig) *JSExecutor {
	if sandbox == nil {
		sandbox = executor.DefaultSandboxConfig()
	}

	return &JSExecutor{
		sandbox: sandbox,
		status: executor.RuntimeStatus{
			Language: executor.LanguageJavaScript,
			Status:   "stopped",
		},
	}
}

// Execute executes JavaScript code
func (e *JSExecutor) Execute(ctx context.Context, req executor.ExecuteRequest) (*executor.ExecuteResponse, error) {
	start := time.Now()

	// Initialize runtime if needed
	if e.runtime == nil {
		if _, err := e.StartRuntime(ctx, executor.LanguageJavaScript); err != nil {
			return &executor.ExecuteResponse{
				Error:       fmt.Sprintf("failed to start runtime: %v", err),
				ExecutionMs: time.Since(start).Milliseconds(),
			}, nil
		}
	}

	// Update last used
	e.mu.Lock()
	e.status.LastUsed = time.Now()
	e.execCount++
	vm := e.runtime
	e.mu.Unlock()

	// Prepare code with input
	code, err := e.prepareCode(req.Code, req.Input)
	if err != nil {
		return &executor.ExecuteResponse{
			Error:       fmt.Sprintf("failed to prepare code: %v", err),
			ExecutionMs: time.Since(start).Milliseconds(),
		}, nil
	}

	// Execute with timeout
	resultChan := make(chan *executor.ExecuteResponse, 1)
	done := make(chan struct{})

	go func() {
		defer close(done)
		result := e.executeCode(vm, code)
		select {
		case resultChan <- result:
		case <-ctx.Done():
		}
	}()

	// Setup interrupt for timeout
	timer := time.NewTimer(req.Timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		vm.Interrupt("timeout")
		<-done
		return &executor.ExecuteResponse{
			Error:       "execution timeout",
			ExecutionMs: time.Since(start).Milliseconds(),
		}, nil
	case <-timer.C:
		vm.Interrupt("timeout")
		<-done
		return &executor.ExecuteResponse{
			Error:       "execution timeout",
			ExecutionMs: time.Since(start).Milliseconds(),
		}, nil
	case result := <-resultChan:
		result.ExecutionMs = time.Since(start).Milliseconds()
		return result, nil
	}
}

// executeCode executes the prepared JavaScript code
func (e *JSExecutor) executeCode(vm *goja.Runtime, code string) *executor.ExecuteResponse {
	// Execute code
	val, err := vm.RunString(code)
	if err != nil {
		// Check if it's an interrupted error
		if strings.Contains(err.Error(), "timeout") {
			return &executor.ExecuteResponse{
				Error: "execution timeout",
			}
		}
		return &executor.ExecuteResponse{
			Error: err.Error(),
		}
	}

	// Export result
	result := val.Export()

	return &executor.ExecuteResponse{
		Output: result,
	}
}

// prepareCode wraps user code with input injection and result extraction
func (e *JSExecutor) prepareCode(userCode string, input map[string]interface{}) (string, error) {
	// Convert input to JSON
	inputJSON, err := executor.MarshalInput(input)
	if err != nil {
		return "", err
	}

	var code strings.Builder

	// Add input variable
	code.WriteString(fmt.Sprintf("var input = JSON.parse('%s');\n", escapeJSString(inputJSON)))
	code.WriteString("\n")

	// Add user code
	code.WriteString(userCode)
	code.WriteString("\n")

	return code.String(), nil
}

// StartRuntime initializes the JavaScript runtime
func (e *JSExecutor) StartRuntime(ctx context.Context, language executor.Language) (*executor.RuntimeStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.runtime != nil {
		return &e.status, nil
	}

	// Create new runtime
	vm := goja.New()

	// Setup sandbox
	if e.sandbox != nil && e.sandbox.JavaScript != nil {
		// Add console if allowed
		if e.sandbox.JavaScript.AllowConsole {
			console := vm.NewObject()
			console.Set("log", func(call goja.FunctionCall) goja.Value {
				// In production, this could log to a buffer
				return goja.Undefined()
			})
			vm.Set("console", console)
		}

		// Remove forbidden globals
		for _, forbidden := range e.sandbox.JavaScript.ForbiddenGlobals {
			vm.Set(forbidden, goja.Undefined())
		}

		// Set max stack depth
		if e.sandbox.JavaScript.MaxStackDepth > 0 {
			vm.SetMaxCallStackSize(e.sandbox.JavaScript.MaxStackDepth)
		}
	}

	e.runtime = vm
	e.status = executor.RuntimeStatus{
		Language:  executor.LanguageJavaScript,
		Status:    "running",
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	return &e.status, nil
}

// StopRuntime stops the JavaScript runtime
func (e *JSExecutor) StopRuntime(ctx context.Context, language executor.Language) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.runtime != nil {
		e.runtime.Interrupt("shutdown")
	}
	e.runtime = nil
	e.status.Status = "stopped"

	return nil
}

// ListRuntimes returns the current runtime status
func (e *JSExecutor) ListRuntimes(ctx context.Context) ([]executor.RuntimeStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := e.status
	status.ExecuteCount = e.execCount

	return []executor.RuntimeStatus{status}, nil
}

// escapeJSString escapes a string for use in JavaScript code
func escapeJSString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
