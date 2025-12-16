package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/golovatskygroup/mcp-lens/internal/artifacts"
	"github.com/golovatskygroup/mcp-lens/internal/router"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
	"golang.org/x/sync/errgroup"
)

type routerInput struct {
	Input         string                `json:"input"`
	Context       map[string]any        `json:"context,omitempty"`
	Output        *router.OutputOptions `json:"output,omitempty"`
	Mode          string                `json:"mode,omitempty"` // auto|planner|executor
	Steps         []router.PlanStep     `json:"steps,omitempty"`
	MaxSteps      int                   `json:"max_steps,omitempty"`
	Parallelism   int                   `json:"parallelism,omitempty"`
	IncludeAnswer bool                  `json:"include_answer,omitempty"`
	DryRun        bool                  `json:"dry_run,omitempty"`
	Format        string                `json:"format,omitempty"` // json|text
}

func (h *Handler) runRouter(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in routerInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Input) == "" {
		return errorResult("input is required"), nil
	}
	if in.Context == nil {
		in.Context = map[string]any{}
	}

	// Deterministic context extraction (URLs/IDs) before any planning.
	if extracted := router.ExtractStructuredContext(in.Input); len(extracted) > 0 {
		for k, v := range extracted {
			if _, exists := in.Context[k]; !exists {
				in.Context[k] = v
			}
		}
	}

	// Convenience: allow prefixing the query with `jira <client>` to route Jira calls to that client.
	if client, rest := extractJiraClientPrefix(in.Input); client != "" {
		in.Context["jira_client"] = client
		if rest != "" {
			in.Input = rest
		}
	}

	// Convenience: allow prefixing the query with `confluence <client>` to route Confluence calls to that client.
	if client, rest := extractConfluenceClientPrefix(in.Input); client != "" {
		in.Context["confluence_client"] = client
		if rest != "" {
			in.Input = rest
		}
	}

	// Convenience: allow prefixing the query with `grafana <client>` to route Grafana calls to that client.
	if client, rest := extractGrafanaClientPrefix(in.Input); client != "" {
		in.Context["grafana_client"] = client
		if rest != "" {
			in.Input = rest
		}
	}

	// Provide configured Jira clients (no secrets) to the planner so it can pick the right instance.
	if clients := jiraPublicClientsFromEnv(); len(clients) > 0 {
		in.Context["jira_clients"] = clients
	}
	if def := strings.TrimSpace(os.Getenv("JIRA_DEFAULT_CLIENT")); def != "" {
		in.Context["jira_default_client"] = def
	}

	// Provide configured Confluence clients (no secrets) to the planner so it can pick the right instance.
	if clients := confluencePublicClientsFromEnv(); len(clients) > 0 {
		in.Context["confluence_clients"] = clients
	}
	if def := strings.TrimSpace(os.Getenv("CONFLUENCE_DEFAULT_CLIENT")); def != "" {
		in.Context["confluence_default_client"] = def
	}

	// Provide configured Grafana clients (no secrets) to the planner so it can pick the right instance.
	if clients := grafanaPublicClientsFromEnv(); len(clients) > 0 {
		in.Context["grafana_clients"] = clients
	}
	if def := strings.TrimSpace(os.Getenv("GRAFANA_DEFAULT_CLIENT")); def != "" {
		in.Context["grafana_default_client"] = def
	}

	if in.MaxSteps <= 0 {
		if len(in.Steps) > 0 {
			in.MaxSteps = len(in.Steps)
		} else {
			in.MaxSteps = 5
		}
	}
	if in.MaxSteps > 8 {
		return errorResult("max_steps must be <= 8"), nil
	}
	if in.Format == "" {
		in.Format = "json"
	}
	if in.Parallelism <= 0 {
		in.Parallelism = 1
	}
	if in.Parallelism > 8 {
		return errorResult("parallelism must be <= 8"), nil
	}

	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "auto", "planner", "executor":
	default:
		return errorResult("mode must be one of: auto, planner, executor"), nil
	}

	if mode == "executor" && len(in.Steps) == 0 {
		return errorResult("executor mode requires non-empty steps"), nil
	}
	if mode == "planner" && len(in.Steps) > 0 {
		return errorResult("planner mode does not allow steps"), nil
	}

	// Fast-path: safe discovery/help without OpenRouter.
	if fp := parseDiscoveryFastPath(in.Input); fp != nil {
		policy := router.DefaultPolicy()
		plan := router.ModelPlan{Steps: []router.PlanStep{fp.step}, FinalAnswerNeeded: false}
		if err := router.ValidatePlan(plan, policy, h.buildRouterCatalog(), 1); err != nil {
			return errorResult(err.Error()), nil
		}
		res := router.RouterResult{Plan: plan}
		if !in.DryRun {
			execSteps, manifest, err := h.executePlan(ctx, plan, policy, in.Output, in.Parallelism)
			res.ExecutedSteps = execSteps
			res.Manifest = manifest
			if err != nil {
				return jsonResult(res), nil
			}
		}
		if fp.answer != "" {
			res.Answer = fp.answer
		}
		if strings.EqualFold(in.Format, "text") {
			b, _ := json.MarshalIndent(res, "", "  ")
			return textResult(fmt.Sprintf("%s\n", string(b))), nil
		}
		return jsonResult(res), nil
	}

	// Executor mode: validate + execute provided steps without calling the planner model.
	if len(in.Steps) > 0 && mode != "planner" {
		policy := router.DefaultPolicy()
		plan := router.ModelPlan{Steps: in.Steps, FinalAnswerNeeded: in.IncludeAnswer}
		if err := router.ValidatePlan(plan, policy, h.buildRouterCatalog(), in.MaxSteps); err != nil {
			return errorResult(err.Error()), nil
		}

		res := router.RouterResult{Plan: plan}
		if !in.DryRun {
			execSteps, manifest, err := h.executePlan(ctx, plan, policy, in.Output, in.Parallelism)
			res.ExecutedSteps = execSteps
			res.Manifest = manifest
			if err != nil {
				if strings.EqualFold(in.Format, "text") {
					b, _ := json.MarshalIndent(res, "", "  ")
					return textResult(fmt.Sprintf("%s\n", string(b))), nil
				}
				return jsonResult(res), nil
			}
		}

		if in.IncludeAnswer {
			// Executor mode is designed to work even without OpenRouter; provide a deterministic summary.
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Executed %d step(s).\n", len(res.ExecutedSteps)))
			for i, st := range res.ExecutedSteps {
				if st.OK {
					sb.WriteString(fmt.Sprintf("%d) %s: ok\n", i+1, st.Name))
				} else {
					sb.WriteString(fmt.Sprintf("%d) %s: error: %s\n", i+1, st.Name, st.Error))
				}
			}
			if res.Manifest != nil && len(res.Manifest.Artifacts) > 0 {
				sb.WriteString("\nArtifacts:\n")
				for _, a := range res.Manifest.Artifacts {
					sb.WriteString(fmt.Sprintf("- %s (%d bytes)\n", a.Path, a.Bytes))
				}
			}
			res.Answer = sb.String()
		}

		if strings.EqualFold(in.Format, "text") {
			b, _ := json.MarshalIndent(res, "", "  ")
			return textResult(fmt.Sprintf("%s\n", string(b))), nil
		}
		return jsonResult(res), nil
	}

	cl, err := router.NewOpenRouterClientFromEnv()
	if err != nil {
		return errorResult(err.Error()), nil
	}

	policy := router.DefaultPolicy()

	catalog := h.buildRouterCatalog()

	plan, rawPlan, err := router.Plan(ctx, cl, in.Input, in.Context, catalog, in.MaxSteps)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Make Jira instance selection deterministic (do not rely on the model to thread it through).
	h.applyJiraClientToPlan(&plan, in.Context)
	// Make Confluence instance selection deterministic (do not rely on the model to thread it through).
	h.applyConfluenceClientToPlan(&plan, in.Context)
	// Make Grafana instance selection deterministic (do not rely on the model to thread it through).
	h.applyGrafanaClientToPlan(&plan, in.Context)
	// Make URL/ID context injection deterministic (do not rely on the model).
	h.applyExtractedContextToPlan(&plan, in.Context)

	if err := router.ValidatePlan(plan, policy, catalog, in.MaxSteps); err != nil {
		return errorResult(err.Error() + "\nplan=" + string(rawPlan)), nil
	}

	res := router.RouterResult{Plan: plan}

	if !in.DryRun {
		execSteps, manifest, err := h.executePlan(ctx, plan, policy, in.Output, in.Parallelism)
		if err != nil {
			res.ExecutedSteps = execSteps
			res.Manifest = manifest
			return jsonResult(res), nil
		}
		res.ExecutedSteps = execSteps
		res.Manifest = manifest
	}

	if in.IncludeAnswer && mode != "planner" {
		answer, err := router.Summarize(ctx, cl, in.Input, res)
		if err == nil {
			res.Answer = answer
		}
	}

	if strings.EqualFold(in.Format, "text") {
		// Very small text wrapper around JSON.
		b, _ := json.MarshalIndent(res, "", "  ")
		return textResult(fmt.Sprintf("%s\n", string(b))), nil
	}
	return jsonResult(res), nil
}

