package router

import (
	"context"
	"encoding/json"
	"fmt"
)

func Plan(ctx context.Context, cl *OpenRouterClient, userInput string, userCtx map[string]any, catalog []ToolCatalogItem, maxSteps int) (ModelPlan, []byte, error) {
	system := BuildPlanSystemPrompt()
	user, err := BuildPlanUserPrompt(userInput, userCtx, catalog, maxSteps)
	if err != nil {
		return ModelPlan{}, nil, err
	}

	raw, err := cl.ChatCompletionJSON(ctx, system, user)
	if err != nil {
		return ModelPlan{}, nil, err
	}

	var plan ModelPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
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
	for _, t := range catalog {
		known[t.Name] = struct{}{}
	}

	for _, s := range plan.Steps {
		if s.Name == "" {
			return fmt.Errorf("step name is required")
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
	}
	return nil
}

func Summarize(ctx context.Context, cl *OpenRouterClient, userInput string, res RouterResult) (string, error) {
	system := BuildSummarizeSystemPrompt()
	user, err := BuildSummarizeUserPrompt(userInput, res)
	if err != nil {
		return "", err
	}
	raw, err := cl.ChatCompletionText(ctx, system, user)
	if err != nil {
		return "", err
	}
	return raw, nil
}
