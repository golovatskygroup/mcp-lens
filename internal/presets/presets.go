package presets

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/golovatskygroup/mcp-lens/internal/proxy"
)

// Category represents a preset category
type Category string

const (
	CategoryReference    Category = "reference"
	CategoryDatabase     Category = "database"
	CategorySearch       Category = "search"
	CategoryProductivity Category = "productivity"
	CategoryDevTools     Category = "devtools"
)

// CategoryInfo contains display information for a category
type CategoryInfo struct {
	Name        string
	Description string
	Order       int
}

// Categories returns all available categories with their display info
func Categories() map[Category]CategoryInfo {
	return map[Category]CategoryInfo{
		CategoryReference: {
			Name:        "Reference",
			Description: "Official MCP reference servers",
			Order:       1,
		},
		CategoryDatabase: {
			Name:        "Database",
			Description: "Database and storage servers",
			Order:       2,
		},
		CategorySearch: {
			Name:        "Search",
			Description: "Search and retrieval servers",
			Order:       3,
		},
		CategoryProductivity: {
			Name:        "Productivity",
			Description: "Productivity and collaboration tools",
			Order:       4,
		},
		CategoryDevTools: {
			Name:        "DevTools",
			Description: "Developer tools and infrastructure",
			Order:       5,
		},
	}
}

// SortedCategories returns categories sorted by their order
func SortedCategories() []Category {
	cats := Categories()
	result := make([]Category, 0, len(cats))
	for cat := range cats {
		result = append(result, cat)
	}
	sort.Slice(result, func(i, j int) bool {
		return cats[result[i]].Order < cats[result[j]].Order
	})
	return result
}

// EnvVar describes an environment variable
type EnvVar struct {
	Name        string // Environment variable name
	Description string // Human-readable description
	Example     string // Example value
	Secret      bool   // Whether to mask input (passwords, tokens)
}

// PresetMeta contains metadata about a preset
type PresetMeta struct {
	Name        string            // Preset identifier (e.g., "github")
	Description string            // Human-readable description
	Category    Category          // Category for grouping
	Config      proxy.Config      // Server configuration
	RequiredEnv []EnvVar          // Required environment variables
	OptionalEnv []EnvVar          // Optional environment variables
	DocsURL     string            // Link to documentation
}

// Preset represents a named preset configuration (legacy compatibility)
type Preset struct {
	Name   string
	Config proxy.Config
}

// Registry holds built-in presets
type Registry struct {
	presets map[string]PresetMeta
}

