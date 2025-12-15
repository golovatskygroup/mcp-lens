//go:build e2e

package tools

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/internal/router"
)

func TestRouterExecutesJiraReadOnlyPlan_GO27(t *testing.T) {
	if os.Getenv("JIRA_BASE_URL") == "" {
		t.Skip("JIRA_BASE_URL is not set (configure .env or env vars)")
	}
	if os.Getenv("JIRA_PAT") == "" && !(os.Getenv("JIRA_EMAIL") != "" && os.Getenv("JIRA_API_TOKEN") != "") && os.Getenv("JIRA_OAUTH_ACCESS_TOKEN") == "" && os.Getenv("JIRA_BEARER_TOKEN") == "" {
		t.Skip("Jira auth is not set (configure JIRA_PAT or JIRA_EMAIL+JIRA_API_TOKEN or JIRA_OAUTH_ACCESS_TOKEN)")
	}

	issueKey := os.Getenv("JIRA_E2E_ISSUE_KEY")
	if issueKey == "" {
		issueKey = "GO-27"
	}

	// Use v2 in tests for maximum compatibility across Cloud + Server/DC.
	apiVersion := 2

	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)
	reg.LoadTools(h.BuiltinTools())

	policy := router.DefaultPolicy()
	catalog := h.buildRouterCatalog()

	plan := router.ModelPlan{
		Steps: []router.PlanStep{
			{Name: "jira_get_myself", Source: "local", Args: mustMarshal(map[string]any{"api_version": apiVersion})},
			{Name: "jira_get_issue", Source: "local", Args: mustMarshal(map[string]any{"issue": issueKey, "api_version": apiVersion})},
			{Name: "jira_search_issues", Source: "local", Args: mustMarshal(map[string]any{"jql": "key = " + issueKey, "startAt": 0, "maxResults": 10, "api_version": apiVersion})},
			{Name: "jira_get_issue_comments", Source: "local", Args: mustMarshal(map[string]any{"issue": issueKey, "startAt": 0, "maxResults": 5, "api_version": apiVersion})},
			{Name: "jira_get_issue_transitions", Source: "local", Args: mustMarshal(map[string]any{"issue": issueKey, "api_version": apiVersion})},
		},
		FinalAnswerNeeded: false,
	}

	if err := router.ValidatePlan(plan, policy, catalog, 8); err != nil {
		t.Fatalf("plan validation failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	execSteps, err := h.executePlan(ctx, plan, policy)
	if err != nil {
		b, _ := json.MarshalIndent(execSteps, "", "  ")
		t.Fatalf("executePlan failed: %v\nsteps=%s", err, string(b))
	}
	if len(execSteps) < 2 {
		t.Fatalf("expected >= 2 executed steps, got %d", len(execSteps))
	}

	// Validate jira_get_issue result includes the expected key.
	var issue map[string]any
	if m, ok := execSteps[1].Result.(map[string]any); ok {
		issue = m
	} else {
		t.Fatalf("expected jira_get_issue result to be JSON object, got %T", execSteps[1].Result)
	}

	if got, _ := issue["key"].(string); got != issueKey {
		b, _ := json.MarshalIndent(issue, "", "  ")
		t.Fatalf("expected issue key %q, got %q\nissue=%s", issueKey, got, string(b))
	}
}
