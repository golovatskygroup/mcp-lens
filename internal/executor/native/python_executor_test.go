package native

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/executor"
)

func TestPythonExecutor_Execute(t *testing.T) {
	// Check if python is available
	if !isPythonAvailable() {
		t.Skip("python3 not available")
	}

	sandbox := executor.DefaultSandboxConfig()
	exec := NewPythonExecutor(sandbox)

	tests := []struct {
		name      string
		code      string
		input     map[string]interface{}
		wantError bool
	}{
		{
			name:      "simple assignment",
			code:      `result = 42`,
			wantError: false,
		},
		{
			name: "with input",
			code: `
x = input['x']
y = input['y']
result = x + y
`,
			input: map[string]interface{}{
				"x": 10,
				"y": 20,
			},
			wantError: false,
		},
		{
			name: "string operation",
			code: `
name = input['name']
result = name.upper()
`,
			input: map[string]interface{}{
				"name": "hello",
			},
			wantError: false,
		},
		{
			name: "json operations",
			code: `
data = {'status': 'ok', 'count': 42}
result = data
`,
			wantError: false,
		},
		{
			name: "list operations",
			code: `
arr = [1, 2, 3, 4, 5]
result = [x * 2 for x in arr]
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := executor.ExecuteRequest{
				Language: executor.LanguagePython,
				Code:     tt.code,
				Input:    tt.input,
				Timeout:  5 * time.Second,
			}

			resp, err := exec.Execute(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantError && resp.Error == "" {
				t.Error("expected error but got none")
			}
			if !tt.wantError && resp.Error != "" {
				t.Errorf("unexpected error: %s", resp.Error)
			}
		})
	}
}

func TestPythonExecutor_StartStopRuntime(t *testing.T) {
	if !isPythonAvailable() {
		t.Skip("python3 not available")
	}

	sandbox := executor.DefaultSandboxConfig()
	exec := NewPythonExecutor(sandbox)
	ctx := context.Background()

	// Start runtime
	status, err := exec.StartRuntime(ctx, executor.LanguagePython)
	if err != nil {
		t.Fatalf("failed to start runtime: %v", err)
	}

	if status.Status != "running" {
		t.Errorf("expected status 'running', got %s", status.Status)
	}

	// Stop runtime
	if err := exec.StopRuntime(ctx, executor.LanguagePython); err != nil {
		t.Fatalf("failed to stop runtime: %v", err)
	}

	// List runtimes
	statuses, err := exec.ListRuntimes(ctx)
	if err != nil {
		t.Fatalf("failed to list runtimes: %v", err)
	}

	if len(statuses) != 1 {
		t.Errorf("expected 1 status, got %d", len(statuses))
	}

	if statuses[0].Status != "stopped" {
		t.Errorf("expected status 'stopped', got %s", statuses[0].Status)
	}
}

func TestPythonExecutor_Timeout(t *testing.T) {
	if !isPythonAvailable() {
		t.Skip("python3 not available")
	}

	sandbox := executor.DefaultSandboxConfig()
	exec := NewPythonExecutor(sandbox)

	ctx := context.Background()
	req := executor.ExecuteRequest{
		Language: executor.LanguagePython,
		Code: `
import time
time.sleep(2)
result = "done"
`,
		Timeout: 100 * time.Millisecond,
	}

	resp, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error != "execution timeout" {
		t.Errorf("expected timeout error, got: %s", resp.Error)
	}
}

func TestPythonExecutor_PrepareCode(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewPythonExecutor(sandbox)

	input := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	code := `result = input['name']`

	prepared, err := exec.prepareCode(code, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prepared == "" {
		t.Error("prepared code is empty")
	}

	// Should contain imports
	if !pyContains(prepared, "import json") {
		t.Error("prepared code missing json import")
	}

	// Should contain input variable
	if !pyContains(prepared, "input =") {
		t.Error("prepared code missing input variable")
	}

	// Should contain user code
	if !pyContains(prepared, code) {
		t.Error("prepared code missing user code")
	}
}

func TestPythonExecutor_SyntaxError(t *testing.T) {
	if !isPythonAvailable() {
		t.Skip("python3 not available")
	}

	sandbox := executor.DefaultSandboxConfig()
	exec := NewPythonExecutor(sandbox)

	ctx := context.Background()
	req := executor.ExecuteRequest{
		Language: executor.LanguagePython,
		Code:     `result = `,
		Timeout:  5 * time.Second,
	}

	resp, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error == "" {
		t.Error("expected syntax error but got none")
	}
}

func TestPythonExecutor_SetPythonPath(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewPythonExecutor(sandbox)

	exec.SetPythonPath("/usr/bin/python3")

	// Verify path was set (access through struct field or method)
	if exec.pythonPath != "/usr/bin/python3" {
		t.Errorf("expected python path to be set to /usr/bin/python3, got %s", exec.pythonPath)
	}
}

func isPythonAvailable() bool {
	cmd := exec.Command("python3", "--version")
	return cmd.Run() == nil
}

func pyContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && pyFindSubstr(s, substr)))
}

func pyFindSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