// NewRegistry creates a new preset registry with built-in presets
func NewRegistry() *Registry {
	return &Registry{
		presets: map[string]PresetMeta{
			// ============================================
			// Reference Servers (official MCP servers)
			// ============================================
			"everything": {
				Name:        "everything",
				Description: "MCP test server with prompts, resources, and tools for testing",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-everything"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/everything",
			},
			"time": {
				Name:        "time",
				Description: "Get current time and timezone conversions",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-time"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/time",
			},
			"memory": {
				Name:        "memory",
				Description: "Knowledge graph-based persistent memory",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-memory"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/memory",
			},
			"fetch": {
				Name:        "fetch",
				Description: "Fetch web content and convert to markdown",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-fetch"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/fetch",
			},
			"filesystem": {
				Name:        "filesystem",
				Description: "File system operations (read, write, list, search)",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-filesystem"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/filesystem",
			},
			"git": {
				Name:        "git",
				Description: "Git repository operations (status, diff, log, commit)",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-git"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/git",
			},
			"sequential-thinking": {
				Name:        "sequential-thinking",
				Description: "Dynamic problem-solving through thought sequences",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking",
			},
			"puppeteer": {
				Name:        "puppeteer",
				Description: "Browser automation with Puppeteer",
				Category:    CategoryReference,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-puppeteer"},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/puppeteer",
			},

			// ============================================
			// Database Servers
			// ============================================
			"sqlite": {
				Name:        "sqlite",
				Description: "SQLite database operations",
				Category:    CategoryDatabase,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-sqlite"},
				},
				OptionalEnv: []EnvVar{
					{
						Name:        "SQLITE_DB_PATH",
						Description: "Path to SQLite database file",
						Example:     "/path/to/database.db",
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/sqlite",
			},
			"postgres": {
				Name:        "postgres",
				Description: "PostgreSQL database operations",
				Category:    CategoryDatabase,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-postgres"},
					Env: map[string]string{
						"POSTGRES_CONNECTION_STRING": "${POSTGRES_CONNECTION_STRING}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "POSTGRES_CONNECTION_STRING",
						Description: "PostgreSQL connection string",
						Example:     "postgresql://user:pass@localhost:5432/dbname",
						Secret:      true,
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/postgres",
			},
			"redis": {
				Name:        "redis",
				Description: "Redis database operations",
				Category:    CategoryDatabase,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-redis"},
					Env: map[string]string{
						"REDIS_URL": "${REDIS_URL}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "REDIS_URL",
						Description: "Redis connection URL",
						Example:     "redis://localhost:6379",
						Secret:      true,
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/redis",
			},
			"mongodb": {
				Name:        "mongodb",
				Description: "MongoDB database operations",
				Category:    CategoryDatabase,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "mcp-server-mongodb"},
					Env: map[string]string{
						"MONGODB_URI": "${MONGODB_URI}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "MONGODB_URI",
						Description: "MongoDB connection URI",
						Example:     "mongodb://localhost:27017/mydb",
						Secret:      true,
					},
				},
				DocsURL: "https://github.com/kiliczsh/mcp-mongo-server",
			},

			// ============================================
			// Search Servers
			// ============================================
			"brave-search": {
				Name:        "brave-search",
				Description: "Web search using Brave Search API",
				Category:    CategorySearch,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-brave-search"},
					Env: map[string]string{
						"BRAVE_API_KEY": "${BRAVE_API_KEY}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "BRAVE_API_KEY",
						Description: "Brave Search API key",
						Example:     "BSA...",
						Secret:      true,
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/brave-search",
			},
			"tavily": {
				Name:        "tavily",
				Description: "AI-powered search using Tavily API",
				Category:    CategorySearch,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "tavily-mcp"},
					Env: map[string]string{
						"TAVILY_API_KEY": "${TAVILY_API_KEY}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "TAVILY_API_KEY",
						Description: "Tavily API key",
						Example:     "tvly-...",
						Secret:      true,
					},
				},
				DocsURL: "https://tavily.com/",
			},
			"search": {
				Name:        "search",
				Description: "Local search server using Sonar Pro (OpenRouter)",
				Category:    CategorySearch,
				Config: proxy.Config{
					Command: "python",
					Args:    []string{"-m", "search_server"},
					Env: map[string]string{
						"OPENROUTER_API_KEY": "${OPENROUTER_API_KEY}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "OPENROUTER_API_KEY",
						Description: "OpenRouter API key for Sonar Pro",
						Example:     "sk-or-...",
						Secret:      true,
					},
				},
				DocsURL: "https://openrouter.ai/",
			},

			// ============================================
			// Productivity Servers
			// ============================================
			"github": {
				Name:        "github",
				Description: "GitHub API integration (repos, issues, PRs)",
				Category:    CategoryProductivity,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-github"},
					Env: map[string]string{
						"GITHUB_PERSONAL_ACCESS_TOKEN": "${GITHUB_TOKEN}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "GITHUB_TOKEN",
						Description: "GitHub Personal Access Token",
						Example:     "ghp_...",
						Secret:      true,
					},
				},
				OptionalEnv: []EnvVar{
					{
						Name:        "GITHUB_API_URL",
						Description: "Custom GitHub API URL (for GitHub Enterprise)",
						Example:     "https://github.company.com/api/v3",
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/github",
			},
			"gitlab": {
				Name:        "gitlab",
				Description: "GitLab API integration",
				Category:    CategoryProductivity,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-gitlab"},
					Env: map[string]string{
						"GITLAB_PERSONAL_ACCESS_TOKEN": "${GITLAB_TOKEN}",
						"GITLAB_API_URL":               "${GITLAB_API_URL}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "GITLAB_TOKEN",
						Description: "GitLab Personal Access Token",
						Example:     "glpat-...",
						Secret:      true,
					},
				},
				OptionalEnv: []EnvVar{
					{
						Name:        "GITLAB_API_URL",
						Description: "GitLab API URL (default: https://gitlab.com/api/v4)",
						Example:     "https://gitlab.company.com/api/v4",
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/gitlab",
			},
			"slack": {
				Name:        "slack",
				Description: "Slack workspace integration",
				Category:    CategoryProductivity,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-slack"},
					Env: map[string]string{
						"SLACK_BOT_TOKEN": "${SLACK_BOT_TOKEN}",
						"SLACK_TEAM_ID":   "${SLACK_TEAM_ID}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "SLACK_BOT_TOKEN",
						Description: "Slack Bot User OAuth Token",
						Example:     "xoxb-...",
						Secret:      true,
					},
					{
						Name:        "SLACK_TEAM_ID",
						Description: "Slack Team/Workspace ID",
						Example:     "T01234567",
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/slack",
			},
			"notion": {
				Name:        "notion",
				Description: "Notion workspace integration",
				Category:    CategoryProductivity,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@notionhq/notion-mcp-server"},
					Env: map[string]string{
						"OPENAPI_MCP_HEADERS": `{"Authorization": "Bearer ${NOTION_API_KEY}", "Notion-Version": "2022-06-28"}`,
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "NOTION_API_KEY",
						Description: "Notion Integration Token",
						Example:     "secret_...",
						Secret:      true,
					},
				},
				DocsURL: "https://github.com/makenotion/notion-mcp-server",
			},
			"linear": {
				Name:        "linear",
				Description: "Linear project management integration",
				Category:    CategoryProductivity,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "mcp-linear"},
					Env: map[string]string{
						"LINEAR_API_KEY": "${LINEAR_API_KEY}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "LINEAR_API_KEY",
						Description: "Linear API Key",
						Example:     "lin_api_...",
						Secret:      true,
					},
				},
				DocsURL: "https://linear.app/",
			},
			"google-maps": {
				Name:        "google-maps",
				Description: "Google Maps API integration",
				Category:    CategoryProductivity,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-google-maps"},
					Env: map[string]string{
						"GOOGLE_MAPS_API_KEY": "${GOOGLE_MAPS_API_KEY}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "GOOGLE_MAPS_API_KEY",
						Description: "Google Maps API Key",
						Example:     "AIza...",
						Secret:      true,
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/google-maps",
			},

			// ============================================
			// DevTools Servers
			// ============================================
			"sentry": {
				Name:        "sentry",
				Description: "Sentry error tracking integration",
				Category:    CategoryDevTools,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-sentry"},
					Env: map[string]string{
						"SENTRY_AUTH_TOKEN": "${SENTRY_AUTH_TOKEN}",
						"SENTRY_ORG":        "${SENTRY_ORG}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "SENTRY_AUTH_TOKEN",
						Description: "Sentry Auth Token",
						Example:     "sntrys_...",
						Secret:      true,
					},
					{
						Name:        "SENTRY_ORG",
						Description: "Sentry Organization slug",
						Example:     "my-org",
					},
				},
				OptionalEnv: []EnvVar{
					{
						Name:        "SENTRY_PROJECT",
						Description: "Default Sentry project slug",
						Example:     "my-project",
					},
				},
				DocsURL: "https://github.com/modelcontextprotocol/servers/tree/main/src/sentry",
			},
			"aws": {
				Name:        "aws",
				Description: "AWS services integration (S3, Lambda, etc.)",
				Category:    CategoryDevTools,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "mcp-server-aws"},
					Env: map[string]string{
						"AWS_ACCESS_KEY_ID":     "${AWS_ACCESS_KEY_ID}",
						"AWS_SECRET_ACCESS_KEY": "${AWS_SECRET_ACCESS_KEY}",
						"AWS_REGION":            "${AWS_REGION}",
					},
				},
				RequiredEnv: []EnvVar{
					{
						Name:        "AWS_ACCESS_KEY_ID",
						Description: "AWS Access Key ID",
						Example:     "AKIA...",
						Secret:      true,
					},
					{
						Name:        "AWS_SECRET_ACCESS_KEY",
						Description: "AWS Secret Access Key",
						Example:     "wJalr...",
						Secret:      true,
					},
					{
						Name:        "AWS_REGION",
						Description: "AWS Region",
						Example:     "us-east-1",
					},
				},
				DocsURL: "https://aws.amazon.com/",
			},
			"kubernetes": {
				Name:        "kubernetes",
				Description: "Kubernetes cluster management",
				Category:    CategoryDevTools,
				Config: proxy.Config{
					Command: "npx",
					Args:    []string{"-y", "mcp-server-kubernetes"},
					Env: map[string]string{
						"KUBECONFIG": "${KUBECONFIG}",
					},
				},
				OptionalEnv: []EnvVar{
					{
						Name:        "KUBECONFIG",
						Description: "Path to kubeconfig file",
						Example:     "~/.kube/config",
					},
					{
						Name:        "KUBERNETES_CONTEXT",
						Description: "Kubernetes context to use",
						Example:     "my-cluster",
					},
				},
				DocsURL: "https://kubernetes.io/",
			},
		},
	}
}

