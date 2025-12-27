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

func TestRegistryGetMeta(t *testing.T) {
	reg := NewRegistry()
	meta, ok := reg.GetMeta("github")
	if !ok {
		t.Fatal("github preset not found")
	}

	if meta.Name != "github" {
		t.Errorf("expected name 'github', got '%s'", meta.Name)
	}

	if meta.Category != CategoryProductivity {
		t.Errorf("expected category 'productivity', got '%s'", meta.Category)
	}

	if meta.Description == "" {
		t.Error("expected non-empty description")
	}

	if len(meta.RequiredEnv) == 0 {
		t.Error("expected at least one required env var")
	}

	foundGitHubToken := false
	for _, env := range meta.RequiredEnv {
		if env.Name == "GITHUB_TOKEN" {
			foundGitHubToken = true
			if !env.Secret {
				t.Error("GITHUB_TOKEN should be marked as secret")
			}
		}
	}
	if !foundGitHubToken {
		t.Error("GITHUB_TOKEN not found in required env vars")
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
		"NEW_VAR":  "new_value",
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

	expectedPresets := []string{
		"github", "search",
		"filesystem", "sqlite", "postgres",
		"puppeteer", "brave-search", "memory",
		"fetch", "git", "slack", "gitlab",
		"google-maps", "sequential-thinking",
		// New presets
		"everything", "time", "tavily", "redis",
		"mongodb", "notion", "linear", "sentry",
		"aws", "kubernetes",
	}
	for _, expected := range expectedPresets {
		found := false
		for _, name := range available {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s preset not in available list", expected)
		}
	}
}

func TestRegistryGetSearchPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("search")
	if !ok {
		t.Fatal("search preset not found")
	}

	if cfg.Command != "python" {
		t.Errorf("expected command 'python', got '%s'", cfg.Command)
	}

	if len(cfg.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(cfg.Args))
	}

	if cfg.Args[0] != "-m" || cfg.Args[1] != "search_server" {
		t.Errorf("unexpected args: %v", cfg.Args)
	}

	if cfg.Env["OPENROUTER_API_KEY"] != "${OPENROUTER_API_KEY}" {
		t.Errorf("unexpected env var value")
	}
}

func TestRegistryGetFilesystemPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("filesystem")
	if !ok {
		t.Fatal("filesystem preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if len(cfg.Args) != 2 {
		t.Errorf("expected 2 args, got %d", len(cfg.Args))
	}

	if cfg.Args[0] != "-y" || cfg.Args[1] != "@modelcontextprotocol/server-filesystem" {
		t.Errorf("unexpected args: %v", cfg.Args)
	}
}

func TestRegistryGetSqlitePreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("sqlite")
	if !ok {
		t.Fatal("sqlite preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Args[1] != "@modelcontextprotocol/server-sqlite" {
		t.Errorf("unexpected args: %v", cfg.Args)
	}
}

func TestRegistryGetPostgresPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("postgres")
	if !ok {
		t.Fatal("postgres preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Env["POSTGRES_CONNECTION_STRING"] != "${POSTGRES_CONNECTION_STRING}" {
		t.Errorf("unexpected env var value")
	}
}

func TestRegistryGetBraveSearchPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("brave-search")
	if !ok {
		t.Fatal("brave-search preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Env["BRAVE_API_KEY"] != "${BRAVE_API_KEY}" {
		t.Errorf("unexpected env var value")
	}
}

func TestRegistryGetSlackPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("slack")
	if !ok {
		t.Fatal("slack preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Env["SLACK_BOT_TOKEN"] != "${SLACK_BOT_TOKEN}" {
		t.Errorf("unexpected SLACK_BOT_TOKEN env var value")
	}

	if cfg.Env["SLACK_TEAM_ID"] != "${SLACK_TEAM_ID}" {
		t.Errorf("unexpected SLACK_TEAM_ID env var value")
	}
}

func TestRegistryGetGitlabPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("gitlab")
	if !ok {
		t.Fatal("gitlab preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Env["GITLAB_PERSONAL_ACCESS_TOKEN"] != "${GITLAB_TOKEN}" {
		t.Errorf("unexpected GITLAB_PERSONAL_ACCESS_TOKEN env var value")
	}

	if cfg.Env["GITLAB_API_URL"] != "${GITLAB_API_URL}" {
		t.Errorf("unexpected GITLAB_API_URL env var value")
	}
}

func TestRegistryGetGoogleMapsPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("google-maps")
	if !ok {
		t.Fatal("google-maps preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Env["GOOGLE_MAPS_API_KEY"] != "${GOOGLE_MAPS_API_KEY}" {
		t.Errorf("unexpected env var value")
	}
}

func TestRegistryGetSequentialThinkingPreset(t *testing.T) {
	reg := NewRegistry()
	cfg, ok := reg.Get("sequential-thinking")
	if !ok {
		t.Fatal("sequential-thinking preset not found")
	}

	if cfg.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", cfg.Command)
	}

	if cfg.Args[1] != "@modelcontextprotocol/server-sequential-thinking" {
		t.Errorf("unexpected args: %v", cfg.Args)
	}
}

