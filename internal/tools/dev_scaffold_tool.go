package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/router"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type devScaffoldToolInput struct {
	ToolName        string         `json:"tool_name"`
	ToolDescription string         `json:"tool_description"`
	InputSchema     map[string]any `json:"input_schema"`
	Spec            string         `json:"spec,omitempty"`

	HandlerMethod string `json:"handler_method,omitempty"`
	Domain        string `json:"domain,omitempty"`

	TargetDir    string `json:"target_dir,omitempty"`
	WorktreeRoot string `json:"worktree_root,omitempty"`
	WorktreeName string `json:"worktree_name,omitempty"`

	UseWorktree   *bool `json:"use_worktree,omitempty"`
	RunGofmt      *bool `json:"run_gofmt,omitempty"`
	RunTests      *bool `json:"run_tests,omitempty"`
	AllowInPolicy *bool `json:"allow_in_policy,omitempty"`
	AddToSearch   *bool `json:"add_to_search,omitempty"`
	AddPromptHint *bool `json:"add_prompt_hint,omitempty"`
}

func (h *Handler) devScaffoldTool(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if !devModeEnabled() {
		return errorResult("dev mode disabled: set MCP_LENS_DEV_MODE=1"), nil
	}

	var in devScaffoldToolInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	in.ToolName = strings.TrimSpace(in.ToolName)
	in.ToolDescription = strings.TrimSpace(in.ToolDescription)
	if in.ToolName == "" || in.ToolDescription == "" || in.InputSchema == nil {
		return errorResult("tool_name, tool_description and input_schema are required"), nil
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(in.ToolName) {
		return errorResult("tool_name must be snake_case: ^[a-z][a-z0-9_]*$"), nil
	}

	useWorktree := true
	if in.UseWorktree != nil {
		useWorktree = *in.UseWorktree
	}
	runGofmt := true
	if in.RunGofmt != nil {
		runGofmt = *in.RunGofmt
	}
	runTests := false
	if in.RunTests != nil {
		runTests = *in.RunTests
	}
	allowInPolicy := false
	if in.AllowInPolicy != nil {
		allowInPolicy = *in.AllowInPolicy
	}
	addToSearch := true
	if in.AddToSearch != nil {
		addToSearch = *in.AddToSearch
	}
	addPromptHint := false
	if in.AddPromptHint != nil {
		addPromptHint = *in.AddPromptHint
	}

	if strings.TrimSpace(in.Domain) == "" {
		in.Domain = strings.Split(in.ToolName, "_")[0]
	}
	in.Domain = strings.ToLower(strings.TrimSpace(in.Domain))

	handlerMethod := strings.TrimSpace(in.HandlerMethod)
	if handlerMethod == "" {
		handlerMethod = lowerFirst(camelFromSnake(in.ToolName))
	}
	if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(handlerMethod) {
		return errorResult("handler_method must be a valid Go identifier"), nil
	}

	repoRoot, err := gitTopLevel(ctx)
	if err != nil {
		return errorResult("git repo not found: " + err.Error()), nil
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	branch := fmt.Sprintf("dev/scaffold/%s-%s", in.ToolName, ts)

	worktreeRoot := strings.TrimSpace(in.WorktreeRoot)
	if worktreeRoot == "" {
		worktreeRoot = ".worktrees"
	}

	worktreeName := strings.TrimSpace(in.WorktreeName)
	if worktreeName == "" {
		worktreeName = fmt.Sprintf("dev-%s-%s", in.ToolName, ts)
	}

	worktreePath := filepath.Join(repoRoot, filepath.Clean(worktreeRoot), filepath.Clean(worktreeName))
	if err := ensureSubpath(repoRoot, worktreePath); err != nil {
		return errorResult("invalid worktree_path: " + err.Error()), nil
	}

	if useWorktree {
		if _, err := runCmd(ctx, repoRoot, "git", "worktree", "add", "-b", branch, worktreePath); err != nil {
			return errorResult("git worktree add failed: " + err.Error()), nil
		}
	} else {
		// Dev mode still requires a worktree target to avoid touching the main tree.
		return errorResult("use_worktree=false is not supported (worktree isolation is required)"), nil
	}

	targetDir := strings.TrimSpace(in.TargetDir)
	if targetDir == "" {
		targetDir = "tasks/scaffolds"
	}
	targetDirAbs := filepath.Join(worktreePath, filepath.Clean(targetDir))
	if err := ensureSubpath(worktreePath, targetDirAbs); err != nil {
		return errorResult("invalid target_dir: " + err.Error()), nil
	}
	if err := os.MkdirAll(targetDirAbs, 0o755); err != nil {
		return errorResult("failed to create target_dir: " + err.Error()), nil
	}

	toolFileRel := filepath.ToSlash(filepath.Join("internal", "tools", in.ToolName+".go"))
	inputType := handlerMethod + "Input"

	toolFileContent, warn, err := generateToolFileBestEffort(ctx, in, handlerMethod, inputType)
	if err != nil {
		return errorResult("failed to generate tool implementation: " + err.Error()), nil
	}

	// Apply edits inside worktree to generate a patch.
	filesTouched := []string{}
	warnings := []string{}
	if warn != "" {
		warnings = append(warnings, warn)
	}

	toolAbs := filepath.Join(worktreePath, filepath.FromSlash(toolFileRel))
	if err := os.MkdirAll(filepath.Dir(toolAbs), 0o755); err != nil {
		return errorResult("failed to create tool directory: " + err.Error()), nil
	}
	if err := os.WriteFile(toolAbs, []byte(toolFileContent), 0o644); err != nil {
		return errorResult("failed to write tool file: " + err.Error()), nil
	}
	filesTouched = append(filesTouched, toolFileRel)

	metaPath := filepath.Join(worktreePath, "internal", "tools", "meta.go")
	if err := patchMetaForNewTool(metaPath, in, handlerMethod, addToSearch); err != nil {
		return errorResult("failed to update internal/tools/meta.go: " + err.Error()), nil
	}
	filesTouched = append(filesTouched, "internal/tools/meta.go")

	policyPath := filepath.Join(worktreePath, "internal", "router", "policy.go")
	if allowInPolicy {
		if err := addToolToPolicyAllowlist(policyPath, in.ToolName); err != nil {
			return errorResult("failed to update internal/router/policy.go: " + err.Error()), nil
		}
		filesTouched = append(filesTouched, "internal/router/policy.go")
	}

	promptPath := filepath.Join(worktreePath, "internal", "router", "prompt.go")
	if addPromptHint {
		if err := addPromptHintForTool(promptPath, in.ToolName, in.ToolDescription); err != nil {
			return errorResult("failed to update internal/router/prompt.go: " + err.Error()), nil
		}
		filesTouched = append(filesTouched, "internal/router/prompt.go")
	}

	if runGofmt {
		gofmtArgs := append([]string{"-w"}, dedupeFiles(filesTouched)...)
		_, _ = runCmd(ctx, worktreePath, "gofmt", gofmtArgs...)
	}

	// Stage tracked changes (including new file) and produce patch.
	if _, err := runCmd(ctx, worktreePath, "git", "add", "-A"); err != nil {
		return errorResult("git add failed: " + err.Error()), nil
	}
	diff, err := runCmd(ctx, worktreePath, "git", "diff", "--cached")
	if err != nil {
		return errorResult("git diff failed: " + err.Error()), nil
	}

	patchPath := filepath.Join(targetDirAbs, in.ToolName+".patch")
	if err := os.WriteFile(patchPath, []byte(diff), 0o644); err != nil {
		return errorResult("failed to write patch: " + err.Error()), nil
	}

	// Reset and re-apply patch inside the worktree.
	if _, err := runCmd(ctx, worktreePath, "git", "reset", "--hard"); err != nil {
		return errorResult("git reset --hard failed: " + err.Error()), nil
	}
	if _, err := runCmd(ctx, worktreePath, "git", "apply", patchPath); err != nil {
		return errorResult("git apply failed: " + err.Error()), nil
	}

	if runTests {
		if _, err := runCmd(ctx, worktreePath, "go", "test", "./..."); err != nil {
			warnings = append(warnings, "go test failed: "+err.Error())
		}
	}

	out := map[string]any{
		"ok":            true,
		"tool_name":     in.ToolName,
		"branch":        branch,
		"worktree_path": worktreePath,
		"patch_path":    patchPath,
		"files_touched": dedupeFiles(filesTouched),
		"warnings":      warnings,
		"next_steps": []string{
			fmt.Sprintf("cd %s", worktreePath),
			"git status",
			"go test ./...",
			"git add -A",
			fmt.Sprintf("git commit -m \"add %s\"", in.ToolName),
		},
	}
	return jsonResult(out), nil
}

func generateToolFileBestEffort(ctx context.Context, in devScaffoldToolInput, handlerMethod string, inputType string) (content string, warning string, err error) {
	// If OpenRouter isn't configured, fall back to a deterministic skeleton.
	cl, clErr := router.NewOpenRouterClientFromEnv()
	if clErr != nil {
		return generateToolFileSkeleton(in, handlerMethod, inputType), "OpenRouter not configured; generated skeleton only", nil
	}

	schemaBytes, _ := json.MarshalIndent(in.InputSchema, "", "  ")
	payload := map[string]any{
		"tool_name":        in.ToolName,
		"tool_description": in.ToolDescription,
		"domain":           in.Domain,
		"handler_method":   handlerMethod,
		"input_type":       inputType,
		"input_schema":     json.RawMessage(schemaBytes),
		"spec":             strings.TrimSpace(in.Spec),
		"repo_rules": []string{
			"package must be 'tools'",
			"handler signature: func (h *Handler) <handler_method>(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error)",
			"use errorResult(...) and jsonResult(...) for outputs",
			"strictly parse/validate input; return user-facing errors via errorResult",
			"do not add new external dependencies",
			"prefer read-only operations; do not create/update/delete remote resources",
		},
	}
	user, _ := json.MarshalIndent(payload, "", "  ")

	system := "You are a senior Go engineer working on mcp-lens. Return ONLY valid Go source code (no markdown, no backticks)."
	resp, err := cl.ChatCompletionText(ctx, system, string(user))
	if err != nil {
		return "", "", err
	}
	code := extractGoFile(resp)
	if strings.TrimSpace(code) == "" || !strings.HasPrefix(strings.TrimSpace(code), "package ") {
		return generateToolFileSkeleton(in, handlerMethod, inputType), "LLM output invalid; generated skeleton only", nil
	}
	return code, "", nil
}

func extractGoFile(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```go")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	if idx := strings.Index(s, "package "); idx >= 0 {
		s = s[idx:]
	}
	return strings.TrimSpace(s) + "\n"
}

func generateToolFileSkeleton(in devScaffoldToolInput, handlerMethod string, inputType string) string {
	props, _ := in.InputSchema["properties"].(map[string]any)
	required, _ := in.InputSchema["required"].([]any)
	reqSet := map[string]struct{}{}
	for _, r := range required {
		if s, ok := r.(string); ok && strings.TrimSpace(s) != "" {
			reqSet[s] = struct{}{}
		}
	}

	type field struct {
		Name string
		Type string
		Tag  string
		Req  bool
	}
	fields := []field{}
	for k, v := range props {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		schema, _ := v.(map[string]any)
		goType := goTypeForSchema(schema)
		name := camelFromSnake(k)
		if name == "" {
			continue
		}
		_, req := reqSet[k]
		tag := fmt.Sprintf("`json:\"%s,omitempty\"`", k)
		if req {
			tag = fmt.Sprintf("`json:\"%s\"`", k)
		}
		fields = append(fields, field{Name: name, Type: goType, Tag: tag, Req: req})
	}

	var sb strings.Builder
	sb.WriteString("package tools\n\n")
	sb.WriteString("import (\n")
	sb.WriteString("\t\"context\"\n")
	sb.WriteString("\t\"encoding/json\"\n")
	sb.WriteString("\t\"strings\"\n\n")
	sb.WriteString("\t\"github.com/golovatskygroup/mcp-lens/pkg/mcp\"\n")
	sb.WriteString(")\n\n")
	sb.WriteString(fmt.Sprintf("type %s struct {\n", inputType))
	for _, f := range fields {
		sb.WriteString(fmt.Sprintf("\t%s %s %s\n", f.Name, f.Type, f.Tag))
	}
	sb.WriteString("}\n\n")
	sb.WriteString(fmt.Sprintf("func (h *Handler) %s(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {\n", handlerMethod))
	sb.WriteString(fmt.Sprintf("\tvar in %s\n", inputType))
	sb.WriteString("\tif err := json.Unmarshal(args, &in); err != nil {\n")
	sb.WriteString("\t\treturn errorResult(\"Invalid input: \" + err.Error()), nil\n")
	sb.WriteString("\t}\n")
	for _, f := range fields {
		if !f.Req {
			continue
		}
		switch f.Type {
		case "string":
			sb.WriteString(fmt.Sprintf("\tif strings.TrimSpace(in.%s) == \"\" {\n", f.Name))
			sb.WriteString(fmt.Sprintf("\t\treturn errorResult(\"%s is required\"), nil\n", strings.ToLower(f.Name)))
			sb.WriteString("\t}\n")
		case "int":
			sb.WriteString(fmt.Sprintf("\tif in.%s == 0 {\n", f.Name))
			sb.WriteString(fmt.Sprintf("\t\treturn errorResult(\"%s is required\"), nil\n", strings.ToLower(f.Name)))
			sb.WriteString("\t}\n")
		}
	}
	sb.WriteString("\t_ = ctx\n")
	sb.WriteString("\treturn jsonResult(map[string]any{\"ok\": true}), nil\n")
	sb.WriteString("}\n")
	return sb.String()
}

