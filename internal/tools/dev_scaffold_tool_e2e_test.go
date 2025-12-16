//go:build integration

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/internal/router"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

func TestDevScaffoldToolE2E_ViaPlanner(t *testing.T) {
	if strings.TrimSpace(os.Getenv("MCP_LENS_DEV_SCAFFOLD_E2E")) == "" {
		t.Skip("set MCP_LENS_DEV_SCAFFOLD_E2E=1 to run this E2E test (creates a git worktree/branch)")
	}

	repoRoot := gitRepoRoot(t)
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	toolName := "confluence_dev_scaffold_e2e_" + suffix
	toolDesc := "E2E scaffolded tool (via planner)"
	worktreeName := "e2e-" + toolName
	worktreePath := filepath.Join(repoRoot, ".worktrees", worktreeName)
	targetDir := "tasks/scaffolds-e2e"

	t.Cleanup(func() {
		runCmdIgnoreErr(context.Background(), repoRoot, "git", "worktree", "remove", "-f", worktreePath)
		runCmdIgnoreErr(context.Background(), repoRoot, "git", "worktree", "prune")

		pattern := "dev/scaffold/" + toolName + "-*"
		out, _ := runCmd(context.Background(), repoRoot, "git", "branch", "--list", pattern)
		for _, line := range strings.Split(out, "\n") {
			b := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
			if b == "" {
				continue
			}
			runCmdIgnoreErr(context.Background(), repoRoot, "git", "branch", "-D", b)
		}
	})

	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"foo": map[string]any{"type": "string"},
		},
		"required": []any{"foo"},
	}

	handlerMethod := lowerFirst(camelFromSnake(toolName))
	inputType := handlerMethod + "Input"
	goCode := fmt.Sprintf(`package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type %s struct {
	Foo string `+"`json:\"foo\"`"+`
}

func (h *Handler) %s(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in %s
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Foo) == "" {
		return errorResult("foo is required"), nil
	}

	_ = ctx
	return jsonResult(map[string]any{"ok": true, "foo": in.Foo}), nil
}
`, inputType, handlerMethod, inputType)

	var mu sync.Mutex
	var callKinds []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(raw, &req)
		system := ""
		if len(req.Messages) > 0 {
			system = req.Messages[0].Content
		}

		var content string
		switch {
		case strings.Contains(system, "tool-routing model"):
			mu.Lock()
			callKinds = append(callKinds, "plan")
			mu.Unlock()

			plan := map[string]any{
				"steps": []any{
					map[string]any{
						"name":   "dev_scaffold_tool",
						"source": "local",
						"args": map[string]any{
							"tool_name":        toolName,
							"tool_description": toolDesc,
							"input_schema":     inputSchema,
							"spec":             "Implement a simple read-only tool that validates foo and echoes it back.",
							"worktree_root":    ".worktrees",
							"worktree_name":    worktreeName,
							"target_dir":       targetDir,
							"run_tests":        false,
							"run_gofmt":        true,
						},
					},
				},
				"final_answer_needed": false,
			}
			b, _ := json.Marshal(plan)
			content = string(b)
		case strings.Contains(system, "Return ONLY valid Go source code"):
			mu.Lock()
			callKinds = append(callKinds, "codegen")
			mu.Unlock()
			content = goCode
		default:
			http.Error(w, "unexpected prompt", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message":       map[string]any{"content": content},
					"finish_reason": "stop",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	t.Setenv("MCP_LENS_DEV_MODE", "1")
	t.Setenv("OPENROUTER_API_KEY", "test")
	t.Setenv("MCP_LENS_ROUTER_MODEL", "test-model")
	t.Setenv("MCP_LENS_ROUTER_BASE_URL", srv.URL)

	reg := registry.NewRegistry()
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.ContentBlock{{Type: "text", Text: `{"error":"upstream not configured"}`}},
			IsError: true,
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	args, _ := json.Marshal(map[string]any{
		"input":     "Generate a new local tool via dev scaffolding",
		"max_steps": 1,
		"format":    "json",
	})

	res, err := h.Handle(ctx, "query", args)
	if err != nil {
		t.Fatalf("Handle(query): %v", err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("expected non-empty result content")
	}
	if res.IsError {
		t.Fatalf("query returned error: %s", res.Content[0].Text)
	}

	var rr router.RouterResult
	if err := json.Unmarshal([]byte(res.Content[0].Text), &rr); err != nil {
		t.Fatalf("failed to parse query result JSON: %v", err)
	}
	if len(rr.ExecutedSteps) != 1 {
		t.Fatalf("expected 1 executed step, got %d", len(rr.ExecutedSteps))
	}
	step := rr.ExecutedSteps[0]
	if step.Name != "dev_scaffold_tool" || step.Source != "local" {
		t.Fatalf("unexpected executed step: %+v", step)
	}
	if !step.OK {
		t.Fatalf("dev_scaffold_tool step failed: %s", step.Error)
	}

	resultMap, ok := step.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected step.result to be object, got %T", step.Result)
	}

	gotWorktreePath, _ := resultMap["worktree_path"].(string)
	gotPatchPath, _ := resultMap["patch_path"].(string)
	if filepath.Clean(gotWorktreePath) != filepath.Clean(worktreePath) {
		t.Fatalf("worktree_path mismatch: got %q want %q", gotWorktreePath, worktreePath)
	}
	wantPatchPath := filepath.Join(worktreePath, targetDir, toolName+".patch")
	if filepath.Clean(gotPatchPath) != filepath.Clean(wantPatchPath) {
		t.Fatalf("patch_path mismatch: got %q want %q", gotPatchPath, wantPatchPath)
	}

	if _, err := os.Stat(gotWorktreePath); err != nil {
		t.Fatalf("worktree_path does not exist: %v", err)
	}
	if _, err := os.Stat(gotPatchPath); err != nil {
		t.Fatalf("patch_path does not exist: %v", err)
	}

	toolFilePath := filepath.Join(gotWorktreePath, "internal", "tools", toolName+".go")
	if _, err := os.Stat(toolFilePath); err != nil {
		t.Fatalf("generated tool file missing: %v", err)
	}

	metaBytes, err := os.ReadFile(filepath.Join(gotWorktreePath, "internal", "tools", "meta.go"))
	if err != nil {
		t.Fatalf("read meta.go: %v", err)
	}
	if !bytes.Contains(metaBytes, []byte(strconv.Quote(toolName))) {
		t.Fatalf("meta.go does not mention %q", toolName)
	}

	patchBytes, err := os.ReadFile(gotPatchPath)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	if !bytes.Contains(patchBytes, []byte(toolName)) {
		t.Fatalf("patch does not mention %q", toolName)
	}

	// Sanity check: the worktree compiles/tests without integration tags.
	cmd := exec.CommandContext(ctx, "go", "test", "./...")
	cmd.Dir = gotWorktreePath
	cmd.Env = append(os.Environ(), "MCP_LENS_DEV_SCAFFOLD_E2E=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go test ./... in worktree failed: %v\n%s", err, string(out))
	}

	mu.Lock()
	gotKinds := append([]string(nil), callKinds...)
	mu.Unlock()
	if len(gotKinds) < 2 || gotKinds[0] != "plan" || gotKinds[1] != "codegen" {
		t.Fatalf("unexpected OpenRouter call sequence: %v", gotKinds)
	}
}

func gitRepoRoot(t *testing.T) string {
	t.Helper()
	out, err := runCmd(context.Background(), "", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel: %v", err)
	}
	root := strings.TrimSpace(out)
	if root == "" {
		t.Fatalf("empty git repo root")
	}
	return root
}

func runCmdIgnoreErr(ctx context.Context, dir string, name string, args ...string) {
	_, _ = runCmd(ctx, dir, name, args...)
}