func TestAllPresetsExistInRegistry(t *testing.T) {
	reg := NewRegistry()
	available := ListAvailable()

	for _, name := range available {
		_, ok := reg.Get(name)
		if !ok {
			t.Errorf("preset %s listed in ListAvailable() but not found in registry", name)
		}
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

// New tests for extended functionality

func TestCategories(t *testing.T) {
	cats := Categories()

	expectedCategories := []Category{
		CategoryReference,
		CategoryDatabase,
		CategorySearch,
		CategoryProductivity,
		CategoryDevTools,
	}

	for _, expected := range expectedCategories {
		info, ok := cats[expected]
		if !ok {
			t.Errorf("category %s not found", expected)
			continue
		}
		if info.Name == "" {
			t.Errorf("category %s has empty name", expected)
		}
		if info.Description == "" {
			t.Errorf("category %s has empty description", expected)
		}
	}
}

func TestSortedCategories(t *testing.T) {
	sorted := SortedCategories()
	cats := Categories()

	if len(sorted) != len(cats) {
		t.Errorf("expected %d categories, got %d", len(cats), len(sorted))
	}

	// Verify ordering
	for i := 1; i < len(sorted); i++ {
		prevOrder := cats[sorted[i-1]].Order
		currOrder := cats[sorted[i]].Order
		if prevOrder > currOrder {
			t.Errorf("categories not sorted: %s (order %d) should come before %s (order %d)",
				sorted[i-1], prevOrder, sorted[i], currOrder)
		}
	}
}

func TestRegistryGetByCategory(t *testing.T) {
	reg := NewRegistry()

	// Test each category has at least one preset
	for _, cat := range SortedCategories() {
		presets := reg.GetByCategory(cat)
		if len(presets) == 0 {
			t.Errorf("category %s has no presets", cat)
		}

		// Verify all presets in category actually have that category
		for _, p := range presets {
			if p.Category != cat {
				t.Errorf("preset %s has category %s but was returned for category %s",
					p.Name, p.Category, cat)
			}
		}
	}
}

func TestRegistryAll(t *testing.T) {
	reg := NewRegistry()
	all := reg.All()

	if len(all) == 0 {
		t.Fatal("no presets returned")
	}

	// Check that all presets have required fields
	for _, p := range all {
		if p.Name == "" {
			t.Error("preset has empty name")
		}
		if p.Description == "" {
			t.Errorf("preset %s has empty description", p.Name)
		}
		if p.Config.Command == "" {
			t.Errorf("preset %s has empty command", p.Name)
		}
	}
}

func TestListByCategory(t *testing.T) {
	byCategory := ListByCategory()

	// Verify all categories exist
	for _, cat := range SortedCategories() {
		names, ok := byCategory[cat]
		if !ok {
			t.Errorf("category %s missing from ListByCategory()", cat)
			continue
		}
		if len(names) == 0 {
			t.Errorf("category %s has no preset names", cat)
		}
	}
}

func TestNewPresetsExist(t *testing.T) {
	reg := NewRegistry()

	newPresets := []struct {
		name     string
		category Category
	}{
		{"everything", CategoryReference},
		{"time", CategoryReference},
		{"tavily", CategorySearch},
		{"redis", CategoryDatabase},
		{"mongodb", CategoryDatabase},
		{"notion", CategoryProductivity},
		{"linear", CategoryProductivity},
		{"sentry", CategoryDevTools},
		{"aws", CategoryDevTools},
		{"kubernetes", CategoryDevTools},
	}

	for _, np := range newPresets {
		meta, ok := reg.GetMeta(np.name)
		if !ok {
			t.Errorf("new preset %s not found", np.name)
			continue
		}

		if meta.Category != np.category {
			t.Errorf("preset %s has category %s, expected %s",
				np.name, meta.Category, np.category)
		}

		if meta.Description == "" {
			t.Errorf("preset %s has empty description", np.name)
		}
	}
}

func TestPresetMetadataComplete(t *testing.T) {
	reg := NewRegistry()
	all := reg.All()

	for _, p := range all {
		// Every preset should have a description
		if p.Description == "" {
			t.Errorf("preset %s missing description", p.Name)
		}

		// Every preset should have a valid category
		if p.Category == "" {
			t.Errorf("preset %s missing category", p.Name)
		}

		// Verify category is valid
		cats := Categories()
		if _, ok := cats[p.Category]; !ok {
			t.Errorf("preset %s has invalid category: %s", p.Name, p.Category)
		}
	}
}

func TestPresetRequiredEnvVarsAreSecret(t *testing.T) {
	reg := NewRegistry()
	all := reg.All()

	// These env var patterns should always be marked as secret
	secretPatterns := []string{"TOKEN", "KEY", "SECRET", "PASSWORD", "PAT", "URI", "STRING"}

	for _, p := range all {
		for _, env := range p.RequiredEnv {
			shouldBeSecret := false
			for _, pattern := range secretPatterns {
				if containsIgnoreCase(env.Name, pattern) {
					shouldBeSecret = true
					break
				}
			}

			// TEAM_ID, ORG, REGION, etc. should not be secret
			if containsIgnoreCase(env.Name, "TEAM_ID") ||
				containsIgnoreCase(env.Name, "_ORG") ||
				containsIgnoreCase(env.Name, "REGION") {
				shouldBeSecret = false
			}

			if shouldBeSecret && !env.Secret {
				t.Errorf("preset %s: env var %s should be marked as secret",
					p.Name, env.Name)
			}
		}
	}
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(s) > len(substr) &&
				(containsCI(s, substr)))
}

func containsCI(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalFoldSlice(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

func equalFoldSlice(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