func goTypeForSchema(s map[string]any) string {
	typ, _ := s["type"].(string)
	switch typ {
	case "string":
		return "string"
	case "integer":
		return "int"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "array":
		items, _ := s["items"].(map[string]any)
		if itemsType, _ := items["type"].(string); itemsType == "string" {
			return "[]string"
		}
		return "[]any"
	case "object":
		return "map[string]any"
	default:
		return "any"
	}
}

func patchMetaForNewTool(path string, in devScaffoldToolInput, handlerMethod string, addToSearch bool) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)

	// 1) BuiltinTools(): insert tool schema.
	toolSchemaBytes, _ := json.MarshalIndent(in.InputSchema, "", "  ")
	toolEntry := fmt.Sprintf("\t\t{\n\t\t\tName:        %q,\n\t\t\tDescription: %q,\n\t\t\tInputSchema: json.RawMessage(`%s`),\n\t\t},\n", in.ToolName, in.ToolDescription, string(toolSchemaBytes))
	insertBefore := builtinInsertAnchor(in.Domain)
	if idx := strings.Index(s, fmt.Sprintf("\t\t{\n\t\t\tName:        %q,", insertBefore)); idx >= 0 {
		s = s[:idx] + toolEntry + s[idx:]
	} else if idx := strings.Index(s, "\t\t{\n\t\t\tName:        \"router\""); idx >= 0 {
		s = s[:idx] + toolEntry + s[idx:]
	} else {
		return fmt.Errorf("failed to locate BuiltinTools insertion point")
	}

	// 2) Handle(): add case.
	caseLine := fmt.Sprintf("\tcase %q:\n\t\treturn h.%s(ctx, args)\n", in.ToolName, handlerMethod)
	if strings.Contains(s, "case "+strconvQuote(in.ToolName)+":") {
		// already present
	} else if idx := strings.Index(s, "\tdefault:\n\t\treturn nil, fmt.Errorf(\"unknown tool: %s\", name)\n\t}\n}\n"); idx >= 0 {
		s = s[:idx] + caseLine + s[idx:]
	} else {
		return fmt.Errorf("failed to locate Handle() insertion point")
	}

	// 3) IsLocalTool(): add to allowlist switch.
	s, err = addToolToIsLocalToolSwitch(s, in.Domain, in.ToolName)
	if err != nil {
		return err
	}

	// 4) searchLocalTools() list.
	if addToSearch {
		needle := fmt.Sprintf("{Name: %q, Category: \"local\",", in.ToolName)
		if !strings.Contains(s, needle) {
			summary := fmt.Sprintf("\t\t{Name: %q, Category: \"local\", Description: %q},\n", in.ToolName, in.ToolDescription)
			if idx := strings.Index(s, "\t\t{Name: \"jira_add_comment\""); idx >= 0 && in.Domain == "jira" {
				s = s[:idx] + summary + s[idx:]
			} else if idx := strings.Index(s, "\t\t{Name: \"grafana_health\""); idx >= 0 && in.Domain == "confluence" {
				s = s[:idx] + summary + s[idx:]
			} else if idx := strings.Index(s, "\t\t{Name: \"router\""); idx >= 0 {
				s = s[:idx] + summary + s[idx:]
			}
		}
	}

	return os.WriteFile(path, []byte(s), 0o644)
}

