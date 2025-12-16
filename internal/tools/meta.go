package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/artifacts"
	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

// Handler processes local tool calls (meta-tools + proxy-provided tools).
type Handler struct {
	registry  *registry.Registry
	executor  func(name string, args json.RawMessage) (*mcp.CallToolResult, error)
	artifacts *artifacts.Store
	execDeps  *ExecutionDependencies
}

// NewHandler creates a new tool handler.
func NewHandler(reg *registry.Registry, executor func(string, json.RawMessage) (*mcp.CallToolResult, error)) *Handler {
	st, _ := artifacts.NewFromEnv()
	return &Handler{registry: reg, executor: executor, artifacts: st}
}

func (h *Handler) ArtifactStore() *artifacts.Store {
	return h.artifacts
}

// SetExecutionDependencies configures the code execution dependencies (Docker, native executor, tool registry, discovery).
func (h *Handler) SetExecutionDependencies(deps *ExecutionDependencies) {
	h.execDeps = deps
}

// BuiltinTools returns local tools provided by the proxy.
func (h *Handler) BuiltinTools() []mcp.Tool {
	tools := []mcp.Tool{
		{
			Name:        "search_tools",
			Description: "Search available tools by keyword or category (local + upstream). By default returns text; set format=json for machine-readable output.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "Search query (e.g., 'pull request', 'files changed', 'diff', 'review')"},
					"category": {"type": "string", "description": "Filter by category", "enum": ["repository", "issues", "pull_requests", "reviews", "code_search", "branches", "releases", "users", "copilot", "sub_issues", "local"]},
					"limit": {"type": "integer", "description": "Max results (default: 10)", "default": 10},
					"format": {"type": "string", "description": "Output format: text (default) or json", "enum": ["text", "json"], "default": "text"},
					"include_schemas": {"type": "boolean", "description": "Include inputSchema for each tool (json format only)", "default": false}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "describe_tool",
			Description: "Get the full schema and description of a specific tool. Use this after search_tools to understand a tool's parameters before executing it.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Exact tool name (from search_tools results)"}
				},
				"required": ["name"]
			}`),
		},
		{
			Name:        "execute_tool",
			Description: "Execute an upstream GitHub tool with the given parameters. The tool will be auto-activated for this session.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Tool name to execute"},
					"params": {"type": "object", "description": "Tool-specific parameters (see describe_tool for schema)"}
				},
				"required": ["name", "params"]
			}`),
		},
		{
			Name:        "dev_scaffold_tool",
			Description: "(Dev mode) Generate a patch + isolated git worktree implementing a new local tool (handler + wiring). Requires MCP_LENS_DEV_MODE=1.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"tool_name": {"type": "string", "description": "New local tool name (snake_case)"},
					"tool_description": {"type": "string", "description": "Short human description"},
					"input_schema": {"type": "object", "description": "JSON Schema for the tool arguments"},
					"spec": {"type": "string", "description": "Optional implementation notes / requirements (used for best-effort code generation)"},
					"handler_method": {"type": "string", "description": "Optional Go method name (default derived from tool_name)"},
					"domain": {"type": "string", "description": "Domain hint: jira|confluence|grafana|github|other (default derived from tool_name prefix)"},
					"target_dir": {"type": "string", "description": "Where to write the patch inside the worktree (default: tasks/scaffolds)"},
					"use_worktree": {"type": "boolean", "description": "Create a new git worktree and apply the patch there (default: true)", "default": true},
					"worktree_root": {"type": "string", "description": "Worktree root relative to repo (default: .worktrees)"},
					"worktree_name": {"type": "string", "description": "Worktree directory name (default: dev-<tool>-<ts>)"},
					"run_gofmt": {"type": "boolean", "description": "Run gofmt in the worktree before producing patch (default: true)", "default": true},
					"run_tests": {"type": "boolean", "description": "Run go test ./... in the worktree after applying patch (default: false)", "default": false},
					"allow_in_policy": {"type": "boolean", "description": "Also add the new tool to internal/router/policy.go allowlist", "default": false},
					"add_to_search": {"type": "boolean", "description": "Also add the new tool to searchLocalTools() for discoverability", "default": true},
					"add_prompt_hint": {"type": "boolean", "description": "Also add a hint to internal/router/prompt.go (planner guidance)", "default": false}
				},
				"required": ["tool_name", "tool_description", "input_schema"]
			}`),
		},
		{
			Name:        "artifact_save_text",
			Description: "Save text as an artifact (local). Useful for storing plans/notes between sessions. Returns artifact:// reference.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"name": {"type": "string", "description": "Optional name/title used for artifact filename"},
					"text": {"type": "string", "description": "Text content to save"},
					"mime": {"type": "string", "description": "MIME type (default: text/markdown)"}
				},
				"required": ["text"]
			}`),
		},
		{
			Name:        "artifact_append_text",
			Description: "Append text to an existing artifact by id/uri (creates a new artifact).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"artifact_id": {"type": "string", "description": "Existing artifact id"},
					"artifact_uri": {"type": "string", "description": "Existing artifact uri (artifact://...)"},
					"text": {"type": "string", "description": "Text to append"}
				},
				"required": ["text"]
			}`),
		},
		{
			Name:        "artifact_list",
			Description: "List recently created artifacts in this server process.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"limit": {"type": "integer", "description": "Max items to return (default: all in-memory)"}
				}
			}`),
		},
		{
			Name:        "artifact_search",
			Description: "Search recently created artifacts by substring match over id/path/mime/tool.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "Search query"},
					"limit": {"type": "integer", "description": "Max matches (default: 20, max: 200)", "default": 20, "maximum": 200}
				},
				"required": ["query"]
			}`),
		},
		{
			Name:        "get_pull_request_details",
			Description: "Get pull request metadata (title, base/head, author, state). Read-only via GitHub REST API.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "list_pull_request_files",
			Description: "List changed files in a PR with pagination (page/per_page). Read-only via GitHub REST API.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"page": {"type": "integer", "description": "Page number (default: 1)", "default": 1},
					"per_page": {"type": "integer", "description": "Items per page (default: 30, max: 100)", "default": 30}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "get_pull_request_diff",
			Description: "Fetch unified diff for a PR in chunks (~4000 tokens/16KB default). Response includes pagination info (current_part/total_parts) and next_offset for fetching subsequent parts. Use file_filter to limit diff to specific files (glob patterns).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"offset": {"type": "integer", "description": "Byte offset into diff (default: 0)", "default": 0},
					"max_bytes": {"type": "integer", "description": "Max bytes per chunk (default: 16000 = ~4000 tokens, max: 64000)", "default": 16000, "maximum": 64000},
					"file_filter": {"type": "array", "items": {"type": "string"}, "description": "Filter diff to specific file paths (glob patterns supported, e.g. '*.go', 'src/*.ts')"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "get_pull_request_summary",
			Description: "Get a compact summary of PR changes including statistics, file types, directories affected. Use this first to understand PR scope before fetching full diff.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "get_pull_request_file_diff",
			Description: "Get diff for a specific file in a PR. Use this when you only need to review one file.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"path": {"type": "string", "description": "File path to get diff for"}
				},
				"required": ["repo", "number", "path"]
			}`),
		},
		{
			Name:        "list_pull_request_commits",
			Description: "List commits in a PR with pagination (page/per_page). Read-only via GitHub REST API.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"page": {"type": "integer", "description": "Page number (default: 1)", "default": 1},
					"per_page": {"type": "integer", "description": "Items per page (default: 30, max: 100)", "default": 30}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "get_pull_request_checks",
			Description: "Get check-runs for a PR head SHA (derived from PR details). Read-only via GitHub REST API.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "get_file_at_ref",
			Description: "Fetch raw file contents at a specific ref (sha/branch/tag). Read-only via GitHub REST API.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"ref": {"type": "string", "description": "Git ref (sha/branch/tag)"},
					"path": {"type": "string", "description": "File path in repo"}
				},
				"required": ["repo", "ref", "path"]
			}`),
		},
		{
			Name:        "github_list_workflow_runs",
			Description: "List GitHub Actions workflow runs for a repository (read-only). Useful for debugging CI failures.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"branch": {"type": "string", "description": "Filter by branch name"},
					"event": {"type": "string", "description": "Filter by event (e.g., push, pull_request)"},
					"status": {"type": "string", "description": "Filter by status (e.g., completed, in_progress, queued)"},
					"head_sha": {"type": "string", "description": "Filter by head SHA (useful for PR head)"},
					"page": {"type": "integer", "description": "Page number (default: 1)", "default": 1},
					"per_page": {"type": "integer", "description": "Items per page (default: 20, max: 100)", "default": 20, "maximum": 100}
				},
				"required": ["repo"]
			}`),
		},
		{
			Name:        "github_list_workflow_jobs",
			Description: "List GitHub Actions jobs for a workflow run (read-only). Use with github_list_workflow_runs.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"run_id": {"type": "integer", "description": "Workflow run id"},
					"page": {"type": "integer", "description": "Page number (default: 1)", "default": 1},
					"per_page": {"type": "integer", "description": "Items per page (default: 50, max: 100)", "default": 50, "maximum": 100}
				},
				"required": ["repo", "run_id"]
			}`),
		},
		{
			Name:        "github_download_job_logs",
			Description: "Download GitHub Actions job logs (read-only) and store them as an artifact (text/plain).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"job_id": {"type": "integer", "description": "Workflow job id"}
				},
				"required": ["repo", "job_id"]
			}`),
		},
		{
			Name:        "prepare_pull_request_review_bundle",
			Description: "Prepare a review bundle: PR details + file list; optionally include diff chunk (~4000 tokens default), commits, and checks.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"files_page": {"type": "integer", "description": "Files page (default: 1)", "default": 1},
					"files_per_page": {"type": "integer", "description": "Files per page (default: 30, max: 100)", "default": 30},
					"include_diff": {"type": "boolean", "description": "Whether to include a unified diff chunk", "default": false},
					"diff_offset": {"type": "integer", "description": "Byte offset into diff (default: 0)", "default": 0},
					"max_diff_bytes": {"type": "integer", "description": "Max diff bytes per chunk (default: 16000 = ~4000 tokens, max: 64000)", "default": 16000, "maximum": 64000},
					"include_commits": {"type": "boolean", "description": "Whether to include PR commits", "default": false},
					"commits_page": {"type": "integer", "description": "Commits page (default: 1)", "default": 1},
					"commits_per_page": {"type": "integer", "description": "Commits per page (default: 30, max: 100)", "default": 30},
					"include_checks": {"type": "boolean", "description": "Whether to include check-runs for PR head sha", "default": false}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "fetch_complete_pr_diff",
			Description: "Fetches COMPLETE PR diff (all parts with auto-pagination) and saves to a temp file. Returns file path and metadata. Use this for comprehensive reviews of large PRs instead of get_pull_request_diff.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"file_filter": {"type": "array", "items": {"type": "string"}, "description": "Filter diff to specific file paths (glob patterns supported, e.g. '*.go', 'src/*.ts')"},
					"output_dir": {"type": "string", "description": "Directory to save diff file (default: system temp dir)"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "fetch_complete_pr_files",
			Description: "Fetches COMPLETE list of all changed files in PR (all pages with auto-pagination) and saves to a temp file. Returns file path and metadata.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"output_dir": {"type": "string", "description": "Directory to save files list (default: system temp dir)"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "jira_get_myself",
			Description: "Get authenticated Jira user info (useful to validate auth). Uses env auth by default; optional base_url/api_version override.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT, then falls back to JIRA_BASE_URL + auth env."},
					"base_url": {"type": "string", "description": "Override base URL (e.g. https://your-site.atlassian.net or https://jira.company.com). If omitted, uses JIRA_BASE_URL or (3LO) https://api.atlassian.com/ex/jira/${JIRA_CLOUD_ID}."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				}
			}`),
		},
		{
			Name:        "jira_get_issue",
			Description: "Get a Jira issue by key or id (read-only). Supports fields/expand and optional base_url/api_version override.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"fields": {"type": "array", "items": {"type": "string"}, "description": "Optional list of fields to return (e.g., [\"summary\",\"status\",\"assignee\"])."},
					"expand": {"type": "array", "items": {"type": "string"}, "description": "Optional expand directives (e.g., [\"renderedFields\",\"names\"])."},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue"]
			}`),
		},
		{
			Name:        "jira_get_issue_bundle",
			Description: "Get a Jira issue plus optional comments and changelog (read-only). Use this to turn a ticket into an implementation plan quickly.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"fields": {"type": "array", "items": {"type": "string"}, "description": "Optional list of issue fields to return."},
					"expand": {"type": "array", "items": {"type": "string"}, "description": "Optional expand directives for issue details."},
					"include_changelog": {"type": "boolean", "description": "Whether to include changelog via expand=changelog (can be large).", "default": false},
					"include_comments": {"type": "boolean", "description": "Whether to include issue comments.", "default": true},
					"comments_startAt": {"type": "integer", "description": "Comments pagination offset (default: 0).", "default": 0},
					"comments_maxResults": {"type": "integer", "description": "Comments page size (default: 50). Set 0 to skip comments.", "default": 50},
					"comments_expand": {"type": "string", "description": "Optional comments expand directive (e.g., \"renderedBody\")."},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default depends on base URL.", "enum": [2,3]}
				},
				"required": ["issue"]
			}`),
		},
		{
			Name:        "jira_search_issues",
			Description: "Search Jira issues using JQL (read-only) with pagination (startAt/maxResults). Supports fields/expand and optional base_url/api_version override.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"jql": {"type": "string", "description": "JQL query (e.g., \"project = PROJ ORDER BY updated DESC\")"},
					"startAt": {"type": "integer", "description": "Pagination offset (default: 0)", "default": 0},
					"maxResults": {"type": "integer", "description": "Page size (default: 50)", "default": 50},
					"fields": {"type": "array", "items": {"type": "string"}, "description": "Optional list of fields to return."},
					"expand": {"type": "array", "items": {"type": "string"}, "description": "Optional expand directives."},
					"validateQuery": {"type": "string", "description": "Validate JQL: strict (default), warn, none", "enum": ["strict","warn","none"]},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["jql"]
			}`),
		},
		{
			Name:        "jira_get_issue_comments",
			Description: "List comments for an issue (read-only) with pagination (startAt/maxResults).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"startAt": {"type": "integer", "description": "Pagination offset (default: 0)", "default": 0},
					"maxResults": {"type": "integer", "description": "Page size (default: 50)", "default": 50},
					"orderBy": {"type": "string", "description": "Optional orderBy value (varies by Jira version)."},
					"expand": {"type": "string", "description": "Optional expand directive (e.g., \"renderedBody\")."},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue"]
			}`),
		},
		{
			Name:        "jira_get_issue_transitions",
			Description: "List available workflow transitions for an issue (read-only).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"expand": {"type": "string", "description": "Optional expand (e.g., \"transitions.fields\")."},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue"]
			}`),
		},
		{
			Name:        "jira_list_projects",
			Description: "List Jira projects (read-only). For v3 uses /project/search (paged); for v2 uses /project.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"startAt": {"type": "integer", "description": "Pagination offset for v3 (default: 0)", "default": 0},
					"maxResults": {"type": "integer", "description": "Page size for v3 (default: 50)", "default": 50},
					"orderBy": {"type": "string", "description": "Sort field for v3 (e.g., \"key\", \"name\")."},
					"query": {"type": "string", "description": "Free-text filter for v3."},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				}
			}`),
		},
		{
			Name:        "jira_export_tasks",
			Description: "Export Jira issues found by JQL to local markdown files, and (optionally) expand known links in descriptions (e.g. Confluence pages). Read-only for remote systems; writes files locally.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"jql": {"type": "string", "description": "JQL query to select issues"},
					"output_dir": {"type": "string", "description": "Directory to write markdown/html exports (default: tasks/jira-export)"},
					"max_issues": {"type": "integer", "description": "Max issues to export (default: 50, max: 200)", "default": 50},
					"fields": {"type": "array", "items": {"type": "string"}, "description": "Jira fields to request (default includes summary/status/description)"},
					"expand": {"type": "array", "items": {"type": "string"}, "description": "Jira expand parameters"},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override Jira base URL (if omitted, uses env/client config)"},
					"api_version": {"type": "integer", "description": "Override Jira REST API version (2 or 3)", "enum": [2,3]},
					"include_confluence": {"type": "boolean", "description": "Whether to expand Confluence links found in descriptions (default: true)", "default": true},
					"confluence_base_url": {"type": "string", "description": "Override Confluence base URL for link expansion (if omitted, uses env/client config)"}
				},
				"required": ["jql"]
			}`),
		},
		{
			Name:        "jira_add_comment",
			Description: "Add a comment to an issue (MUTATING; blocked by default policy). For API v3, the comment is sent as ADF; for v2, plain text.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"body": {"type": "string", "description": "Comment text"},
					"format": {"type": "string", "description": "text (default) or adf", "enum": ["text","adf"], "default": "text"},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue","body"]
			}`),
		},
		{
			Name:        "jira_transition_issue",
			Description: "Transition an issue in the workflow (MUTATING; blocked by default policy).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"transition_id": {"type": "string", "description": "Transition id (from jira_get_issue_transitions)"},
					"comment": {"type": "string", "description": "Optional comment to add during transition"},
					"fields": {"type": "object", "description": "Optional fields to set"},
					"update": {"type": "object", "description": "Optional update operations"},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue","transition_id"]
			}`),
		},
		{
			Name:        "jira_create_issue",
			Description: "Create an issue (MUTATING; blocked by default policy).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"fields": {"type": "object", "description": "Issue fields for create. Example: {\"project\":{\"key\":\"PROJ\"},\"issuetype\":{\"name\":\"Task\"},\"summary\":\"...\"}"},
					"update": {"type": "object", "description": "Optional update operations"},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["fields"]
			}`),
		},
		{
			Name:        "jira_update_issue",
			Description: "Update an issue (MUTATING; blocked by default policy).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"fields": {"type": "object", "description": "Fields to set"},
					"update": {"type": "object", "description": "Update operations"},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue"]
			}`),
		},
		{
			Name:        "jira_add_attachment",
			Description: "Add attachment to an issue (MUTATING; blocked by default policy). Requires local file_path; uses multipart/form-data and X-Atlassian-Token: no-check.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"issue": {"type": "string", "description": "Issue key or id (e.g., PROJ-123)"},
					"file_path": {"type": "string", "description": "Local file path to upload"},
					"client": {"type": "string", "description": "Jira client alias (key in JIRA_CLIENTS_JSON). If omitted, uses JIRA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"api_version": {"type": "integer", "description": "REST API version (2 or 3). Default: 2", "enum": [2,3], "default": 2}
				},
				"required": ["issue","file_path"]
			}`),
		},
		{
			Name:        "confluence_list_spaces",
			Description: "List Confluence spaces (read-only). Uses Cloud v2 (/wiki/api/v2/spaces) when available; falls back to v1 (/rest/api/space).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL (e.g., https://your-site.atlassian.net or https://your-site.atlassian.net/wiki or https://confluence.company.com). If omitted, uses env."},
					"use_v2": {"type": "boolean", "description": "Prefer Cloud REST API v2 when available (default: true).", "default": true},
					"limit": {"type": "integer", "description": "Max results per page (default: 25, max: 250).", "default": 25},
					"cursor": {"type": "string", "description": "Pagination cursor (Cloud v2)."},
					"start": {"type": "integer", "description": "Pagination start offset (v1).", "default": 0}
				}
			}`),
		},
		{
			Name:        "confluence_get_page",
			Description: "Get Confluence page by id (read-only). For Cloud uses v2 when body_format=storage; otherwise uses v1 with expand=body.*.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "string", "description": "Page/content id."},
					"body_format": {"type": "string", "description": "Body representation for v1 expand. For Cloud v2 only storage is supported here.", "enum": ["storage","view","export_view"], "default": "storage"},
					"expand": {"type": "array", "items": {"type": "string"}, "description": "Additional v1 expand fields (e.g., [\"history\",\"ancestors\"])."},
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"use_v2": {"type": "boolean", "description": "Prefer Cloud REST API v2 when available (default: true).", "default": true}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "confluence_get_page_by_title",
			Description: "Find a Confluence page by space_key + title (read-only). Uses v1 content endpoint with expand=body.*.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"space_key": {"type": "string", "description": "Space key (e.g., DOCS)."},
					"title": {"type": "string", "description": "Exact page title."},
					"body_format": {"type": "string", "description": "Body representation (storage/view/export_view).", "enum": ["storage","view","export_view"], "default": "storage"},
					"expand": {"type": "array", "items": {"type": "string"}, "description": "Additional expand fields."},
					"limit": {"type": "integer", "description": "Max results to return (default: 5, max: 25).", "default": 5},
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."}
				},
				"required": ["space_key","title"]
			}`),
		},
		{
			Name:        "confluence_search_cql",
			Description: "Search Confluence content using CQL (read-only). Uses v1 search endpoint (/rest/api/search). Supports cursor pagination when provided by Confluence Cloud.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"cql": {"type": "string", "description": "CQL query (e.g., \"type=page AND space=DOCS AND text ~ \\\"oncall\\\"\")."},
					"limit": {"type": "integer", "description": "Max results per page (default: 25, max: 250).", "default": 25},
					"cursor": {"type": "string", "description": "Pagination cursor (Cloud)."},
					"start": {"type": "integer", "description": "Pagination start offset (v1/DC).", "default": 0},
					"include_archived_spaces": {"type": "boolean", "description": "Whether to include archived spaces."},
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."}
				},
				"required": ["cql"]
			}`),
		},
		{
			Name:        "confluence_get_page_children",
			Description: "List child pages for a Confluence page id (read-only). Uses v1 /rest/api/content/{id}/child with pagination.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"page_id": {"type": "string", "description": "Parent page id"},
					"start": {"type": "integer", "description": "Pagination start offset (default: 0).", "default": 0},
					"limit": {"type": "integer", "description": "Page size (default: 25, max: 250).", "default": 25, "maximum": 250},
					"expand": {"type": "string", "description": "Optional v1 expand (default: page)."},
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."}
				},
				"required": ["page_id"]
			}`),
		},
		{
			Name:        "confluence_list_page_attachments",
			Description: "List attachments for a Confluence page id (read-only). Uses v1 /rest/api/content/{id}/child/attachment with pagination.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"page_id": {"type": "string", "description": "Parent page id"},
					"start": {"type": "integer", "description": "Pagination start offset (default: 0).", "default": 0},
					"limit": {"type": "integer", "description": "Page size (default: 25, max: 250).", "default": 25, "maximum": 250},
					"expand": {"type": "string", "description": "Optional v1 expand."},
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."}
				},
				"required": ["page_id"]
			}`),
		},
		{
			Name:        "confluence_download_attachment",
			Description: "Download a Confluence attachment by URL (read-only) and store it as an artifact. download_url must match base_url host.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"download_url": {"type": "string", "description": "Absolute or relative Confluence attachment download URL"},
					"name": {"type": "string", "description": "Optional filename hint (used for extension)"},
					"client": {"type": "string", "description": "Confluence client alias (key in CONFLUENCE_CLIENTS_JSON). If omitted, uses CONFLUENCE_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."}
				},
				"required": ["download_url"]
			}`),
		},
		{
			Name:        "confluence_xhtml_to_text",
			Description: "Convert Confluence storage XHTML (body.storage.value) into readable plain text (read-only, offline).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"xhtml": {"type": "string", "description": "Confluence storage XHTML to convert (e.g., body.storage.value)."},
					"preserve_links": {"type": "boolean", "description": "Append link targets as '(url)' after anchor text.", "default": false},
					"max_chars": {"type": "integer", "description": "Max output characters (0 = no limit). Truncates on rune boundary.", "default": 20000, "minimum": 0, "maximum": 2000000}
				},
				"required": ["xhtml"]
			}`),
		},
		{
			Name:        "grafana_health",
			Description: "Check Grafana health (read-only). Calls GET /api/health. This can work without auth on some Grafana instances.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL (e.g., https://grafana.company.com). If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_get_current_user",
			Description: "Get the current Grafana user (read-only). Calls GET /api/user. Use this to validate authentication/permissions.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_search",
			Description: "Search Grafana folders and dashboards (read-only). Calls GET /api/search with filters and pagination (page/limit).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "Search query string."},
					"type": {"type": "string", "description": "Result type filter.", "enum": ["dash-db", "dash-folder"]},
					"tags": {"type": "array", "items": {"type": "string"}, "description": "Filter by Grafana tags (AND when multiple)."},
					"folder_uids": {"type": "array", "items": {"type": "string"}, "description": "Only search in these folder UIDs."},
					"dashboard_uids": {"type": "array", "items": {"type": "string"}, "description": "Only return these dashboard UIDs."},
					"starred": {"type": "boolean", "description": "Only starred dashboards."},
					"limit": {"type": "integer", "description": "Page size (default: 100; max server-side depends on Grafana).", "default": 100},
					"page": {"type": "integer", "description": "Page number (starts at 1).", "default": 1},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_get_dashboard",
			Description: "Get a Grafana dashboard by UID (read-only). Calls GET /api/dashboards/uid/:uid.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"uid": {"type": "string", "description": "Dashboard UID."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				},
				"required": ["uid"]
			}`),
		},
		{
			Name:        "grafana_get_dashboard_summary",
			Description: "Get a compact summary of a Grafana dashboard by UID or URL (read-only). Fetches /api/dashboards/uid/:uid and extracts panels/queries/variables to keep output small.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"uid": {"type": "string", "description": "Dashboard UID. Either uid or url is required."},
					"url": {"type": "string", "description": "Grafana dashboard URL (e.g. https://grafana.example.com/d/<uid>/...). If provided, uid/base_url/org_id can be inferred."},
					"max_panels": {"type": "integer", "description": "Max panels to include (default: 200).", "default": 200},
					"max_targets_per_panel": {"type": "integer", "description": "Max targets (queries) per panel to include (default: 20).", "default": 20},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env (or inferred from url)."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header). If omitted, may be inferred from url orgId query param."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_list_folders",
			Description: "List Grafana folders (read-only). Calls GET /api/folders with pagination (page/limit) and optional parent_uid (nested folders).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"limit": {"type": "integer", "description": "Page size (default: 1000).", "default": 1000},
					"page": {"type": "integer", "description": "Page number (starts at 1).", "default": 1},
					"parent_uid": {"type": "string", "description": "Parent folder UID (nested folders)."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_get_folder",
			Description: "Get a Grafana folder by UID (read-only). Calls GET /api/folders/:uid.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"uid": {"type": "string", "description": "Folder UID."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				},
				"required": ["uid"]
			}`),
		},
		{
			Name:        "grafana_list_datasources",
			Description: "List Grafana data sources (read-only). Calls GET /api/datasources.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_get_datasource",
			Description: "Get a Grafana data source by uid or name (read-only). Calls GET /api/datasources/uid/:uid or GET /api/datasources/name/:name.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"uid": {"type": "string", "description": "Data source UID."},
					"name": {"type": "string", "description": "Data source name."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_query_annotations",
			Description: "Query Grafana annotations (read-only). Calls GET /api/annotations. Supports time range, tags, dashboard_uid, panel_id, and other filters.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"from": {"type": "integer", "description": "Epoch milliseconds from (optional)."},
					"to": {"type": "integer", "description": "Epoch milliseconds to (optional)."},
					"limit": {"type": "integer", "description": "Max results (default: 100).", "default": 100},
					"alert_id": {"type": "integer", "description": "Filter by alert rule ID (deprecated in Grafana; prefer alert_uid)."},
					"alert_uid": {"type": "string", "description": "Filter by alert rule UID."},
					"dashboard_uid": {"type": "string", "description": "Filter by dashboard UID."},
					"panel_id": {"type": "integer", "description": "Filter by panel ID."},
					"user_id": {"type": "integer", "description": "Filter by user ID."},
					"type": {"type": "string", "description": "Return alerts or user annotations.", "enum": ["alert", "annotation"]},
					"tags": {"type": "array", "items": {"type": "string"}, "description": "Filter organization annotations by tags (AND when multiple)."},
					"match_any": {"type": "boolean", "description": "Match any tag (OR) instead of AND when tags are provided."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_list_annotation_tags",
			Description: "List Grafana annotation tags (read-only). Calls GET /api/annotations/tags.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"tag": {"type": "string", "description": "Filter tag prefix/string."},
					"limit": {"type": "integer", "description": "Max results (default: 100).", "default": 100},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_list_alerts",
			Description: "List Grafana alerts (legacy alerting) (read-only). Calls GET /api/alerts.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"state": {"type": "array", "items": {"type": "string"}, "description": "Filter by alert state(s), e.g. [\"alerting\",\"ok\",\"paused\"]."},
					"query": {"type": "string", "description": "Filter by alert name like."},
					"dashboard_id": {"type": "array", "items": {"type": "integer"}, "description": "Filter by dashboard id(s)."},
					"folder_id": {"type": "array", "items": {"type": "integer"}, "description": "Filter by folder id(s)."},
					"limit": {"type": "integer", "description": "Max results."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_get_alert",
			Description: "Get a Grafana alert by id (legacy alerting) (read-only). Calls GET /api/alerts/:id.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"id": {"type": "integer", "description": "Alert id."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				},
				"required": ["id"]
			}`),
		},
		{
			Name:        "grafana_list_alert_rules",
			Description: "List Grafana alert rules (unified alerting). Tries provisioning API first, falls back to ruler API.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				}
			}`),
		},
		{
			Name:        "grafana_get_alert_rule",
			Description: "Get Grafana alert rule by UID via provisioning API (read-only).",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"uid": {"type": "string", "description": "Alert rule UID."},
					"client": {"type": "string", "description": "Grafana client alias (key in GRAFANA_CLIENTS_JSON). If omitted, uses GRAFANA_DEFAULT_CLIENT."},
					"base_url": {"type": "string", "description": "Override base URL. If omitted, uses env."},
					"org_id": {"type": "integer", "description": "Override organization id (adds X-Grafana-Org-Id header)."},
					"cf_access_client_id": {"type": "string", "description": "Cloudflare Access client id header (CF-Access-Client-Id) override."},
					"cf_access_client_secret": {"type": "string", "description": "Cloudflare Access client secret header (CF-Access-Client-Secret) override."},
					"timeout_ms": {"type": "integer", "description": "HTTP timeout override (ms)."},
					"user_agent": {"type": "string", "description": "Override User-Agent header."}
				},
				"required": ["uid"]
			}`),
		},
		{
			Name:        "router",
			Description: "(Internal) Planning router used by this proxy. Most MCP clients should call the `query` tool instead. `router` and `query` share the same input/output.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"input": {"type": "string", "description": "User request / task (free-form). Use this when you don't know which tool to call."},
					"context": {"type": "object", "description": "Optional structured context"},
					"mode": {"type": "string", "description": "Router mode. auto=plan+execute (default), planner=plan only, executor=execute provided steps only", "enum": ["auto", "planner", "executor"], "default": "auto"},
					"steps": {
						"type": "array",
						"description": "Optional explicit execution plan (executor mode). If provided, the router will validate + execute these steps without calling the planner model.",
						"items": {
							"type": "object",
							"properties": {
								"name": {"type": "string"},
								"source": {"type": "string", "enum": ["local", "upstream"]},
								"args": {"type": "object"},
								"reason": {"type": "string"},
								"parallel_group": {"type": "string", "description": "Optional group id for parallel execution (requires parallelism > 1)."}
							},
							"required": ["name", "source", "args"]
						}
					},
					"output": {
						"type": "object",
						"description": "Optional output shaping for JSON-heavy tool results (applied to executed_steps[].result). Backwards compatible: omit to keep full outputs.",
						"properties": {
							"view": {"type": "string", "description": "View preset", "enum": ["full", "summary", "metadata", "errors_only"], "default": "full"},
							"include_fields": {"type": "array", "items": {"type": "string"}, "description": "Whitelist of fields/paths to keep (e.g. 'a.b[0].c' or '/a/b/0/c')"},
							"exclude_fields": {"type": "array", "items": {"type": "string"}, "description": "Blacklist of fields/paths to remove"},
							"max_items": {"type": "integer", "description": "Max items for arrays (applied recursively)"},
							"max_depth": {"type": "integer", "description": "Max nesting depth (objects/arrays deeper than this are replaced with '<truncated>')"},
							"redact": {"type": "array", "items": {"type": "string"}, "description": "Paths to redact (replaced with '[REDACTED]')"}
						}
					},
					"max_steps": {"type": "integer", "description": "Max steps (default: 5, max: 8)", "default": 5},
					"parallelism": {"type": "integer", "description": "Max parallelism for steps with the same parallel_group (default: 1).", "default": 1, "minimum": 1, "maximum": 8},
					"include_answer": {"type": "boolean", "description": "Also produce a final human-readable answer", "default": false},
					"dry_run": {"type": "boolean", "description": "Return plan only; do not execute tools", "default": false},
					"format": {"type": "string", "description": "Output format", "enum": ["json", "text"], "default": "json"}
				},
				"required": ["input"]
			}`),
		},
		{
			Name:        "query",
			Description: "Single entrypoint for MCP clients. Provide a free-form request and the proxy will (1) plan up to max_steps, (2) validate the plan against a read-only policy, and (3) execute the necessary local/upstream tools. Use this whenever you're not sure which tool to call.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"input": {"type": "string", "description": "User request / task (free-form)"},
					"context": {"type": "object", "description": "Optional structured context"},
					"mode": {"type": "string", "description": "Router mode. auto=plan+execute (default), planner=plan only, executor=execute provided steps only", "enum": ["auto", "planner", "executor"], "default": "auto"},
					"steps": {
						"type": "array",
						"description": "Optional explicit execution plan (executor mode). If provided, the router will validate + execute these steps without calling the planner model.",
						"items": {
							"type": "object",
							"properties": {
								"name": {"type": "string"},
								"source": {"type": "string", "enum": ["local", "upstream"]},
								"args": {"type": "object"},
								"reason": {"type": "string"},
								"parallel_group": {"type": "string", "description": "Optional group id for parallel execution (requires parallelism > 1)."}
							},
							"required": ["name", "source", "args"]
						}
					},
					"output": {
						"type": "object",
						"description": "Optional output shaping for JSON-heavy tool results (applied to executed_steps[].result). Backwards compatible: omit to keep full outputs.",
						"properties": {
							"view": {"type": "string", "description": "View preset", "enum": ["full", "summary", "metadata", "errors_only"], "default": "full"},
							"include_fields": {"type": "array", "items": {"type": "string"}, "description": "Whitelist of fields/paths to keep (e.g. 'a.b[0].c' or '/a/b/0/c')"},
							"exclude_fields": {"type": "array", "items": {"type": "string"}, "description": "Blacklist of fields/paths to remove"},
							"max_items": {"type": "integer", "description": "Max items for arrays (applied recursively)"},
							"max_depth": {"type": "integer", "description": "Max nesting depth (objects/arrays deeper than this are replaced with '<truncated>')"},
							"redact": {"type": "array", "items": {"type": "string"}, "description": "Paths to redact (replaced with '[REDACTED]')"}
						}
					},
					"max_steps": {"type": "integer", "description": "Max steps (default: 5, max: 8)", "default": 5},
					"parallelism": {"type": "integer", "description": "Max parallelism for steps with the same parallel_group (default: 1).", "default": 1, "minimum": 1, "maximum": 8},
					"include_answer": {"type": "boolean", "description": "Also produce a final human-readable answer", "default": false},
					"dry_run": {"type": "boolean", "description": "Return plan only; do not execute tools", "default": false},
					"format": {"type": "string", "description": "Output format", "enum": ["json", "text"], "default": "json"}
				},
				"required": ["input"]
			}`),
		},
	}

	// Append execution tools (execute_code, start_runtime, register_tool, etc.)
	tools = append(tools, ExecutionToolDefinitions()...)

	return tools
}

type SearchToolsInput struct {
	Query          string `json:"query"`
	Category       string `json:"category,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Format         string `json:"format,omitempty"` // "text" (default) or "json"
	IncludeSchemas bool   `json:"include_schemas,omitempty"`
}