// Get returns a preset by name (returns proxy.Config for backward compatibility)
func (r *Registry) Get(name string) (proxy.Config, bool) {
	meta, ok := r.presets[name]
	if !ok {
		return proxy.Config{}, false
	}
	return meta.Config, true
}

// GetMeta returns full preset metadata by name
func (r *Registry) GetMeta(name string) (PresetMeta, bool) {
	meta, ok := r.presets[name]
	return meta, ok
}

// GetByCategory returns all presets in a category
func (r *Registry) GetByCategory(cat Category) []PresetMeta {
	var result []PresetMeta
	for _, meta := range r.presets {
		if meta.Category == cat {
			result = append(result, meta)
		}
	}
	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

// All returns all presets
func (r *Registry) All() []PresetMeta {
	result := make([]PresetMeta, 0, len(r.presets))
	for _, meta := range r.presets {
		result = append(result, meta)
	}
	// Sort by category order, then by name
	cats := Categories()
	sort.Slice(result, func(i, j int) bool {
		if result[i].Category != result[j].Category {
			return cats[result[i].Category].Order < cats[result[j].Category].Order
		}
		return result[i].Name < result[j].Name
	})
	return result
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
	reg := NewRegistry()
	all := reg.All()
	result := make([]string, len(all))
	for i, meta := range all {
		result[i] = meta.Name
	}
	return result
}

// ListByCategory returns a map of category to preset names
func ListByCategory() map[Category][]string {
	reg := NewRegistry()
	result := make(map[Category][]string)
	for _, cat := range SortedCategories() {
		presets := reg.GetByCategory(cat)
		names := make([]string, len(presets))
		for i, p := range presets {
			names[i] = p.Name
		}
		result[cat] = names
	}
	return result
}
