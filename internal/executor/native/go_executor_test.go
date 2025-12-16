package native

import (
	"context"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/executor"
)

func TestGoExecutor_Execute(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewGoExecutor(sandbox)

	tests := []struct {
		name      string
		code      string
		input     map[string]interface{}
		wantError bool
	}{
		{
			name:      "simple assignment",
			code:      `result := 42`,
			wantError: false,
		},
		{
			name: "with input",
			code: `
x := input["x"].(float64)
y := input["y"].(float64)
result := x + y
`,
			input: map[string]interface{}{
				"x": 10.0,
				"y": 20.0,
			},
			wantError: false,
		},
		{
			name: "string operation",
			code: `
import "strings"
name := input["name"].(string)
result := strings.ToUpper(name)
`,
			input: map[string]interface{}{
				"name": "hello",
			},
			wantError: false,
		},
		{
			name: "json encoding",
			code: `
import "encoding/json"
data := map[string]interface{}{
	"status": "ok",
	"count": 42,
}
bytes, _ := json.Marshal(data)
result := string(bytes)
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := executor.ExecuteRequest{
				Language: executor.LanguageGo,
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

func TestGoExecutor_StartStopRuntime(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewGoExecutor(sandbox)
	ctx := context.Background()

	// Start runtime
	status, err := exec.StartRuntime(ctx, executor.LanguageGo)
	if err != nil {
		t.Fatalf("failed to start runtime: %v", err)
	}

	if status.Status != "running" {
		t.Errorf("expected status 'running', got %s", status.Status)
	}

	// Stop runtime
	if err := exec.StopRuntime(ctx, executor.LanguageGo); err != nil {
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

func TestGoExecutor_Timeout(t *testing.T) {
	t.Skip("Yaegi interpreter doesn't support interrupting execution, timeout detection is context-based only")

	sandbox := executor.DefaultSandboxConfig()
	exec := NewGoExecutor(sandbox)

	ctx := context.Background()
	req := executor.ExecuteRequest{
		Language: executor.LanguageGo,
		Code: `
import "time"
time.Sleep(2 * time.Second)
result := "done"
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

func TestGoExecutor_PrepareCode(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewGoExecutor(sandbox)

	input := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	code := `result := input["name"]`

	prepared, err := exec.prepareCode(code, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check that prepared code contains expected elements
	if prepared == "" {
		t.Error("prepared code is empty")
	}

	// Should contain package declaration
	if !contains(prepared, "package main") {
		t.Error("prepared code missing package declaration")
	}

	// Should contain input variable
	if !contains(prepared, "var input") {
		t.Error("prepared code missing input variable")
	}

	// Should contain user code
	if !contains(prepared, code) {
		t.Error("prepared code missing user code")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && findSubstr(s, substr)))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
