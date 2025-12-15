package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/registry"
)

func TestGrafanaHealthAllowsNoAuth(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/api/health" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"database":"ok","version":"10.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GRAFANA_BASE_URL", srv.URL)
	t.Setenv("GRAFANA_API_TOKEN", "")
	t.Setenv("GRAFANA_BEARER_TOKEN", "")
	t.Setenv("GRAFANA_USERNAME", "")
	t.Setenv("GRAFANA_PASSWORD", "")

	h := NewHandler(registry.NewRegistry(), nil)

	res, err := h.grafanaHealth(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got: %+v", res)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header, got %q", gotAuth)
	}
}

func TestGrafanaGetCurrentUserRequiresAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("server should not be called without auth")
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GRAFANA_BASE_URL", srv.URL)
	t.Setenv("GRAFANA_API_TOKEN", "")
	t.Setenv("GRAFANA_BEARER_TOKEN", "")
	t.Setenv("GRAFANA_USERNAME", "")
	t.Setenv("GRAFANA_PASSWORD", "")

	h := NewHandler(registry.NewRegistry(), nil)

	res, err := h.grafanaGetCurrentUser(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected error result, got: %+v", res)
	}
}

func TestGrafanaSearchBuildsQueryAndHeaders(t *testing.T) {
	var got struct {
		auth string
		org  string
		cfID string
		cfS  string
		q    map[string][]string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth = r.Header.Get("Authorization")
		got.org = r.Header.Get("X-Grafana-Org-Id")
		got.cfID = r.Header.Get("CF-Access-Client-Id")
		got.cfS = r.Header.Get("CF-Access-Client-Secret")
		got.q = r.URL.Query()
		if r.URL.Path != "/api/search" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"uid":"abc","type":"dash-db","title":"x"}]`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GRAFANA_BASE_URL", srv.URL)
	t.Setenv("GRAFANA_API_TOKEN", "token123")
	t.Setenv("GRAFANA_ORG_ID", "2")
	t.Setenv("GRAFANA_CF_ACCESS_CLIENT_ID", "env-id")
	t.Setenv("GRAFANA_CF_ACCESS_CLIENT_SECRET", "env-secret")

	h := NewHandler(registry.NewRegistry(), nil)

	args := json.RawMessage(`{
		"query": "prod",
		"type": "dash-db",
		"tags": ["tag1", "tag2"],
		"folder_uids": ["f1"],
		"dashboard_uids": ["d1", "d2"],
		"starred": true,
		"limit": 1,
		"page": 3,
		"org_id": 5,
		"cf_access_client_id": "arg-id",
		"cf_access_client_secret": "arg-secret"
	}`)
	res, err := h.grafanaSearch(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got: %+v", res)
	}

	if got.auth != "Bearer token123" {
		t.Fatalf("expected Authorization Bearer, got %q", got.auth)
	}
	// args.org_id should override env GRAFANA_ORG_ID.
	if got.org != "5" {
		t.Fatalf("expected X-Grafana-Org-Id=5, got %q", got.org)
	}
	// args.cf_access_* should override env.
	if got.cfID != "arg-id" || got.cfS != "arg-secret" {
		t.Fatalf("expected CF-Access headers from args, got id=%q secret=%q", got.cfID, got.cfS)
	}
	if got.q["query"][0] != "prod" {
		t.Fatalf("unexpected query param 'query': %+v", got.q["query"])
	}
	if got.q["type"][0] != "dash-db" {
		t.Fatalf("unexpected query param 'type': %+v", got.q["type"])
	}
	if len(got.q["tag"]) != 2 {
		t.Fatalf("expected 2 tag params, got %+v", got.q["tag"])
	}
	if got.q["folderUIDs"][0] != "f1" {
		t.Fatalf("unexpected folderUIDs: %+v", got.q["folderUIDs"])
	}
	if len(got.q["dashboardUIDs"]) != 2 {
		t.Fatalf("expected 2 dashboardUIDs params, got %+v", got.q["dashboardUIDs"])
	}
	if got.q["starred"][0] != "true" {
		t.Fatalf("unexpected starred: %+v", got.q["starred"])
	}
	if got.q["limit"][0] != "1" || got.q["page"][0] != "3" {
		t.Fatalf("unexpected paging: limit=%v page=%v", got.q["limit"], got.q["page"])
	}
}

func TestParseGrafanaDashboardURL(t *testing.T) {
	info, err := parseGrafanaDashboardURL("https://grafana.example.com/d/LMnaW0S7z/pathfinder-metrics?orgId=1&from=now-15m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BaseURL != "https://grafana.example.com" {
		t.Fatalf("unexpected base url: %q", info.BaseURL)
	}
	if info.UID != "LMnaW0S7z" {
		t.Fatalf("unexpected uid: %q", info.UID)
	}
	if info.OrgID != 1 {
		t.Fatalf("unexpected org id: %d", info.OrgID)
	}
}

func TestGrafanaGetDashboardSummaryFromURLInfersBaseAndOrg(t *testing.T) {
	var got struct {
		auth string
		org  string
		path string
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.auth = r.Header.Get("Authorization")
		got.org = r.Header.Get("X-Grafana-Org-Id")
		got.path = r.URL.Path
		if r.URL.Path != "/api/dashboards/uid/LMnaW0S7z" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"dashboard": {
				"uid": "LMnaW0S7z",
				"title": "pathfinder-metrics",
				"timezone": "utc",
				"refresh": "5s",
				"time": {"from": "now-15m", "to": "now"},
				"templating": {"list": [{"name": "k8s_cluster", "type": "query", "query": "label_values(up, k8s_cluster)", "multi": true}]},
				"panels": [
					{"id": 1, "title": "RPS", "type": "timeseries", "datasource": "Prometheus", "targets": [{"refId": "A", "expr": "sum(rate(http_requests_total[5m]))"}]},
					{"id": 2, "title": "Row", "type": "row", "panels": [{"id": 3, "title": "Latency", "type": "timeseries", "targets": [{"refId": "A", "expr": "histogram_quantile(0.99, ...)"}, {"refId": "B", "query": "something"}]}]}
				]
			},
			"meta": {"url": "/d/LMnaW0S7z/pathfinder-metrics", "slug": "pathfinder-metrics", "folderTitle": "Prod", "folderUid": "folder1"}
		}`))
	}))
	t.Cleanup(srv.Close)

	t.Setenv("GRAFANA_BASE_URL", "")
	t.Setenv("GRAFANA_API_TOKEN", "token123")
	t.Setenv("GRAFANA_BEARER_TOKEN", "")
	t.Setenv("GRAFANA_USERNAME", "")
	t.Setenv("GRAFANA_PASSWORD", "")

	h := NewHandler(registry.NewRegistry(), nil)

	dashboardURL := srv.URL + "/d/LMnaW0S7z/pathfinder-metrics?orgId=1&from=now-15m"
	args := json.RawMessage(`{"url": "` + dashboardURL + `", "max_panels": 10, "max_targets_per_panel": 10}`)
	res, err := h.grafanaGetDashboardSummary(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected non-error result, got: %+v", res)
	}

	if got.path == "" {
		t.Fatal("expected server to be called")
	}
	if got.auth != "Bearer token123" {
		t.Fatalf("expected Authorization Bearer, got %q", got.auth)
	}
	if got.org != "1" {
		t.Fatalf("expected X-Grafana-Org-Id=1 inferred from url, got %q", got.org)
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
		t.Fatalf("failed to parse result json: %v", err)
	}
	if out["uid"] != "LMnaW0S7z" {
		t.Fatalf("unexpected uid in output: %#v", out["uid"])
	}
	if out["title"] != "pathfinder-metrics" {
		t.Fatalf("unexpected title in output: %#v", out["title"])
	}
	if _, ok := out["panels"].([]any); !ok {
		t.Fatalf("expected panels array in output, got: %#v", out["panels"])
	}
	if baseURL, _ := out["base_url"].(string); !strings.HasPrefix(baseURL, "http") {
		t.Fatalf("expected base_url in output, got: %#v", out["base_url"])
	}
}