type discoveryFastPath struct {
	step   router.PlanStep
	answer string
}

var reDescribe = regexp.MustCompile(`(?i)^\s*(describe_tool|describe\s+tool)\s+([a-zA-Z0-9_./-]+)\s*$`)
var reSearch = regexp.MustCompile(`(?i)^\s*(search_tools|search\s+tools)\s+(.*)\s*$`)

func parseDiscoveryFastPath(input string) *discoveryFastPath {
	s := strings.TrimSpace(input)
	if s == "" {
		return nil
	}

	if strings.EqualFold(s, "help") || strings.EqualFold(s, "tools") || strings.EqualFold(s, "list tools") || strings.EqualFold(s, "list_tools") {
		args, _ := json.Marshal(map[string]any{"query": "tools", "format": "text", "limit": 20})
		return &discoveryFastPath{
			step:   router.PlanStep{Name: "search_tools", Source: "local", Args: args},
			answer: "Use `query` with a free-form request. For discovery, you can ask:\n- \"search tools grafana\"\n- \"describe tool grafana_get_dashboard\"",
		}
	}

	if m := reDescribe.FindStringSubmatch(s); len(m) == 3 {
		name := strings.TrimSpace(m[2])
		args, _ := json.Marshal(map[string]any{"name": name})
		return &discoveryFastPath{step: router.PlanStep{Name: "describe_tool", Source: "local", Args: args}}
	}

	if m := reSearch.FindStringSubmatch(s); len(m) == 3 {
		q := strings.TrimSpace(m[2])
		if q == "" {
			q = "tools"
		}
		args, _ := json.Marshal(map[string]any{"query": q, "format": "text", "limit": 20})
		return &discoveryFastPath{step: router.PlanStep{Name: "search_tools", Source: "local", Args: args}}
	}

	// Natural language discovery intent.
	lower := strings.ToLower(s)
	if strings.Contains(lower, "tools") || strings.Contains(lower, "инструмент") || strings.Contains(lower, "тулы") {
		if strings.Contains(lower, "доступ") || strings.Contains(lower, "available") || strings.Contains(lower, "list") || strings.Contains(lower, "какие") || strings.Contains(lower, "what") || strings.Contains(lower, "show") {
			q := ""
			for _, dom := range []string{"grafana", "jira", "confluence", "github"} {
				if strings.Contains(lower, dom) {
					q = dom
					break
				}
			}
			if q == "" {
				q = "tools"
			}
			args, _ := json.Marshal(map[string]any{"query": q, "format": "text", "limit": 20})
			return &discoveryFastPath{step: router.PlanStep{Name: "search_tools", Source: "local", Args: args}}
		}
	}

	return nil
}

