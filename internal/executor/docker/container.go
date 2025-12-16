package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// ContainerLifecycle manages the lifecycle of a single container
type ContainerLifecycle struct {
	client      *client.Client
	containerID string
	language    Language
	config      ContainerConfig
	startedAt   time.Time
}

// NewContainerLifecycle creates a new container lifecycle manager
func NewContainerLifecycle(cli *client.Client, lang Language, cfg ContainerConfig) *ContainerLifecycle {
	return &ContainerLifecycle{
		client:   cli,
		language: lang,
		config:   cfg,
	}
}

// Start creates and starts a container
func (c *ContainerLifecycle) Start(ctx context.Context) error {
	// Pull image if not available
	if err := c.ensureImage(ctx); err != nil {
		return fmt.Errorf("failed to ensure image: %w", err)
	}

	// Create container configuration
	containerConfig := &container.Config{
		Image:        c.config.Image,
		Tty:          false,
		AttachStdout: true,
		AttachStderr: true,
		WorkingDir:   c.config.WorkDir,
		Cmd:          []string{"sh", "-c", "while true; do sleep 1; done"}, // Keep container running
	}

	hostConfig := &container.HostConfig{
		Resources: container.Resources{
			Memory:   c.config.MemoryLimit,
			CPUQuota: c.config.CPUQuota,
		},
		AutoRemove: false, // We'll manage removal manually
	}

	// Create container
	resp, err := c.client.ContainerCreate(ctx, containerConfig, hostConfig, nil, nil, "")
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	c.containerID = resp.ID

	// Start container
	if err := c.client.ContainerStart(ctx, c.containerID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	c.startedAt = time.Now()

	return nil
}

// Execute runs code in the container
func (c *ContainerLifecycle) Execute(ctx context.Context, code string, input interface{}) (string, error) {
	if c.containerID == "" {
		return "", fmt.Errorf("container not started")
	}

	// Check if container is still running
	inspect, err := c.client.ContainerInspect(ctx, c.containerID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect container: %w", err)
	}

	if !inspect.State.Running {
		return "", fmt.Errorf("container is not running")
	}

	// Prepare execution command
	cmd, err := c.prepareCommand(code, input)
	if err != nil {
		return "", fmt.Errorf("failed to prepare command: %w", err)
	}

	// Create exec instance
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execID, err := c.client.ContainerExecCreate(ctx, c.containerID, execConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create exec: %w", err)
	}

	// Attach to exec
	resp, err := c.client.ContainerExecAttach(ctx, execID.ID, container.ExecStartOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to attach to exec: %w", err)
	}
	defer resp.Close()

	// Read output
	output, err := c.readOutput(resp.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to read output: %w", err)
	}

	// Check exit code
	inspectResp, err := c.client.ContainerExecInspect(ctx, execID.ID)
	if err != nil {
		return "", fmt.Errorf("failed to inspect exec: %w", err)
	}

	if inspectResp.ExitCode != 0 {
		return "", fmt.Errorf("execution failed with exit code %d: %s", inspectResp.ExitCode, output)
	}

	return output, nil
}

// Stop stops and removes the container
func (c *ContainerLifecycle) Stop(ctx context.Context) error {
	if c.containerID == "" {
		return nil
	}

	timeout := 10
	stopOptions := container.StopOptions{
		Timeout: &timeout,
	}

	if err := c.client.ContainerStop(ctx, c.containerID, stopOptions); err != nil {
		// Ignore error if container already stopped
		if !strings.Contains(err.Error(), "is already stopped") {
			return fmt.Errorf("failed to stop container: %w", err)
		}
	}

	removeOptions := container.RemoveOptions{
		Force: true,
	}

	if err := c.client.ContainerRemove(ctx, c.containerID, removeOptions); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}

	c.containerID = ""
	return nil
}

