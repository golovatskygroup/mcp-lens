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
