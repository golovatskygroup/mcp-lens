# Docker Runtime Manager

Docker-based runtime manager for executing code in multiple programming languages.

## Features

- **Multi-language support**: Ruby, Rust, Java, PHP, Bash, TypeScript
- **Container lifecycle management**: Start, execute, stop, cleanup
- **Warm pool**: Pre-started containers for fast execution
- **Resource limits**: Memory and CPU quota management
- **Timeout handling**: Configurable execution timeouts
- **Status tracking**: Ready, starting, stopped states
- **Retry logic**: Automatic retry suggestions for starting containers

## Supported Languages

### Docker-based Runtimes

| Language   | Image                      | Status |
|------------|----------------------------|--------|
| Ruby       | ruby:3.3-alpine            | ✅     |
| Rust       | rust:1.75-alpine           | ✅     |
| Java       | eclipse-temurin:21-alpine  | ✅     |
| PHP        | php:8.3-cli-alpine         | ✅     |
| Bash       | alpine:3.19                | ✅     |
| TypeScript | node:20-alpine             | ✅     |

### Native Runtimes (no Docker)

| Language   | Runtime    | Status |
|------------|------------|--------|
| Go         | Yaegi      | 🚧     |
| JavaScript | goja       | 🚧     |
| Python     | go-python  | 🚧     |

## Architecture

```
┌─────────────────────────────────────────────┐
│            Manager                          │
│  - Docker Client                            │
│  - Container Pool (map[Language]Lifecycle)  │
│  - Warm Pool                                │
│  - Resource Limits                          │
└──────────────┬──────────────────────────────┘
               │
               ├──► ContainerLifecycle (Ruby)
               ├──► ContainerLifecycle (Rust)
               ├──► ContainerLifecycle (Java)
               └──► ContainerLifecycle (...)
                     │
                     ├─ Start()   → Pull image, create, start
                     ├─ Execute() → docker exec with code
                     ├─ Stop()    → Stop and remove
                     └─ Status()  → Ready/Starting/Stopped
```

## Usage

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "github.com/golovatskygroup/mcp-lens/internal/executor/docker"
)

func main() {
    // Create manager with default config
    cfg := docker.DefaultConfig()
    manager, err := docker.NewManager(cfg)
    if err != nil {
        panic(err)
    }
    defer manager.Close(context.Background())

    // Start Ruby runtime
    ctx := context.Background()
    status, err := manager.Start(ctx, docker.LangRuby)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Ruby runtime: %s\n", status.Status)

    // Execute code
    code := "puts 'Hello, World!'"
    result, err := manager.Execute(ctx, status.ContainerID, code, nil)
    if err != nil {
        panic(err)
    }

    fmt.Printf("Result: %v\n", result)
}
```

### Custom Configuration

```go
cfg := &docker.ManagerConfig{
    MemoryLimitMB: 512,        // 512 MB memory limit
    CPUQuota:      50000,      // 50% CPU quota
    Timeout:       60 * time.Second,
    WorkDir:       "/workspace",
    WarmPool:      []docker.Language{
        docker.LangRuby,
        docker.LangPHP,
    },
}

manager, err := docker.NewManager(cfg)
```

### Warm Pool

Pre-start containers for faster execution:

```go
// Warm up specific languages
languages := []docker.Language{
    docker.LangRuby,
    docker.LangRust,
    docker.LangPHP,
}

err := manager.WarmUp(ctx, languages)
if err != nil {
    panic(err)
}

// Check if warmed
if manager.IsWarmed(docker.LangRuby) {
    fmt.Println("Ruby is warmed up!")
}
```

### Container Starting State

When a container is starting, the API returns a special status:

```go
status, err := manager.Start(ctx, docker.LangRust)
if err != nil {
    panic(err)
}

if status.Status == docker.StatusStarting {
    fmt.Printf("Container is starting. Retry after %d seconds.\n",
        docker.DefaultRetryAfterSeconds)

    // Wait and retry
    time.Sleep(time.Duration(docker.DefaultRetryAfterSeconds) * time.Second)
    status, err = manager.Status(ctx, docker.LangRust)
}
```

### Status Checking

```go
// Check status for a language
status, err := manager.Status(ctx, docker.LangRuby)
if err != nil {
    panic(err)
}

