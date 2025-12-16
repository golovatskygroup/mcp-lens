package executor

import (
	"time"
)

// SandboxConfig defines sandbox security and resource limits
type SandboxConfig struct {
	// Execution limits
	MaxExecutionTime time.Duration `json:"max_execution_time"`
	MaxMemoryMB      int           `json:"max_memory_mb"`
	MaxCPUPercent    int           `json:"max_cpu_percent"`

	// Network restrictions
	AllowNetwork     bool     `json:"allow_network"`
	AllowedHosts     []string `json:"allowed_hosts,omitempty"`
	AllowedPorts     []int    `json:"allowed_ports,omitempty"`

	// Filesystem restrictions
	AllowFileSystem  bool     `json:"allow_filesystem"`
	ReadOnlyPaths    []string `json:"readonly_paths,omitempty"`
	WritablePaths    []string `json:"writable_paths,omitempty"`

	// Language-specific settings
	Go         *GoSandboxConfig         `json:"go,omitempty"`
	JavaScript *JavaScriptSandboxConfig `json:"javascript,omitempty"`
	Python     *PythonSandboxConfig     `json:"python,omitempty"`
}

// GoSandboxConfig defines Go-specific sandbox settings
type GoSandboxConfig struct {
	AllowedPackages   []string `json:"allowed_packages"`
	ForbiddenPackages []string `json:"forbidden_packages"`
	AllowUnsafe       bool     `json:"allow_unsafe"`
	AllowReflect      bool     `json:"allow_reflect"`
	AllowCGo          bool     `json:"allow_cgo"`
}

// JavaScriptSandboxConfig defines JavaScript-specific sandbox settings
type JavaScriptSandboxConfig struct {
	AllowConsole      bool     `json:"allow_console"`
	AllowTimers       bool     `json:"allow_timers"`
	AllowPromises     bool     `json:"allow_promises"`
	ForbiddenGlobals  []string `json:"forbidden_globals"`
	MaxStackDepth     int      `json:"max_stack_depth"`
}

// PythonSandboxConfig defines Python-specific sandbox settings
type PythonSandboxConfig struct {
	AllowedModules    []string `json:"allowed_modules"`
	ForbiddenModules  []string `json:"forbidden_modules"`
	AllowBuiltins     bool     `json:"allow_builtins"`
	RestrictedBuiltins []string `json:"restricted_builtins"`
}

// DefaultSandboxConfig returns a secure default sandbox configuration
func DefaultSandboxConfig() *SandboxConfig {
	return &SandboxConfig{
		MaxExecutionTime: 30 * time.Second,
		MaxMemoryMB:      256,
		MaxCPUPercent:    80,
		AllowNetwork:     false,
		AllowFileSystem:  false,
		Go: &GoSandboxConfig{
			AllowedPackages: []string{
				"encoding/json",
				"fmt",
				"strings",
				"strconv",
				"math",
				"time",
				"regexp",
				"sort",
				"bytes",
				"io",
				"errors",
			},
			ForbiddenPackages: []string{
				"os",
				"os/exec",
				"syscall",
				"unsafe",
				"reflect",
				"net",
				"net/http",
			},
			AllowUnsafe:  false,
			AllowReflect: false,
			AllowCGo:     false,
		},
		JavaScript: &JavaScriptSandboxConfig{
			AllowConsole:  true,
			AllowTimers:   false,
			AllowPromises: true,
			ForbiddenGlobals: []string{
				"eval",
				"Function",
				"require",
				"import",
			},
			MaxStackDepth: 1000,
		},
		Python: &PythonSandboxConfig{
			AllowedModules: []string{
				"json",
				"math",
				"datetime",
				"re",
				"string",
				"collections",
				"itertools",
				"functools",
			},
			ForbiddenModules: []string{
				"os",
				"sys",
				"subprocess",
				"socket",
				"requests",
				"urllib",
				"__builtin__",
				"__builtins__",
			},
			AllowBuiltins: true,
			RestrictedBuiltins: []string{
				"eval",
				"exec",
				"compile",
				"open",
				"input",
				"__import__",
			},
		},
	}
}

// PermissiveSandboxConfig returns a more permissive sandbox configuration for testing
func PermissiveSandboxConfig() *SandboxConfig {
	config := DefaultSandboxConfig()
	config.AllowNetwork = true
	config.AllowFileSystem = true
	config.MaxExecutionTime = 60 * time.Second
	config.MaxMemoryMB = 512

	// Add network-related packages for Go
	config.Go.AllowedPackages = append(config.Go.AllowedPackages, "net/http", "net/url")
	config.Go.ForbiddenPackages = []string{
		"os/exec",
		"syscall",
		"unsafe",
	}

	// Allow more JavaScript features
	config.JavaScript.AllowTimers = true

	// Allow more Python modules
	config.Python.AllowedModules = append(config.Python.AllowedModules, "requests", "urllib")

	return config
}

// Validate checks if the sandbox configuration is valid
func (c *SandboxConfig) Validate() error {
	if c.MaxExecutionTime <= 0 {
		c.MaxExecutionTime = 30 * time.Second
	}
	if c.MaxMemoryMB <= 0 {
		c.MaxMemoryMB = 256
	}
	if c.MaxCPUPercent <= 0 || c.MaxCPUPercent > 100 {
		c.MaxCPUPercent = 80
	}
	return nil
}

// IsPackageAllowed checks if a Go package is allowed
func (c *SandboxConfig) IsPackageAllowed(pkg string) bool {
	if c.Go == nil {
		return false
	}

	// Check forbidden first
	for _, forbidden := range c.Go.ForbiddenPackages {
		if pkg == forbidden {
			return false
		}
	}

	// Check allowed
	for _, allowed := range c.Go.AllowedPackages {
		if pkg == allowed {
			return true
		}
	}

	return false
}

// IsModuleAllowed checks if a Python module is allowed
func (c *SandboxConfig) IsModuleAllowed(module string) bool {
	if c.Python == nil {
		return false
	}

	// Check forbidden first
	for _, forbidden := range c.Python.ForbiddenModules {
		if module == forbidden {
			return false
		}
	}

	// Check allowed
	for _, allowed := range c.Python.AllowedModules {
		if module == allowed {
			return true
		}
	}

	return false
}

// IsBuiltinAllowed checks if a Python builtin is allowed
func (c *SandboxConfig) IsBuiltinAllowed(builtin string) bool {
	if c.Python == nil || !c.Python.AllowBuiltins {
		return false
	}

	// Check restricted
	for _, restricted := range c.Python.RestrictedBuiltins {
		if builtin == restricted {
			return false
		}
	}

	return true
}
