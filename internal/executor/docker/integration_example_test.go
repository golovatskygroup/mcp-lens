// +build integration

// Example of integration test using real Docker
// Run with: go test -v -tags=integration -timeout=5m
//
// This test demonstrates how to test the Docker Runtime Manager
// with real Docker containers. It requires Docker to be running.

package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestDockerRuntimeManager_FullWorkflow demonstrates a complete workflow
// of starting a container, executing code, and cleaning up.
func TestDockerRuntimeManager_FullWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create manager with custom config
	cfg := &ManagerConfig{
		MemoryLimitMB: 256,
		CPUQuota:      100000,
		Timeout:       30 * time.Second,
		WorkDir:       "/workspace",
		WarmPool:      []Language{},
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer func() {
		if err := manager.Close(context.Background()); err != nil {
			t.Errorf("Failed to close manager: %v", err)
		}
	}()

	ctx := context.Background()

	// Test Ruby execution
	t.Run("Ruby Hello World", func(t *testing.T) {
		status, err := manager.Start(ctx, LangRuby)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Wait if starting
		if status.Status == StatusStarting {
			t.Logf("Container starting, waiting...")
			time.Sleep(5 * time.Second)
			status, err = manager.Status(ctx, LangRuby)
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
		}

		if status.Status != StatusReady {
			t.Fatalf("Container not ready: %s", status.Status)
		}

		code := `puts "Hello from Ruby!"`
		result, err := manager.Execute(ctx, status.ContainerID, code, nil)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Result is not string: %T", result)
		}

		if !strings.Contains(resultStr, "Hello from Ruby") {
			t.Errorf("Result does not contain expected string: %s", resultStr)
		}

		t.Logf("Ruby output: %s", resultStr)
	})

	// Test PHP execution
	t.Run("PHP Calculation", func(t *testing.T) {
		status, err := manager.Start(ctx, LangPHP)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Wait if starting
		if status.Status == StatusStarting {
			time.Sleep(5 * time.Second)
		}

		code := `echo 2 + 2;`
		result, err := manager.Execute(ctx, status.ContainerID, code, nil)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Result is not string: %T", result)
		}

		if !strings.Contains(resultStr, "4") {
			t.Errorf("Result does not contain expected value: %s", resultStr)
		}

		t.Logf("PHP output: %s", resultStr)
	})

	// Test Bash execution
	t.Run("Bash Script", func(t *testing.T) {
		status, err := manager.Start(ctx, LangBash)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// Wait if starting
		if status.Status == StatusStarting {
			time.Sleep(5 * time.Second)
		}

		code := `echo "Current time: $(date)"`
		result, err := manager.Execute(ctx, status.ContainerID, code, nil)
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		resultStr, ok := result.(string)
		if !ok {
			t.Fatalf("Result is not string: %T", result)
		}

		if !strings.Contains(resultStr, "Current time") {
			t.Errorf("Result does not contain expected string: %s", resultStr)
		}

		t.Logf("Bash output: %s", resultStr)
	})

	// Test status checking
	t.Run("Status Check", func(t *testing.T) {
		status, err := manager.Status(ctx, LangRuby)
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}

		if status.Status != StatusReady {
			t.Errorf("Status = %v, want %v", status.Status, StatusReady)
		}

		if status.ContainerID == "" {
			t.Error("Container ID is empty")
		}

		t.Logf("Ruby container status: %s (ID: %s)", status.Status, status.ContainerID)
	})

	// Test listing runtimes
	t.Run("List Runtimes", func(t *testing.T) {
		runtimes, err := manager.ListRuntimes(ctx)
		if err != nil {
			t.Fatalf("ListRuntimes() error = %v", err)
		}

		if len(runtimes) < 3 {
			t.Errorf("Expected at least 3 runtimes, got %d", len(runtimes))
		}

		for _, runtime := range runtimes {
			t.Logf("Runtime: %s - %s (ID: %s)", runtime.Language, runtime.Status, runtime.ContainerID)
		}
	})

	// Test cleanup
	t.Run("Cleanup", func(t *testing.T) {
		err := manager.Cleanup(ctx)
		if err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}

		// Verify all containers are stopped
		status, err := manager.Status(ctx, LangRuby)
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}

		if status.Status != StatusStopped {
			t.Errorf("Status after cleanup = %v, want %v", status.Status, StatusStopped)
		}

		t.Log("All containers cleaned up successfully")
	})
}