func (h *Handler) applyExtractedContextToPlan(plan *router.ModelPlan, ctx map[string]any) {
	if plan == nil || len(plan.Steps) == 0 || ctx == nil {
		return
	}

	getStr := func(k string) string {
		if v, ok := ctx[k].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	getInt := func(k string) int {
		switch v := ctx[k].(type) {
		case int:
			return v
		case float64:
			return int(v)
		default:
			return 0
		}
	}

	githubRepo := getStr("github_repo")
	githubPRNum := getInt("github_pr_number")
	jiraIssue := getStr("jira_issue_key")
	confluencePageID := getStr("confluence_page_id")

	grafanaBaseURL := getStr("grafana_base_url")
	grafanaOrgID := getInt("grafana_org_id")
	grafanaDashUID := getStr("grafana_dashboard_uid")

	for i := range plan.Steps {
		step := plan.Steps[i]
		var args map[string]any
		if err := json.Unmarshal(step.Args, &args); err != nil || args == nil {
			continue
		}

		switch {
		case step.Source == "local" && strings.HasPrefix(step.Name, "get_pull_request_"):
			if githubRepo != "" {
				if v, ok := args["repo"].(string); !ok || strings.TrimSpace(v) == "" {
					args["repo"] = githubRepo
				}
			}
			if githubPRNum > 0 {
				switch v := args["number"].(type) {
				case nil:
					args["number"] = githubPRNum
				case float64:
					if int(v) == 0 {
						args["number"] = githubPRNum
					}
				case int:
					if v == 0 {
						args["number"] = githubPRNum
					}
				}
			}
		case step.Source == "local" && strings.HasPrefix(step.Name, "list_pull_request_"):
			if githubRepo != "" {
				if v, ok := args["repo"].(string); !ok || strings.TrimSpace(v) == "" {
					args["repo"] = githubRepo
				}
			}
			if githubPRNum > 0 {
				switch v := args["number"].(type) {
				case nil:
					args["number"] = githubPRNum
				case float64:
					if int(v) == 0 {
						args["number"] = githubPRNum
					}
				case int:
					if v == 0 {
						args["number"] = githubPRNum
					}
				}
			}
		case step.Source == "local" && strings.HasPrefix(step.Name, "jira_"):
			if jiraIssue != "" {
				if v, ok := args["issue"].(string); !ok || strings.TrimSpace(v) == "" {
					args["issue"] = jiraIssue
				}
			}
		case step.Source == "local" && strings.HasPrefix(step.Name, "confluence_"):
			if confluencePageID != "" && step.Name == "confluence_get_page" {
				if v, ok := args["id"].(string); !ok || strings.TrimSpace(v) == "" {
					args["id"] = confluencePageID
				}
			}
		case step.Source == "local" && strings.HasPrefix(step.Name, "grafana_"):
			// Respect explicit base_url, but fill if empty.
			if grafanaBaseURL != "" {
				if v, ok := args["base_url"].(string); !ok || strings.TrimSpace(v) == "" {
					args["base_url"] = grafanaBaseURL
				}
			}
			if grafanaOrgID > 0 {
				switch v := args["org_id"].(type) {
				case nil:
					args["org_id"] = grafanaOrgID
				case float64:
					if int(v) == 0 {
						args["org_id"] = grafanaOrgID
					}
				case int:
					if v == 0 {
						args["org_id"] = grafanaOrgID
					}
				}
			}
			if grafanaDashUID != "" && (step.Name == "grafana_get_dashboard" || step.Name == "grafana_get_dashboard_summary") {
				if v, ok := args["uid"].(string); !ok || strings.TrimSpace(v) == "" {
					args["uid"] = grafanaDashUID
				}
			}
		}

		if b, err := json.Marshal(args); err == nil {
			plan.Steps[i].Args = b
		}
	}
}

func extractJiraClientPrefix(input string) (client string, rest string) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "jira ") {
		return "", ""
	}
	after := strings.TrimSpace(s[len("jira "):])
	if after == "" {
		return "", ""
	}
	parts := strings.Fields(after)
	if len(parts) == 0 {
		return "", ""
	}
	client = parts[0]
	rest = strings.TrimSpace(after[len(parts[0]):])
	return client, rest
}

