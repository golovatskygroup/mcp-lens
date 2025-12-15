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
