package router

import "strings"

type Policy struct {
	AllowLocal    map[string]struct{}
	AllowUpstream map[string]struct{}
}

func DefaultPolicy() Policy {
	allowLocal := map[string]struct{}{
		"get_pull_request_details":           {},
		"list_pull_request_files":            {},
		"get_pull_request_diff":              {},
		"get_pull_request_summary":           {},
		"get_pull_request_file_diff":         {},
		"list_pull_request_commits":          {},
		"get_pull_request_checks":            {},
		"get_file_at_ref":                    {},
		"prepare_pull_request_review_bundle": {},
		"fetch_complete_pr_diff":             {},
		"fetch_complete_pr_files":            {},
		"jira_get_myself":                    {},
		"jira_get_issue":                     {},
		"jira_search_issues":                 {},
		"jira_get_issue_comments":            {},
		"jira_get_issue_transitions":         {},
		"jira_list_projects":                 {},
		"confluence_list_spaces":             {},
		"confluence_get_page":                {},
		"confluence_get_page_by_title":       {},
		"confluence_search_cql":              {},
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
	}

	// Start conservative: empty upstream allowlist.
	return Policy{AllowLocal: allowLocal, AllowUpstream: map[string]struct{}{}}
}

func (p Policy) IsAllowed(source string, toolName string) bool {
	nameLower := strings.ToLower(strings.TrimSpace(toolName))
	if nameLower == "" {
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
