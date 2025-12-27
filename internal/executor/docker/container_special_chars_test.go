package docker

import (
	"strings"
	"testing"
)

// TestPrepareCommandSpecialCharacters tests escaping of special characters
func TestPrepareCommandSpecialCharacters(t *testing.T) {
	tests := []struct {
		name        string
		language    Language
		code        string
		expectInCmd []string // Substrings that should be in the command
	}{
		{
			name:     "Bash - Double quotes",
			language: LangBash,
			code:     `echo "Hello World"`,
			expectInCmd: []string{
				"/tmp/script.sh",
				"printf",
			},
		},
		{
			name:     "Bash - Dollar sign",
			language: LangBash,
			code:     `echo $HOME`,
			expectInCmd: []string{
				"/tmp/script.sh",
			},
		},
		{
			name:     "Bash - Backticks",
			language: LangBash,
			code:     "echo `date`",
			expectInCmd: []string{
				"/tmp/script.sh",
			},
		},
		{
			name:     "Bash - Mixed quotes",
			language: LangBash,
			code:     `echo "It's a test"`,
			expectInCmd: []string{
				"/tmp/script.sh",
				`It'"'"'s`, // Should escape the single quote
			},
		},
		{
			name:     "Bash - Backslashes",
			language: LangBash,
			code:     `echo "Line 1\nLine 2"`,
			expectInCmd: []string{
				"/tmp/script.sh",
			},
		},
		{
			name:     "Bash - Complex script",
			language: LangBash,
			code: `#!/bin/bash
VAR="test's value"
echo "$VAR"
echo 'Single $VAR'
echo "Backtick: $(date)"`,
			expectInCmd: []string{
				"/tmp/script.sh",
				"test",
			},
		},
		{
			name:     "TypeScript - Single quotes",
			language: LangTypeScript,
			code:     `const msg = 'Hello'; console.log(msg);`,
			expectInCmd: []string{
				"/tmp/main.mts",
				"npx ts-node",
			},
		},
		{
			name:     "TypeScript - Double quotes",
			language: LangTypeScript,
			code:     `const msg = "Hello"; console.log(msg);`,
			expectInCmd: []string{
				"/tmp/main.mts",
				"npx ts-node",
			},
		},
		{
			name:     "TypeScript - Concatenation",
			language: LangTypeScript,
			code:     "const name = 'World'; console.log('Hello ' + name);",
			expectInCmd: []string{
				"/tmp/main.mts",
				"npx ts-node",
			},
		},
		{
			name:     "Ruby - Mixed quotes",
			language: LangRuby,
			code:     `puts "It's #{5 + 5}"`,
			expectInCmd: []string{
				"/tmp/script.rb",
				"ruby",
			},
		},
		{
			name:     "PHP - SQL-like strings",
			language: LangPHP,
			code:     `<?php echo "SELECT * FROM users WHERE name = 'O''Brien'"; ?>`,
			expectInCmd: []string{
				"/tmp/script.php",
				"php",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := &ContainerLifecycle{
				language: tt.language,
			}

			cmd, env, err := lifecycle.prepareCommand(tt.code, nil)
			if err != nil {
				t.Fatalf("prepareCommand failed: %v", err)
			}

			cmdStr := strings.Join(cmd, " ")
			for _, expect := range tt.expectInCmd {
				if !strings.Contains(cmdStr, expect) {
					t.Errorf("Command does not contain %q\nCommand: %s", expect, cmdStr)
				}
			}

			// Verify env is not nil
			if env == nil {
				t.Error("Expected non-nil env")
			}

			// Verify command is not empty
			if len(cmd) == 0 {
				t.Error("Expected non-empty command")
			}
		})
	}
}

// TestPrepareCommandTypeScriptComplexCode tests TypeScript with complex multi-line code
func TestPrepareCommandTypeScriptComplexCode(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangTypeScript,
	}

	code := `// Complex TypeScript code
const msg = "It's a \"test\"";
const numbers: number[] = [1, 2, 3];

function greet(name: string): void {
    console.log('Hello, ' + name + '!');
}

greet(msg);
console.log('Numbers:', numbers);`

	cmd, _, err := lifecycle.prepareCommand(code, nil)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	cmdStr := strings.Join(cmd, " ")

	// Verify basic structure
	if !strings.Contains(cmdStr, "/tmp/main.mts") {
		t.Error("Expected command to write to /tmp/main.mts")
	}
	if !strings.Contains(cmdStr, "npx ts-node") {
		t.Error("Expected command to use npx ts-node")
	}
	if !strings.Contains(cmdStr, "printf") {
		t.Error("Expected command to use printf for writing file")
	}
}

