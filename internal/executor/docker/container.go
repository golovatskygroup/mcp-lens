package docker

import (
	"context"
	"encoding/json"
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

	// Prepare execution command and environment (Iteration 3: Task 0009)
	cmd, env, err := c.prepareCommand(code, input)
	if err != nil {
		return "", fmt.Errorf("failed to prepare command: %w", err)
	}

	// Create exec instance
	execConfig := container.ExecOptions{
		Cmd:          cmd,
		Env:          env,
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

// prepareCommand prepares the execution command and environment based on language (Iteration 3: Task 0009)
func (c *ContainerLifecycle) prepareCommand(code string, input interface{}) ([]string, []string, error) {
	const maxEnvSize = 32 * 1024 // 32KB limit for environment variables

	baseCmd, err := GetCommand(c.language)
	if err != nil {
		return nil, nil, err
	}

	// Serialize input to JSON for MCP_INPUT_JSON env var
	env := []string{}
	var inputJSON []byte
	if input != nil {
		inputJSON, err = json.Marshal(input)
		if err == nil {
			if len(inputJSON) > maxEnvSize {
				// Input is too large for env var, will write to file instead
				env = append(env, "MCP_INPUT_FILE=/tmp/mcp_input.json")
				env = append(env, "MCP_INPUT_JSON=") // Empty to indicate file should be used
			} else {
				env = append(env, fmt.Sprintf("MCP_INPUT_JSON=%s", string(inputJSON)))
			}
		}
	}

	// For compiled languages, we need to compile first
	if NeedsCompilation(c.language) {
		cmd, err := c.prepareCompiledCommand(code, input, baseCmd, inputJSON, maxEnvSize)
		return cmd, env, err
	}

	// For interpreted languages, execute directly
	cmd, err := c.prepareInterpretedCommand(code, input, baseCmd, inputJSON, maxEnvSize)
	return cmd, env, err
}

// prepareInterpretedCommand prepares command for interpreted languages (Iteration 4: Task 0009)
func (c *ContainerLifecycle) prepareInterpretedCommand(code string, input interface{}, baseCmd []string, inputJSON []byte, maxEnvSize int) ([]string, error) {
	// Prepare input file creation command if needed
	inputFileCmd := ""
	if inputJSON != nil && len(inputJSON) > maxEnvSize {
		// Escape JSON for safe inclusion in shell command
		escapedJSON := strings.ReplaceAll(string(inputJSON), "'", "'\"'\"'")
		inputFileCmd = fmt.Sprintf("printf '%%s' '%s' > /tmp/mcp_input.json && ", escapedJSON)
	}

	switch c.language {
	case LangBash:
		// Write to file to support multi-line scripts with proper newlines
		escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
		cmd := []string{"sh", "-c", fmt.Sprintf("%sprintf '%%s' '%s' > /tmp/script.sh && sh /tmp/script.sh", inputFileCmd, escapedCode)}
		return cmd, nil

	case LangRuby:
		// Write to file to support multi-line code
		escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
		cmd := []string{"sh", "-c", fmt.Sprintf("%sprintf '%%s' '%s' > /tmp/script.rb && ruby /tmp/script.rb", inputFileCmd, escapedCode)}
		return cmd, nil

	case LangPHP:
		// Write to file to support multi-line code
		escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
		cmd := []string{"sh", "-c", fmt.Sprintf("%sprintf '%%s' '%s' > /tmp/script.php && php /tmp/script.php", inputFileCmd, escapedCode)}
		return cmd, nil

	case LangTypeScript:
		// Write code to a file to support multi-line code
		// Use .mts extension and --esm flag for ES Modules support
		escapedCode := strings.ReplaceAll(code, "'", "'\"'\"'")
		cmd := []string{"sh", "-c", fmt.Sprintf("%sprintf '%%s' '%s' > /tmp/main.mts && npx ts-node --esm /tmp/main.mts", inputFileCmd, escapedCode)}
		return cmd, nil

	default:
		return nil, fmt.Errorf("unsupported language: %s", c.language)
	}
}

// prepareCompiledCommand prepares command for compiled languages
func (c *ContainerLifecycle) prepareCompiledCommand(code string, input interface{}, baseCmd []string, inputJSON []byte, maxEnvSize int) ([]string, error) {
	// Prepare input file creation command if needed
	inputFileCmd := ""
	if inputJSON != nil && len(inputJSON) > maxEnvSize {
		// Escape JSON for safe inclusion in shell command
		escapedJSON := strings.ReplaceAll(string(inputJSON), "'", "'\"'\"'")
		inputFileCmd = fmt.Sprintf("printf '%%s' '%s' > /tmp/mcp_input.json && ", escapedJSON)
	}

	switch c.language {
	case LangRust:
		// Compile and run
		cmd := []string{"sh", "-c", fmt.Sprintf(
			"%secho %q > /tmp/main.rs && rustc /tmp/main.rs -o /tmp/program && /tmp/program",
			inputFileCmd, code,
		)}
		return cmd, nil

	case LangJava:
		// Extract class name from code (simplified)
		className := "Main"
		cmd := []string{"sh", "-c", fmt.Sprintf(
			"%secho %q > /tmp/%s.java && cd /tmp && javac %s.java && java %s",
			inputFileCmd, code, className, className, className,
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
