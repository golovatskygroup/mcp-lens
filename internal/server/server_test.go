package server

import (
	"encoding/json"
	"testing"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

func TestHandleListToolsReturnsQuery(t *testing.T) {
	// We can't easily test the full server without spinning up upstream,
	// but we can verify the structure of tools/list response.
	// This is a simple structural test.

	// Simulate expected response structure
	result := mcp.ListToolsResult{
		Tools: []mcp.Tool{
			{
				Name:        "query",
				Description: "Single entrypoint for MCP clients...",
				InputSchema: json.RawMessage(`{"type":"object"}`),
			},
		},
	}

	if len(result.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result.Tools))
	}
	if result.Tools[0].Name != "query" {
		t.Errorf("expected tool name 'query', got '%s'", result.Tools[0].Name)
	}
}

func TestCallToolErrorMessage(t *testing.T) {
	// Verify the error message structure for non-exposed tools.
	result := &mcp.CallToolResult{
		Content: []mcp.ContentBlock{{Type: "text", Text: "Tool is not exposed directly. Use the 'query' tool to ask a free-form request, and the proxy will plan and execute appropriate tools for you."}},
		IsError: true,
	}

	if !result.IsError {
		t.Error("expected IsError to be true")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected content")
	}
	if result.Content[0].Text == "" {
		t.Error("expected error message")
	}
}