// TestDockerRuntimeManager_WarmPool tests the warm pool functionality
func TestDockerRuntimeManager_WarmPool(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create manager with warm pool
	cfg := &ManagerConfig{
		MemoryLimitMB: 256,
		CPUQuota:      100000,
		Timeout:       30 * time.Second,
		WorkDir:       "/workspace",
		WarmPool:      []Language{LangRuby, LangPHP},
	}

	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close(context.Background())

	// Wait for warm pool to initialize
	time.Sleep(3 * time.Second)

	ctx := context.Background()

	// Check if languages are warmed
	t.Run("Check Warm Pool", func(t *testing.T) {
		if !manager.IsWarmed(LangRuby) {
			t.Error("Ruby should be warmed")
		}

		if !manager.IsWarmed(LangPHP) {
			t.Error("PHP should be warmed")
		}

		if manager.IsWarmed(LangBash) {
			t.Error("Bash should not be warmed")
		}
	})

	// Verify containers are ready
	t.Run("Verify Warm Containers", func(t *testing.T) {
		status, err := manager.Status(ctx, LangRuby)
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}

		if status.Status != StatusReady && status.Status != StatusStarting {
			t.Errorf("Warm container status = %v, want %v or %v", status.Status, StatusReady, StatusStarting)
		}

		t.Logf("Warm Ruby container: %s", status.Status)
	})

	// Execute code on warmed container (should be fast)
	t.Run("Execute on Warm Container", func(t *testing.T) {
		status, err := manager.Status(ctx, LangRuby)
		if err != nil {
			t.Fatalf("Status() error = %v", err)
		}

		if status.Status == StatusStarting {
			time.Sleep(5 * time.Second)
			status, err = manager.Status(ctx, LangRuby)
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}
		}

		start := time.Now()
		code := `puts "Fast execution!"`
		result, err := manager.Execute(ctx, status.ContainerID, code, nil)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}

		t.Logf("Execution time: %v", elapsed)
		t.Logf("Result: %v", result)

		if elapsed > 5*time.Second {
			t.Logf("Warning: Execution took longer than expected: %v", elapsed)
		}
	})
}

// TestDockerRuntimeManager_ErrorHandling tests error scenarios
func TestDockerRuntimeManager_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultConfig()
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close(context.Background())

	ctx := context.Background()

	// Test native runtime error
	t.Run("Native Runtime Error", func(t *testing.T) {
		_, err := manager.Start(ctx, LangGo)
		if err == nil {
			t.Error("Start() should fail for native runtime")
		}

		if !strings.Contains(err.Error(), "native runtime") {
			t.Errorf("Error message should mention native runtime: %v", err)
		}
	})

	// Test invalid container ID
	t.Run("Invalid Container ID", func(t *testing.T) {
		_, err := manager.Execute(ctx, "invalid-id", "puts 'test'", nil)
		if err == nil {
			t.Error("Execute() should fail for invalid container ID")
		}
	})

	// Test syntax error in code
	t.Run("Syntax Error", func(t *testing.T) {
		status, err := manager.Start(ctx, LangRuby)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		if status.Status == StatusStarting {
			time.Sleep(5 * time.Second)
			status, _ = manager.Status(ctx, LangRuby)
		}

		code := `puts "missing quote`
		_, err = manager.Execute(ctx, status.ContainerID, code, nil)
		if err == nil {
			t.Error("Execute() should fail for syntax error")
		}

		t.Logf("Expected error: %v", err)
	})
}
