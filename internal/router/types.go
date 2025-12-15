package router

import "encoding/json"

type ToolCatalogItem struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	Source      string          `json:"source"` // local|upstream
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type PlanStep struct {
	Name   string          `json:"name"`
	Source string          `json:"source"` // local|upstream
	Args   json.RawMessage `json:"args"`
	Reason string          `json:"reason,omitempty"`
}

type ModelPlan struct {
	Steps             []PlanStep `json:"steps"`
	FinalAnswerNeeded bool       `json:"final_answer_needed"`
}

type ExecutedStep struct {
	Name   string          `json:"name"`
	Source string          `json:"source"`
	Args   json.RawMessage `json:"args"`
	OK     bool            `json:"ok"`
	Result any             `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type RouterResult struct {
	Plan          ModelPlan      `json:"plan"`
	ExecutedSteps []ExecutedStep `json:"executed_steps,omitempty"`
	Answer        string         `json:"answer,omitempty"`
	Debug         any            `json:"debug,omitempty"`
}
