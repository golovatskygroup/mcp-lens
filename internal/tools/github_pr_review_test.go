package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
)

func TestSplitRepo(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"valid", "owner/repo", "owner", "repo", false},
		{"with spaces", "  owner/repo  ", "owner", "repo", false},
		{"empty", "", "", "", true},
		{"no slash", "ownerrepo", "", "", true},
		{"empty owner", "/repo", "", "", true},
		{"empty repo", "owner/", "", "", true},
		{"too many slashes", "owner/repo/extra", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, err := splitRepo(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("splitRepo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if owner != tt.wantOwner {
				t.Errorf("splitRepo() owner = %v, want %v", owner, tt.wantOwner)
			}
			if repo != tt.wantRepo {
				t.Errorf("splitRepo() repo = %v, want %v", repo, tt.wantRepo)
			}
		})
	}
}

func TestFilterDiffByFile(t *testing.T) {
	sampleDiff := `diff --git a/file1.go b/file1.go
index abc1234..def5678 100644
--- a/file1.go
+++ b/file1.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {}
diff --git a/file2.go b/file2.go
index 111111..222222 100644
--- a/file2.go
+++ b/file2.go
@@ -1,2 +1,3 @@
 package main
+// comment
diff --git a/nested/file3.go b/nested/file3.go
index 333333..444444 100644
--- a/nested/file3.go
+++ b/nested/file3.go
@@ -1,1 +1,2 @@
+// new file`

	tests := []struct {
		name       string
		targetPath string
		wantLines  int
		wantEmpty  bool
	}{
		{"exact match file1", "file1.go", 7, false},
		{"exact match file2", "file2.go", 7, false},
		{"nested file", "nested/file3.go", 6, false},
		{"non-existent", "notfound.go", 0, true},
		{"partial name no match", "file", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDiffByFile(sampleDiff, tt.targetPath)
			if tt.wantEmpty && result != "" {
				t.Errorf("expected empty result, got: %s", result)
			}
			if !tt.wantEmpty && result == "" {
				t.Errorf("expected non-empty result for %s", tt.targetPath)
			}
		})
	}
}

func TestFilterDiffByPatterns(t *testing.T) {
	sampleDiff := `diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1 +1 @@
-old
+new
diff --git a/pkg/util.go b/pkg/util.go
index 111..222 100644
--- a/pkg/util.go
+++ b/pkg/util.go
@@ -1 +1 @@
-util old
+util new
diff --git a/README.md b/README.md
index 333..444 100644
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-readme old
+readme new`

	tests := []struct {
		name        string
		patterns    []string
		wantContain []string
		wantMissing []string
	}{
		{
			name:        "filter go files only",
			patterns:    []string{"*.go"},
			wantContain: []string{"main.go", "util.go"},
			wantMissing: []string{"README.md"},
		},
		{
			name:        "filter md files only",
			patterns:    []string{"*.md"},
			wantContain: []string{"README.md"},
			wantMissing: []string{"main.go"},
		},
		{
			name:        "multiple patterns",
			patterns:    []string{"*.go", "*.md"},
			wantContain: []string{"main.go", "README.md"},
			wantMissing: []string{},
		},
		{
			name:        "empty patterns returns all",
			patterns:    []string{},
			wantContain: []string{"main.go", "README.md"},
			wantMissing: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterDiffByPatterns(sampleDiff, tt.patterns)

			for _, want := range tt.wantContain {
				if !contains(result, want) {
					t.Errorf("expected result to contain %s, got: %s", want, result)
				}
			}

			for _, notWant := range tt.wantMissing {
				if contains(result, notWant) {
					t.Errorf("expected result NOT to contain %s", notWant)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGitHubClientSingleton(t *testing.T) {
	// Reset singleton for test
	ghClientOnce = sync.Once{}
	ghClient = nil

	// First call should create client
	client1 := newGitHubClient()
	if client1 == nil {
		t.Fatal("expected non-nil client")
	}

	// Second call should return same instance
	client2 := newGitHubClient()
	if client1 != client2 {
		t.Error("expected same client instance (singleton)")
	}
}

func TestGitHubClientRateLimitTracking(t *testing.T) {
	// Reset singleton
	ghClientOnce = sync.Once{}
	ghClient = nil

	client := newGitHubClient()

	// Initial state
	remaining, resetAt := client.GetRateLimitInfo()
	if remaining != -1 {
		t.Errorf("expected initial remaining = -1, got %d", remaining)
	}

	// Simulate header update
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "4999")
	headers.Set("X-RateLimit-Reset", "1700000000")

	client.updateRateLimit(headers)

	remaining, resetAt = client.GetRateLimitInfo()
	if remaining != 4999 {
		t.Errorf("expected remaining = 4999, got %d", remaining)
	}
	expectedReset := time.Unix(1700000000, 0)
	if !resetAt.Equal(expectedReset) {
		t.Errorf("expected reset time %v, got %v", expectedReset, resetAt)
	}
}

func TestParseNextPage(t *testing.T) {
	tests := []struct {
		name       string
		linkHeader string
		wantPage   int
		wantHas    bool
	}{
		{
			name:       "has next page",
			linkHeader: `<https://api.github.com/repos/owner/repo/pulls/1/files?page=2>; rel="next", <https://api.github.com/repos/owner/repo/pulls/1/files?page=5>; rel="last"`,
			wantPage:   2,
			wantHas:    true,
		},
		{
			name:       "no next page",
			linkHeader: `<https://api.github.com/repos/owner/repo/pulls/1/files?page=1>; rel="first"`,
			wantPage:   0,
			wantHas:    false,
		},
		{
			name:       "empty header",
			linkHeader: "",
			wantPage:   0,
			wantHas:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, has := parseNextPage(tt.linkHeader)
			if page != tt.wantPage {
				t.Errorf("parseNextPage() page = %v, want %v", page, tt.wantPage)
			}
			if has != tt.wantHas {
				t.Errorf("parseNextPage() has = %v, want %v", has, tt.wantHas)
			}
		})
	}
}

func TestGitHubAuthHint(t *testing.T) {
	tests := []struct {
		status       int
		wantNonEmpty bool
	}{
		{401, true},
		{403, true},
		{404, true},
		{200, false},
		{500, false},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			hint := githubAuthHint(tt.status)
			hasHint := hint != ""
			if hasHint != tt.wantNonEmpty {
				t.Errorf("githubAuthHint(%d) = %q, wantNonEmpty = %v", tt.status, hint, tt.wantNonEmpty)
			}
		})
	}
}

func TestNewToolsExist(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	expectedTools := []string{
		"get_pull_request_summary",
		"get_pull_request_file_diff",
	}

	tools := h.BuiltinTools()
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		if !toolNames[expected] {
			t.Errorf("expected tool %s to exist in BuiltinTools", expected)
		}
	}
}

