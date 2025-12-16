package native

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/executor"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// GoExecutor executes Go code using Yaegi interpreter
type GoExecutor struct {
	sandbox  *executor.SandboxConfig
	runtime  *interp.Interpreter
	mu       sync.RWMutex
	status   executor.RuntimeStatus
	execCount int
}

// NewGoExecutor creates a new Go executor
func NewGoExecutor(sandbox *executor.SandboxConfig) *GoExecutor {
	if sandbox == nil {
		sandbox = executor.DefaultSandboxConfig()
	}

	return &GoExecutor{
		sandbox: sandbox,
		status: executor.RuntimeStatus{
			Language: executor.LanguageGo,
			Status:   "stopped",
		},
	}
}

// Execute executes Go code
func (e *GoExecutor) Execute(ctx context.Context, req executor.ExecuteRequest) (*executor.ExecuteResponse, error) {
	start := time.Now()

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
	resultChan := make(chan *executor.ExecuteResponse, 1)
	go func() {
		// Create a fresh interpreter for each execution
		result := e.executeCode(code)
		resultChan <- result
	}()

	select {
	case <-ctx.Done():
		return &executor.ExecuteResponse{
			Error:       "execution timeout",
			ExecutionMs: time.Since(start).Milliseconds(),
		}, nil
	case result := <-resultChan:
		result.ExecutionMs = time.Since(start).Milliseconds()
		return result, nil
	}
}

// executeCode executes the prepared Go code
func (e *GoExecutor) executeCode(code string) *executor.ExecuteResponse {
	// Create a fresh interpreter for each execution to avoid state issues
	i := interp.New(interp.Options{})

	// Load standard library
	if err := i.Use(stdlib.Symbols); err != nil {
		return &executor.ExecuteResponse{
			Error: fmt.Sprintf("failed to load stdlib: %v", err),
		}
	}

	// Execute code
	_, err := i.Eval(code)
	if err != nil {
		return &executor.ExecuteResponse{
			Error: err.Error(),
		}
	}

	// Try to get the result variable
	result, err := i.Eval("result")
	if err != nil {
		return &executor.ExecuteResponse{
			Output: nil,
		}
	}

	return &executor.ExecuteResponse{
		Output: result.Interface(),
	}
}

// prepareCode wraps user code with input injection and result extraction
func (e *GoExecutor) prepareCode(userCode string, input map[string]interface{}) (string, error) {
	// Convert input to JSON
	inputJSON, err := executor.MarshalInput(input)
	if err != nil {
		return "", err
	}

	// Check if code already has package declaration
	hasPackage := strings.Contains(userCode, "package ")
	hasImports := strings.Contains(userCode, "import ")

	var code strings.Builder

	// Add package if not present
	if !hasPackage {
		code.WriteString("package main\n\n")
	}

	// Collect imports from user code
	var imports []string
	if hasImports {
		// Extract imports from user code
		lines := strings.Split(userCode, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "import ") {
				// Extract the import statement
				imports = append(imports, line)
			}
		}
		// Remove import lines from user code
		var codeLines []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "import ") {
				codeLines = append(codeLines, line)
			}
		}
		userCode = strings.Join(codeLines, "\n")
	}

	// Add all imports together
	code.WriteString("import (\n")
	code.WriteString("\t\"encoding/json\"\n")
	for _, imp := range imports {
		// Extract package name from import statement
		imp = strings.TrimPrefix(imp, "import ")
		imp = strings.TrimSpace(imp)
		if !strings.Contains(imp, "encoding/json") {
			code.WriteString(fmt.Sprintf("\t%s\n", imp))
		}
	}
	code.WriteString(")\n\n")

	// Declare input variable
	code.WriteString("var input map[string]interface{}\n")
	code.WriteString("var result interface{}\n\n")

	// Add input variable initialization function
	code.WriteString("func init() {\n")
	code.WriteString(fmt.Sprintf("\tvar inputJSON = `%s`\n", inputJSON))
	code.WriteString("\tinputData := make(map[string]interface{})\n")
	code.WriteString("\tjson.Unmarshal([]byte(inputJSON), &inputData)\n")
	code.WriteString("\tinput = inputData\n")
	code.WriteString("}\n\n")

	// Wrap user code in main function for execution
	code.WriteString("func main() {\n")
	// Indent user code
	lines := strings.Split(userCode, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			code.WriteString("\t")
			code.WriteString(line)
			code.WriteString("\n")
		}
	}
	code.WriteString("}\n")

	return code.String(), nil
}

// StartRuntime initializes the Go runtime
func (e *GoExecutor) StartRuntime(ctx context.Context, language executor.Language) (*executor.RuntimeStatus, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.runtime != nil {
		return &e.status, nil
	}

	// Create new interpreter
	i := interp.New(interp.Options{})

	// Load standard library
	if err := i.Use(stdlib.Symbols); err != nil {
		return nil, fmt.Errorf("failed to load stdlib: %w", err)
	}

	// Configure allowed packages based on sandbox
	if e.sandbox != nil && e.sandbox.Go != nil {
		// Yaegi doesn't have a built-in way to restrict packages at runtime,
		// so we rely on validation before execution
	}

	e.runtime = i
	e.status = executor.RuntimeStatus{
		Language:  executor.LanguageGo,
		Status:    "running",
		StartedAt: time.Now(),
		LastUsed:  time.Now(),
	}

	return &e.status, nil
}

// StopRuntime stops the Go runtime
func (e *GoExecutor) StopRuntime(ctx context.Context, language executor.Language) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.runtime = nil
	e.status.Status = "stopped"

	return nil
}

// ListRuntimes returns the current runtime status
func (e *GoExecutor) ListRuntimes(ctx context.Context) ([]executor.RuntimeStatus, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	status := e.status
	status.ExecuteCount = e.execCount

	return []executor.RuntimeStatus{status}, nil
}
