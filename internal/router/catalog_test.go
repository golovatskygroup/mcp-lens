package router

import (
	"encoding/json"
	"testing"
)

func TestToolCatalog_TopKByQuery(t *testing.T) {
	tools := []ToolCatalogItem{
		{Name: "get_pull_request_details", Description: "Get details of a pull request", Source: "local"},
		{Name: "jira_get_issue", Description: "Get Jira issue details", Source: "local"},
		{Name: "confluence_get_page", Description: "Get Confluence page", Source: "local"},
		{Name: "github_list_workflow_runs", Description: "List GitHub workflow runs", Source: "local"},
	}

	catalog := NewToolCatalog(tools)

	// Test exact match
	results := catalog.TopKByQuery("pull request", 2)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "get_pull_request_details" {
		t.Errorf("expected get_pull_request_details, got %s", results[0].Name)
	}

	// Test top-K selection
	results = catalog.TopKByQuery("github", 2)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Name != "github_list_workflow_runs" {
		t.Errorf("expected github_list_workflow_runs, got %s", results[0].Name)
	}

	// Test with k larger than available
	results = catalog.TopKByQuery("", 10)
	if len(results) != 4 {
		t.Errorf("expected 4 results, got %d", len(results))
	}

	// Test with k=0
	results = catalog.TopKByQuery("test", 0)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestToolCatalog_GetCompressedSchema(t *testing.T) {
	largeSchema := json.RawMessage(`{
		"type": "object",
		"description": "This is a very long description that should be truncated when compressed to save tokens in the context window",
		"properties": {
			"param1": {
				"type": "string",
				"description": "First parameter with a very long description that should be shortened"
			},
			"param2": {
				"type": "number",
				"description": "Second parameter"
			}
		},
		"required": ["param1"]
	}`)

	tools := []ToolCatalogItem{
		{Name: "test_tool", InputSchema: largeSchema, Source: "local"},
	}

	catalog := NewToolCatalog(tools)

	// Test compression with limit
	compressed, err := catalog.GetCompressedSchema("test_tool", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(compressed) > 250 { // Allow some overhead
		t.Errorf("compressed schema too large: %d bytes", len(compressed))
	}

	// Verify it's still valid JSON
	var test map[string]any
	if err := json.Unmarshal(compressed, &test); err != nil {
		t.Errorf("compressed schema is not valid JSON: %v", err)
	}

	// Test caching - second call should return same result
	compressed2, err := catalog.GetCompressedSchema("test_tool", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(compressed) != string(compressed2) {
		t.Error("cached schema should match")
	}
}

func TestToolCatalog_GetTopKWithCompressedSchemas(t *testing.T) {
	tools := []ToolCatalogItem{
		{
			Name:        "tool1",
			Description: "First tool",
			InputSchema: json.RawMessage(`{"type":"object","description":"Tool 1 schema"}`),
			Source:      "local",
		},
		{
			Name:        "tool2",
			Description: "Second tool",
			InputSchema: json.RawMessage(`{"type":"object","description":"Tool 2 schema"}`),
			Source:      "local",
		},
	}

	catalog := NewToolCatalog(tools)

	results, err := catalog.GetTopKWithCompressedSchemas("tool", 1, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	// Verify schema was compressed (should be <= 100 chars)
	if len(results[0].InputSchema) > 150 { // Allow some overhead
		t.Errorf("schema should be compressed, got %d bytes", len(results[0].InputSchema))
	}
}
