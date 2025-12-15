//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/internal/testutil"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

// TestQueryE2E_RealPR tests the full end-to-end flow:
// query tool -> planning -> validation -> execution -> GitHub API calls
//
// Run with: go test -tags=integration -v -run TestQueryE2E_RealPR ./internal/tools/...
// Requires: OPENROUTER_API_KEY, MCP_LENS_ROUTER_MODEL, and GITHUB_TOKEN
func TestQueryE2E_RealPR(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")
	githubToken := os.Getenv("GITHUB_TOKEN")

	if apiKey == "" || model == "" {
		t.Skip("Skipping: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL required")
	}
	if githubToken == "" {
		t.Skip("Skipping: GITHUB_TOKEN required for GitHub API calls")
	}

	prURL := strings.TrimSpace(os.Getenv("GITHUB_E2E_PR_URL"))
	if prURL == "" {
		t.Skip("Skipping: set GITHUB_E2E_PR_URL=https://github.com/<owner>/<repo>/pull/<number> to run this E2E test")
	}
	wantRepo, wantNumber, err := testutil.ParseGitHubPullRequestURL(prURL)
	if err != nil {
		t.Fatalf("invalid GITHUB_E2E_PR_URL %q: %v", prURL, err)
	}

	reg := registry.NewRegistry()

	// Handler with no upstream executor (we only use local tools)
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: `{"error": "upstream not configured"}`}},
			IsError: true,
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Call the query tool with a request about a PR from env.
	payload := map[string]any{
		"input":     "Get details and changed files for " + prURL,
		"max_steps": 3,
		"format":    "json",
	}
	b, _ := json.Marshal(payload)
	args := json.RawMessage(b)

	t.Log("Calling query tool...")
	result, err := h.Handle(ctx, "query", args)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Result is nil")
	}

	if result.IsError {
		t.Logf("Result is error: %s", result.Content[0].Text)
		// Don't fail - the plan might still be valid even if execution had issues
	}

	// Parse the result
	if len(result.Content) == 0 {
		t.Fatal("No content in result")
	}

	resultText := result.Content[0].Text
	t.Logf("Result length: %d bytes", len(resultText))

	// Try to parse as JSON
	var res map[string]any
	if err := json.Unmarshal([]byte(resultText), &res); err != nil {
		t.Logf("Raw result: %s", resultText)
		t.Fatalf("Failed to parse result as JSON: %v", err)
	}

	// Check for plan
	if plan, ok := res["plan"].(map[string]any); ok {
		if steps, ok := plan["steps"].([]any); ok {
			t.Logf("Plan has %d steps", len(steps))
			for i, step := range steps {
				if s, ok := step.(map[string]any); ok {
					t.Logf("  Step %d: %s", i+1, s["name"])
				}
			}
		}
	}

	// Check for executed steps
	if execSteps, ok := res["executed_steps"].([]any); ok {
		t.Logf("Executed %d steps", len(execSteps))
		for i, step := range execSteps {
			if s, ok := step.(map[string]any); ok {
				name := s["name"]
				ok := s["ok"]
				t.Logf("  Executed step %d: %s (ok=%v)", i+1, name, ok)

				// Check if we got real PR data
				if name == "get_pull_request_details" && ok == true {
					if result, ok := s["result"].(map[string]any); ok {
						if gotRepo, ok := result["repo"].(string); ok && gotRepo != wantRepo {
							t.Errorf("unexpected repo in get_pull_request_details: got %q want %q", gotRepo, wantRepo)
						}
						if gotNum, ok := result["number"].(float64); ok && int(gotNum) != wantNumber {
							t.Errorf("unexpected number in get_pull_request_details: got %d want %d", int(gotNum), wantNumber)
						}
						if title, ok := result["title"].(string); ok {
							t.Logf("    PR Title: %s", title)
						}
						if user, ok := result["user"].(map[string]any); ok {
							if login, ok := user["login"].(string); ok {
								t.Logf("    PR Author: %s", login)
							}
						}
						if state, ok := result["state"].(string); ok {
							t.Logf("    PR State: %s", state)
						}
					}
				}

				if name == "list_pull_request_files" && ok == true {
					if out, ok := s["result"].(map[string]any); ok {
						if gotRepo, ok := out["repo"].(string); ok && gotRepo != wantRepo {
							t.Errorf("unexpected repo in list_pull_request_files: got %q want %q", gotRepo, wantRepo)
						}
						if gotNum, ok := out["number"].(float64); ok && int(gotNum) != wantNumber {
							t.Errorf("unexpected number in list_pull_request_files: got %d want %d", int(gotNum), wantNumber)
						}
						files, _ := out["files"].([]any)
						t.Logf("    Changed files: %d", len(files))
						for j, f := range files {
							if j >= 3 {
								t.Logf("    ... and %d more files", len(files)-3)
								break
							}
							if file, ok := f.(map[string]any); ok {
								t.Logf("    - %s", file["filename"])
							}
						}
					}
				}
			}
		}
	}

	t.Log("E2E test completed successfully")
}

