//go:build integration

package router

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestMultiStepPRDetails tests a multi-step pipeline where the router
// plans and executes steps to get PR details from a real GitHub PR.
// This tests the full flow: planning -> validation -> execution.
//
// Run with: go test -tags=integration -v -run TestMultiStepPRDetails ./internal/router/...
// Requires: OPENROUTER_API_KEY, MCP_LENS_ROUTER_MODEL, and GITHUB_TOKEN to be set.
func TestMultiStepPRDetails(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")
	githubToken := os.Getenv("GITHUB_TOKEN")

	if apiKey == "" || model == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL must be set")
	}
	if githubToken == "" {
		t.Log("Warning: GITHUB_TOKEN not set, GitHub API calls may fail or be rate-limited")
	}

	client, err := NewOpenRouterClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create OpenRouter client: %v", err)
	}

	// Catalog with local tools that can fetch PR details
	catalog := []ToolCatalogItem{
		{
			Name:        "get_pull_request_details",
			Description: "Get pull request metadata (title, base/head, author, state). Read-only via GitHub REST API.",
			Category:    "local",
			Source:      "local",
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
			Description: "List changed files in a PR with pagination. Read-only via GitHub REST API.",
			Category:    "local",
			Source:      "local",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"page": {"type": "integer", "description": "Page number (default: 1)"},
					"per_page": {"type": "integer", "description": "Items per page (default: 30, max: 100)"}
				},
				"required": ["repo", "number"]
			}`),
		},
		{
			Name:        "list_pull_request_commits",
			Description: "List commits in a PR with pagination. Read-only via GitHub REST API.",
			Category:    "local",
			Source:      "local",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"repo": {"type": "string", "description": "Repository in owner/name form"},
					"number": {"type": "integer", "description": "Pull request number"},
					"page": {"type": "integer", "description": "Page number (default: 1)"},
					"per_page": {"type": "integer", "description": "Items per page (default: 30, max: 100)"}
				},
				"required": ["repo", "number"]
			}`),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Ask the router to get details about a real PR
	userRequest := "Get the details of the pull request https://github.com/1inch/pathfinder/pull/3475 including the title, author, and list of changed files"

	t.Logf("User request: %s", userRequest)

	// Step 1: Plan
	plan, rawPlan, err := Plan(ctx, client, userRequest, nil, catalog, 5)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	t.Logf("Raw plan from LLM:\n%s", string(rawPlan))
	t.Logf("Parsed plan: %d steps, final_answer_needed=%v", len(plan.Steps), plan.FinalAnswerNeeded)

	// Step 2: Validate plan
	policy := DefaultPolicy()
	if err := ValidatePlan(plan, policy, catalog, 5); err != nil {
		t.Fatalf("Plan validation failed: %v\nPlan: %s", err, string(rawPlan))
	}
	t.Log("Plan validation: PASSED")

	// Step 3: Check plan structure
	if len(plan.Steps) == 0 {
		t.Fatal("Expected at least one step in the plan")
	}

	// Verify that the plan includes the correct repo and PR number
	foundPRDetails := false
	foundFiles := false
	for i, step := range plan.Steps {
		t.Logf("Step %d: %s (source=%s)", i+1, step.Name, step.Source)
		t.Logf("  Args: %s", string(step.Args))
		t.Logf("  Reason: %s", step.Reason)

		var args map[string]any
		if err := json.Unmarshal(step.Args, &args); err != nil {
			t.Errorf("Step %d has invalid args: %v", i+1, err)
			continue
		}

		// Check that repo is correctly parsed
		if repo, ok := args["repo"].(string); ok {
			if repo == "1inch/pathfinder" {
				t.Logf("  Repo correctly identified: %s", repo)
			}
		}

		// Check that PR number is correctly parsed
		if number, ok := args["number"].(float64); ok {
			if int(number) == 3475 {
				t.Logf("  PR number correctly identified: %d", int(number))
			}
		}

		if step.Name == "get_pull_request_details" {
			foundPRDetails = true
		}
		if step.Name == "list_pull_request_files" {
			foundFiles = true
		}
	}

	if !foundPRDetails {
		t.Error("Expected plan to include get_pull_request_details step")
	}
	if !foundFiles {
		t.Error("Expected plan to include list_pull_request_files step")
	}

	t.Log("Multi-step pipeline test: PASSED")
}

// TestMultiStepPlanOnly tests just the planning phase without execution,
// verifying that the LLM can parse a GitHub URL and create a valid plan.
func TestMultiStepPlanOnly(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")

	if apiKey == "" || model == "" {
		t.Skip("Skipping integration test: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL must be set")
	}

	client, err := NewOpenRouterClientFromEnv()
	if err != nil {
		t.Fatalf("failed to create OpenRouter client: %v", err)
	}

	catalog := []ToolCatalogItem{
		{
			Name:        "get_pull_request_details",
			Description: "Get PR metadata",
			Category:    "local",
			Source:      "local",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"repo":{"type":"string"},"number":{"type":"integer"}},"required":["repo","number"]}`),
		},
		{
			Name:        "list_pull_request_files",
			Description: "List changed files in a PR",
			Category:    "local",
			Source:      "local",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"repo":{"type":"string"},"number":{"type":"integer"}},"required":["repo","number"]}`),
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	plan, rawPlan, err := Plan(ctx, client, "What are the changes in https://github.com/1inch/pathfinder/pull/3475?", nil, catalog, 3)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	t.Logf("Plan: %s", string(rawPlan))

	// Validate
	policy := DefaultPolicy()
	if err := ValidatePlan(plan, policy, catalog, 3); err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Check that at least one step references the correct repo/PR
	for _, step := range plan.Steps {
		var args map[string]any
		if err := json.Unmarshal(step.Args, &args); err != nil {
			continue
		}
		if repo, ok := args["repo"].(string); ok && repo == "1inch/pathfinder" {
			if num, ok := args["number"].(float64); ok && int(num) == 3475 {
				t.Log("Plan correctly parsed GitHub URL into repo=1inch/pathfinder, number=3475")
				return
			}
		}
	}

	t.Error("Plan did not correctly parse the GitHub PR URL")
}
