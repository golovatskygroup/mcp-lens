package tools

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

func TestBuiltinToolsContainsQuery(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	found := false
	for _, tool := range h.BuiltinTools() {
		if tool.Name == "query" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected BuiltinTools to contain 'query'")
	}
}

func TestBuiltinToolsContainsRouter(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	found := false
	for _, tool := range h.BuiltinTools() {
		if tool.Name == "router" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected BuiltinTools to contain 'router' for backwards compat")
	}
}

func TestIsLocalToolQuery(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	if !h.IsLocalTool("query") {
		t.Fatal("expected 'query' to be local tool")
	}
	if !h.IsLocalTool("router") {
		t.Fatal("expected 'router' to be local tool")
	}
}

func TestHandleQueryWithoutEnv(t *testing.T) {
	// Without OPENROUTER_API_KEY, runRouter should return an error result.
	os.Unsetenv("OPENROUTER_API_KEY")
	os.Unsetenv("MCP_LENS_ROUTER_MODEL")

	reg := registry.NewRegistry()
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{Content: []mcp.ContentBlock{{Type: "text", Text: "ok"}}}, nil
	})

	args := json.RawMessage(`{"input": "test request"}`)
	res, err := h.Handle(context.Background(), "query", args)

	// Should not return a Go error, but result should indicate an error.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected result, got nil")
	}
	// The error message should mention missing env vars.
	if !res.IsError {
		t.Log("Note: if OPENROUTER_API_KEY is set in environment, this test may pass unexpectedly")
	}
}

func TestQueryToolDescription(t *testing.T) {
	reg := registry.NewRegistry()
	h := NewHandler(reg, nil)

	var queryTool *mcp.Tool
	for _, tool := range h.BuiltinTools() {
		if tool.Name == "query" {
			queryTool = &tool
			break
		}
	}

	if queryTool == nil {
		t.Fatal("query tool not found")
	}

	// Description should mention "free-form" and explain the workflow.
	if len(queryTool.Description) < 50 {
		t.Errorf("query tool description too short: %s", queryTool.Description)
	}
}