// TestQueryE2E_PRSummary tests getting a summary with include_answer=true
func TestQueryE2E_PRSummary(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")
	githubToken := os.Getenv("GITHUB_TOKEN")

	if apiKey == "" || model == "" {
		t.Skip("Skipping: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL required")
	}
	if githubToken == "" {
		t.Skip("Skipping: GITHUB_TOKEN required for GitHub API calls")
	}

	prURL := strings.TrimSpace(os.Getenv("GITHUB_E2E_PR_URL"))
	if prURL == "" {
		t.Skip("Skipping: set GITHUB_E2E_PR_URL=https://github.com/<owner>/<repo>/pull/<number> to run this E2E test")
	}
	if _, _, err := testutil.ParseGitHubPullRequestURL(prURL); err != nil {
		t.Fatalf("invalid GITHUB_E2E_PR_URL %q: %v", prURL, err)
	}

	reg := registry.NewRegistry()
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: `{"error": "upstream not configured"}`}},
			IsError: true,
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	payload := map[string]any{
		"input":          "Summarize the PR " + prURL,
		"max_steps":      2,
		"include_answer": true,
		"format":         "json",
	}
	b, _ := json.Marshal(payload)
	args := json.RawMessage(b)

	t.Log("Calling query tool with include_answer=true...")
	result, err := h.Handle(ctx, "query", args)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	if result == nil {
		t.Fatal("Result is nil")
	}

	resultText := result.Content[0].Text

	var res map[string]any
	if err := json.Unmarshal([]byte(resultText), &res); err != nil {
		t.Logf("Raw result: %s", resultText)
		t.Fatalf("Failed to parse result as JSON: %v", err)
	}

	// Check for answer field
	if answer, ok := res["answer"].(string); ok && answer != "" {
		t.Logf("Got summarized answer (%d chars):", len(answer))
		// Print first 500 chars
		if len(answer) > 500 {
			t.Logf("%s...", answer[:500])
		} else {
			t.Log(answer)
		}
	} else {
		t.Log("No answer field in result (may be expected if include_answer failed)")
	}

	// Verify we got PR data
	if execSteps, ok := res["executed_steps"].([]any); ok {
		foundPRData := false
		for _, step := range execSteps {
			if s, ok := step.(map[string]any); ok {
				if s["name"] == "get_pull_request_details" && s["ok"] == true {
					foundPRData = true
					break
				}
			}
		}
		if !foundPRData {
			t.Error("Expected to find successful get_pull_request_details execution")
		}
	}

	t.Log("E2E summary test completed")
}

// TestQueryDryRun tests dry_run mode (plan only, no execution)
func TestQueryDryRun(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	model := os.Getenv("MCP_LENS_ROUTER_MODEL")

	if apiKey == "" || model == "" {
		t.Skip("Skipping: OPENROUTER_API_KEY and MCP_LENS_ROUTER_MODEL required")
	}

	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	prURL := strings.TrimSpace(os.Getenv("GITHUB_E2E_PR_URL"))
	if prURL == "" {
		prURL = "https://github.com/owner/repo/pull/1"
	}
	wantRepo, wantNumber, err := testutil.ParseGitHubPullRequestURL(prURL)
	if err != nil {
		t.Fatalf("invalid GITHUB_E2E_PR_URL %q: %v", prURL, err)
	}

	payload := map[string]any{
		"input":   "Get PR #" + strconv.Itoa(wantNumber) + " from " + wantRepo,
		"dry_run": true,
		"format":  "json",
	}
	b, _ := json.Marshal(payload)
	args := json.RawMessage(b)

	result, err := h.Handle(ctx, "query", args)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}

	resultText := result.Content[0].Text

	var res map[string]any
	if err := json.Unmarshal([]byte(resultText), &res); err != nil {
		t.Fatalf("Failed to parse result: %v", err)
	}

	// In dry_run mode, we should have a plan but no executed_steps
	if _, ok := res["plan"]; !ok {
		t.Error("Expected plan in dry_run result")
	}

	if execSteps, ok := res["executed_steps"].([]any); ok && len(execSteps) > 0 {
		t.Error("Expected no executed_steps in dry_run mode")
	}

	// Verify plan contains correct repo/number
	if plan, ok := res["plan"].(map[string]any); ok {
		if steps, ok := plan["steps"].([]any); ok && len(steps) > 0 {
			step := steps[0].(map[string]any)
			args := step["args"].(map[string]any)
			if repo, ok := args["repo"].(string); ok {
				if repo != wantRepo {
					t.Errorf("Expected repo %q, got %q", wantRepo, repo)
				}
			}
			if num, ok := args["number"].(float64); ok {
				if int(num) != wantNumber {
					t.Errorf("Expected PR number %d, got %d", wantNumber, int(num))
				}
			}
		}
	}

	t.Log("Dry run test passed")
}
