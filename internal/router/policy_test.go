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
		"get_pull_request_summary",
		"get_pull_request_file_diff",
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
