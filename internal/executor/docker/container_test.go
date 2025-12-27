package docker

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestPrepareCommandWithInput tests that prepareCommand includes MCP_INPUT_JSON in env
func TestPrepareCommandWithInput(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangBash,
	}

	input := map[string]interface{}{
		"value": 42,
		"name":  "test",
	}

	cmd, env, err := lifecycle.prepareCommand("echo $MCP_INPUT_JSON", input)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify env contains MCP_INPUT_JSON
	found := false
	for _, e := range env {
		if strings.HasPrefix(e, "MCP_INPUT_JSON=") {
			found = true
			// Verify it's valid JSON
			jsonStr := strings.TrimPrefix(e, "MCP_INPUT_JSON=")
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
				t.Errorf("MCP_INPUT_JSON is not valid JSON: %v", err)
			}
			if parsed["value"] != float64(42) {
				t.Errorf("Expected value=42, got %v", parsed["value"])
			}
			break
		}
	}

	if !found {
		t.Error("Expected MCP_INPUT_JSON in env, but it was not found")
	}

	// Verify cmd is not nil
	if len(cmd) == 0 {
		t.Error("Expected non-empty command")
	}
}

// TestPrepareCommandTypeScriptUsesFile tests that TypeScript code is written to a file
func TestPrepareCommandTypeScriptUsesFile(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangTypeScript,
	}

	code := `
const x = 5;
const y = 10;
console.log(x + y);
`

	cmd, env, err := lifecycle.prepareCommand(code, nil)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify command writes to file and executes it
	cmdStr := strings.Join(cmd, " ")
	if !strings.Contains(cmdStr, "/tmp/main.mts") {
		t.Error("Expected command to write to /tmp/main.mts")
	}
	if !strings.Contains(cmdStr, "npx ts-node") {
		t.Error("Expected command to use npx ts-node")
	}
	if !strings.Contains(cmdStr, "printf") {
		t.Error("Expected command to use printf for writing file")
	}

	// Verify env is initialized (even if empty)
	if env == nil {
		t.Error("Expected env to be initialized")
	}
}

// TestPrepareCommandBashUsesFile tests that Bash code is written to a file
func TestPrepareCommandBashUsesFile(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangBash,
	}

	code := `#!/bin/bash
echo "line 1"
echo "line 2"
`

	cmd, _, err := lifecycle.prepareCommand(code, nil)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify command writes to file
	cmdStr := strings.Join(cmd, " ")
	if !strings.Contains(cmdStr, "/tmp/script.sh") {
		t.Error("Expected command to write to /tmp/script.sh")
	}
	if !strings.Contains(cmdStr, "printf") {
		t.Error("Expected command to use printf for writing file")
	}
}

// TestPrepareCommandNilInputDoesNotCrash tests that nil input doesn't cause issues
func TestPrepareCommandNilInputDoesNotCrash(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangBash,
	}

	cmd, env, err := lifecycle.prepareCommand("echo hello", nil)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify it doesn't crash and returns valid structures
	if cmd == nil {
		t.Error("Expected non-nil cmd")
	}
	if env == nil {
		t.Error("Expected non-nil env")
	}

	// With nil input, env should be empty
	if len(env) > 0 {
		t.Errorf("Expected empty env with nil input, got %d items", len(env))
	}
}

// TestPrepareCommandLargeInputUsesFile tests that large input uses MCP_INPUT_FILE
func TestPrepareCommandLargeInputUsesFile(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangBash,
	}

	// Create input larger than 32KB
	largeString := strings.Repeat("x", 33*1024)
	input := map[string]interface{}{
		"data": largeString,
	}

	cmd, env, err := lifecycle.prepareCommand("echo 'test'", input)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify env contains MCP_INPUT_FILE
	foundFile := false
	foundEmptyJSON := false
	for _, e := range env {
		if e == "MCP_INPUT_FILE=/tmp/mcp_input.json" {
			foundFile = true
		}
		if e == "MCP_INPUT_JSON=" {
			foundEmptyJSON = true
		}
	}

	if !foundFile {
		t.Error("Expected MCP_INPUT_FILE=/tmp/mcp_input.json in env for large input")
	}
	if !foundEmptyJSON {
		t.Error("Expected empty MCP_INPUT_JSON in env when using file")
	}

	// Verify command includes writing input file
	cmdStr := strings.Join(cmd, " ")
	if !strings.Contains(cmdStr, "/tmp/mcp_input.json") {
		t.Error("Expected command to write to /tmp/mcp_input.json")
	}
}

// TestPrepareCommandSmallInputUsesEnvVar tests that small input uses MCP_INPUT_JSON
func TestPrepareCommandSmallInputUsesEnvVar(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangBash,
	}

	// Create small input (under 32KB)
	input := map[string]interface{}{
		"value": 123,
		"data":  "small data",
	}

	cmd, env, err := lifecycle.prepareCommand("echo 'test'", input)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify env contains MCP_INPUT_JSON with actual data (not empty)
	foundJSON := false
	for _, e := range env {
		if strings.HasPrefix(e, "MCP_INPUT_JSON=") {
			foundJSON = true
			jsonStr := strings.TrimPrefix(e, "MCP_INPUT_JSON=")
			if jsonStr == "" {
				t.Error("Expected non-empty MCP_INPUT_JSON for small input")
			}
			// Verify it's valid JSON
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
				t.Errorf("MCP_INPUT_JSON is not valid JSON: %v", err)
			}
			break
		}
	}

	if !foundJSON {
		t.Error("Expected MCP_INPUT_JSON in env for small input")
	}

	// Verify env does NOT contain MCP_INPUT_FILE
	for _, e := range env {
		if strings.Contains(e, "MCP_INPUT_FILE") {
			t.Error("Expected no MCP_INPUT_FILE in env for small input")
		}
	}

	// Verify command does NOT include writing input file
	cmdStr := strings.Join(cmd, " ")
	if strings.Contains(cmdStr, "/tmp/mcp_input.json") {
		t.Error("Expected command to NOT write to /tmp/mcp_input.json for small input")
	}
}

// TestPrepareCommandLargeInputAllLanguages tests that all languages support large input
func TestPrepareCommandLargeInputAllLanguages(t *testing.T) {
	languages := []Language{LangBash, LangRuby, LangPHP, LangTypeScript, LangRust, LangJava}

	// Create input larger than 32KB
	largeString := strings.Repeat("x", 33*1024)
	input := map[string]interface{}{
		"data": largeString,
	}

	for _, lang := range languages {
		t.Run(string(lang), func(t *testing.T) {
			lifecycle := &ContainerLifecycle{
				language: lang,
			}

			cmd, env, err := lifecycle.prepareCommand("echo 'test'", input)
			if err != nil {
				t.Fatalf("prepareCommand failed for %s: %v", lang, err)
			}

			// Verify env contains MCP_INPUT_FILE
			foundFile := false
			for _, e := range env {
				if e == "MCP_INPUT_FILE=/tmp/mcp_input.json" {
					foundFile = true
					break
				}
			}

			if !foundFile {
				t.Errorf("Expected MCP_INPUT_FILE=/tmp/mcp_input.json in env for %s", lang)
			}

			// Verify command includes writing input file
			cmdStr := strings.Join(cmd, " ")
			if !strings.Contains(cmdStr, "/tmp/mcp_input.json") {
				t.Errorf("Expected command to write to /tmp/mcp_input.json for %s", lang)
			}
		})
	}
}
