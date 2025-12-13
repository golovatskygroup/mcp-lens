package presets

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/golovatskygroup/mcp-lens/internal/proxy"
)

// Preset represents a named preset configuration
type Preset struct {
	Name   string
	Config proxy.Config
}

// Registry holds built-in presets
type Registry struct {
	presets map[string]proxy.Config
}

// NewRegistry creates a new preset registry with built-in presets
func NewRegistry() *Registry {
	return &Registry{
		presets: map[string]proxy.Config{
			"github": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-github"},
				Env: map[string]string{
					"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}",
				},
			},
		},
	}
}

// Get returns a preset by name
func (r *Registry) Get(name string) (proxy.Config, bool) {
	cfg, ok := r.presets[name]
	return cfg, ok
}

// GetEnvPreset attempts to load a preset from environment variables
func (r *Registry) GetEnvPreset() (proxy.Config, error) {
	preset := os.Getenv("MCP_LENS_PRESET")
	if preset == "" {
		return proxy.Config{}, nil
	}

	cfg, ok := r.Get(preset)
	if !ok {
		return proxy.Config{}, fmt.Errorf("unknown preset: %s", preset)
	}
	return cfg, nil
}

// MergeWithEnv merges the config with environment variables
// ENV vars take precedence over config file values
// MCP_LENS_UPSTREAM_COMMAND: Override command
// MCP_LENS_UPSTREAM_ARGS_JSON: Override args (JSON array)
// MCP_LENS_UPSTREAM_ENV_JSON: Merge environment variables (JSON object)
func MergeWithEnv(cfg proxy.Config) (proxy.Config, error) {
	result := cfg

	// Override command if env var is set
	if cmd := os.Getenv("MCP_LENS_UPSTREAM_COMMAND"); cmd != "" {
		result.Command = cmd
	}

	// Override args if env var is set
	if argsJSON := os.Getenv("MCP_LENS_UPSTREAM_ARGS_JSON"); argsJSON != "" {
		var args []string
		if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
			return proxy.Config{}, fmt.Errorf("invalid MCP_LENS_UPSTREAM_ARGS_JSON: %w", err)
		}
		result.Args = args
	}

	// Merge environment variables from env var if set
	if envJSON := os.Getenv("MCP_LENS_UPSTREAM_ENV_JSON"); envJSON != "" {
		var envMap map[string]string
		if err := json.Unmarshal([]byte(envJSON), &envMap); err != nil {
			return proxy.Config{}, fmt.Errorf("invalid MCP_LENS_UPSTREAM_ENV_JSON: %w", err)
		}

		// Initialize map if nil
		if result.Env == nil {
			result.Env = make(map[string]string)
		}

		// Merge: env vars take precedence
		for k, v := range envMap {
			result.Env[k] = v
		}
	}

	return result, nil
}

// LoadConfig loads config from environment first, then falls back to defaults/file
// Priority:
// 1. MCP_LENS_PRESET (preset from registry)
// 2. MCP_LENS_UPSTREAM_* env vars (override specific fields)
// 3. Fallback to provided config
// 4. Apply env var merges on top of all
func LoadConfig(fallbackConfig proxy.Config) (proxy.Config, error) {
	reg := NewRegistry()
	result := fallbackConfig

	// Try to load preset from env var first
	if presetCfg, err := reg.GetEnvPreset(); err != nil {
		return proxy.Config{}, err
	} else if !isZeroConfig(presetCfg) {
		// Preset was found, use it as base
		result = presetCfg
	}

	// Apply env var overrides on top
	merged, err := MergeWithEnv(result)
	if err != nil {
		return proxy.Config{}, err
	}

	return merged, nil
}

// isZeroConfig checks if a config is the zero value
func isZeroConfig(cfg proxy.Config) bool {
	return cfg.Command == "" && len(cfg.Args) == 0 && len(cfg.Env) == 0
}

// ListAvailable returns names of all available presets
func ListAvailable() []string {
	return []string{"github"}
}