func addToolToIsLocalToolSwitch(file string, domain string, toolName string) (string, error) {
	funcStart := strings.Index(file, "func (h *Handler) IsLocalTool(name string) bool {")
	if funcStart < 0 {
		return file, fmt.Errorf("IsLocalTool() not found")
	}
	caseStartRel := strings.Index(file[funcStart:], "case \"router\", \"query\"")
	if caseStartRel < 0 {
		return file, fmt.Errorf("IsLocalTool() case list not found")
	}
	caseStart := funcStart + caseStartRel

	colonRel := strings.Index(file[caseStart:], ":\n\t\treturn true")
	if colonRel < 0 {
		return file, fmt.Errorf("IsLocalTool() case list end not found")
	}
	colonIdx := caseStart + colonRel
	caseBlock := file[caseStart : colonIdx+1] // include ':'

	quotedTool := strconvQuote(toolName)
	if strings.Contains(caseBlock, quotedTool) {
		return file, nil
	}

	anchor := isLocalToolAnchor(domain)
	quotedAnchor := strconvQuote(anchor)

	newCaseBlock := caseBlock
	if strings.Contains(caseBlock, quotedAnchor+",") {
		newCaseBlock = strings.Replace(caseBlock, quotedAnchor+",", quotedAnchor+", "+quotedTool+",", 1)
	} else if strings.Contains(caseBlock, quotedAnchor+":") {
		newCaseBlock = strings.Replace(caseBlock, quotedAnchor+":", quotedAnchor+", "+quotedTool+":", 1)
	} else {
		// fallback: insert right before ':' at the end of the case list.
		if strings.HasSuffix(newCaseBlock, ":") {
			newCaseBlock = strings.TrimSuffix(newCaseBlock, ":") + ", " + quotedTool + ":"
		} else {
			return file, fmt.Errorf("failed to update IsLocalTool() case list")
		}
	}

	return file[:caseStart] + newCaseBlock + file[colonIdx+1:], nil
}

