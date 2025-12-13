package presets

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/proxy"
)

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	if reg == nil {
		t.Fatal("NewRegistry returned nil")
	}
}

func TestRegistryGetGithubPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("github")
	if !ok {
		t.Fatal("github preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if len(cfg.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(cfg.Args))
	}

	if cfg.Args[0] != "-y" || cfg.Args[1] != "@modelcontextprotocol/server-github" {
		t.Errorf("unexpected args: %v", cfg.Args)
	}

	if cfg.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "${GITHUB_TOKEN}" {
		t.Errorf("unexpected env var value")
	}
}

func TestRegistryGetUnknownPreset(t *testing.T) {
	reg := NewRegistry()
	_, ok := reg.Get("nonexistent")
	if ok {
		t.Fatal("expected unknown preset to return false")
	}
}

func TestGetEnvPresetEmpty(t *testing.T) {
	// Clear env var if set
	os.Unsetenv("MCP_LENS_PRESET")

	reg := NewRegistry()
	cfg, err := reg.GetEnvPreset()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Command != "" {
		t.Errorf("expected empty config when env var not set")
	}
}

func TestGetEnvPresetGithub(t *testing.T) {
	os.Setenv("MCP_LENS_PRESET", "github")
	defer os.Unsetenv("MCP_LENS_PRESET")

	reg := NewRegistry()
	cfg, err := reg.GetEnvPreset()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}
}

func TestGetEnvPresetUnknown(t *testing.T) {
	os.Setenv("MCP_LENS_PRESET", "unknown-preset")
	defer os.Unsetenv("MCP_LENS_PRESET")

	reg := NewRegistry()
	_, err := reg.GetEnvPreset()
	if err == nil {
		t.Fatal("expected error for unknown preset")
	}
}

func TestMergeWithEnvNoOverrides(t *testing.T) {
	// Clear all env vars
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	cfg := proxy.Config{
		Command: "npx",
		Args:    []string{"-y", "test"},
		Env:     map[string]string{"KEY": "value"},
	}

	result, err := MergeWithEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Command != "npx" {
		t.Errorf("command should not change")
	}
}

func TestMergeWithEnvCommandOverride(t *testing.T) {
	os.Setenv("MCP_LENS_UPSTREAM_COMMAND", "python")
	defer os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	cfg := proxy.Config{
		Command: "npx",
		Args:    []string{"-y", "test"},
	}

	result, err := MergeWithEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Command != "python" {
		t.Errorf("expected command 'python', got '%s'", result.Command)
	}
}

func TestMergeWithEnvArgsOverride(t *testing.T) {
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	argsJSON, _ := json.Marshal([]string{"arg1", "arg2", "arg3"})
	os.Setenv("MCP_LENS_UPSTREAM_ARGS_JSON", string(argsJSON))
	defer os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	cfg := proxy.Config{
		Command: "npx",
		Args:    []string{"-y"},
	}

	result, err := MergeWithEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Args) != 3 {
		t.Errorf("expected 3 args, got %d", len(result.Args))
	}
	if result.Args[0] != "arg1" || result.Args[1] != "arg2" || result.Args[2] != "arg3" {
		t.Errorf("unexpected args: %v", result.Args)
	}
}

