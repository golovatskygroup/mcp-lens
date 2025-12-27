package router

import (
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/envutil"
)

type Policy struct {
	AllowLocal    map[string]struct{}
	AllowUpstream map[string]struct{}
}

func DefaultPolicy() Policy {
	allowLocal := map[string]struct{}{
		// Note: search_tools is explicitly NOT allowed - router must use explicit ToolCatalog
		"describe_tool": {},
		"execute_tool":  {},
		// Code execution tools (Stage 5 integration)
		"execute_code":  {},
		"start_runtime": {},
		"register_tool": {},
		"rollback_tool": {},
		"discover_api":  {},
		"list_adapters": {},
		// Artifact tools
		"artifact_save_text":                 {},
		"artifact_append_text":               {},
		"artifact_list":                      {},
		"artifact_search":                    {},
		"get_pull_request_details":           {},
		"list_pull_request_files":            {},
		"get_pull_request_diff":              {},
		"get_pull_request_summary":           {},
		"get_pull_request_file_diff":         {},
		"list_pull_request_commits":          {},
		"get_pull_request_checks":            {},
		"get_file_at_ref":                    {},
		"prepare_pull_request_review_bundle": {},
		"github_list_workflow_runs":          {},
		"github_list_workflow_jobs":          {},
		"github_download_job_logs":           {},
		"fetch_complete_pr_diff":             {},
		"fetch_complete_pr_files":            {},
		"jira_get_myself":                    {},
		"jira_get_issue":                     {},
		"jira_get_issue_bundle":              {},
		"jira_search_issues":                 {},
		"jira_get_issue_comments":            {},
		"jira_get_issue_transitions":         {},
		"jira_list_projects":                 {},
		"jira_export_tasks":                  {},
		"confluence_list_spaces":             {},
		"confluence_get_page":                {},
		"confluence_get_page_by_title":       {},
		"confluence_search_cql":              {},
		"confluence_get_page_children":       {},
		"confluence_list_page_attachments":   {},
		"confluence_download_attachment":     {},
		"confluence_xhtml_to_text":           {},
		"grafana_health":                     {},
		"grafana_get_current_user":           {},
		"grafana_search":                     {},
		"grafana_get_dashboard":              {},
		"grafana_get_dashboard_summary":      {},
		"grafana_list_folders":               {},
		"grafana_get_folder":                 {},
		"grafana_list_datasources":           {},
		"grafana_get_datasource":             {},
		"grafana_query_annotations":          {},
		"grafana_list_annotation_tags":       {},
		"grafana_list_alerts":                {},
		"grafana_get_alert":                  {},
		"grafana_list_alert_rules":           {},
		"grafana_get_alert_rule":             {},
	}

	if envutil.DevModeEnabled() {
		allowLocal["dev_scaffold_tool"] = struct{}{}
	}

	// Start conservative: empty upstream allowlist.
	return Policy{AllowLocal: allowLocal, AllowUpstream: map[string]struct{}{}}
}

// IsAllowed checks if a tool is allowed by policy.
// Explicitly blocks tool discovery tools (search_tools, list_tools) to enforce explicit catalog usage.
func (p Policy) IsAllowed(source string, toolName string) bool {
	nameLower := strings.ToLower(strings.TrimSpace(toolName))
	if nameLower == "" {
		return false
	}
	
	// Explicitly block tool discovery/introspection tools
	// Router must use explicit ToolCatalog instead of runtime discovery
	if isDiscoveryTool(nameLower) {
		return false
	}
	
	if isMutatingName(nameLower) {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(source)) {
	case "local":
		_, ok := p.AllowLocal[toolName]
		return ok
	case "upstream":
		_, ok := p.AllowUpstream[toolName]
		return ok
	default:
		return false
	}
}

// isDiscoveryTool checks if a tool name indicates tool discovery/introspection.
// These tools are blocked to enforce explicit catalog usage and save context.
func isDiscoveryTool(nameLower string) bool {
	discoveryNames := []string{
		"search_tools",
		"list_tools",
		"discover_tools",
		"enumerate_tools",
		"get_available_tools",
	}
	for _, d := range discoveryNames {
		if nameLower == d {
			return true
		}
	}
	return false
}

func isMutatingName(nameLower string) bool {
	deny := []string{
		"create_", "update_", "merge_", "delete_", "push_", "write",
		"create-or-update", "remove", "mutate", "approve", "request_changes",
	}
	for _, d := range deny {
		if strings.Contains(nameLower, d) {
			return true
		}
	}
	return false
}
