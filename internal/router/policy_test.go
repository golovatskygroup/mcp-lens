package router

import "testing"

func TestPolicyBlocksMutations(t *testing.T) {
	p := DefaultPolicy()

	if p.IsAllowed("upstream", "create_pull_request") {
		t.Fatalf("expected create_* to be blocked")
	}
	if p.IsAllowed("upstream", "merge_pull_request") {
		t.Fatalf("expected merge_* to be blocked")
	}
	if p.IsAllowed("upstream", "delete_file") {
		t.Fatalf("expected delete_* to be blocked")
	}
}

func TestPolicyAllowsKnownLocalReadOnly(t *testing.T) {
	p := DefaultPolicy()
	if !p.IsAllowed("local", "search_tools") {
		t.Fatalf("expected search_tools allowed")
	}
	if !p.IsAllowed("local", "describe_tool") {
		t.Fatalf("expected describe_tool allowed")
	}
	if !p.IsAllowed("local", "get_pull_request_details") {
		t.Fatalf("expected local tool allowed")
	}
	if p.IsAllowed("local", "create_pull_request") {
		t.Fatalf("expected unknown local tool blocked")
	}
}

func TestPolicyAllowsNewLocalTools(t *testing.T) {
	p := DefaultPolicy()

	newTools := []string{
		"artifact_save_text",
		"artifact_append_text",
		"artifact_list",
		"artifact_search",
		"get_pull_request_summary",
		"get_pull_request_file_diff",
		"github_list_workflow_runs",
		"github_list_workflow_jobs",
		"github_download_job_logs",
		"jira_get_issue_bundle",
		"jira_export_tasks",
		"confluence_get_page_children",
		"confluence_list_page_attachments",
		"confluence_download_attachment",
		"grafana_list_alerts",
		"grafana_get_alert",
		"grafana_list_alert_rules",
		"grafana_get_alert_rule",
	}

	for _, tool := range newTools {
		if !p.IsAllowed("local", tool) {
			t.Errorf("expected new local tool %s to be allowed", tool)
		}
	}
}

func TestPolicyAllowsAllPRTools(t *testing.T) {
	p := DefaultPolicy()

	prTools := []string{
		"get_pull_request_details",
		"list_pull_request_files",
		"get_pull_request_diff",
		"get_pull_request_summary",
		"get_pull_request_file_diff",
		"list_pull_request_commits",
		"get_pull_request_checks",
		"get_file_at_ref",
		"prepare_pull_request_review_bundle",
	}

	for _, tool := range prTools {
		if !p.IsAllowed("local", tool) {
			t.Errorf("expected PR tool %s to be allowed", tool)
		}
	}
}

func TestPolicyDevScaffoldToolRequiresDevMode(t *testing.T) {
	t.Setenv("MCP_LENS_DEV_MODE", "")
	p := DefaultPolicy()
	if p.IsAllowed("local", "dev_scaffold_tool") {
		t.Fatalf("expected dev_scaffold_tool blocked when dev mode off")
	}

	t.Setenv("MCP_LENS_DEV_MODE", "1")
	p2 := DefaultPolicy()
	if !p2.IsAllowed("local", "dev_scaffold_tool") {
		t.Fatalf("expected dev_scaffold_tool allowed when dev mode on")
	}
}
