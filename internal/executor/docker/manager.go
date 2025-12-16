package docker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/docker/docker/client"
)

// Manager implements RuntimeManager interface
type Manager struct {
	client     *client.Client
	containers map[Language]*ContainerLifecycle
	mu         sync.RWMutex

	// Configuration
	memoryLimit int64
	cpuQuota    int64
	timeout     time.Duration
	workDir     string

	// Warm pool for fast startup
	warmPool map[Language]bool
}

// ManagerConfig holds configuration for the manager
type ManagerConfig struct {
	MemoryLimitMB int64         // Memory limit in MB (default: 256)
	CPUQuota      int64         // CPU quota (default: 100000)
	Timeout       time.Duration // Execution timeout (default: 30s)
	WorkDir       string        // Working directory in container (default: /workspace)
	WarmPool      []Language    // Languages to pre-start
}

// DefaultConfig returns default manager configuration
func DefaultConfig() *ManagerConfig {
	return &ManagerConfig{
		MemoryLimitMB: 256,
		CPUQuota:      100000,
		Timeout:       30 * time.Second,
		WorkDir:       "/workspace",
		WarmPool:      []Language{},
	}
}

// NewManager creates a new Docker runtime manager
func NewManager(cfg *ManagerConfig) (*Manager, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	m := &Manager{
		client:      cli,
		containers:  make(map[Language]*ContainerLifecycle),
		memoryLimit: cfg.MemoryLimitMB * 1024 * 1024, // Convert MB to bytes
		cpuQuota:    cfg.CPUQuota,
		timeout:     cfg.Timeout,
		workDir:     cfg.WorkDir,
		warmPool:    make(map[Language]bool),
	}

	// Pre-start warm pool containers
	for _, lang := range cfg.WarmPool {
		m.warmPool[lang] = true
		// Start in background
		go func(l Language) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			_, _ = m.Start(ctx, l)
		}(lang)
	}

	return m, nil
}

// Start starts a container for the specified language
func (m *Manager) Start(ctx context.Context, lang Language) (*RuntimeStatus, error) {
	// Check if it's a native runtime
	if IsNativeRuntime(lang) {
		return nil, fmt.Errorf("language %s uses native runtime, no Docker container needed", lang)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if container already exists
	if existing, ok := m.containers[lang]; ok {
		status, err := existing.GetStatus(ctx)
		if err == nil && status.Status == StatusReady {
			return status, nil
		}

		// Clean up stale container
		_ = existing.Stop(ctx)
		delete(m.containers, lang)
	}

	// Get image for language
	image, err := GetImage(lang)
	if err != nil {
		return nil, err
	}

	// Create container configuration
	config := ContainerConfig{
		Image:       image,
		Language:    lang,
		MemoryLimit: m.memoryLimit,
		CPUQuota:    m.cpuQuota,
		Timeout:     m.timeout,
		WorkDir:     m.workDir,
	}

	// Create container lifecycle
	lifecycle := NewContainerLifecycle(m.client, lang, config)

	// Start container in background to avoid blocking
	errChan := make(chan error, 1)
	go func() {
		errChan <- lifecycle.Start(ctx)
	}()

	// Wait for start with timeout
	select {
	case err := <-errChan:
		if err != nil {
			return nil, fmt.Errorf("failed to start container: %w", err)
		}
	case <-time.After(2 * time.Second):
		// Container is starting, return starting status
		m.containers[lang] = lifecycle
		return &RuntimeStatus{
			Language:    lang,
			Status:      StatusStarting,
			ContainerID: lifecycle.GetContainerID(),
			Message:     "Container is starting. Retry in a few seconds.",
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Container started successfully
	m.containers[lang] = lifecycle

	status, err := lifecycle.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	return status, nil
}

// Execute runs code in the specified container
func (m *Manager) Execute(ctx context.Context, containerID string, code string, input interface{}) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find container by ID
	var lifecycle *ContainerLifecycle
	for _, c := range m.containers {
		if c.GetContainerID() == containerID {
			lifecycle = c
			break
		}
	}

	if lifecycle == nil {
		return nil, fmt.Errorf("container not found: %s", containerID)
	}

	// Execute code
	output, err := lifecycle.Execute(ctx, code, input)
	if err != nil {
		return nil, err
	}

	return output, nil
}

// Stop stops and removes the specified container
func (m *Manager) Stop(ctx context.Context, containerID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find and remove container
	for lang, c := range m.containers {
		if c.GetContainerID() == containerID {
			if err := c.Stop(ctx); err != nil {
				return err
			}
			delete(m.containers, lang)
			return nil
		}
	}

	return fmt.Errorf("container not found: %s", containerID)
}

// Status returns the status of the runtime for the specified language
func (m *Manager) Status(ctx context.Context, lang Language) (*RuntimeStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if it's a native runtime
	if IsNativeRuntime(lang) {
		return &RuntimeStatus{
			Language: lang,
			Status:   StatusReady,
			Message:  "Native runtime, no Docker container needed",
		}, nil
	}

	// Check if container exists
	lifecycle, ok := m.containers[lang]
	if !ok {
		return &RuntimeStatus{
			Language: lang,
			Status:   StatusStopped,
			Message:  "Container not started",
		}, nil
	}

	return lifecycle.GetStatus(ctx)
}

// Cleanup removes all managed containers
func (m *Manager) Cleanup(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	for lang, c := range m.containers {
		if err := c.Stop(ctx); err != nil {
			errs = append(errs, fmt.Errorf("failed to stop container for %s: %w", lang, err))
		}
		delete(m.containers, lang)
	}

	if len(errs) > 0 {
		return fmt.Errorf("cleanup errors: %v", errs)
	}

	return nil
}

// ListRuntimes returns the status of all running containers
func (m *Manager) ListRuntimes(ctx context.Context) ([]RuntimeStatus, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	statuses := make([]RuntimeStatus, 0, len(m.containers))

	for _, c := range m.containers {
		status, err := c.GetStatus(ctx)
		if err != nil {
			continue
		}
		statuses = append(statuses, *status)
	}

	return statuses, nil
}

// Close closes the Docker client and cleans up all containers
func (m *Manager) Close(ctx context.Context) error {
	if err := m.Cleanup(ctx); err != nil {
		return err
	}

	if m.client != nil {
		return m.client.Close()
	}

	return nil
}

// WarmUp pre-starts containers for specified languages
func (m *Manager) WarmUp(ctx context.Context, languages []Language) error {
	var errs []error

	for _, lang := range languages {
		if IsNativeRuntime(lang) {
			continue
		}

		m.warmPool[lang] = true

		if _, err := m.Start(ctx, lang); err != nil {
			errs = append(errs, fmt.Errorf("failed to warm up %s: %w", lang, err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("warm up errors: %v", errs)
	}

	return nil
}

// IsWarmed checks if a language is in the warm pool
func (m *Manager) IsWarmed(lang Language) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.warmPool[lang]
}