// GetStatus returns the current status of the container
func (c *ContainerLifecycle) GetStatus(ctx context.Context) (*RuntimeStatus, error) {
	status := &RuntimeStatus{
		Language:    c.language,
		ContainerID: c.containerID,
		StartedAt:   c.startedAt,
	}

	if c.containerID == "" {
		status.Status = StatusStopped
		return status, nil
	}

	inspect, err := c.client.ContainerInspect(ctx, c.containerID)
	if err != nil {
		status.Status = StatusStopped
		return status, nil
	}

	if inspect.State.Running {
		status.Status = StatusReady
		status.Message = "Container is ready for code execution"
	} else if inspect.State.StartedAt != "" {
		status.Status = StatusStarting
		status.Message = "Container is starting"
	} else {
		status.Status = StatusStopped
		status.Message = "Container is stopped"
	}

	return status, nil
}

// ensureImage pulls the image if not available
func (c *ContainerLifecycle) ensureImage(ctx context.Context) error {
	// Check if image exists
	_, _, err := c.client.ImageInspectWithRaw(ctx, c.config.Image)
	if err == nil {
		// Image exists
		return nil
	}

	// Pull image
	reader, err := c.client.ImagePull(ctx, c.config.Image, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	defer reader.Close()

	// Wait for pull to complete
	_, err = io.Copy(io.Discard, reader)
	if err != nil {
		return fmt.Errorf("failed to wait for image pull: %w", err)
	}

	return nil
}

// prepareCommand prepares the execution command based on language
func (c *ContainerLifecycle) prepareCommand(code string, input interface{}) ([]string, error) {
	baseCmd, err := GetCommand(c.language)
	if err != nil {
		return nil, err
	}

	// For compiled languages, we need to compile first
	if NeedsCompilation(c.language) {
		return c.prepareCompiledCommand(code, input, baseCmd)
	}

	// For interpreted languages, execute directly
	return c.prepareInterpretedCommand(code, input, baseCmd)
}

// prepareInterpretedCommand prepares command for interpreted languages
func (c *ContainerLifecycle) prepareInterpretedCommand(code string, input interface{}, baseCmd []string) ([]string, error) {
	switch c.language {
	case LangRuby, LangPHP:
		// Execute code directly
		cmd := append(baseCmd, "-e", code)
		return cmd, nil

	case LangBash:
		// Execute code as shell script
		cmd := append(baseCmd, "-c", code)
		return cmd, nil

	case LangTypeScript:
		// For TypeScript, we need to write to a file
		// This is simplified - in production, you'd write to /tmp
		cmd := []string{"sh", "-c", fmt.Sprintf("echo %q | npx ts-node", code)}
		return cmd, nil

	default:
		return nil, fmt.Errorf("unsupported language: %s", c.language)
	}
}

// prepareCompiledCommand prepares command for compiled languages
func (c *ContainerLifecycle) prepareCompiledCommand(code string, input interface{}, baseCmd []string) ([]string, error) {
	switch c.language {
	case LangRust:
		// Compile and run
		cmd := []string{"sh", "-c", fmt.Sprintf(
			"echo %q > /tmp/main.rs && rustc /tmp/main.rs -o /tmp/program && /tmp/program",
			code,
		)}
		return cmd, nil

	case LangJava:
		// Extract class name from code (simplified)
		className := "Main"
		cmd := []string{"sh", "-c", fmt.Sprintf(
			"echo %q > /tmp/%s.java && cd /tmp && javac %s.java && java %s",
			code, className, className, className,
		)}
		return cmd, nil

	default:
		return nil, fmt.Errorf("unsupported compiled language: %s", c.language)
	}
}

// readOutput reads output from container exec
func (c *ContainerLifecycle) readOutput(reader io.Reader) (string, error) {
	var stdout, stderr strings.Builder

	_, err := stdcopy.StdCopy(&stdout, &stderr, reader)
	if err != nil {
		return "", err
	}

	// Combine stdout and stderr
	output := stdout.String()
	if stderr.Len() > 0 {
		output += "\nSTDERR:\n" + stderr.String()
	}

	return output, nil
}

// GetContainerID returns the container ID
func (c *ContainerLifecycle) GetContainerID() string {
	return c.containerID
}
