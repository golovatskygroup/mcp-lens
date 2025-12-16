package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

func TestQueryDiscoveryFastPathDoesNotRequireOpenRouter(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		t.Fatalf("executor should not be called")
		return nil, nil
	})
	// Load local tool schemas into registry so describe_tool works.
	reg.LoadTools(h.BuiltinTools())

	args, _ := json.Marshal(map[string]any{"input": "search tools grafana", "format": "json"})
	res, err := h.Handle(context.Background(), "query", args)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %+v", res)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Fatalf("expected content")
	}
}

func TestQueryExecutorModeDoesNotRequireOpenRouter(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		t.Fatalf("upstream executor should not be called")
		return nil, nil
	})
	reg.LoadTools(h.BuiltinTools())

	args, _ := json.Marshal(map[string]any{
		"input": "execute explicit steps",
		"mode":  "executor",
		"steps": []map[string]any{
			{
				"name":   "search_tools",
				"source": "local",
				"args": map[string]any{
					"query":  "grafana",
					"format": "json",
					"limit":  1,
				},
			},
		},
		"include_answer": true,
		"format":         "json",
	})
	res, err := h.Handle(context.Background(), "query", args)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %+v", res)
	}
	if len(res.Content) == 0 || res.Content[0].Text == "" {
		t.Fatalf("expected content")
	}
}
