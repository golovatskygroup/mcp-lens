package router

import "testing"

func TestExtractStructuredContextURLs(t *testing.T) {
	in := `please review https://github.com/acme/repo/pull/123 and also https://grafana.example.com/d/uid123/name?orgId=2.`
	ctx := ExtractStructuredContext(in)
	if ctx["github_repo"] != "acme/repo" {
		t.Fatalf("github_repo: %v", ctx["github_repo"])
	}
	if ctx["github_pr_number"].(int) != 123 {
		t.Fatalf("github_pr_number: %v", ctx["github_pr_number"])
	}
	if ctx["grafana_dashboard_uid"] != "uid123" {
		t.Fatalf("grafana uid: %v", ctx["grafana_dashboard_uid"])
	}
}
