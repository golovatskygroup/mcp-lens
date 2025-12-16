package native

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/executor"
)

// PythonExecutor executes Python code using subprocess
type PythonExecutor struct {
	sandbox    *executor.SandboxConfig
	pythonPath string
	mu         sync.RWMutex
	status     executor.RuntimeStatus
	execCount  int
}

// NewPythonExecutor creates a new Python executor
func NewPythonExecutor(sandbox *executor.SandboxConfig) *PythonExecutor {
	if sandbox == nil {
		sandbox = executor.DefaultSandboxConfig()
	}

	return &PythonExecutor{
		sandbox:    sandbox,
		pythonPath: "python3",
		status: executor.RuntimeStatus{
			Language: executor.LanguagePython,
			Status:   "stopped",
		},
	}
}

// Execute executes Python code
func (e *PythonExecutor) Execute(ctx context.Context, req executor.ExecuteRequest) (*executor.ExecuteResponse, error) {
	start := time.Now()

	// Initialize runtime if needed (check Python availability)
	if e.status.Status == "stopped" {
		if _, err := e.StartRuntime(ctx, executor.LanguagePython); err != nil {
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
	execCtx, cancel := context.WithTimeout(ctx, req.Timeout)
	defer cancel()

	result := e.executeCode(execCtx, code)
	result.ExecutionMs = time.Since(start).Milliseconds()

	return result, nil
}

// executeCode executes the prepared Python code
func (e *PythonExecutor) executeCode(ctx context.Context, code string) *executor.ExecuteResponse {
	// Create command
	cmd := exec.CommandContext(ctx, e.pythonPath, "-c", code)

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Execute
	err := cmd.Run()

	// Build response
	resp := &executor.ExecuteResponse{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}

	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			resp.Error = "execution timeout"
		} else {
			resp.Error = fmt.Sprintf("execution error: %v\n%s", err, stderr.String())
		}
		return resp
	}

	// Try to parse stdout as JSON
	if stdout.Len() > 0 {
		var result interface{}
		if err := json.Unmarshal(stdout.Bytes(), &result); err == nil {
			resp.Output = result
		} else {
			// If not JSON, return as string
			resp.Output = stdout.String()
		}
	}

	return resp
}

// prepareCode wraps user code with input injection and result extraction
func (e *PythonExecutor) prepareCode(userCode string, input map[string]interface{}) (string, error) {
	// Convert input to JSON
	inputJSON, err := executor.MarshalInput(input)
	if err != nil {
		return "", err
	}

	var code strings.Builder

	// Add imports
	code.WriteString("import json\n")
	code.WriteString("import sys\n")
	code.WriteString("\n")

	// Add input variable
	code.WriteString(fmt.Sprintf("input_json = '''%s'''\n", inputJSON))
	code.WriteString("input = json.loads(input_json)\n")
	code.WriteString("\n")

	// Add user code
	code.WriteString(userCode)
	code.WriteString("\n")

	// Add result output (if result variable exists)
	code.WriteString("\n")
	code.WriteString("if 'result' in dir():\n")
	code.WriteString("    print(json.dumps(result))\n")

	return code.String(), nil
}

// StartRuntime initializes the Python runtime (checks availability)
func (e *PythonExecutor) StartRuntime(ctx context.Context, language executor.Language) (*executor.RuntimeStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Check if Python is available
	cmd := exec.CommandContext(ctx, e.pythonPath, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		e.status.Status = "error"
		e.status.Error = fmt.Sprintf("python not available: %v", err)
		return &e.status, fmt.Errorf("python not available: %w", err)
	}

	e.status = executor.RuntimeStatus{
		Language:  executor.LanguagePython,
		Status:    "running",
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	return &e.status, nil
}

// StopRuntime stops the Python runtime
func (e *PythonExecutor) StopRuntime(ctx context.Context, language executor.Language) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.status.Status = "stopped"
	return nil
}

// ListRuntimes returns the current runtime status
func (e *PythonExecutor) ListRuntimes(ctx context.Context) ([]executor.RuntimeStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := e.status
	status.ExecuteCount = e.execCount

	return []executor.RuntimeStatus{status}, nil
}

// SetPythonPath sets the path to Python executable
func (e *PythonExecutor) SetPythonPath(path string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.pythonPath = path
}
