package router

import (
	"encoding/json"

	"github.com/golovatskygroup/mcp-lens/internal/artifacts"
)

type ToolCatalogItem struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Category    string          `json:"category,omitempty"`
	Source      string          `json:"source"` // local|upstream
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

type PlanStep struct {
	Name          string          `json:"name"`
	Source        string          `json:"source"` // local|upstream
	Args          json.RawMessage `json:"args"`
	Reason        string          `json:"reason,omitempty"`
	ParallelGroup string          `json:"parallel_group,omitempty"`
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
	Plan          ModelPlan           `json:"plan"`
	ExecutedSteps []ExecutedStep      `json:"executed_steps,omitempty"`
	Answer        string              `json:"answer,omitempty"`
	Manifest      *artifacts.Manifest `json:"manifest,omitempty"`
	Debug         any                 `json:"debug,omitempty"`
}