func extractConfluenceClientPrefix(input string) (client string, rest string) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "confluence ") {
		return "", ""
	}
	after := strings.TrimSpace(s[len("confluence "):])
	if after == "" {
		return "", ""
	}
	parts := strings.Fields(after)
	if len(parts) == 0 {
		return "", ""
	}
	client = parts[0]
	rest = strings.TrimSpace(after[len(parts[0]):])
	return client, rest
}

func extractGrafanaClientPrefix(input string) (client string, rest string) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", ""
	}
	lower := strings.ToLower(s)
	if !strings.HasPrefix(lower, "grafana ") {
		return "", ""
	}
	after := strings.TrimSpace(s[len("grafana "):])
	if after == "" {
		return "", ""
	}
	parts := strings.Fields(after)
	if len(parts) == 0 {
		return "", ""
	}
	client = parts[0]
	rest = strings.TrimSpace(after[len(parts[0]):])
	return client, rest
}

func (h *Handler) applyJiraClientToPlan(plan *router.ModelPlan, ctx map[string]any) {
	if plan == nil || len(plan.Steps) == 0 {
		return
	}
	if ctx == nil {
		return
	}
	client, _ := ctx["jira_client"].(string)
	client = strings.TrimSpace(client)
	if client == "" {
		return
	}

	for i := range plan.Steps {
		step := plan.Steps[i]
		if step.Source != "local" || !strings.HasPrefix(step.Name, "jira_") {
			continue
		}

		var args map[string]any
		if err := json.Unmarshal(step.Args, &args); err != nil || args == nil {
			continue
		}
		if _, ok := args["client"]; ok {
			continue
		}
		// If caller explicitly set base_url, do not override it.
		if base, ok := args["base_url"].(string); ok && strings.TrimSpace(base) != "" {
			continue
		}
		args["client"] = client
		if b, err := json.Marshal(args); err == nil {
			plan.Steps[i].Args = b
		}
	}
}

