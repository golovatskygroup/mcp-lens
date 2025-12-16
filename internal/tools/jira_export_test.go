package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

func TestJiraExportTasksExpandsConfluenceLinks(t *testing.T) {
	t.Setenv("JIRA_PAT", "dummy")
	t.Setenv("CONFLUENCE_PAT", "dummy")

	if got := extractURLs("See https://confluence.webpower.ru/spaces/BP/pages/12345/Test"); len(got) != 1 {
		t.Fatalf("extractURLs failed: %v", got)
	}

	confluenceSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/content/12345" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"12345",
			"title":"Test Page",
			"body":{
				"view":{"value":"<p>Hello <b>world</b></p>"},
				"storage":{"value":"<p>Hello <b>world</b></p>"}
			},
			"version":{"number":1}
		}`))
	}))
	t.Cleanup(confluenceSrv.Close)

	jiraSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/search":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"issues":[{"key":"GO-1"}]}`))
		case "/rest/api/2/issue/GO-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"key":"GO-1",
				"fields":{
					"summary":"S",
					"status":{"name":"To Do"},
					"description":"See https://confluence.webpower.ru/spaces/BP/pages/12345/Test"
				}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(jiraSrv.Close)

	reg := registry.NewRegistry()
	h := NewHandler(reg, func(name string, args json.RawMessage) (*mcp.CallToolResult, error) { return nil, nil })
	reg.LoadTools(h.BuiltinTools())

	outDir := t.TempDir()
	args, _ := json.Marshal(map[string]any{
		"jql":                 `project = "GO"`,
		"output_dir":          outDir,
		"base_url":            jiraSrv.URL,
		"api_version":         2,
		"confluence_base_url": confluenceSrv.URL,
		"include_confluence":  true,
	})
	res, err := h.jiraExportTasks(context.Background(), args)
	if err != nil {
		t.Fatalf("jiraExportTasks: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content[0].Text)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &payload); err != nil {
		t.Fatalf("parse result: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "GO-1.md")); err != nil {
		t.Fatalf("expected issue file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "confluence", "12345.html")); err != nil {
		entries, _ := os.ReadDir(outDir)
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("expected confluence html: %v (outDir entries=%v)", err, names)
	}
	if _, err := os.Stat(filepath.Join(outDir, "confluence", "descriptions", "12345.md")); err != nil {
		t.Fatalf("expected confluence text: %v", err)
	}
}