fmt.Printf("Language: %s\n", status.Language)
fmt.Printf("Status: %s\n", status.Status)
fmt.Printf("Container ID: %s\n", status.ContainerID)
fmt.Printf("Started At: %s\n", status.StartedAt)

// List all running runtimes
runtimes, err := manager.ListRuntimes(ctx)
if err != nil {
    panic(err)
}

for _, runtime := range runtimes {
    fmt.Printf("%s: %s\n", runtime.Language, runtime.Status)
}
```

### Cleanup

```go
// Stop specific container
err := manager.Stop(ctx, containerID)

// Cleanup all containers
err := manager.Cleanup(ctx)

// Close manager and cleanup
err := manager.Close(ctx)
```

## Testing

### Unit Tests

```bash
cd internal/executor/docker
go test -v
```

### Integration Tests

Integration tests require Docker to be running:

```bash
# Run all tests including integration
go test -v -tags=integration

# Skip integration tests
go test -v -short
```

### Example Integration Test

```go
// +build integration

package docker

import (
    "context"
    "testing"
)

func TestManager_Integration(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    manager, err := NewManager(DefaultConfig())
    if err != nil {
        t.Fatal(err)
    }
    defer manager.Close(context.Background())

    // Test Ruby execution
    ctx := context.Background()
    status, err := manager.Start(ctx, LangRuby)
    if err != nil {
        t.Fatal(err)
    }

    code := "puts 2 + 2"
    result, err := manager.Execute(ctx, status.ContainerID, code, nil)
    if err != nil {
        t.Fatal(err)
    }

    t.Logf("Result: %v", result)
}
```

## Error Handling

```go
status, err := manager.Start(ctx, lang)
if err != nil {
    switch {
    case strings.Contains(err.Error(), "native runtime"):
        // Use native executor instead
    case strings.Contains(err.Error(), "Docker"):
        // Docker not available
    default:
        // Other error
    }
}
```

## Performance

### Container Startup Times

| Language   | Cold Start | Warm Start |
|------------|------------|------------|
| Ruby       | ~2-3s      | < 100ms    |
| PHP        | ~2-3s      | < 100ms    |
| Bash       | ~1-2s      | < 100ms    |
| Rust       | ~5-7s      | < 100ms    |
| Java       | ~4-6s      | < 100ms    |
| TypeScript | ~3-4s      | < 100ms    |

### Recommendations

1. **Use warm pool** for frequently used languages
2. **Pre-start containers** before first execution
3. **Reuse containers** instead of creating new ones
4. **Set appropriate timeouts** based on code complexity
5. **Monitor resource usage** to avoid OOM

## Security

### Resource Limits

- **Memory**: Default 256 MB, configurable
- **CPU**: Default 100% of one core, configurable
- **Network**: Isolated by default
- **Filesystem**: Read-only except /tmp

### Sandboxing

- Containers run with limited privileges
- No access to host filesystem
- Automatic cleanup on exit
- Timeout enforcement

## Troubleshooting

### Docker not available

```
Error: failed to create Docker client: Cannot connect to the Docker daemon
```

**Solution**: Ensure Docker is running

### Container stuck in starting

```
Status: starting
Message: Container is starting. Retry in a few seconds.
```

**Solution**: Wait and retry, or check Docker logs

### Image pull failed

```
Error: failed to pull image: unauthorized
```

**Solution**: Check Docker Hub credentials or use public images

### Out of memory

```
Error: execution failed with exit code 137
```

**Solution**: Increase memory limit in config

## Future Improvements

- [ ] Support for more languages (C, C++, Kotlin, etc.)
- [ ] Container pooling with LRU eviction
- [ ] Metrics and monitoring integration
- [ ] Distributed execution support
- [ ] GPU support for ML workloads
- [ ] Network sandboxing options
- [ ] Volume mounting for large datasets
- [ ] Multi-stage compilation optimization
