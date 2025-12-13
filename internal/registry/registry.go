package registry

import (
	"strings"
	"sync"

	"github.com/lithammer/fuzzysearch/fuzzy"
	"github.com/nyarum/mcp-proxy/pkg/mcp"
)

// Category represents a group of related tools
type Category struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Keywords    []string `yaml:"keywords"`
	Tools       []string `yaml:"tools"`
}

// Registry manages tool discovery and activation
type Registry struct {
	tools      map[string]mcp.Tool       // All available tools from upstream
	categories []Category                // Tool categories for search
	active     map[string]struct{}       // Currently activated tools
	summaries  map[string]mcp.ToolSummary // Tool summaries for search results
	mu         sync.RWMutex
}

// NewRegistry creates a new tool registry
func NewRegistry() *Registry {
	return &Registry{
		tools:     make(map[string]mcp.Tool),
		active:    make(map[string]struct{}),
		summaries: make(map[string]mcp.ToolSummary),
		categories: defaultCategories(),
	}
}

// LoadTools loads tools from upstream MCP server
func (r *Registry) LoadTools(tools []mcp.Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, tool := range tools {
		r.tools[tool.Name] = tool
		r.summaries[tool.Name] = mcp.ToolSummary{
			Name:        tool.Name,
			Description: truncateDescription(tool.Description, 100),
			Category:    r.findCategory(tool.Name),
		}
	}
}

// Search finds tools matching the query
func (r *Registry) Search(query string, category string, limit int) []mcp.ToolSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	var results []mcp.ToolSummary
	query = strings.ToLower(query)

	// Collect tool names to search
	var toolNames []string
	if category != "" {
		// Filter by category first
		for _, cat := range r.categories {
			if strings.EqualFold(cat.Name, category) {
				toolNames = cat.Tools
				break
			}
		}
	} else {
		for name := range r.tools {
			toolNames = append(toolNames, name)
		}
	}

	// Score and rank tools
	type scored struct {
		summary mcp.ToolSummary
		score   int
	}
	var scored_results []scored

	for _, name := range toolNames {
		summary, ok := r.summaries[name]
		if !ok {
			continue
		}

		// Calculate match score
		score := 0
		nameLower := strings.ToLower(name)
		descLower := strings.ToLower(summary.Description)

		// Exact match in name
		if strings.Contains(nameLower, query) {
			score += 100
		}

		// Fuzzy match in name
		if fuzzy.Match(query, nameLower) {
			score += 50
		}

		// Match in description
		if strings.Contains(descLower, query) {
			score += 30
		}

		// Check category keywords
		for _, cat := range r.categories {
			if r.findCategory(name) == cat.Name {
				for _, kw := range cat.Keywords {
					if strings.Contains(query, strings.ToLower(kw)) {
						score += 20
					}
				}
			}
		}

		if score > 0 {
			scored_results = append(scored_results, scored{summary, score})
		}
	}

	// Sort by score (simple bubble sort for small lists)
	for i := 0; i < len(scored_results); i++ {
		for j := i + 1; j < len(scored_results); j++ {
			if scored_results[j].score > scored_results[i].score {
				scored_results[i], scored_results[j] = scored_results[j], scored_results[i]
			}
		}
	}

	// Take top results
	for i := 0; i < len(scored_results) && i < limit; i++ {
		results = append(results, scored_results[i].summary)
	}

	return results
}

// GetTool returns a tool by name
func (r *Registry) GetTool(name string) (mcp.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Activate marks a tool as active for this session
func (r *Registry) Activate(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tools[name]; ok {
		r.active[name] = struct{}{}
		return true
	}
	return false
}

// ListActive returns all activated tools
func (r *Registry) ListActive() []mcp.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []mcp.Tool
	for name := range r.active {
		if tool, ok := r.tools[name]; ok {
			result = append(result, tool)
		}
	}
	return result
}

// ListCategories returns all available categories
func (r *Registry) ListCategories() []Category {
	return r.categories
}

// ToolCount returns total number of available tools
func (r *Registry) ToolCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// findCategory finds which category a tool belongs to
func (r *Registry) findCategory(toolName string) string {
	for _, cat := range r.categories {
		for _, t := range cat.Tools {
			if t == toolName {
				return cat.Name
			}
		}
	}
	return "other"
}

func truncateDescription(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	return desc[:maxLen-3] + "..."
}

func defaultCategories() []Category {
	return []Category{
		{
			Name:        "repository",
			Description: "Repository management and file operations",
			Keywords:    []string{"repo", "file", "content", "fork", "clone", "create"},
			Tools: []string{
				"get_file_contents", "create_repository", "fork_repository",
				"search_repositories", "create_or_update_file", "delete_file", "push_files",
			},
		},
		{
			Name:        "issues",
			Description: "Issue tracking and management",
			Keywords:    []string{"issue", "bug", "task", "ticket", "label"},
			Tools: []string{
				"list_issues", "issue_read", "issue_write", "search_issues",
				"add_issue_comment", "get_label", "list_issue_types",
			},
		},
		{
			Name:        "pull_requests",
			Description: "Pull request operations",
			Keywords:    []string{"pr", "pull", "merge", "review", "diff"},
			Tools: []string{
				"list_pull_requests", "pull_request_read", "create_pull_request",
				"update_pull_request", "merge_pull_request", "search_pull_requests",
				"update_pull_request_branch",
			},
		},
		{
			Name:        "reviews",
			Description: "Code review operations",
			Keywords:    []string{"review", "comment", "approve", "request changes"},
			Tools: []string{
				"pull_request_review_write", "add_comment_to_pending_review",
				"request_copilot_review",
			},
		},
		{
			Name:        "code_search",
			Description: "Search code across repositories",
			Keywords:    []string{"search", "find", "code", "grep"},
			Tools:       []string{"search_code"},
		},
		{
			Name:        "branches",
			Description: "Branch and tag management",
			Keywords:    []string{"branch", "tag", "ref", "commit"},
			Tools: []string{
				"list_branches", "create_branch", "list_tags", "get_tag",
				"list_commits", "get_commit",
			},
		},
		{
			Name:        "releases",
			Description: "Release management",
			Keywords:    []string{"release", "version", "publish"},
			Tools:       []string{"list_releases", "get_latest_release", "get_release_by_tag"},
		},
		{
			Name:        "users",
			Description: "User and team operations",
			Keywords:    []string{"user", "team", "member", "org", "me"},
			Tools: []string{
				"get_me", "search_users", "get_teams", "get_team_members",
			},
		},
		{
			Name:        "copilot",
			Description: "GitHub Copilot integration",
			Keywords:    []string{"copilot", "ai", "assistant"},
			Tools:       []string{"assign_copilot_to_issue", "request_copilot_review"},
		},
		{
			Name:        "sub_issues",
			Description: "Sub-issue management",
			Keywords:    []string{"sub", "child", "parent", "hierarchy"},
			Tools:       []string{"sub_issue_write"},
		},
	}
}
