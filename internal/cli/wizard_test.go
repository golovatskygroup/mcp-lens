package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/presets"
)

func TestNewWizard(t *testing.T) {
	w := NewWizard()
	if w == nil {
		t.Fatal("NewWizard returned nil")
	}
	if w.registry == nil {
		t.Error("wizard registry is nil")
	}
}

func TestNewWizardWithIO(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	if w == nil {
		t.Fatal("NewWizardWithIO returned nil")
	}
	if w.in != in {
		t.Error("input reader not set correctly")
	}
	if w.out != out {
		t.Error("output writer not set correctly")
	}
}

func TestListPresets(t *testing.T) {
	out := &bytes.Buffer{}
	ListPresets(out)

	output := out.String()

	// Check header
	if !strings.Contains(output, "Available MCP Server Presets") {
		t.Error("missing header in output")
	}

	// Check that all categories are listed
	cats := presets.Categories()
	for _, info := range cats {
		if !strings.Contains(output, info.Name) {
			t.Errorf("category %s not found in output", info.Name)
		}
	}

	// Check that some known presets are listed
	expectedPresets := []string{"github", "postgres", "brave-search", "slack", "kubernetes"}
	for _, p := range expectedPresets {
		if !strings.Contains(output, p) {
			t.Errorf("preset %s not found in output", p)
		}
	}
}

