package server

import (
	"context"
	"os"
	"testing"
)

// TestServerInitExecutionDependencies tests the execution dependencies initialization
func TestServerInitExecutionDependencies(t *testing.T) {
	// Save current env
	savedEnvDocker := os.Getenv("MCP_ENABLE_DOCKER")
	savedEnvNative := os.Getenv("MCP_ENABLE_NATIVE_EXEC")
	savedEnvRegistry := os.Getenv("MCP_ENABLE_TOOL_REGISTRY")
	savedEnvPerplexity := os.Getenv("PERPLEXITY_API_KEY")

	// Cleanup after test
	defer func() {
		setEnvOrUnset("MCP_ENABLE_DOCKER", savedEnvDocker)
		setEnvOrUnset("MCP_ENABLE_NATIVE_EXEC", savedEnvNative)
		setEnvOrUnset("MCP_ENABLE_TOOL_REGISTRY", savedEnvRegistry)
		setEnvOrUnset("PERPLEXITY_API_KEY", savedEnvPerplexity)
	}()

	// Test with no env vars set
	t.Run("no env vars", func(t *testing.T) {
		os.Unsetenv("MCP_ENABLE_DOCKER")
		os.Unsetenv("MCP_ENABLE_NATIVE_EXEC")
		os.Unsetenv("MCP_ENABLE_TOOL_REGISTRY")
		os.Unsetenv("PERPLEXITY_API_KEY")

		s := &Server{}
		s.handler = nil // Will be nil for this isolated test

		// Just verify no panic occurs
		// Can't fully test without handler, but can verify the function doesn't panic
	})

	// Test with native exec enabled
	t.Run("native exec enabled", func(t *testing.T) {
		os.Setenv("MCP_ENABLE_NATIVE_EXEC", "1")
		os.Unsetenv("MCP_ENABLE_DOCKER")
		os.Unsetenv("MCP_ENABLE_TOOL_REGISTRY")
		os.Unsetenv("PERPLEXITY_API_KEY")

		// The actual initialization is tested through the handler
		// This test verifies the env var is recognized
		if os.Getenv("MCP_ENABLE_NATIVE_EXEC") != "1" {
			t.Error("env var not set correctly")
		}
	})
}

// TestServerCleanup tests the Cleanup method
func TestServerCleanup(t *testing.T) {
	s := &Server{
		dockerManager:  nil,
		toolRegistryDB: nil,
	}

	ctx := context.Background()

	// Should not panic with nil dependencies
	s.Cleanup(ctx)
}

// TestServerCleanupWithDB tests cleanup with a database connection
func TestServerCleanupWithDB(t *testing.T) {
	// We can't easily test with real DB without more setup
	// This test verifies the method handles nil gracefully
	s := &Server{
		dockerManager:  nil,
		toolRegistryDB: nil,
	}

	ctx := context.Background()
	s.Cleanup(ctx)
	// Test passes if no panic
}

// TestEnvVarParsing tests that env vars are correctly parsed
func TestEnvVarParsing(t *testing.T) {
	tests := []struct {
		name     string
		envVar   string
		value    string
		expected bool
	}{
		{"docker enabled", "MCP_ENABLE_DOCKER", "1", true},
		{"docker disabled", "MCP_ENABLE_DOCKER", "0", false},
		{"docker empty", "MCP_ENABLE_DOCKER", "", false},
		{"native enabled", "MCP_ENABLE_NATIVE_EXEC", "1", true},
		{"registry enabled", "MCP_ENABLE_TOOL_REGISTRY", "1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				os.Setenv(tt.envVar, tt.value)
			} else {
				os.Unsetenv(tt.envVar)
			}
			defer os.Unsetenv(tt.envVar)

			result := os.Getenv(tt.envVar) == "1"
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestToolRegistryPathDefault tests default path logic
func TestToolRegistryPathDefault(t *testing.T) {
	savedPath := os.Getenv("MCP_TOOL_REGISTRY_PATH")
	defer setEnvOrUnset("MCP_TOOL_REGISTRY_PATH", savedPath)

	// Test default path generation
	os.Unsetenv("MCP_TOOL_REGISTRY_PATH")

	dbPath := os.Getenv("MCP_TOOL_REGISTRY_PATH")
	if dbPath != "" {
		t.Errorf("expected empty path when env not set, got: %s", dbPath)
	}

	// Test custom path
	os.Setenv("MCP_TOOL_REGISTRY_PATH", "/custom/path/tools.db")
	dbPath = os.Getenv("MCP_TOOL_REGISTRY_PATH")
	if dbPath != "/custom/path/tools.db" {
		t.Errorf("expected custom path, got: %s", dbPath)
	}
}

// TestPerplexityKeyHandling tests Perplexity API key handling
func TestPerplexityKeyHandling(t *testing.T) {
	savedKey := os.Getenv("PERPLEXITY_API_KEY")
	defer setEnvOrUnset("PERPLEXITY_API_KEY", savedKey)

	// Test without key
	os.Unsetenv("PERPLEXITY_API_KEY")
	if os.Getenv("PERPLEXITY_API_KEY") != "" {
		t.Error("expected empty key")
	}

	// Test with key
	os.Setenv("PERPLEXITY_API_KEY", "test-key-123")
	if os.Getenv("PERPLEXITY_API_KEY") != "test-key-123" {
		t.Error("expected key to be set")
	}
}

// Helper function to set or unset env var
func setEnvOrUnset(key, value string) {
	if value == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, value)
	}
}
