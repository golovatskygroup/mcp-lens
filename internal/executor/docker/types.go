package docker

import (
	"context"
	"time"
)

// Language represents supported programming languages
type Language string

const (
	// Native runtimes (executed without Docker)
	LangGo         Language = "go"
	LangJavaScript Language = "javascript"
	LangPython     Language = "python"

	// Docker-based runtimes
	LangRuby       Language = "ruby"
	LangRust       Language = "rust"
	LangJava       Language = "java"
	LangPHP        Language = "php"
	LangBash       Language = "bash"
	LangTypeScript Language = "typescript"
)

// NativeRuntimes contains languages that execute without Docker
var NativeRuntimes = map[Language]bool{
	LangGo:         true, // Yaegi
	LangJavaScript: true, // goja
	LangPython:     true, // go-python or exec
}

// RuntimeStatus represents the status of a language runtime
type RuntimeStatus struct {
	Language    Language  `json:"language"`
	Status      string    `json:"status"` // ready, starting, stopped
	ContainerID string    `json:"container_id,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	Message     string    `json:"message,omitempty"`
}

// ExecuteRequest represents a code execution request
type ExecuteRequest struct {
	Language Language               `json:"language"`
	Code     string                 `json:"code"`
	Input    map[string]interface{} `json:"input,omitempty"`
	Timeout  time.Duration          `json:"timeout"`
}

// ExecuteResponse represents the result of code execution
type ExecuteResponse struct {
	Status          string      `json:"status"` // success, error, container_starting
	Result          interface{} `json:"result,omitempty"`
	Error           string      `json:"error,omitempty"`
	Language        Language    `json:"language"`
	ExecutionTimeMs int64       `json:"execution_time_ms"`
	ContainerID     string      `json:"container_id,omitempty"`
	RetryAfterSec   int         `json:"retry_after_seconds,omitempty"`
}

// RuntimeManager manages Docker containers for code execution
type RuntimeManager interface {
	// Start starts a container for the specified language
	Start(ctx context.Context, lang Language) (*RuntimeStatus, error)

	// Execute runs code in the specified container
	Execute(ctx context.Context, containerID string, code string, input interface{}) (interface{}, error)

	// Stop stops and removes the specified container
	Stop(ctx context.Context, containerID string) error

	// Status returns the status of the runtime for the specified language
	Status(ctx context.Context, lang Language) (*RuntimeStatus, error)

	// Cleanup removes all managed containers
	Cleanup(ctx context.Context) error
}

// ContainerConfig holds configuration for a container
type ContainerConfig struct {
	Image       string
	Language    Language
	MemoryLimit int64  // in bytes
	CPUQuota    int64  // CPU quota
	Timeout     time.Duration
	WorkDir     string
}

// Status constants
const (
	StatusReady    = "ready"
	StatusStarting = "starting"
	StatusStopped  = "stopped"
)

// Default retry delay when container is starting
const DefaultRetryAfterSeconds = 5