func TestIsLocalToolNewTools(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	newTools := []string{
		"get_pull_request_summary",
		"get_pull_request_file_diff",
	}

	for _, tool := range newTools {
		if !h.IsLocalTool(tool) {
			t.Errorf("expected %s to be a local tool", tool)
		}
	}
}

func TestGetPullRequestDiffInputValidation(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "missing repo",
			input:     `{"number": 123}`,
			wantError: true,
		},
		{
			name:      "missing number",
			input:     `{"repo": "owner/repo"}`,
			wantError: true,
		},
		{
			name:      "negative offset",
			input:     `{"repo": "owner/repo", "number": 123, "offset": -1}`,
			wantError: true,
		},
		{
			name:      "invalid json",
			input:     `{invalid}`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := h.Handle(context.Background(), "get_pull_request_diff", json.RawMessage(tt.input))
			if result == nil {
				t.Fatal("expected result, got nil")
			}
			if tt.wantError && !result.IsError {
				t.Error("expected error result")
			}
		})
	}
}

func TestGetPullRequestSummaryInputValidation(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "missing repo",
			input:     `{"number": 123}`,
			wantError: true,
		},
		{
			name:      "missing number",
			input:     `{"repo": "owner/repo"}`,
			wantError: true,
		},
		{
			name:      "zero number",
			input:     `{"repo": "owner/repo", "number": 0}`,
			wantError: true,
		},
		{
			name:      "invalid repo format",
			input:     `{"repo": "invalid", "number": 123}`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := h.Handle(context.Background(), "get_pull_request_summary", json.RawMessage(tt.input))
			if result == nil {
				t.Fatal("expected result, got nil")
			}
			if tt.wantError && !result.IsError {
				t.Error("expected error result")
			}
		})
	}
}

func TestGetPullRequestFileDiffInputValidation(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	tests := []struct {
		name      string
		input     string
		wantError bool
	}{
		{
			name:      "missing path",
			input:     `{"repo": "owner/repo", "number": 123}`,
			wantError: true,
		},
		{
			name:      "missing repo",
			input:     `{"number": 123, "path": "file.go"}`,
			wantError: true,
		},
		{
			name:      "missing number",
			input:     `{"repo": "owner/repo", "path": "file.go"}`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := h.Handle(context.Background(), "get_pull_request_file_diff", json.RawMessage(tt.input))
			if result == nil {
				t.Fatal("expected result, got nil")
			}
			if tt.wantError && !result.IsError {
				t.Error("expected error result")
			}
		})
	}
}

func TestSearchLocalToolsFindsNewTools(t *testing.T) {
	tests := []struct {
		query     string
		wantTools []string
	}{
		{
			query:     "summary",
			wantTools: []string{"get_pull_request_summary"},
		},
		{
			query:     "file diff",
			wantTools: []string{"get_pull_request_file_diff"},
		},
		{
			query:     "pr",
			wantTools: []string{"get_pull_request_summary", "get_pull_request_file_diff"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			results := searchLocalTools(tt.query, "", 20)

			resultNames := make(map[string]bool)
			for _, r := range results {
				resultNames[r.Name] = true
			}

			for _, want := range tt.wantTools {
				if !resultNames[want] {
					t.Errorf("expected search for %q to find %s", tt.query, want)
				}
			}
		})
	}
}
