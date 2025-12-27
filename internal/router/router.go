package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Plan creates a routing plan using the full catalog (legacy behavior).
// For context-saving, prefer PlanWithCatalog which uses top-K selection.
func Plan(ctx context.Context, cl *OpenRouterClient, userInput string, userCtx map[string]any, catalog []ToolCatalogItem, maxSteps int) (ModelPlan, []byte, error) {
	return PlanWithCatalog(ctx, cl, userInput, userCtx, NewToolCatalog(catalog), maxSteps, 0, 0)
}

// PlanWithCatalog creates a routing plan using ToolCatalog with top-K selection and schema compression.
// This is the recommended method for context-saving smart routing.
// - topK: number of tools to include (0 = use all)
// - schemaMaxChars: max characters per schema (0 = no compression)
func PlanWithCatalog(ctx context.Context, cl *OpenRouterClient, userInput string, userCtx map[string]any, catalog *ToolCatalog, maxSteps int, topK int, schemaMaxChars int) (ModelPlan, []byte, error) {
	var selectedTools []ToolCatalogItem
	var err error
	
	if topK > 0 && schemaMaxChars > 0 {
		// Use top-K selection with compressed schemas
		selectedTools, err = catalog.GetTopKWithCompressedSchemas(userInput, topK, schemaMaxChars)
		if err != nil {
			return ModelPlan{}, nil, fmt.Errorf("failed to select tools: %w", err)
		}
	} else if topK > 0 {
		// Use top-K selection without compression
		selectedTools = catalog.TopKByQuery(userInput, topK)
	} else {
		// Use all tools (legacy behavior)
		selectedTools = catalog.ListTools()
	}
	
	system := BuildPlanSystemPrompt()
	user, err := BuildPlanUserPrompt(userInput, userCtx, selectedTools, maxSteps)
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
		
		// Explicitly block tool discovery/introspection tools.
		// Router must use explicit ToolCatalog instead of runtime discovery to save context.
		nameLower := strings.ToLower(s.Name)
		if nameLower == "search_tools" || nameLower == "list_tools" || 
		   nameLower == "discover_tools" || nameLower == "enumerate_tools" {
			return fmt.Errorf("invalid plan: tool discovery/introspection (%q) is not allowed; router uses explicit ToolCatalog instead of runtime discovery", s.Name)
		}
		
		// `query` is an entrypoint for MCP clients, not a callable step.
		// If a plan contains it, it indicates recursive planning (often from chaining mcp-lens instances).
		if s.Name == "query" {
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