type DescribeToolInput struct {
	Name string `json:"name"`
}

type ExecuteToolInput struct {
	Name   string          `json:"name"`
	Params json.RawMessage `json:"params"`
}

// Handle processes a local tool call (meta-tools + other proxy-provided tools).
func (h *Handler) Handle(ctx context.Context, name string, args json.RawMessage) (*mcp.CallToolResult, error) {
	switch name {
	case "search_tools":
		return h.handleSearch(args)
	case "describe_tool":
		return h.handleDescribe(args)
	case "execute_tool":
		return h.handleExecute(args)
	case "dev_scaffold_tool":
		return h.devScaffoldTool(ctx, args)
	case "artifact_save_text":
		return h.artifactSaveText(ctx, args)
	case "artifact_append_text":
		return h.artifactAppendText(ctx, args)
	case "artifact_list":
		return h.artifactList(ctx, args)
	case "artifact_search":
		return h.artifactSearch(ctx, args)
	case "router":
		return h.runRouter(ctx, args)
	case "query":
		return h.runRouter(ctx, args)
	case "get_pull_request_details":
		return h.getPullRequestDetails(ctx, args)
	case "list_pull_request_files":
		return h.listPullRequestFiles(ctx, args)
	case "get_pull_request_diff":
		return h.getPullRequestDiff(ctx, args)
	case "get_pull_request_summary":
		return h.getPullRequestSummary(ctx, args)
	case "get_pull_request_file_diff":
		return h.getPullRequestFileDiff(ctx, args)
	case "get_file_at_ref":
		return h.getFileAtRef(ctx, args)
	case "prepare_pull_request_review_bundle":
		return h.preparePullRequestReviewBundle(ctx, args)
	case "list_pull_request_commits":
		return h.listPullRequestCommits(ctx, args)
	case "get_pull_request_checks":
		return h.getPullRequestChecks(ctx, args)
	case "github_list_workflow_runs":
		return h.githubListWorkflowRuns(ctx, args)
	case "github_list_workflow_jobs":
		return h.githubListWorkflowJobs(ctx, args)
	case "github_download_job_logs":
		return h.githubDownloadJobLogs(ctx, args)
	case "fetch_complete_pr_diff":
		return h.fetchCompletePRDiff(ctx, args)
	case "fetch_complete_pr_files":
		return h.fetchCompletePRFiles(ctx, args)
	case "jira_get_myself":
		return h.jiraGetMyself(ctx, args)
	case "jira_get_issue":
		return h.jiraGetIssue(ctx, args)
	case "jira_get_issue_bundle":
		return h.jiraGetIssueBundle(ctx, args)
	case "jira_search_issues":
		return h.jiraSearchIssues(ctx, args)
	case "jira_get_issue_comments":
		return h.jiraGetIssueComments(ctx, args)
	case "jira_get_issue_transitions":
		return h.jiraGetIssueTransitions(ctx, args)
	case "jira_list_projects":
		return h.jiraListProjects(ctx, args)
	case "jira_export_tasks":
		return h.jiraExportTasks(ctx, args)
	case "jira_add_comment":
		return h.jiraAddComment(ctx, args)
	case "jira_transition_issue":
		return h.jiraTransitionIssue(ctx, args)
	case "jira_create_issue":
		return h.jiraCreateIssue(ctx, args)
	case "jira_update_issue":
		return h.jiraUpdateIssue(ctx, args)
	case "jira_add_attachment":
		return h.jiraAddAttachment(ctx, args)
	case "confluence_list_spaces":
		return h.confluenceListSpaces(ctx, args)
	case "confluence_get_page":
		return h.confluenceGetPage(ctx, args)
	case "confluence_get_page_by_title":
		return h.confluenceGetPageByTitle(ctx, args)
	case "confluence_search_cql":
		return h.confluenceSearchCQL(ctx, args)
	case "confluence_get_page_children":
		return h.confluenceGetPageChildren(ctx, args)
	case "confluence_list_page_attachments":
		return h.confluenceListPageAttachments(ctx, args)
	case "confluence_download_attachment":
		return h.confluenceDownloadAttachment(ctx, args)
	case "confluence_xhtml_to_text":
		return h.confluenceXhtmlToText(ctx, args)
	case "grafana_health":
		return h.grafanaHealth(ctx, args)
	case "grafana_get_current_user":
		return h.grafanaGetCurrentUser(ctx, args)
	case "grafana_search":
		return h.grafanaSearch(ctx, args)
	case "grafana_get_dashboard":
		return h.grafanaGetDashboard(ctx, args)
	case "grafana_get_dashboard_summary":
		return h.grafanaGetDashboardSummary(ctx, args)
	case "grafana_list_folders":
		return h.grafanaListFolders(ctx, args)
	case "grafana_get_folder":
		return h.grafanaGetFolder(ctx, args)
	case "grafana_list_datasources":
		return h.grafanaListDatasources(ctx, args)
	case "grafana_get_datasource":
		return h.grafanaGetDatasource(ctx, args)
	case "grafana_query_annotations":
		return h.grafanaQueryAnnotations(ctx, args)
	case "grafana_list_annotation_tags":
		return h.grafanaListAnnotationTags(ctx, args)
	case "grafana_list_alerts":
		return h.grafanaListAlerts(ctx, args)
	case "grafana_get_alert":
		return h.grafanaGetAlert(ctx, args)
	case "grafana_list_alert_rules":
		return h.grafanaListAlertRules(ctx, args)
	case "grafana_get_alert_rule":
		return h.grafanaGetAlertRule(ctx, args)
	// Execution tools (Stage 5 integration)
	case "list_adapters":
		return h.listAdapters(ctx, args)
	case "execute_code":
		return h.executeCode(ctx, args)
	case "start_runtime":
		return h.startRuntime(ctx, args)
	case "register_tool":
		return h.registerTool(ctx, args)
	case "rollback_tool":
		return h.rollbackTool(ctx, args)
	case "discover_api":
		return h.discoverAPI(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (h *Handler) IsMetaTool(name string) bool {
	return name == "search_tools" || name == "describe_tool" || name == "execute_tool"
}

func (h *Handler) IsLocalTool(name string) bool {
	switch name {
	case "router", "query",
		"dev_scaffold_tool",
		"artifact_save_text", "artifact_append_text", "artifact_list", "artifact_search",
		"get_pull_request_details", "list_pull_request_files", "get_pull_request_diff", "get_pull_request_summary", "get_pull_request_file_diff", "get_file_at_ref", "prepare_pull_request_review_bundle", "list_pull_request_commits", "get_pull_request_checks", "fetch_complete_pr_diff", "fetch_complete_pr_files",
		"github_list_workflow_runs", "github_list_workflow_jobs", "github_download_job_logs",
		"jira_get_myself", "jira_get_issue", "jira_get_issue_bundle", "jira_search_issues", "jira_get_issue_comments", "jira_get_issue_transitions", "jira_list_projects", "jira_export_tasks",
		"jira_add_comment", "jira_transition_issue", "jira_create_issue", "jira_update_issue", "jira_add_attachment",
		"confluence_list_spaces", "confluence_get_page", "confluence_get_page_by_title", "confluence_search_cql", "confluence_get_page_children", "confluence_list_page_attachments", "confluence_download_attachment", "confluence_xhtml_to_text",
		"grafana_health", "grafana_get_current_user", "grafana_search", "grafana_get_dashboard", "grafana_get_dashboard_summary", "grafana_list_folders", "grafana_get_folder", "grafana_list_datasources", "grafana_get_datasource", "grafana_query_annotations", "grafana_list_annotation_tags", "grafana_list_alerts", "grafana_get_alert", "grafana_list_alert_rules", "grafana_get_alert_rule",
		// Execution tools (Stage 5 integration)
		"list_adapters", "execute_code", "start_runtime", "register_tool", "rollback_tool", "discover_api":
		return true
	default:
		return false
	}
}

func (h *Handler) handleSearch(args json.RawMessage) (*mcp.CallToolResult, error) {
	var input SearchToolsInput
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if input.Query == "" {
		return errorResult("Query is required"), nil
	}

	results := h.registry.Search(input.Query, input.Category, input.Limit)

	// Also search local tools (so PR-review tools are discoverable even if upstream lacks them).
	local := searchLocalTools(input.Query, input.Category, input.Limit)

	// Merge (dedupe by name).
	seen := map[string]struct{}{}
	merged := make([]mcp.ToolSummary, 0, len(results)+len(local))
	for _, r := range append(local, results...) {
		if _, ok := seen[r.Name]; ok {
			continue
		}
		seen[r.Name] = struct{}{}
		merged = append(merged, r)
		if input.Limit > 0 && len(merged) >= input.Limit {
			break
		}
	}

	if strings.EqualFold(input.Format, "json") {
		payload := map[string]any{
			"query":    input.Query,
			"category": input.Category,
			"count":    len(merged),
			"tools": func() []any {
				out := make([]any, 0, len(merged))
				for _, s := range merged {
					item := map[string]any{
						"name":        s.Name,
						"category":    s.Category,
						"description": s.Description,
					}
					if input.IncludeSchemas {
						if t, ok := h.registry.GetTool(s.Name); ok {
							var schema any
							_ = json.Unmarshal(t.InputSchema, &schema)
							item["inputSchema"] = schema
						}
					}
					out = append(out, item)
				}
				return out
			}(),
		}
		return jsonResult(payload), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d tools matching '%s':\n\n", len(merged), input.Query))
	for i, r := range merged {
		sb.WriteString(fmt.Sprintf("%d. **%s** [%s]\n   %s\n\n", i+1, r.Name, r.Category, r.Description))
	}

	if len(merged) == 0 {
		sb.WriteString("No tools found. Try a different query or browse categories:\n")
		for _, cat := range h.registry.ListCategories() {
			sb.WriteString(fmt.Sprintf("- %s: %s\n", cat.Name, cat.Description))
		}
		sb.WriteString("- local: Proxy-provided read-only helpers\n")
	}

	return textResult(sb.String()), nil
}

func expandQuery(q string) []string {
	// Very small intent/synonym expansion for review workflows.
	// Keep this simple and deterministic.
	terms := []string{q}

	add := func(ts ...string) {
		terms = append(terms, ts...)
	}

	if strings.Contains(q, "pr") || strings.Contains(q, "pull") || strings.Contains(q, "pull request") {
		add("pull_request", "pull_request_read", "list_pull_request_files", "diff", "patch", "files")
	}
	if strings.Contains(q, "diff") || strings.Contains(q, "patch") || strings.Contains(q, "files changed") || strings.Contains(q, "changed files") {
		add("diff", "patch", "files", "list_pull_request_files")
	}
	if strings.Contains(q, "review") || strings.Contains(q, "approve") || strings.Contains(q, "comment") {
		add("review", "pull_request_review", "comment")
	}
	if strings.Contains(q, "jira") || strings.Contains(q, "jql") || strings.Contains(q, "ticket") || strings.Contains(q, "issue") {
		add("jira", "jql", "issue", "ticket", "search", "comment", "transition", "project", "export")
	}
	if strings.Contains(q, "confluence") || strings.Contains(q, "wiki") || strings.Contains(q, "cql") || strings.Contains(q, "space") || strings.Contains(q, "page") {
		add("confluence", "wiki", "cql", "search", "space", "page", "content", "title", "xhtml", "storage", "convert", "text")
	}
	if strings.Contains(q, "grafana") || strings.Contains(q, "dashboard") || strings.Contains(q, "datasource") || strings.Contains(q, "folder") || strings.Contains(q, "annotation") {
		add("grafana", "dashboard", "dashboards", "folder", "folders", "search", "datasource", "data source", "annotations")
	}
	if strings.Contains(q, "scaffold") || strings.Contains(q, "generate") || strings.Contains(q, "template") || strings.Contains(q, "patch") || strings.Contains(q, "worktree") {
		add("scaffold", "generate", "template", "patch", "worktree", "dev")
	}

	// Dedupe
	seen := map[string]struct{}{}
	out := make([]string, 0, len(terms))
	for _, t := range terms {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func searchLocalTools(query string, category string, limit int) []mcp.ToolSummary {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}

	expanded := expandQuery(q)

	tools := []mcp.ToolSummary{
		func() mcp.ToolSummary {
			if devModeEnabled() {
				return mcp.ToolSummary{Name: "dev_scaffold_tool", Category: "local", Description: "(Dev mode) Generate patch + worktree for new local tools."}
			}
			return mcp.ToolSummary{}
		}(),
		{Name: "artifact_save_text", Category: "local", Description: "Save text as artifact (plan/notes)."},
		{Name: "artifact_append_text", Category: "local", Description: "Append text to artifact (creates new artifact)."},
		{Name: "artifact_list", Category: "local", Description: "List recent artifacts."},
		{Name: "artifact_search", Category: "local", Description: "Search recent artifacts."},
		{Name: "get_pull_request_details", Category: "local", Description: "PR metadata (title, base/head, author, state)."},
		{Name: "list_pull_request_files", Category: "local", Description: "Changed files list with pagination."},
		{Name: "get_pull_request_diff", Category: "local", Description: "Unified PR diff in chunks (offset/max_bytes). Supports file_filter for glob patterns."},
		{Name: "get_pull_request_summary", Category: "local", Description: "Compact PR summary with statistics, file types, directories. Use first to understand PR scope."},
		{Name: "get_pull_request_file_diff", Category: "local", Description: "Diff for a single specific file in PR."},
		{Name: "list_pull_request_commits", Category: "local", Description: "PR commits list with pagination."},
		{Name: "get_pull_request_checks", Category: "local", Description: "Check-runs for PR head sha."},
		{Name: "get_file_at_ref", Category: "local", Description: "Raw file contents at a git ref."},
		{Name: "github_list_workflow_runs", Category: "local", Description: "List GitHub Actions workflow runs (CI context)."},
		{Name: "github_list_workflow_jobs", Category: "local", Description: "List jobs for a workflow run (CI context)."},
		{Name: "github_download_job_logs", Category: "local", Description: "Download job logs and save as artifact (CI debugging)."},
		{Name: "prepare_pull_request_review_bundle", Category: "local", Description: "PR details + file list (+ optional diff chunk/commits/checks) in one call."},
		{Name: "fetch_complete_pr_diff", Category: "local", Description: "Fetches COMPLETE PR diff (all parts) and saves to file. Use for comprehensive reviews."},
		{Name: "fetch_complete_pr_files", Category: "local", Description: "Fetches COMPLETE list of all changed files (all pages) and saves to file."},
		{Name: "jira_get_myself", Category: "local", Description: "Authenticated Jira user info (auth validation)."},
		{Name: "jira_get_issue", Category: "local", Description: "Get Jira issue by key/id (fields/expand supported)."},
		{Name: "jira_get_issue_bundle", Category: "local", Description: "Get Jira issue + optional comments/changelog (faster ticket context)."},
		{Name: "jira_search_issues", Category: "local", Description: "Search Jira issues by JQL with pagination (startAt/maxResults)."},
		{Name: "jira_get_issue_comments", Category: "local", Description: "List Jira issue comments with pagination."},
		{Name: "jira_get_issue_transitions", Category: "local", Description: "List available Jira workflow transitions for an issue."},
		{Name: "jira_list_projects", Category: "local", Description: "List Jira projects (v3 paged /project/search; v2 /project)."},
		{Name: "jira_export_tasks", Category: "local", Description: "Export Jira issues by JQL to local markdown files, expanding known links (e.g. Confluence pages)."},
		{Name: "jira_add_comment", Category: "local", Description: "Add Jira issue comment (mutating; blocked by default policy)."},
		{Name: "jira_transition_issue", Category: "local", Description: "Transition Jira issue (mutating; blocked by default policy)."},
		{Name: "jira_create_issue", Category: "local", Description: "Create Jira issue (mutating; blocked by default policy)."},
		{Name: "jira_update_issue", Category: "local", Description: "Update Jira issue (mutating; blocked by default policy)."},
		{Name: "jira_add_attachment", Category: "local", Description: "Add Jira issue attachment (mutating; blocked by default policy)."},
		{Name: "confluence_list_spaces", Category: "local", Description: "List Confluence spaces (Cloud v2 preferred; v1 fallback)."},
		{Name: "confluence_get_page", Category: "local", Description: "Get Confluence page by id (v2 storage preferred; v1 fallback)."},
		{Name: "confluence_get_page_by_title", Category: "local", Description: "Find Confluence page by space_key + title."},
		{Name: "confluence_search_cql", Category: "local", Description: "Search Confluence using CQL with pagination."},
		{Name: "confluence_get_page_children", Category: "local", Description: "List child pages for a Confluence page id."},
		{Name: "confluence_list_page_attachments", Category: "local", Description: "List attachments for a Confluence page id."},
		{Name: "confluence_download_attachment", Category: "local", Description: "Download attachment and save to artifact."},
		{Name: "confluence_xhtml_to_text", Category: "local", Description: "Convert Confluence storage XHTML to plain text (offline)."},
		{Name: "grafana_health", Category: "local", Description: "Grafana health check (/api/health)."},
		{Name: "grafana_get_current_user", Category: "local", Description: "Current Grafana user (/api/user) to validate auth."},
		{Name: "grafana_search", Category: "local", Description: "Search Grafana folders/dashboards (/api/search) with pagination."},
		{Name: "grafana_get_dashboard", Category: "local", Description: "Get Grafana dashboard by uid (/api/dashboards/uid/:uid)."},
		{Name: "grafana_get_dashboard_summary", Category: "local", Description: "Compact dashboard summary (panels/queries/variables) by uid or URL."},
		{Name: "grafana_list_folders", Category: "local", Description: "List Grafana folders (/api/folders) with pagination."},
		{Name: "grafana_get_folder", Category: "local", Description: "Get Grafana folder by uid (/api/folders/:uid)."},
		{Name: "grafana_list_datasources", Category: "local", Description: "List Grafana datasources (/api/datasources)."},
		{Name: "grafana_get_datasource", Category: "local", Description: "Get Grafana datasource by uid or name."},
		{Name: "grafana_query_annotations", Category: "local", Description: "Query Grafana annotations (/api/annotations)."},
		{Name: "grafana_list_annotation_tags", Category: "local", Description: "List Grafana annotation tags (/api/annotations/tags)."},
		{Name: "grafana_list_alerts", Category: "local", Description: "List Grafana alerts (legacy alerting) (/api/alerts)."},
		{Name: "grafana_get_alert", Category: "local", Description: "Get Grafana alert by id (legacy alerting) (/api/alerts/:id)."},
		{Name: "grafana_list_alert_rules", Category: "local", Description: "List Grafana alert rules (unified alerting)."},
		{Name: "grafana_get_alert_rule", Category: "local", Description: "Get Grafana alert rule by uid (provisioning API)."},
	}

	if len(tools) > 0 && tools[0].Name == "" {
		tools = tools[1:]
	}

	// Basic scoring.
	type scored struct {
		s     mcp.ToolSummary
		score int
	}
	var scoredRes []scored

	for _, t := range tools {
		if category != "" && !strings.EqualFold(category, t.Category) {
			continue
		}
		nameLower := strings.ToLower(t.Name)
		descLower := strings.ToLower(t.Description)
		score := 0
		for _, term := range expanded {
			if strings.Contains(nameLower, term) {
				score += 100
			}
			if strings.Contains(descLower, term) {
				score += 30
			}
		}
		if score > 0 {
			scoredRes = append(scoredRes, scored{s: t, score: score})
		}
	}

	for i := 0; i < len(scoredRes); i++ {
		for j := i + 1; j < len(scoredRes); j++ {
			if scoredRes[j].score > scoredRes[i].score {
				scoredRes[i], scoredRes[j] = scoredRes[j], scoredRes[i]
			}
		}
	}

	if limit <= 0 {
		limit = 10
	}
	out := make([]mcp.ToolSummary, 0, limit)
	for i := 0; i < len(scoredRes) && i < limit; i++ {
		out = append(out, scoredRes[i].s)
	}
	return out
}

func (h *Handler) handleDescribe(args json.RawMessage) (*mcp.CallToolResult, error) {
	var input DescribeToolInput
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if input.Name == "" {
		return errorResult("Tool name is required"), nil
	}

	// Local tools first.
	for _, t := range h.BuiltinTools() {
		if t.Name == input.Name {
			return formatTool(t), nil
		}
	}

	tool, ok := h.registry.GetTool(input.Name)
	if !ok {
		return errorResult(fmt.Sprintf("Tool '%s' not found. Use search_tools to find available tools.", input.Name)), nil
	}
	return formatTool(tool), nil
}

func formatTool(tool mcp.Tool) *mcp.CallToolResult {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s\n\n", tool.Name))
	sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", tool.Description))
	sb.WriteString("**Input Schema:**\n```json\n")

	var prettySchema map[string]any
	_ = json.Unmarshal(tool.InputSchema, &prettySchema)
	schemaBytes, _ := json.MarshalIndent(prettySchema, "", "  ")
	sb.Write(schemaBytes)
	sb.WriteString("\n```\n")

	return textResult(sb.String())
}

func (h *Handler) handleExecute(args json.RawMessage) (*mcp.CallToolResult, error) {
	var input ExecuteToolInput
	if err := json.Unmarshal(args, &input); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if input.Name == "" {
		return errorResult("Tool name is required"), nil
	}

	_, ok := h.registry.GetTool(input.Name)
	if !ok {
		return errorResult(fmt.Sprintf("Tool '%s' not found or not an upstream tool. Use search_tools to find available tools.", input.Name)), nil
	}

	// Activate tool so it appears in tools/list for the session.
	h.registry.Activate(input.Name)

	return h.executor(input.Name, input.Params)
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: text}}}
}

func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: "Error: " + msg}}, IsError: true}
}