func builtinInsertAnchor(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "github":
		return "jira_get_myself"
	case "jira":
		return "jira_add_comment"
	case "confluence":
		return "grafana_health"
	case "grafana":
		return "router"
	default:
		return "router"
	}
}

func isLocalToolAnchor(domain string) string {
	switch strings.ToLower(strings.TrimSpace(domain)) {
	case "github":
		return "fetch_complete_pr_files"
	case "jira":
		return "jira_list_projects"
	case "confluence":
		return "confluence_search_cql"
	case "grafana":
		return "grafana_list_annotation_tags"
	default:
		return "grafana_list_annotation_tags"
	}
}

func addToolToPolicyAllowlist(path string, toolName string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	if strings.Contains(s, strconvQuote(toolName)+":") {
		return nil
	}
	marker := "allowLocal := map[string]struct{}{"
	idx := strings.Index(s, marker)
	if idx < 0 {
		return fmt.Errorf("allowLocal map not found")
	}
	// Insert near the end of the map (before closing brace of allowLocal literal).
	end := strings.Index(s[idx:], "\t}\n\n\t// Start conservative")
	if end < 0 {
		return fmt.Errorf("allowLocal end not found")
	}
	insertAt := idx + end
	line := fmt.Sprintf("\t\t%s:                  {},\n", strconvQuote(toolName))
	s = s[:insertAt] + line + s[insertAt:]
	return os.WriteFile(path, []byte(s), 0o644)
}

