package native

import (
	"context"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/executor"
)

func TestJSExecutor_Execute(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewJSExecutor(sandbox)

	tests := []struct {
		name      string
		code      string
		input     map[string]interface{}
		wantError bool
	}{
		{
			name:      "simple assignment",
			code:      `var result = 42;`,
			wantError: false,
		},
		{
			name: "with input",
			code: `
var x = input.x;
var y = input.y;
var result = x + y;
result;
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
var name = input.name;
var result = name.toUpperCase();
result;
`,
			input: map[string]interface{}{
				"name": "hello",
			},
			wantError: false,
		},
		{
			name: "json operations",
			code: `
var data = {status: "ok", count: 42};
var json = JSON.stringify(data);
var parsed = JSON.parse(json);
parsed;
`,
			wantError: false,
		},
		{
			name: "array operations",
			code: `
var arr = [1, 2, 3, 4, 5];
var result = arr.map(function(x) { return x * 2; });
result;
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req := executor.ExecuteRequest{
				Language: executor.LanguageJavaScript,
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

func TestJSExecutor_StartStopRuntime(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewJSExecutor(sandbox)
	ctx := context.Background()

	// Start runtime
	status, err := exec.StartRuntime(ctx, executor.LanguageJavaScript)
	if err != nil {
		t.Fatalf("failed to start runtime: %v", err)
	}

	if status.Status != "running" {
		t.Errorf("expected status 'running', got %s", status.Status)
	}

	// Stop runtime
	if err := exec.StopRuntime(ctx, executor.LanguageJavaScript); err != nil {
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

func TestJSExecutor_Timeout(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewJSExecutor(sandbox)

	ctx := context.Background()
	req := executor.ExecuteRequest{
		Language: executor.LanguageJavaScript,
		Code: `
while(true) {
	// infinite loop
}
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

func TestJSExecutor_PrepareCode(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	exec := NewJSExecutor(sandbox)

	input := map[string]interface{}{
		"name": "test",
		"age":  30,
	}

	code := `var result = input.name;`

	prepared, err := exec.prepareCode(code, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if prepared == "" {
		t.Error("prepared code is empty")
	}

	// Should contain input variable
	if !jsContains(prepared, "var input") {
		t.Error("prepared code missing input variable")
	}

	// Should contain user code
	if !jsContains(prepared, code) {
		t.Error("prepared code missing user code")
	}
}

func TestJSExecutor_EscapeString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`hello`, `hello`},
		{`hello"world`, `hello\"world`},
		{`hello'world`, `hello\'world`},
		{`hello\nworld`, `hello\\nworld`},
		{"hello\nworld", `hello\nworld`},
		{"hello\tworld", `hello\tworld`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeJSString(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestJSExecutor_Console(t *testing.T) {
	sandbox := executor.DefaultSandboxConfig()
	sandbox.JavaScript.AllowConsole = true
	exec := NewJSExecutor(sandbox)

	ctx := context.Background()
	req := executor.ExecuteRequest{
		Language: executor.LanguageJavaScript,
		Code: `
console.log("hello world");
var result = 42;
result;
`,
		Timeout: 5 * time.Second,
	}

	resp, err := exec.Execute(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Error != "" {
		t.Errorf("unexpected error: %s", resp.Error)
	}
}

func jsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			len(s) > len(substr)+1 && jsFindSubstr(s, substr)))
}

func jsFindSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