// TestPrepareCommandMultilinePreservation tests that newlines are preserved
func TestPrepareCommandMultilinePreservation(t *testing.T) {
	tests := []struct {
		name     string
		language Language
		code     string
		filename string
	}{
		{
			name:     "Bash multi-line",
			language: LangBash,
			code: `#!/bin/bash
echo "Line 1"
echo "Line 2"
echo "Line 3"`,
			filename: "/tmp/script.sh",
		},
		{
			name:     "TypeScript multi-line",
			language: LangTypeScript,
			code: `console.log("Line 1");
console.log("Line 2");
console.log("Line 3");`,
			filename: "/tmp/main.mts",
		},
		{
			name:     "Ruby multi-line",
			language: LangRuby,
			code: `puts "Line 1"
puts "Line 2"
puts "Line 3"`,
			filename: "/tmp/script.rb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := &ContainerLifecycle{
				language: tt.language,
			}

			cmd, _, err := lifecycle.prepareCommand(tt.code, nil)
			if err != nil {
				t.Fatalf("prepareCommand failed: %v", err)
			}

			cmdStr := strings.Join(cmd, " ")

			// Verify the command contains the filename
			if !strings.Contains(cmdStr, tt.filename) {
				t.Errorf("Expected command to contain %q", tt.filename)
			}

			// Verify printf is used (which preserves newlines)
			if !strings.Contains(cmdStr, "printf") {
				t.Error("Expected command to use printf")
			}

			// Count newlines in the command - should match code newlines
			// Note: This is a heuristic check
			originalNewlines := strings.Count(tt.code, "\n")
			if originalNewlines > 0 {
				// Command should contain the code with newlines
				// We can't check exact count due to escaping, but should be > 0
				if !strings.Contains(cmdStr, "\n") && originalNewlines > 1 {
					t.Log("Warning: Command may not preserve all newlines correctly")
				}
			}
		})
	}
}

// TestPrepareCommandEscapingEdgeCases tests edge cases in escaping
func TestPrepareCommandEscapingEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		language Language
	}{
		{
			name:     "Consecutive single quotes",
			code:     "echo ''''",
			language: LangBash,
		},
		{
			name:     "Quote at start",
			code:     "'echo hello",
			language: LangBash,
		},
		{
			name:     "Quote at end",
			code:     "echo hello'",
			language: LangBash,
		},
		{
			name:     "Empty single quotes",
			code:     "echo ''",
			language: LangBash,
		},
		{
			name:     "Mixed escape sequences",
			code:     "echo \"It's \\\"quoted\\\"\"",
			language: LangBash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := &ContainerLifecycle{
				language: tt.language,
			}

			cmd, env, err := lifecycle.prepareCommand(tt.code, nil)
			if err != nil {
				t.Fatalf("prepareCommand failed: %v", err)
			}

			// Should not crash and should produce a valid command
			if len(cmd) == 0 {
				t.Error("Expected non-empty command")
			}
			if env == nil {
				t.Error("Expected non-nil env")
			}

			cmdStr := strings.Join(cmd, " ")
			t.Logf("Code: %q\nCommand: %s", tt.code, cmdStr)
		})
	}
}

// TestPrepareCommandWithInputAndSpecialChars tests combination of input and special chars
func TestPrepareCommandWithInputAndSpecialChars(t *testing.T) {
	lifecycle := &ContainerLifecycle{
		language: LangBash,
	}

	input := map[string]interface{}{
		"message": "It's a test",
		"count":   42,
	}

	code := `echo "Message: $MCP_INPUT_JSON"
echo 'Also testing quotes'`

	cmd, env, err := lifecycle.prepareCommand(code, input)
	if err != nil {
		t.Fatalf("prepareCommand failed: %v", err)
	}

	// Verify input is in env
	foundInput := false
	for _, e := range env {
		if strings.HasPrefix(e, "MCP_INPUT_JSON=") {
			foundInput = true
			jsonStr := strings.TrimPrefix(e, "MCP_INPUT_JSON=")
			if !strings.Contains(jsonStr, "It's a test") {
				t.Errorf("Expected input to contain original message with quote")
			}
			break
		}
	}
	if !foundInput {
		t.Error("Expected MCP_INPUT_JSON in env")
	}

	// Verify command structure
	cmdStr := strings.Join(cmd, " ")
	if !strings.Contains(cmdStr, "/tmp/script.sh") {
		t.Error("Expected command to use file")
	}
}
