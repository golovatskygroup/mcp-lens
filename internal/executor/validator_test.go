package executor

import (
	"testing"
)

func TestValidator_ValidateGo(t *testing.T) {
	sandbox := DefaultSandboxConfig()
	validator := NewValidator(sandbox)

	tests := []struct {
		name    string
		code    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid code",
			code: `
package main
import "fmt"
func main() {
	result := 42
	fmt.Println(result)
}`,
			wantErr: false,
		},
		{
			name: "forbidden package os/exec",
			code: `
package main
import "os/exec"
func main() {
	exec.Command("ls")
}`,
			wantErr: true,
			errMsg:  "forbidden package",
		},
		{
			name: "forbidden package syscall",
			code: `
package main
import "syscall"
func main() {
	syscall.Kill(1, 9)
}`,
			wantErr: true,
			errMsg:  "forbidden package",
		},
		{
			name: "allowed package",
			code: `
package main
import "encoding/json"
func main() {
	data := []byte("{}")
	var m map[string]interface{}
	json.Unmarshal(data, &m)
}`,
			wantErr: false,
		},
		{
			name: "syntax error",
			code: `
package main
func main() {
	result :=
}`,
			wantErr: true,
			errMsg:  "syntax error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(LanguageGo, tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestValidator_ValidateJavaScript(t *testing.T) {
	sandbox := DefaultSandboxConfig()
	validator := NewValidator(sandbox)

	tests := []struct {
		name    string
		code    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid code",
			code:    `const result = 42; console.log(result);`,
			wantErr: false,
		},
		{
			name:    "forbidden eval",
			code:    `eval("alert('hello')");`,
			wantErr: true,
			errMsg:  "forbidden: eval()",
		},
		{
			name:    "forbidden Function constructor",
			code:    `const fn = new Function('return 1');`,
			wantErr: true,
			errMsg:  "forbidden: new Function()",
		},
		{
			name:    "forbidden require",
			code:    `const fs = require('fs');`,
			wantErr: true,
			errMsg:  "forbidden: require()",
		},
		{
			name:    "forbidden import",
			code:    `import fs from 'fs';`,
			wantErr: true,
			errMsg:  "forbidden: import statement",
		},
		{
			name:    "valid JSON operations",
			code:    `const obj = JSON.parse('{"key": "value"}'); const str = JSON.stringify(obj);`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(LanguageJavaScript, tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestValidator_ValidatePython(t *testing.T) {
	sandbox := DefaultSandboxConfig()
	validator := NewValidator(sandbox)

	tests := []struct {
		name    string
		code    string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid code",
			code:    `result = 42\nprint(result)`,
			wantErr: false,
		},
		{
			name:    "forbidden eval",
			code:    `eval("print('hello')")`,
			wantErr: true,
			errMsg:  "forbidden: eval()",
		},
		{
			name:    "forbidden exec",
			code:    `exec("print('hello')")`,
			wantErr: true,
			errMsg:  "forbidden: exec()",
		},
		{
			name:    "forbidden __import__",
			code:    `os = __import__('os')`,
			wantErr: true,
			errMsg:  "forbidden: __import__()",
		},
		{
			name:    "forbidden module os",
			code:    `import os\nos.system('ls')`,
			wantErr: true,
			errMsg:  "forbidden module: os",
		},
		{
			name:    "forbidden module subprocess",
			code:    `import subprocess\nsubprocess.run(['ls'])`,
			wantErr: true,
			errMsg:  "subprocess",
		},
		{
			name:    "valid json operations",
			code:    `import json\ndata = json.dumps({'key': 'value'})`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(LanguagePython, tt.code)
			if (err != nil) != tt.wantErr {
				t.Errorf("expected error: %v, got: %v", tt.wantErr, err)
			}
			if tt.wantErr && tt.errMsg != "" && err != nil {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error to contain %q, got: %v", tt.errMsg, err)
				}
			}
		})
	}
}

func TestExtractGoImports(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []string
	}{
		{
			name: "single import",
			code: `
package main
import "fmt"
`,
			expected: []string{"fmt"},
		},
		{
			name: "multiple imports",
			code: `
package main
import (
	"fmt"
	"encoding/json"
	"strings"
)
`,
			expected: []string{"fmt", "encoding/json", "strings"},
		},
		{
			name: "no imports",
			code: `
package main
func main() {}
`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imports, err := ExtractGoImports(tt.code)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(imports) != len(tt.expected) {
				t.Errorf("expected %d imports, got %d", len(tt.expected), len(imports))
			}
			for i, imp := range imports {
				if imp != tt.expected[i] {
					t.Errorf("expected import %q, got %q", tt.expected[i], imp)
				}
			}
		})
	}
}

func TestExtractPythonImports(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []string
	}{
		{
			name:     "single import",
			code:     `import json`,
			expected: []string{"json"},
		},
		{
			name: "multiple imports",
			code: `import json
import math
import datetime`,
			expected: []string{"json", "math", "datetime"},
		},
		{
			name:     "from import",
			code:     `from datetime import datetime`,
			expected: []string{"datetime"},
		},
		{
			name: "mixed imports",
			code: `import json
from math import sqrt`,
			expected: []string{"json", "math"},
		},
		{
			name:     "no imports",
			code:     `result = 42`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imports := ExtractPythonImports(tt.code)
			if len(imports) != len(tt.expected) {
				t.Errorf("expected %d imports, got %d: %v", len(tt.expected), len(imports), imports)
			}
			for i, imp := range imports {
				if imp != tt.expected[i] {
					t.Errorf("expected import %q, got %q", tt.expected[i], imp)
				}
			}
		})
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
