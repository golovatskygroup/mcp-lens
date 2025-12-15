package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/router"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type routerInput struct {
	Input         string         `json:"input"`
	Context       map[string]any `json:"context,omitempty"`
	MaxSteps      int            `json:"max_steps,omitempty"`
	IncludeAnswer bool           `json:"include_answer,omitempty"`
	DryRun        bool           `json:"dry_run,omitempty"`
	Format        string         `json:"format,omitempty"` // json|text
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

	// Convenience: allow prefixing the query with `jira <client>` to route Jira calls to that client.
	if client, rest := extractJiraClientPrefix(in.Input); client != "" {
		in.Context["jira_client"] = client
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

	if in.MaxSteps <= 0 {
		in.MaxSteps = 5
	}
	if in.MaxSteps > 8 {
		return errorResult("max_steps must be <= 8"), nil
	}
	if in.Format == "" {
		in.Format = "json"
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

	if err := router.ValidatePlan(plan, policy, catalog, in.MaxSteps); err != nil {
		return errorResult(err.Error() + "\nplan=" + string(rawPlan)), nil
	}

	res := router.RouterResult{Plan: plan}

	if !in.DryRun {
		execSteps, err := h.executePlan(ctx, plan, policy)
		if err != nil {
			res.ExecutedSteps = execSteps
			return jsonResult(res), nil
		}
		res.ExecutedSteps = execSteps
	}

	if in.IncludeAnswer {
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

func (h *Handler) buildRouterCatalog() []router.ToolCatalogItem {
	// Local tools: use built-in schemas.
	local := make(map[string]mcp.Tool)
	for _, t := range h.BuiltinTools() {
		local[t.Name] = t
	}

	items := make([]router.ToolCatalogItem, 0, len(h.registry.ListActive())+len(local))

	for _, t := range h.BuiltinTools() {
		// Do not expose router itself to the router.
		if t.Name == "router" {
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

func (h *Handler) executePlan(ctx context.Context, plan router.ModelPlan, policy router.Policy) ([]router.ExecutedStep, error) {
	out := make([]router.ExecutedStep, 0, len(plan.Steps))

	// Copy steps so we can append continuation steps
	steps := make([]router.PlanStep, len(plan.Steps))
	copy(steps, plan.Steps)

	// Max continuation iterations to prevent infinite loops
	maxContinuations := 10
	continuationCount := 0

	for i := 0; i < len(steps); i++ {
		step := steps[i]
		st := router.ExecutedStep{Name: step.Name, Source: step.Source, Args: step.Args}

		if !policy.IsAllowed(step.Source, step.Name) {
			st.OK = false
			st.Error = "blocked by policy"
			out = append(out, st)
			return out, fmt.Errorf("blocked tool: %s", step.Name)
		}

		// Ensure args is an object.
		var asObj map[string]any
		if err := json.Unmarshal(step.Args, &asObj); err != nil {
			st.OK = false
			st.Error = "invalid args: must be JSON object"
			out = append(out, st)
			return out, fmt.Errorf("invalid args for %s", step.Name)
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
			out = append(out, st)
			return out, err
		}
		st.OK = !res.IsError
		if res.IsError {
			if len(res.Content) > 0 {
				st.Error = res.Content[0].Text
			} else {
				st.Error = "tool returned error"
			}
			out = append(out, st)
			return out, fmt.Errorf("tool error")
		}

		// Best-effort parse JSON if tool returned JSON text.
		var resultMap map[string]any
		if len(res.Content) > 0 {
			var anyRes any
			if json.Unmarshal([]byte(res.Content[0].Text), &anyRes) == nil {
				st.Result = anyRes
				// Try to get as map for continuation check
				resultMap, _ = anyRes.(map[string]any)
			} else {
				st.Result = res.Content[0].Text
			}
		}

		out = append(out, st)

		// Check for continuation (has_next) and auto-paginate
		if resultMap != nil && continuationCount < maxContinuations {
			if contStep := h.createContinuationStep(step, resultMap); contStep != nil {
				steps = append(steps, *contStep)
				continuationCount++
			}
		}
	}
	return out, nil
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

	return nil
}