func (h *Handler) applyConfluenceClientToPlan(plan *router.ModelPlan, ctx map[string]any) {
	if plan == nil || len(plan.Steps) == 0 {
		return
	}
	if ctx == nil {
		return
	}
	client, _ := ctx["confluence_client"].(string)
	client = strings.TrimSpace(client)
	if client == "" {
		return
	}

	for i := range plan.Steps {
		step := plan.Steps[i]
		if step.Source != "local" || !strings.HasPrefix(step.Name, "confluence_") {
			continue
		}

		var args map[string]any
		if err := json.Unmarshal(step.Args, &args); err != nil || args == nil {
			continue
		}
		if _, ok := args["client"]; ok {
			continue
		}
		// If caller explicitly set base_url, do not override it.
		if base, ok := args["base_url"].(string); ok && strings.TrimSpace(base) != "" {
			continue
		}
		args["client"] = client
		if b, err := json.Marshal(args); err == nil {
			plan.Steps[i].Args = b
		}
	}
}

func (h *Handler) applyGrafanaClientToPlan(plan *router.ModelPlan, ctx map[string]any) {
	if plan == nil || len(plan.Steps) == 0 {
		return
	}
	if ctx == nil {
		return
	}

	client, _ := ctx["grafana_client"].(string)
	client = strings.TrimSpace(client)

	baseURL, _ := ctx["grafana_base_url"].(string)
	baseURL = strings.TrimSpace(baseURL)

	dashboardUID, _ := ctx["grafana_dashboard_uid"].(string)
	dashboardUID = strings.TrimSpace(dashboardUID)

	orgID := 0
	switch v := ctx["grafana_org_id"].(type) {
	case int:
		orgID = v
	case float64:
		orgID = int(v)
	}

	if client == "" && baseURL == "" && dashboardUID == "" && orgID == 0 {
		return
	}

	for i := range plan.Steps {
		step := plan.Steps[i]
		if step.Source != "local" || !strings.HasPrefix(step.Name, "grafana_") {
			continue
		}

		var args map[string]any
		if err := json.Unmarshal(step.Args, &args); err != nil || args == nil {
			continue
		}

		// If caller explicitly set base_url, do not override it (but still allow filling uid/org_id/client).
		baseExplicit := false
		if base, ok := args["base_url"].(string); ok && strings.TrimSpace(base) != "" {
			baseExplicit = true
		}

		if client != "" {
			if _, ok := args["client"]; !ok {
				args["client"] = client
			}
		}

		if !baseExplicit && baseURL != "" {
			if base, ok := args["base_url"].(string); !ok || strings.TrimSpace(base) == "" {
				args["base_url"] = baseURL
			}
		}

		if orgID > 0 {
			switch v := args["org_id"].(type) {
			case nil:
				args["org_id"] = orgID
			case float64:
				if int(v) == 0 {
					args["org_id"] = orgID
				}
			case int:
				if v == 0 {
					args["org_id"] = orgID
				}
			}
		}

		if dashboardUID != "" && (step.Name == "grafana_get_dashboard" || step.Name == "grafana_get_dashboard_summary") {
			if v, ok := args["uid"].(string); !ok || strings.TrimSpace(v) == "" {
				args["uid"] = dashboardUID
			}
		}

		if b, err := json.Marshal(args); err == nil {
			plan.Steps[i].Args = b
		}
	}
}

