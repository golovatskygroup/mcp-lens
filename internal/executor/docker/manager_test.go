// +build integration

package docker

import (
	"context"
	"testing"
	"time"
)

func TestManager_Start(t *testing.T) {
	// Skip if Docker is not available
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultConfig()
	cfg.Timeout = 60 * time.Second
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close(context.Background())

	tests := []struct {
		name     string
		language Language
		wantErr  bool
	}{
		{
			name:     "Start Ruby container",
			language: LangRuby,
			wantErr:  false,
		},
		{
			name:     "Start PHP container",
			language: LangPHP,
			wantErr:  false,
		},
		{
			name:     "Start Bash container",
			language: LangBash,
			wantErr:  false,
		},
		{
			name:     "Native runtime should fail",
			language: LangGo,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			status, err := manager.Start(ctx, tt.language)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if status == nil {
					t.Error("Start() returned nil status")
					return
				}

				if status.Language != tt.language {
					t.Errorf("Start() language = %v, want %v", status.Language, tt.language)
				}

				if status.Status != StatusReady && status.Status != StatusStarting {
					t.Errorf("Start() status = %v, want %v or %v", status.Status, StatusReady, StatusStarting)
				}

				// If starting, wait for ready
				if status.Status == StatusStarting {
					time.Sleep(5 * time.Second)
					status, err = manager.Status(ctx, tt.language)
					if err != nil {
						t.Errorf("Status() error = %v", err)
						return
					}
					if status.Status != StatusReady {
						t.Errorf("Status() after wait = %v, want %v", status.Status, StatusReady)
					}
				}
			}
		})
	}
}

func TestManager_Execute(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultConfig()
	cfg.Timeout = 60 * time.Second
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}
	defer manager.Close(context.Background())

	tests := []struct {
		name     string
		language Language
		code     string
		wantErr  bool
	}{
		{
			name:     "Execute Ruby code",
			language: LangRuby,
			code:     "puts 'Hello, World!'",
			wantErr:  false,
		},
		{
			name:     "Execute PHP code",
			language: LangPHP,
			code:     "echo 'Hello, World!';",
			wantErr:  false,
		},
		{
			name:     "Execute Bash code",
			language: LangBash,
			code:     "echo 'Hello, World!'",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			// Start container
			status, err := manager.Start(ctx, tt.language)
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}

			// Wait if starting
			if status.Status == StatusStarting {
				time.Sleep(5 * time.Second)
			}

			// Get container ID
			status, err = manager.Status(ctx, tt.language)
			if err != nil {
				t.Fatalf("Status() error = %v", err)
			}

			if status.ContainerID == "" {
				t.Fatal("Container ID is empty")
			}

			// Execute code
			result, err := manager.Execute(ctx, status.ContainerID, tt.code, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if result == nil {
					t.Error("Execute() returned nil result")
					return
				}

				t.Logf("Result: %v", result)
			}
		})
	}
}

func TestManager_Status(t *testing.T) {
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

	// Check status before starting
	status, err := manager.Status(ctx, LangRuby)
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}

	if status.Status != StatusStopped {
		t.Errorf("Status() = %v, want %v", status.Status, StatusStopped)
	}

	// Start container
	_, err = manager.Start(ctx, LangRuby)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Check status after starting
	status, err = manager.Status(ctx, LangRuby)
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}

	if status.Status != StatusReady && status.Status != StatusStarting {
		t.Errorf("Status() = %v, want %v or %v", status.Status, StatusReady, StatusStarting)
	}
}

func TestManager_Stop(t *testing.T) {
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

	// Start container
	status, err := manager.Start(ctx, LangRuby)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if status.Status == StatusStarting {
		time.Sleep(5 * time.Second)
	}

	// Get container ID
	status, err = manager.Status(ctx, LangRuby)
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}

	containerID := status.ContainerID

	// Stop container
	err = manager.Stop(ctx, containerID)
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Check status after stopping
	status, err = manager.Status(ctx, LangRuby)
	if err != nil {
		t.Errorf("Status() error = %v", err)
	}

	if status.Status != StatusStopped {
		t.Errorf("Status() = %v, want %v", status.Status, StatusStopped)
	}
}

func TestManager_Cleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	cfg := DefaultConfig()
	manager, err := NewManager(cfg)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Start multiple containers
	languages := []Language{LangRuby, LangPHP, LangBash}
	for _, lang := range languages {
		_, err := manager.Start(ctx, lang)
		if err != nil {
			t.Errorf("Start() error = %v", err)
		}
	}

	// Cleanup
	err = manager.Cleanup(ctx)
	if err != nil {
		t.Errorf("Cleanup() error = %v", err)
	}

	// Check all containers are stopped
	for _, lang := range languages {
		status, err := manager.Status(ctx, lang)
		if err != nil {
			t.Errorf("Status() error = %v", err)
		}

		if status.Status != StatusStopped {
			t.Errorf("Status() = %v, want %v", status.Status, StatusStopped)
		}
	}
}

func TestManager_WarmUp(t *testing.T) {
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

	// Warm up containers
	languages := []Language{LangRuby, LangPHP}
	err = manager.WarmUp(ctx, languages)
	if err != nil {
		t.Errorf("WarmUp() error = %v", err)
	}

	// Check all containers are warmed
	for _, lang := range languages {
		if !manager.IsWarmed(lang) {
			t.Errorf("IsWarmed() = false, want true for %s", lang)
		}
	}
}