func addPromptHintForTool(path string, toolName string, desc string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	s := string(b)
	hint := fmt.Sprintf("\t\t\t\t\"If you need the %s tool: %s\",\n", toolName, desc)
	if strings.Contains(s, hint) {
		return nil
	}
	marker := "\"jira_workflow\": []string{"
	if idx := strings.Index(s, marker); idx >= 0 {
		// Add to Jira workflow by default.
		after := idx + len(marker)
		s = s[:after] + "\n" + hint + s[after:]
		return os.WriteFile(path, []byte(s), 0o644)
	}
	return fmt.Errorf("prompt insertion point not found")
}

func gitTopLevel(ctx context.Context) (string, error) {
	out, err := runCmd(ctx, "", "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runCmd(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s: %s", name, msg)
	}
	return stdout.String(), nil
}

func ensureSubpath(root string, sub string) error {
	root = filepath.Clean(root)
	sub = filepath.Clean(sub)
	rel, err := filepath.Rel(root, sub)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("path escapes root")
	}
	return nil
}

func camelFromSnake(s string) string {
	parts := strings.Split(strings.TrimSpace(s), "_")
	var out strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		out.WriteString(strings.ToUpper(p[:1]))
		if len(p) > 1 {
			out.WriteString(p[1:])
		}
	}
	return out.String()
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToLower(s[:1]) + s[1:]
}

func dedupeFiles(paths []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