func (h *Handler) buildRouterCatalog() []router.ToolCatalogItem {
	// Local tools: use built-in schemas.
	local := make(map[string]mcp.Tool)
	for _, t := range h.BuiltinTools() {
		local[t.Name] = t
	}

	items := make([]router.ToolCatalogItem, 0, len(h.registry.ListActive())+len(local))

	for _, t := range h.BuiltinTools() {
		// Do not expose router/query itself to the router (prevent recursive calls).
		if t.Name == "router" || t.Name == "query" {
			continue
		}
		// Dev-only tools are hidden from routing unless dev mode is enabled.
		if t.Name == "dev_scaffold_tool" && !devModeEnabled() {
			continue
		}
		items = append(items, router.ToolCatalogItem{
			Name:        t.Name,
			Description: t.Description,
			Category:    "local",
			Source:      "local",
			InputSchema: t.InputSchema,
		})
	}

	// Upstream tools: iterate over known categories' tool lists (registry doesn't currently expose raw tool map).
	for _, cat := range h.registry.ListCategories() {
		for _, name := range cat.Tools {
			// Never route to an upstream tool called query/router (prevents recursion when chaining mcp-lens instances).
			if name == "router" || name == "query" {
				continue
			}
			tool, ok := h.registry.GetTool(name)
			if !ok {
				continue
			}
			items = append(items, router.ToolCatalogItem{
				Name:        tool.Name,
				Description: tool.Description,
				Category:    cat.Name,
				Source:      "upstream",
				InputSchema: tool.InputSchema,
			})
		}
	}

	// Also include any tools not in categories (best-effort via summaries map isn't exposed).
	// Those tools can still be invoked if the model selects them, but catalog won't list them.

	return items
}

