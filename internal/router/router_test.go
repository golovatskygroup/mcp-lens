package router

import (
	"encoding/json"
	"testing"
)

func TestValidatePlanRejectsUnknownTool(t *testing.T) {
	policy := DefaultPolicy()
	catalog := []ToolCatalogItem{{Name: "get_pull_request_details", Source: "local"}}

	plan := ModelPlan{Steps: []PlanStep{{Name: "no_such_tool", Source: "local", Args: json.RawMessage(`{}`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(plan, policy, catalog, 5); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidatePlanRejectsNonObjectArgs(t *testing.T) {
	policy := DefaultPolicy()
	catalog := []ToolCatalogItem{{Name: "get_pull_request_details", Source: "local"}}

	plan := ModelPlan{Steps: []PlanStep{{Name: "get_pull_request_details", Source: "local", Args: json.RawMessage(`[]`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(plan, policy, catalog, 5); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidatePlanRejectsQueryEntrypointTool(t *testing.T) {
	policy := DefaultPolicy()
	catalog := []ToolCatalogItem{{Name: "query", Source: "local"}}

	plan := ModelPlan{Steps: []PlanStep{{Name: "query", Source: "local", Args: json.RawMessage(`{}`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(plan, policy, catalog, 5); err == nil {
		t.Fatalf("expected error")
	}
}

func TestValidatePlanRejectsDiscoveryTools(t *testing.T) {
	policy := DefaultPolicy()
	catalog := []ToolCatalogItem{{Name: "search_tools", Source: "local"}}

	// Test search_tools is rejected
	plan := ModelPlan{Steps: []PlanStep{{Name: "search_tools", Source: "local", Args: json.RawMessage(`{}`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(plan, policy, catalog, 5); err == nil {
		t.Fatalf("expected error for search_tools")
	}
	if err := ValidatePlan(plan, policy, catalog, 5); err != nil && !contains(err.Error(), "discovery") {
		t.Fatalf("expected error message about discovery, got: %v", err)
	}

	// Test list_tools is rejected
	catalog2 := []ToolCatalogItem{{Name: "list_tools", Source: "local"}}
	plan2 := ModelPlan{Steps: []PlanStep{{Name: "list_tools", Source: "local", Args: json.RawMessage(`{}`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(plan2, policy, catalog2, 5); err == nil {
		t.Fatalf("expected error for list_tools")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestValidatePlanValidatesArgsAgainstSchema(t *testing.T) {
	policy := DefaultPolicy()
	catalog := []ToolCatalogItem{{
		Name:   "get_pull_request_details",
		Source: "local",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"repo": {"type": "string"},
				"number": {"type": "integer"}
			},
			"required": ["repo", "number"]
		}`),
	}}

	planBad := ModelPlan{Steps: []PlanStep{{Name: "get_pull_request_details", Source: "local", Args: json.RawMessage(`{"repo":"o/r"}`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(planBad, policy, catalog, 5); err == nil {
		t.Fatalf("expected schema validation error")
	}

	planOK := ModelPlan{Steps: []PlanStep{{Name: "get_pull_request_details", Source: "local", Args: json.RawMessage(`{"repo":"o/r","number":1}`)}}, FinalAnswerNeeded: false}
	if err := ValidatePlan(planOK, policy, catalog, 5); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}