func TestMergeWithEnvArgsInvalidJSON(t *testing.T) {
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Setenv("MCP_LENS_UPSTREAM_ARGS_JSON", "invalid json")
	defer os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	cfg := proxy.Config{Command: "npx"}

	_, err := MergeWithEnv(cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMergeWithEnvEnvMerge(t *testing.T) {
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	envJSON, _ := json.Marshal(map[string]string{
		"NEW_VAR": "new_value",
		"OVERRIDE": "overridden",
	})
	os.Setenv("MCP_LENS_UPSTREAM_ENV_JSON", string(envJSON))
	defer os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	cfg := proxy.Config{
		Command: "npx",
		Env: map[string]string{
			"EXISTING": "exists",
			"OVERRIDE": "original",
		},
	}

	result, err := MergeWithEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Env["EXISTING"] != "exists" {
		t.Errorf("existing env var should be preserved")
	}
	if result.Env["NEW_VAR"] != "new_value" {
		t.Errorf("new env var should be added")
	}
	if result.Env["OVERRIDE"] != "overridden" {
		t.Errorf("env var should be overridden")
	}
}

func TestMergeWithEnvEnvInvalidJSON(t *testing.T) {
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Setenv("MCP_LENS_UPSTREAM_ENV_JSON", "invalid json")
	defer os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	cfg := proxy.Config{Command: "npx"}

	_, err := MergeWithEnv(cfg)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestMergeWithEnvMultipleOverrides(t *testing.T) {
	os.Setenv("MCP_LENS_UPSTREAM_COMMAND", "python")
	argsJSON, _ := json.Marshal([]string{"script.py"})
	os.Setenv("MCP_LENS_UPSTREAM_ARGS_JSON", string(argsJSON))
	envJSON, _ := json.Marshal(map[string]string{"PYTHONPATH": "/custom/path"})
	os.Setenv("MCP_LENS_UPSTREAM_ENV_JSON", string(envJSON))
	defer func() {
		os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
		os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
		os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")
	}()

	cfg := proxy.Config{
		Command: "npx",
		Args:    []string{"-y"},
		Env:     map[string]string{"PATH": "/usr/bin"},
	}

	result, err := MergeWithEnv(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Command != "python" {
		t.Errorf("command override failed")
	}
	if len(result.Args) != 1 || result.Args[0] != "script.py" {
		t.Errorf("args override failed")
	}
	if result.Env["PYTHONPATH"] != "/custom/path" {
		t.Errorf("env merge failed")
	}
	if result.Env["PATH"] != "/usr/bin" {
		t.Errorf("existing env var lost")
	}
}

func TestLoadConfigWithFallback(t *testing.T) {
	os.Unsetenv("MCP_LENS_PRESET")
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	fallback := proxy.Config{
		Command: "fallback-cmd",
		Args:    []string{"arg1"},
		Env:     map[string]string{"VAR": "value"},
	}

	result, err := LoadConfig(fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Command != "fallback-cmd" {
		t.Errorf("fallback config not used")
	}
}

func TestLoadConfigWithPreset(t *testing.T) {
	os.Setenv("MCP_LENS_PRESET", "github")
	defer os.Unsetenv("MCP_LENS_PRESET")
	os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	fallback := proxy.Config{
		Command: "fallback-cmd",
	}

	result, err := LoadConfig(fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Command != "npx" {
		t.Errorf("preset not loaded, got '%s'", result.Command)
	}
}

func TestLoadConfigWithEnvOverride(t *testing.T) {
	os.Unsetenv("MCP_LENS_PRESET")
	os.Setenv("MCP_LENS_UPSTREAM_COMMAND", "overridden-cmd")
	defer os.Unsetenv("MCP_LENS_UPSTREAM_COMMAND")
	os.Unsetenv("MCP_LENS_UPSTREAM_ARGS_JSON")
	os.Unsetenv("MCP_LENS_UPSTREAM_ENV_JSON")

	fallback := proxy.Config{
		Command: "fallback-cmd",
	}

	result, err := LoadConfig(fallback)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Command != "overridden-cmd" {
		t.Errorf("env override failed, got '%s'", result.Command)
	}
}

func TestListAvailable(t *testing.T) {
	available := ListAvailable()
	if len(available) == 0 {
		t.Fatal("expected at least one preset")
	}

	found := false
	for _, name := range available {
		if name == "github" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("github preset not in available list")
	}
}

func TestIsZeroConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      proxy.Config
		expected bool
	}{
		{
			name:     "empty config",
			cfg:      proxy.Config{},
			expected: true,
		},
		{
			name: "config with command",
			cfg: proxy.Config{
				Command: "cmd",
			},
			expected: false,
		},
		{
			name: "config with args",
			cfg: proxy.Config{
				Args: []string{"arg"},
			},
			expected: false,
		},
		{
			name: "config with env",
			cfg: proxy.Config{
				Env: map[string]string{"KEY": "value"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isZeroConfig(tt.cfg)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
