// +build integration

package router

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestOpenRouterIntegration tests the actual OpenRouter API call.
// Run with: go test -tags=integration -v ./internal/router/...
// Requires: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL to be set.
func TestOpenRouterIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")

	if apiKey == "" || model == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL must be set")
	}

	client, err := NewOpenRouterClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	catalog := []ToolCatalogItem{
		{
			Name:        "get_pull_request_details",
			Description: "Get PR metadata",
			Category:    "local",
			Source:      "local",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"repo":{"type":"string"},"number":{"type":"integer"}},"required":["repo","number"]}`),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	plan, rawPlan, err := Plan(ctx, client, "Get details of PR #1 in owner/repo", nil, catalog, 3)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	t.Logf("Raw plan: %s", string(rawPlan))
	t.Logf("Parsed plan steps: %d", len(plan.Steps))

	if len(plan.Steps) == 0 {
		t.Error("expected at least one step in plan")
	}

	// Validate plan
	policy := DefaultPolicy()
	if err := ValidatePlan(plan, policy, catalog, 3); err != nil {
		t.Errorf("plan validation failed: %v", err)
	}
}

// TestOpenRouterDryRun tests that dry_run returns a valid plan without execution.
func TestOpenRouterDryRun(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")

	if apiKey == "" || model == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL must be set")
	}

	client, err := NewOpenRouterClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	catalog := []ToolCatalogItem{
		{
			Name:        "search_tools",
			Description: "Search tools",
			Category:    "local",
			Source:      "local",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	plan, _, err := Plan(ctx, client, "Find tools related to pull requests", nil, catalog, 2)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	t.Logf("Plan has %d steps, final_answer_needed=%v", len(plan.Steps), plan.FinalAnswerNeeded)
}