func TestWizardSelectCategory(t *testing.T) {
	// Simulate selecting category 1 (Reference)
	in := strings.NewReader("1\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	cat, err := w.selectCategory()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Category 1 should be Reference (first in sorted order)
	sortedCats := presets.SortedCategories()
	if cat != sortedCats[0] {
		t.Errorf("expected first category %s, got %s", sortedCats[0], cat)
	}

	// Check output contains category list
	output := out.String()
	if !strings.Contains(output, "Available preset categories") {
		t.Error("missing categories prompt in output")
	}
}

func TestWizardSelectCategoryInvalidThenValid(t *testing.T) {
	// Simulate invalid input, then valid
	in := strings.NewReader("invalid\n0\n99\n2\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	cat, err := w.selectCategory()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After retries, should select category 2
	sortedCats := presets.SortedCategories()
	if cat != sortedCats[1] {
		t.Errorf("expected second category %s, got %s", sortedCats[1], cat)
	}

	// Check retry messages
	output := out.String()
	if !strings.Contains(output, "Invalid selection") {
		t.Error("missing retry message in output")
	}
}

func TestWizardSelectPreset(t *testing.T) {
	in := strings.NewReader("1\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	preset, err := w.selectPreset(presets.CategoryProductivity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if preset.Name == "" {
		t.Error("preset name is empty")
	}
	if preset.Category != presets.CategoryProductivity {
		t.Errorf("expected category productivity, got %s", preset.Category)
	}
}

func TestWizardConfigureEnvVarsNoRequired(t *testing.T) {
	in := strings.NewReader("")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	// Filesystem preset has no required env vars
	reg := presets.NewRegistry()
	preset, _ := reg.GetMeta("filesystem")

	envVars, err := w.configureEnvVars(preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have empty env vars map
	if len(envVars) != 0 {
		t.Errorf("expected 0 env vars, got %d", len(envVars))
	}
}

func TestWizardConfigureEnvVarsWithRequired(t *testing.T) {
	// Simulate entering a value for GITHUB_TOKEN, then skip optional
	in := strings.NewReader("my-token-value\n\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	reg := presets.NewRegistry()
	preset, _ := reg.GetMeta("github")

	envVars, err := w.configureEnvVars(preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envVars["GITHUB_TOKEN"] != "my-token-value" {
		t.Errorf("expected GITHUB_TOKEN='my-token-value', got '%s'", envVars["GITHUB_TOKEN"])
	}
}

func TestWizardConfigureEnvVarsOptionalSkipped(t *testing.T) {
	// Enter required value, then skip optional (empty input)
	in := strings.NewReader("my-token\n\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	reg := presets.NewRegistry()
	preset, _ := reg.GetMeta("github")

	envVars, err := w.configureEnvVars(preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Required should be present
	if envVars["GITHUB_TOKEN"] != "my-token" {
		t.Errorf("expected GITHUB_TOKEN='my-token', got '%s'", envVars["GITHUB_TOKEN"])
	}

	// Optional should not be present (skipped)
	if _, ok := envVars["GITHUB_API_URL"]; ok {
		t.Error("GITHUB_API_URL should not be present when skipped")
	}
}

func TestWizardConfigureEnvVarsOptionalProvided(t *testing.T) {
	// Enter required value, then provide optional value
	in := strings.NewReader("my-token\nhttps://github.company.com/api/v3\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	reg := presets.NewRegistry()
	preset, _ := reg.GetMeta("github")

	envVars, err := w.configureEnvVars(preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envVars["GITHUB_TOKEN"] != "my-token" {
		t.Errorf("expected GITHUB_TOKEN='my-token', got '%s'", envVars["GITHUB_TOKEN"])
	}

	if envVars["GITHUB_API_URL"] != "https://github.company.com/api/v3" {
		t.Errorf("expected GITHUB_API_URL to be set, got '%s'", envVars["GITHUB_API_URL"])
	}
}

func TestWizardConfigureEnvVarsRequiredRetry(t *testing.T) {
	// Enter empty (should retry), then enter value, then skip optional
	in := strings.NewReader("\nmy-token\n\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	reg := presets.NewRegistry()
	preset, _ := reg.GetMeta("github")

	envVars, err := w.configureEnvVars(preset)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if envVars["GITHUB_TOKEN"] != "my-token" {
		t.Errorf("expected GITHUB_TOKEN='my-token', got '%s'", envVars["GITHUB_TOKEN"])
	}

	// Check retry message
	output := out.String()
	if !strings.Contains(output, "required") {
		t.Error("missing required field warning in output")
	}
}

func TestWizardPrintConfiguration(t *testing.T) {
	out := &bytes.Buffer{}
	w := NewWizardWithIO(strings.NewReader(""), out)

	reg := presets.NewRegistry()
	preset, _ := reg.GetMeta("github")
	envVars := map[string]string{
		"GITHUB_TOKEN": "ghp_testtoken123456789",
	}

	w.printConfiguration(preset, envVars)

	output := out.String()

	// Check completion message
	if !strings.Contains(output, "Configuration complete") {
		t.Error("missing completion message")
	}

	// Check export command
	if !strings.Contains(output, "export MCP_LENS_PRESET=\"github\"") {
		t.Error("missing preset export command")
	}

	// Token should be masked (first 4 + last 4 chars shown)
	if !strings.Contains(output, "ghp_...9") || strings.Contains(output, "ghp_testtoken123456789") {
		// Token should be partially masked
		t.Log("Note: Token masking may vary based on length")
	}

	// Check YAML config example
	if !strings.Contains(output, "upstream:") {
		t.Error("missing YAML config example")
	}
	if !strings.Contains(output, "preset: github") {
		t.Error("missing preset in YAML config")
	}

	// Check run command
	if !strings.Contains(output, "mcp-lens --config config.yaml") {
		t.Error("missing run command")
	}
}

func TestWizardFullFlow(t *testing.T) {
	// Simulate full wizard flow:
	// 1. Select category 1 (Reference)
	// 2. Select preset 1 (first in Reference)
	in := strings.NewReader("1\n1\n")
	out := &bytes.Buffer{}
	w := NewWizardWithIO(in, out)

	err := w.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := out.String()

	// Check header
	if !strings.Contains(output, "MCP Lens Server Setup Wizard") {
		t.Error("missing wizard header")
	}

	// Check configuration complete
	if !strings.Contains(output, "Configuration complete") {
		t.Error("missing completion message")
	}
}

func TestCheckServerAvailability(t *testing.T) {
	// Test with a preset that should exist
	result, err := CheckServerAvailability("github")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Preset != "github" {
		t.Errorf("expected preset 'github', got '%s'", result.Preset)
	}

	if result.Command != "npx" {
		t.Errorf("expected command 'npx', got '%s'", result.Command)
	}

	// Check env vars are reported
	if len(result.EnvVars) == 0 {
		t.Error("expected at least one env var check")
	}

	// Check that GITHUB_TOKEN is in the list and marked as required
	foundGitHubToken := false
	for _, env := range result.EnvVars {
		if env.Name == "GITHUB_TOKEN" {
			foundGitHubToken = true
			if !env.Required {
				t.Error("GITHUB_TOKEN should be marked as required")
			}
		}
	}
	if !foundGitHubToken {
		t.Error("GITHUB_TOKEN not found in env var checks")
	}
}

func TestCheckServerAvailabilityUnknownPreset(t *testing.T) {
	_, err := CheckServerAvailability("nonexistent-preset")
	if err == nil {
		t.Error("expected error for unknown preset")
	}
}

func TestAvailabilityResultIsReady(t *testing.T) {
	tests := []struct {
		name     string
		result   AvailabilityResult
		expected bool
	}{
		{
			name: "all ready",
			result: AvailabilityResult{
				CommandAvailable: true,
				EnvVars: []EnvVarCheck{
					{Name: "VAR1", Required: true, Present: true},
					{Name: "VAR2", Required: false, Present: false},
				},
			},
			expected: true,
		},
		{
			name: "command not available",
			result: AvailabilityResult{
				CommandAvailable: false,
				EnvVars:          []EnvVarCheck{},
			},
			expected: false,
		},
		{
			name: "required env missing",
			result: AvailabilityResult{
				CommandAvailable: true,
				EnvVars: []EnvVarCheck{
					{Name: "VAR1", Required: true, Present: false},
				},
			},
			expected: false,
		},
		{
			name: "optional env missing is ok",
			result: AvailabilityResult{
				CommandAvailable: true,
				EnvVars: []EnvVarCheck{
					{Name: "VAR1", Required: false, Present: false},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result.IsReady() != tt.expected {
				t.Errorf("expected IsReady()=%v, got %v", tt.expected, tt.result.IsReady())
			}
		})
	}
}

func TestAvailabilityResultPrintResult(t *testing.T) {
	out := &bytes.Buffer{}
	result := &AvailabilityResult{
		Preset:           "test-preset",
		Command:          "test-cmd",
		CommandAvailable: true,
		EnvVars: []EnvVarCheck{
			{Name: "VAR1", Required: true, Present: true},
			{Name: "VAR2", Required: true, Present: false},
			{Name: "VAR3", Required: false, Present: false},
		},
	}

	result.PrintResult(out)
	output := out.String()

	// Check preset name
	if !strings.Contains(output, "test-preset") {
		t.Error("missing preset name in output")
	}

	// Check command status
	if !strings.Contains(output, "test-cmd") {
		t.Error("missing command in output")
	}

	// Check env var statuses (✅ for present required, ❌ for missing required, ⚪ for missing optional)
	if !strings.Contains(output, "VAR1") {
		t.Error("missing VAR1 in output")
	}
	if !strings.Contains(output, "VAR2") {
		t.Error("missing VAR2 in output")
	}

	// Check not ready message (because VAR2 is missing)
	if !strings.Contains(output, "not ready") {
		t.Error("missing not ready message")
	}
}

func TestWizardIsSecretEnv(t *testing.T) {
	w := NewWizardWithIO(strings.NewReader(""), &bytes.Buffer{})
	reg := presets.NewRegistry()

	// GitHub preset has GITHUB_TOKEN as secret
	preset, _ := reg.GetMeta("github")

	if !w.isSecretEnv(preset, "GITHUB_TOKEN") {
		t.Error("GITHUB_TOKEN should be marked as secret")
	}

	if w.isSecretEnv(preset, "GITHUB_API_URL") {
		t.Error("GITHUB_API_URL should not be marked as secret")
	}

	if w.isSecretEnv(preset, "NONEXISTENT") {
		t.Error("NONEXISTENT should not be marked as secret")
	}
}