func (h *Handler) executePlan(ctx context.Context, plan router.ModelPlan, policy router.Policy, output *router.OutputOptions, parallelism int) ([]router.ExecutedStep, *artifacts.Manifest, error) {
	if parallelism <= 0 {
		parallelism = 1
	}
	out := make([]router.ExecutedStep, 0, len(plan.Steps))
	manifest := &artifacts.Manifest{Artifacts: []artifacts.Item{}}

	// Copy steps so we can append continuation steps
	steps := make([]router.PlanStep, len(plan.Steps))
	copy(steps, plan.Steps)

	// Max continuation iterations to prevent infinite loops
	maxContinuations := 10
	continuationCount := 0

	var manifestMu sync.Mutex
	execOne := func(step router.PlanStep) (router.ExecutedStep, map[string]any, *artifacts.Item, error) {
		st := router.ExecutedStep{Name: step.Name, Source: step.Source, Args: step.Args}

		if !policy.IsAllowed(step.Source, step.Name) {
			st.OK = false
			st.Error = "blocked by policy"
			return st, nil, nil, fmt.Errorf("blocked tool: %s", step.Name)
		}

		// Ensure args is an object.
		var asObj map[string]any
		if err := json.Unmarshal(step.Args, &asObj); err != nil {
			st.OK = false
			st.Error = "invalid args: must be JSON object"
			return st, nil, nil, fmt.Errorf("invalid args for %s", step.Name)
		}

		var res *mcp.CallToolResult
		var err error
		if strings.EqualFold(step.Source, "local") {
			res, err = h.Handle(ctx, step.Name, step.Args)
		} else {
			// Upstream tool execution.
			res, err = h.executor(step.Name, step.Args)
			// We intentionally do not expose upstream tools via tools/list; activation is optional.
			h.registry.Activate(step.Name)
		}
		if err != nil {
			st.OK = false
			st.Error = err.Error()
			return st, nil, nil, err
		}
		st.OK = !res.IsError
		if res.IsError {
			if len(res.Content) > 0 {
				st.Error = res.Content[0].Text
			} else {
				st.Error = "tool returned error"
			}
			return st, nil, nil, fmt.Errorf("tool error")
		}

		// Best-effort parse JSON if tool returned JSON text.
		var resultMap map[string]any
		if len(res.Content) > 0 {
			var anyRes any
			if json.Unmarshal([]byte(res.Content[0].Text), &anyRes) == nil {
				st.Result = anyRes
				resultMap, _ = anyRes.(map[string]any)
			} else {
				st.Result = res.Content[0].Text
			}
		}

		var created *artifacts.Item
		if st.Result != nil {
			// Artifact decision should be based on the original tool output, before shaping.
			originalForArtifacts := st.Result

			shaped, err := router.ApplyOutputShaping(step.Name, st.Result, output)
			if err != nil {
				st.OK = false
				st.Error = "output shaping error: " + err.Error()
				return st, resultMap, nil, err
			}
			st.Result = shaped

			if h.artifacts != nil && router.JSONSize(originalForArtifacts) > h.artifacts.InlineMaxBytes() {
				repl, item, err := h.artifacts.MaybeStore(step.Name, step.Args, originalForArtifacts)
				if err != nil {
					st.OK = false
					st.Error = "artifact store error: " + err.Error()
					return st, resultMap, nil, err
				}
				if item != nil {
					st.Result = repl
					created = item
				}
			}
		}

		return st, resultMap, created, nil
	}

	for i := 0; i < len(steps); i++ {
		step := steps[i]

		pg := strings.TrimSpace(step.ParallelGroup)
		if pg != "" && parallelism > 1 && strings.EqualFold(step.Source, "local") {
			// Execute a contiguous group of steps in parallel.
			group := []router.PlanStep{step}
			for j := i + 1; j < len(steps); j++ {
				if strings.TrimSpace(steps[j].ParallelGroup) != pg || !strings.EqualFold(steps[j].Source, "local") {
					break
				}
				group = append(group, steps[j])
			}
			results := make([]router.ExecutedStep, len(group))
			resultMaps := make([]map[string]any, len(group))
			createdItems := make([]*artifacts.Item, len(group))
			errs := make([]error, len(group))

			var g errgroup.Group
			g.SetLimit(parallelism)
			for idx := range group {
				idx := idx
				step := group[idx]
				g.Go(func() error {
					st, rm, item, err := execOne(step)
					results[idx] = st
					resultMaps[idx] = rm
					createdItems[idx] = item
					errs[idx] = err
					return nil
				})
			}
			_ = g.Wait()

			for idx := range group {
				if createdItems[idx] != nil {
					manifestMu.Lock()
					manifest.Artifacts = append(manifest.Artifacts, *createdItems[idx])
					manifestMu.Unlock()
				}
				out = append(out, results[idx])
			}

			// Continuations (append at end, like sequential mode).
			for idx := range group {
				if resultMaps[idx] != nil && continuationCount < maxContinuations {
					if contStep := h.createContinuationStep(group[idx], resultMaps[idx]); contStep != nil {
						steps = append(steps, *contStep)
						continuationCount++
					}
				}
			}

			// If any error occurred, return the first error in plan order.
			for idx := range group {
				if errs[idx] != nil {
					return out, manifest, errs[idx]
				}
			}

			i += len(group) - 1
			continue
		}

		st, resultMap, created, err := execOne(step)
		if created != nil {
			manifest.Artifacts = append(manifest.Artifacts, *created)
		}
		out = append(out, st)
		if err != nil {
			return out, manifest, err
		}

		// Check for continuation (has_next) and auto-paginate
		if resultMap != nil && continuationCount < maxContinuations {
			if contStep := h.createContinuationStep(step, resultMap); contStep != nil {
				steps = append(steps, *contStep)
				continuationCount++
			}
		}
	}
	if len(manifest.Artifacts) == 0 {
		return out, nil, nil
	}
	return out, manifest, nil
}

