package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

func Plan(ctx context.Context, cl *OpenRouterClient, userInput string, userCtx map[string]any, catalog []ToolCatalogItem, maxSteps int) (ModelPlan, []byte, error) {
	system := BuildPlanSystemPrompt()
	user, err := BuildPlanUserPrompt(userInput, userCtx, catalog, maxSteps)
	if err != nil {
		return ModelPlan{}, nil, err
	}

	raw, finish, err := cl.ChatCompletionJSONWithFinishReason(ctx, system, user)
	if err != nil {
		return ModelPlan{}, nil, err
	}

	var plan ModelPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		// If model truncated, surface a better error.
		if finish == "length" {
			return ModelPlan{}, raw, fmt.Errorf("router plan truncated (finish_reason=length); try lower max_steps or shorter input/context")
		}
		return ModelPlan{}, raw, fmt.Errorf("failed to parse router plan JSON: %w", err)
	}
	return plan, raw, nil
}

func ValidatePlan(plan ModelPlan, policy Policy, catalog []ToolCatalogItem, maxSteps int) error {
	if len(plan.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}
	if len(plan.Steps) > maxSteps {
		return fmt.Errorf("plan exceeds max_steps")
	}

	known := map[string]struct{}{}
	schemas := map[string]json.RawMessage{}
	for _, t := range catalog {
		known[t.Name] = struct{}{}
		schemas[t.Name] = t.InputSchema
	}

	for _, s := range plan.Steps {
		if s.Name == "" {
			return fmt.Errorf("step name is required")
		}
		// `query`/`router` are entrypoints for MCP clients, not callable steps.
		// If a plan contains them, it indicates recursive planning (often from chaining mcp-lens instances).
		if s.Name == "query" || s.Name == "router" {
			return fmt.Errorf("invalid plan: step uses reserved tool %q (entrypoint); remove it from the tool catalog/upstreams", s.Name)
		}
		if s.Source != "local" && s.Source != "upstream" {
			return fmt.Errorf("invalid step source: %s", s.Source)
		}
		if _, ok := known[s.Name]; !ok {
			// In strict mode we reject unknown tools, because tool list is hidden from clients.
			return fmt.Errorf("unknown tool: %s", s.Name)
		}
		if !policy.IsAllowed(s.Source, s.Name) {
			return fmt.Errorf("tool blocked by policy: %s", s.Name)
		}
		// args must be an object
		var obj map[string]any
		if err := json.Unmarshal(s.Args, &obj); err != nil {
			return fmt.Errorf("args for %s must be a JSON object", s.Name)
		}
		if err := validateArgsAgainstSchema(s.Name, schemas[s.Name], obj); err != nil {
			return err
		}
	}
	return nil
}

func Summarize(ctx context.Context, cl *OpenRouterClient, userInput string, res RouterResult) (string, error) {
	system := BuildSummarizeSystemPrompt()
	user, err := BuildSummarizeUserPrompt(userInput, res)
	if err != nil {
		return "", err
	}
	raw, finish, err := cl.ChatCompletionTextWithFinishReason(ctx, system, user)
	if err != nil {
		return "", err
	}
	if finish == "length" {
		// Provide a deterministic fallback if the model truncated.
		return SummarizeFallback(userInput, res), nil
	}
	return raw, nil
}

func SummarizeFallback(_ string, res RouterResult) string {
	var sb strings.Builder
	sb.WriteString("Summary truncated by model (finish_reason=length). Showing a compact overview.\n\n")
	sb.WriteString("Steps:\n")
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
			sb.WriteString(fmt.Sprintf("- %s (%d bytes) %s\n", a.Path, a.Bytes, a.SHA256))
		}
	}
	return sb.String()
}
