package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/golovatskygroup/mcp-lens/internal/presets"
	"github.com/golovatskygroup/mcp-lens/internal/proxy"
	"github.com/golovatskygroup/mcp-lens/internal/server"
	"gopkg.in/yaml.v3"
)

// UpstreamConfig wraps proxy.Config with optional preset field
type UpstreamConfig struct {
	// Preset selects a built-in preset (e.g., "github")
	Preset string `yaml:"preset,omitempty"`
	// Command to execute
	Command string `yaml:"command,omitempty"`
	// Arguments to pass to the command
	Args []string `yaml:"args,omitempty"`
	// Environment variables to set
	Env map[string]string `yaml:"env,omitempty"`
}

// Config wraps the upstream configuration
type Config struct {
	Upstream UpstreamConfig `yaml:"upstream"`
}

func main() {
	configPath := flag.String("config", "", "Path to config file")
	flag.Parse()

	// Default config with github preset
	cfg := Config{
		Upstream: UpstreamConfig{
			Command: "npx",
			Args:    []string{"-y", "@modelcontextprotocol/server-github"},
			Env: map[string]string{
				"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}",
			},
		},
	}

	// Load config file if provided
	if *configPath != "" {
		data, err := os.ReadFile(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading config: %v\n", err)
			os.Exit(1)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing config: %v\n", err)
			os.Exit(1)
		}
	}

	// Load upstream config with preset registry and environment variable support
	// Priority: env vars > config file > defaults
	upstreamCfg, err := resolveUpstreamConfig(cfg.Upstream)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving upstream config: %v\n", err)
		os.Exit(1)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	// Create and run server
	srv := server.New(ctx, upstreamCfg)
	if err := srv.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

// resolveUpstreamConfig resolves the upstream config using preset registry and env vars
// Priority:
// 1. If preset is specified, use it as base
// 2. Override with inline fields from config file
// 3. Apply environment variable overrides on top
func resolveUpstreamConfig(upstreamCfg UpstreamConfig) (proxy.Config, error) {
	reg := presets.NewRegistry()
	var result proxy.Config

	// If preset is specified in config file, load it first
	if upstreamCfg.Preset != "" {
		presetCfg, ok := reg.Get(upstreamCfg.Preset)
		if !ok {
			return proxy.Config{}, fmt.Errorf("unknown preset: %s", upstreamCfg.Preset)
		}
		result = presetCfg
	}

	// Override with inline fields from config file
	// These take precedence over preset values
	if upstreamCfg.Command != "" {
		result.Command = upstreamCfg.Command
	}
	if len(upstreamCfg.Args) > 0 {
		result.Args = upstreamCfg.Args
	}
	if len(upstreamCfg.Env) > 0 {
		if result.Env == nil {
			result.Env = make(map[string]string)
		}
		for k, v := range upstreamCfg.Env {
			result.Env[k] = v
		}
	}

	// Apply environment variable overrides (highest priority)
	merged, err := presets.MergeWithEnv(result)
	if err != nil {
		return proxy.Config{}, err
	}

	return merged, nil
}