// createContinuationStep creates a continuation step if the result has pagination info
func (h *Handler) createContinuationStep(originalStep router.PlanStep, result map[string]any) *router.PlanStep {
	// Parse original args
	var args map[string]any
	if err := json.Unmarshal(originalStep.Args, &args); err != nil {
		return nil
	}

	// Jira-style pagination: { startAt, maxResults, total } (and sometimes values/issues arrays).
	// We auto-continue for common Jira list/search endpoints even if they don't return has_next.
	switch originalStep.Name {
	case "jira_search_issues", "jira_get_issue_comments", "jira_list_projects":
		startAt, ok1 := result["startAt"].(float64)
		maxResults, ok2 := result["maxResults"].(float64)
		total, ok3 := result["total"].(float64)
		if ok1 && ok2 && ok3 {
			next := int(startAt) + int(maxResults)
			if next < int(total) {
				args["startAt"] = next
				newArgs, _ := json.Marshal(args)
				return &router.PlanStep{
					Name:   originalStep.Name,
					Source: originalStep.Source,
					Args:   newArgs,
					Reason: fmt.Sprintf("Auto-continuation: fetching next page at startAt %d", next),
				}
			}
		}
	}

	// Generic continuation (opt-in by tools via has_next=true).
	hasNext, _ := result["has_next"].(bool)
	if !hasNext {
		return nil
	}

	// Check for different pagination types
	switch originalStep.Name {
	case "get_pull_request_diff":
		// Use next_offset for diff pagination
		if nextOffset, ok := result["next_offset"].(float64); ok {
			args["offset"] = int(nextOffset)
			newArgs, _ := json.Marshal(args)
			return &router.PlanStep{
				Name:   originalStep.Name,
				Source: originalStep.Source,
				Args:   newArgs,
				Reason: fmt.Sprintf("Auto-continuation: fetching next diff chunk at offset %d", int(nextOffset)),
			}
		}
	case "list_pull_request_files", "list_pull_request_commits":
		// Use next_page for list pagination
		if nextPage, ok := result["next_page"].(float64); ok {
			args["page"] = int(nextPage)
			newArgs, _ := json.Marshal(args)
			return &router.PlanStep{
				Name:   originalStep.Name,
				Source: originalStep.Source,
				Args:   newArgs,
				Reason: fmt.Sprintf("Auto-continuation: fetching page %d", int(nextPage)),
			}
		}
	}

	// Generic cursor/start pagination (e.g. Confluence).
	if nextCursor, ok := result["next_cursor"].(string); ok && strings.TrimSpace(nextCursor) != "" {
		args["cursor"] = nextCursor
		newArgs, _ := json.Marshal(args)
		return &router.PlanStep{
			Name:   originalStep.Name,
			Source: originalStep.Source,
			Args:   newArgs,
			Reason: "Auto-continuation: fetching next page using cursor",
		}
	}
	if nextStart, ok := result["next_start"].(float64); ok {
		args["start"] = int(nextStart)
		newArgs, _ := json.Marshal(args)
		return &router.PlanStep{
			Name:   originalStep.Name,
			Source: originalStep.Source,
			Args:   newArgs,
			Reason: fmt.Sprintf("Auto-continuation: fetching next page at start %d", int(nextStart)),
		}
	}

	return nil
}
