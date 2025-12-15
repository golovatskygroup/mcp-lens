package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type grafanaClient struct {
	baseURL    string
	authHeader string
	orgID      int
	userAgent  string
	cfClientID string
	cfSecret   string
	c          *http.Client
}

type grafanaConfig struct {
	baseURL    string
	authHeader string
	orgID      int
}

type grafanaClientEnvConfig struct {
	BaseURL         string `json:"base_url,omitempty"`
	APIToken        string `json:"api_token,omitempty"`
	BearerToken     string `json:"bearer_token,omitempty"`
	Username        string `json:"username,omitempty"`
	Password        string `json:"password,omitempty"`
	OrgID           int    `json:"org_id,omitempty"`
	CFAccessID      string `json:"cf_access_client_id,omitempty"`
	CFAccessSecret  string `json:"cf_access_client_secret,omitempty"`
	AllowNoAuth     bool   `json:"allow_no_auth,omitempty"`
	TimeoutMS       int    `json:"timeout_ms,omitempty"`
	UserAgent       string `json:"user_agent,omitempty"`
	InsecureSkipTLS bool   `json:"insecure_skip_verify,omitempty"` // reserved; not used (no custom TLS)
}

var (
	grafanaClientsOnce sync.Once
	grafanaClientsMap  map[string]grafanaClientEnvConfig
)

func loadGrafanaClientsFromEnv() map[string]grafanaClientEnvConfig {
	grafanaClientsOnce.Do(func() {
		grafanaClientsMap = map[string]grafanaClientEnvConfig{}
		raw := strings.TrimSpace(os.Getenv("GRAFANA_CLIENTS_JSON"))
		if raw == "" {
			return
		}
		_ = json.Unmarshal([]byte(raw), &grafanaClientsMap)
	})
	return grafanaClientsMap
}

func grafanaPublicClientsFromEnv() map[string]map[string]any {
	clients := loadGrafanaClientsFromEnv()
	if len(clients) == 0 {
		return nil
	}
	out := map[string]map[string]any{}
	for name, cfg := range clients {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out[name] = map[string]any{
			"base_url": strings.TrimSpace(cfg.BaseURL),
			"org_id":   cfg.OrgID,
		}
	}
	return out
}

func resolveGrafanaConfig(clientName string, baseOverride string, orgIDOverride int, allowNoAuth bool, timeoutOverrideMS int, userAgentOverride string) (grafanaConfig, int, string, error) {
	clientName = strings.TrimSpace(clientName)
	clients := loadGrafanaClientsFromEnv()
	if clientName == "" {
		clientName = strings.TrimSpace(os.Getenv("GRAFANA_DEFAULT_CLIENT"))
	}

	var clientCfg grafanaClientEnvConfig
	var clientCfgOK bool
	if clientName != "" && len(clients) > 0 {
		clientCfg, clientCfgOK = clients[clientName]
	}

	baseURL := strings.TrimSpace(baseOverride)
	if baseURL == "" {
		if clientCfgOK && strings.TrimSpace(clientCfg.BaseURL) != "" {
			baseURL = strings.TrimSpace(clientCfg.BaseURL)
		} else {
			baseURL = strings.TrimSpace(os.Getenv("GRAFANA_BASE_URL"))
		}
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		if clientName != "" && !clientCfgOK {
			return grafanaConfig{}, 0, "", fmt.Errorf("unknown Grafana client %q: not found in GRAFANA_CLIENTS_JSON", clientName)
		}
		return grafanaConfig{}, 0, "", fmt.Errorf("missing Grafana base URL: set GRAFANA_BASE_URL or configure GRAFANA_CLIENTS_JSON")
	}

	orgID := orgIDOverride
	if orgID == 0 {
		if clientCfgOK && clientCfg.OrgID != 0 {
			orgID = clientCfg.OrgID
		} else if v := strings.TrimSpace(os.Getenv("GRAFANA_ORG_ID")); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				orgID = n
			}
		}
	}

	// Auth precedence:
	// 1) Explicit bearer/API token.
	apiToken := strings.TrimSpace(os.Getenv("GRAFANA_API_TOKEN"))
	bearerToken := strings.TrimSpace(os.Getenv("GRAFANA_BEARER_TOKEN"))
	if clientCfgOK {
		if apiToken == "" {
			apiToken = strings.TrimSpace(clientCfg.APIToken)
		}
		if bearerToken == "" {
			bearerToken = strings.TrimSpace(clientCfg.BearerToken)
		}
		if !allowNoAuth && clientCfg.AllowNoAuth {
			allowNoAuth = true
		}
		if timeoutOverrideMS == 0 && clientCfg.TimeoutMS > 0 {
			timeoutOverrideMS = clientCfg.TimeoutMS
		}
		if strings.TrimSpace(userAgentOverride) == "" && strings.TrimSpace(clientCfg.UserAgent) != "" {
			userAgentOverride = strings.TrimSpace(clientCfg.UserAgent)
		}
	}

	token := apiToken
	if token == "" {
		token = bearerToken
	}
	if token != "" {
		return grafanaConfig{
			baseURL:    baseURL,
			authHeader: "Bearer " + token,
			orgID:      orgID,
		}, timeoutOverrideMS, userAgentOverride, nil
	}

	// 2) Basic auth.
	username := strings.TrimSpace(os.Getenv("GRAFANA_USERNAME"))
	password := strings.TrimSpace(os.Getenv("GRAFANA_PASSWORD"))
	if clientCfgOK {
		if username == "" {
			username = strings.TrimSpace(clientCfg.Username)
		}
		if password == "" {
			password = strings.TrimSpace(clientCfg.Password)
		}
	}
	if username != "" && password != "" {
		enc := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		return grafanaConfig{
			baseURL:    baseURL,
			authHeader: "Basic " + enc,
			orgID:      orgID,
		}, timeoutOverrideMS, userAgentOverride, nil
	}

	if allowNoAuth {
		return grafanaConfig{
			baseURL:    baseURL,
			authHeader: "",
			orgID:      orgID,
		}, timeoutOverrideMS, userAgentOverride, nil
	}

	if clientName != "" && clientCfgOK {
		return grafanaConfig{}, 0, "", fmt.Errorf("missing Grafana auth for client %q: configure api_token/bearer_token or username+password (or allow_no_auth=true) in GRAFANA_CLIENTS_JSON", clientName)
	}
	return grafanaConfig{}, 0, "", fmt.Errorf("missing Grafana auth: set GRAFANA_API_TOKEN (recommended) or GRAFANA_BEARER_TOKEN, or GRAFANA_USERNAME+GRAFANA_PASSWORD (or allow_no_auth=true for health-only/public endpoints)")
}

func newGrafanaClient(clientName string, baseOverride string, orgIDOverride int, allowNoAuth bool, timeoutOverrideMS int, userAgentOverride string, cfClientIDOverride string, cfClientSecretOverride string) (*grafanaClient, error) {
	cfg, timeoutMS, ua, err := resolveGrafanaConfig(clientName, baseOverride, orgIDOverride, allowNoAuth, timeoutOverrideMS, userAgentOverride)
	if err != nil {
		return nil, err
	}
	timeout := 30 * time.Second
	if timeoutMS > 0 {
		timeout = time.Duration(timeoutMS) * time.Millisecond
	}
	if strings.TrimSpace(ua) == "" {
		ua = "mcp-lens/1.0 (+https://github.com/golovatskygroup/mcp-lens)"
	}

	cfClientID := strings.TrimSpace(cfClientIDOverride)
	cfClientSecret := strings.TrimSpace(cfClientSecretOverride)

	// Cloudflare Access (Zero Trust) headers (optional).
	// Support both generic and Grafana-specific env var names.
	if cfClientID == "" {
		cfClientID = strings.TrimSpace(os.Getenv("GRAFANA_CF_ACCESS_CLIENT_ID"))
		if cfClientID == "" {
			cfClientID = strings.TrimSpace(os.Getenv("CF_ACCESS_CLIENT_ID"))
		}
	}
	if cfClientSecret == "" {
		cfClientSecret = strings.TrimSpace(os.Getenv("GRAFANA_CF_ACCESS_CLIENT_SECRET"))
		if cfClientSecret == "" {
			cfClientSecret = strings.TrimSpace(os.Getenv("CF_ACCESS_CLIENT_SECRET"))
		}
	}
	effectiveClient := strings.TrimSpace(clientName)
	if effectiveClient == "" {
		effectiveClient = strings.TrimSpace(os.Getenv("GRAFANA_DEFAULT_CLIENT"))
	}
	if effectiveClient != "" {
		if clients := loadGrafanaClientsFromEnv(); len(clients) > 0 {
			if c, ok := clients[effectiveClient]; ok {
				if cfClientID == "" {
					cfClientID = strings.TrimSpace(c.CFAccessID)
				}
				if cfClientSecret == "" {
					cfClientSecret = strings.TrimSpace(c.CFAccessSecret)
				}
			}
		}
	}

	return &grafanaClient{
		baseURL:    cfg.baseURL,
		authHeader: cfg.authHeader,
		orgID:      cfg.orgID,
		userAgent:  ua,
		cfClientID: cfClientID,
		cfSecret:   cfClientSecret,
		c: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (g *grafanaClient) do(ctx context.Context, method string, path string, q url.Values, body any) (int, http.Header, []byte, error) {
	u := g.baseURL + path
	if q != nil && len(q) > 0 {
		u += "?" + q.Encode()
	}

	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, nil, err
		}
		r = strings.NewReader(string(b))
	}

	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(g.userAgent) != "" {
		req.Header.Set("User-Agent", g.userAgent)
	}
	if strings.TrimSpace(g.authHeader) != "" {
		req.Header.Set("Authorization", g.authHeader)
	}
	if g.orgID != 0 {
		req.Header.Set("X-Grafana-Org-Id", strconv.Itoa(g.orgID))
	}
	if strings.TrimSpace(g.cfClientID) != "" {
		req.Header.Set("CF-Access-Client-Id", g.cfClientID)
	}
	if strings.TrimSpace(g.cfSecret) != "" {
		req.Header.Set("CF-Access-Client-Secret", g.cfSecret)
	}

	resp, err := g.c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, b, nil
}

func grafanaAuthHint(status int) string {
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return "Hint: set GRAFANA_BASE_URL and provide auth via GRAFANA_API_TOKEN (recommended) or GRAFANA_BEARER_TOKEN or GRAFANA_USERNAME+GRAFANA_PASSWORD. If Grafana is behind Cloudflare Access, also set GRAFANA_CF_ACCESS_CLIENT_ID + GRAFANA_CF_ACCESS_CLIENT_SECRET (or CF_ACCESS_CLIENT_*). If you use multi-client config, set args.client (GRAFANA_CLIENTS_JSON)."
	}
	return ""
}

type grafanaDashboardURLInfo struct {
	BaseURL string
	UID     string
	OrgID   int
	URL     string
}

var grafanaURLRe = regexp.MustCompile(`https?://[^\s]+`)

func parseGrafanaDashboardURL(raw string) (grafanaDashboardURLInfo, error) {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, "`\"'")
	s = strings.TrimRight(s, ".,);]\"'")
	u, err := url.Parse(s)
	if err != nil {
		return grafanaDashboardURLInfo{}, fmt.Errorf("invalid Grafana URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return grafanaDashboardURLInfo{}, fmt.Errorf("invalid Grafana URL scheme: %s", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return grafanaDashboardURLInfo{}, fmt.Errorf("invalid Grafana URL: missing host")
	}

	var uid string
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "d" && i+1 < len(parts) {
			uid = parts[i+1]
			break
		}
	}
	if strings.TrimSpace(uid) == "" {
		return grafanaDashboardURLInfo{}, fmt.Errorf("Grafana URL does not look like a dashboard URL (expected /d/<uid>/...): %s", s)
	}

	orgID := 0
	if v := strings.TrimSpace(u.Query().Get("orgId")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			orgID = n
		}
	}

	return grafanaDashboardURLInfo{
		BaseURL: fmt.Sprintf("%s://%s", u.Scheme, u.Host),
		UID:     uid,
		OrgID:   orgID,
		URL:     s,
	}, nil
}

func extractGrafanaDashboardURLInfo(text string) (grafanaDashboardURLInfo, bool) {
	for _, m := range grafanaURLRe.FindAllString(text, -1) {
		info, err := parseGrafanaDashboardURL(m)
		if err == nil && strings.TrimSpace(info.UID) != "" {
			return info, true
		}
	}
	return grafanaDashboardURLInfo{}, false
}

type grafanaBaseInput struct {
	Client   string `json:"client,omitempty"`
	Base     string `json:"base_url,omitempty"`
	OrgID    int    `json:"org_id,omitempty"`
	CFID     string `json:"cf_access_client_id,omitempty"`
	CFSecret string `json:"cf_access_client_secret,omitempty"`
}

type grafanaHealthInput struct {
	grafanaBaseInput
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaHealth(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaHealthInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, true, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/health", nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(map[string]any{
		"status": status,
		"health": v,
	}), nil
}

type grafanaMeInput struct {
	grafanaBaseInput
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaGetCurrentUser(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaMeInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/user", nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}
	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(v), nil
}

type grafanaSearchInput struct {
	grafanaBaseInput
	Query         string   `json:"query,omitempty"`
	Type          string   `json:"type,omitempty"` // dash-db|dash-folder
	Tags          []string `json:"tags,omitempty"`
	FolderUIDs    []string `json:"folder_uids,omitempty"`
	DashboardUIDs []string `json:"dashboard_uids,omitempty"`
	Starred       *bool    `json:"starred,omitempty"`
	Limit         int      `json:"limit,omitempty"`
	Page          int      `json:"page,omitempty"`
	TimeoutMS     int      `json:"timeout_ms,omitempty"`
	UserAgent     string   `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaSearch(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaSearchInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	if in.Limit <= 0 {
		in.Limit = 100
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.Type != "" && in.Type != "dash-db" && in.Type != "dash-folder" {
		return errorResult("type must be one of: dash-db, dash-folder"), nil
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	if strings.TrimSpace(in.Query) != "" {
		q.Set("query", strings.TrimSpace(in.Query))
	}
	if strings.TrimSpace(in.Type) != "" {
		q.Set("type", strings.TrimSpace(in.Type))
	}
	for _, t := range in.Tags {
		t = strings.TrimSpace(t)
		if t != "" {
			q.Add("tag", t)
		}
	}
	for _, uid := range in.FolderUIDs {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			q.Add("folderUIDs", uid)
		}
	}
	for _, uid := range in.DashboardUIDs {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			q.Add("dashboardUIDs", uid)
		}
	}
	if in.Starred != nil {
		q.Set("starred", strconv.FormatBool(*in.Starred))
	}
	q.Set("limit", strconv.Itoa(in.Limit))
	q.Set("page", strconv.Itoa(in.Page))

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/search", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var items []any
	_ = json.Unmarshal(body, &items)

	hasNext := len(items) == in.Limit && in.Limit > 0
	out := map[string]any{
		"items":    items,
		"page":     in.Page,
		"limit":    in.Limit,
		"count":    len(items),
		"has_next": hasNext,
	}
	if hasNext {
		out["next_page"] = in.Page + 1
	}
	return jsonResult(out), nil
}

type grafanaGetDashboardInput struct {
	grafanaBaseInput
	UID       string `json:"uid"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaGetDashboard(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaGetDashboardInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.UID = strings.TrimSpace(in.UID)
	if in.UID == "" {
		return errorResult("uid is required"), nil
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/dashboards/uid/"+url.PathEscape(in.UID), nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(v), nil
}

type grafanaGetDashboardSummaryInput struct {
	grafanaBaseInput
	UID               string `json:"uid,omitempty"`
	URL               string `json:"url,omitempty"`
	MaxPanels         int    `json:"max_panels,omitempty"`
	MaxTargetsPerPane int    `json:"max_targets_per_panel,omitempty"`
	TimeoutMS         int    `json:"timeout_ms,omitempty"`
	UserAgent         string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaGetDashboardSummary(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaGetDashboardSummaryInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	if in.MaxPanels <= 0 {
		in.MaxPanels = 200
	}
	if in.MaxTargetsPerPane <= 0 {
		in.MaxTargetsPerPane = 20
	}

	uid := strings.TrimSpace(in.UID)
	baseOverride := strings.TrimSpace(in.Base)
	orgIDOverride := in.OrgID

	if uid == "" && strings.TrimSpace(in.URL) != "" {
		info, err := parseGrafanaDashboardURL(in.URL)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		uid = strings.TrimSpace(info.UID)
		if baseOverride == "" {
			baseOverride = info.BaseURL
		}
		if orgIDOverride == 0 && info.OrgID > 0 {
			orgIDOverride = info.OrgID
		}
	}

	if uid == "" {
		return errorResult("uid or url is required"), nil
	}

	cl, err := newGrafanaClient(in.Client, baseOverride, orgIDOverride, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/dashboards/uid/"+url.PathEscape(uid), nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var payload map[string]any
	_ = json.Unmarshal(body, &payload)
	dashboard, _ := payload["dashboard"].(map[string]any)
	meta, _ := payload["meta"].(map[string]any)

	flatPanels := make([]map[string]any, 0, 64)
	var walkPanels func([]any)
	walkPanels = func(panels []any) {
		for _, p := range panels {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}

			if typ, _ := pm["type"].(string); typ == "row" {
				if nested, ok := pm["panels"].([]any); ok {
					walkPanels(nested)
				}
				continue
			}

			flatPanels = append(flatPanels, pm)
			if nested, ok := pm["panels"].([]any); ok {
				walkPanels(nested)
			}
		}
	}
	if dashboard != nil {
		if panels, ok := dashboard["panels"].([]any); ok {
			walkPanels(panels)
		}
	}

	outPanels := make([]map[string]any, 0, minInt(len(flatPanels), in.MaxPanels))
	queriesIncluded := 0
	for i, pm := range flatPanels {
		if i >= in.MaxPanels {
			break
		}
		panelOut := map[string]any{
			"id":    pm["id"],
			"title": pm["title"],
			"type":  pm["type"],
		}
		if ds, ok := pm["datasource"]; ok {
			panelOut["datasource"] = ds
		}
		if gp, ok := pm["gridPos"]; ok {
			panelOut["grid_pos"] = gp
		}

		if targets, ok := pm["targets"].([]any); ok && len(targets) > 0 {
			targetsOut := make([]map[string]any, 0, minInt(len(targets), in.MaxTargetsPerPane))
			for j, tv := range targets {
				if j >= in.MaxTargetsPerPane {
					break
				}
				tm, ok := tv.(map[string]any)
				if !ok {
					continue
				}
				tOut := map[string]any{}
				if v, ok := tm["refId"].(string); ok && strings.TrimSpace(v) != "" {
					tOut["ref_id"] = v
				}
				if v, ok := tm["expr"].(string); ok && strings.TrimSpace(v) != "" {
					tOut["expr"] = v
				}
				if v, ok := tm["query"].(string); ok && strings.TrimSpace(v) != "" {
					tOut["query"] = v
				}
				if v, ok := tm["legendFormat"].(string); ok && strings.TrimSpace(v) != "" {
					tOut["legend"] = v
				}
				if ds, ok := tm["datasource"]; ok {
					tOut["datasource"] = ds
				}
				if v, ok := tm["format"].(string); ok && strings.TrimSpace(v) != "" {
					tOut["format"] = v
				}
				if v, ok := tm["interval"].(string); ok && strings.TrimSpace(v) != "" {
					tOut["interval"] = v
				}
				if len(tOut) > 0 {
					targetsOut = append(targetsOut, tOut)
					queriesIncluded++
				}
			}
			if len(targetsOut) > 0 {
				panelOut["targets"] = targetsOut
			}
			if len(targets) > in.MaxTargetsPerPane {
				panelOut["targets_total"] = len(targets)
				panelOut["targets_truncated"] = true
			}
		}
		outPanels = append(outPanels, panelOut)
	}

	varsOut := []map[string]any{}
	if dashboard != nil {
		if templ, ok := dashboard["templating"].(map[string]any); ok {
			if list, ok := templ["list"].([]any); ok {
				for _, vv := range list {
					vm, ok := vv.(map[string]any)
					if !ok {
						continue
					}
					vOut := map[string]any{
						"name":  vm["name"],
						"type":  vm["type"],
						"label": vm["label"],
					}
					if ds, ok := vm["datasource"]; ok {
						vOut["datasource"] = ds
					}
					if q, ok := vm["query"]; ok {
						vOut["query"] = q
					} else if def, ok := vm["definition"]; ok {
						vOut["query"] = def
					}
					if multi, ok := vm["multi"]; ok {
						vOut["multi"] = multi
					}
					if includeAll, ok := vm["includeAll"]; ok {
						vOut["include_all"] = includeAll
					}
					varsOut = append(varsOut, vOut)
				}
			}
		}
	}

	out := map[string]any{
		"uid":              uid,
		"base_url":         cl.baseURL,
		"org_id":           cl.orgID,
		"title":            nil,
		"tags":             nil,
		"timezone":         nil,
		"refresh":          nil,
		"time":             nil,
		"variables":        varsOut,
		"panels":           outPanels,
		"panel_count":      len(flatPanels),
		"panels_included":  len(outPanels),
		"queries_included": queriesIncluded,
	}
	if dashboard != nil {
		out["title"] = dashboard["title"]
		out["tags"] = dashboard["tags"]
		out["timezone"] = dashboard["timezone"]
		out["refresh"] = dashboard["refresh"]
		if tm, ok := dashboard["time"].(map[string]any); ok {
			out["time"] = tm
		}
	}
	if meta != nil {
		out["dashboard_url"] = meta["url"]
		out["slug"] = meta["slug"]
		out["folder_title"] = meta["folderTitle"]
		out["folder_uid"] = meta["folderUid"]
	}
	if len(flatPanels) > in.MaxPanels {
		out["panels_truncated"] = true
		out["panels_total"] = len(flatPanels)
	}
	return jsonResult(out), nil
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type grafanaListFoldersInput struct {
	grafanaBaseInput
	Limit     int    `json:"limit,omitempty"`
	Page      int    `json:"page,omitempty"`
	ParentUID string `json:"parent_uid,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaListFolders(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaListFoldersInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Limit <= 0 {
		in.Limit = 1000
	}
	if in.Page <= 0 {
		in.Page = 1
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("limit", strconv.Itoa(in.Limit))
	q.Set("page", strconv.Itoa(in.Page))
	if strings.TrimSpace(in.ParentUID) != "" {
		q.Set("parentUid", strings.TrimSpace(in.ParentUID))
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/folders", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var items []any
	_ = json.Unmarshal(body, &items)

	hasNext := len(items) == in.Limit && in.Limit > 0
	out := map[string]any{
		"items":    items,
		"page":     in.Page,
		"limit":    in.Limit,
		"count":    len(items),
		"has_next": hasNext,
	}
	if hasNext {
		out["next_page"] = in.Page + 1
	}
	return jsonResult(out), nil
}

type grafanaGetFolderInput struct {
	grafanaBaseInput
	UID       string `json:"uid"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaGetFolder(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaGetFolderInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.UID = strings.TrimSpace(in.UID)
	if in.UID == "" {
		return errorResult("uid is required"), nil
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/folders/"+url.PathEscape(in.UID), nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(v), nil
}

type grafanaListDatasourcesInput struct {
	grafanaBaseInput
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaListDatasources(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaListDatasourcesInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/datasources", nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(v), nil
}

type grafanaGetDatasourceInput struct {
	grafanaBaseInput
	UID       string `json:"uid,omitempty"`
	Name      string `json:"name,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaGetDatasource(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaGetDatasourceInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.UID = strings.TrimSpace(in.UID)
	in.Name = strings.TrimSpace(in.Name)
	if in.UID == "" && in.Name == "" {
		return errorResult("uid or name is required"), nil
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	path := ""
	if in.UID != "" {
		path = "/api/datasources/uid/" + url.PathEscape(in.UID)
	} else {
		path = "/api/datasources/name/" + url.PathEscape(in.Name)
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, path, nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(v), nil
}

type grafanaAnnotationsInput struct {
	grafanaBaseInput
	From         int64    `json:"from,omitempty"`
	To           int64    `json:"to,omitempty"`
	Limit        int      `json:"limit,omitempty"`
	AlertID      int64    `json:"alert_id,omitempty"`
	AlertUID     string   `json:"alert_uid,omitempty"`
	DashboardUID string   `json:"dashboard_uid,omitempty"`
	PanelID      int64    `json:"panel_id,omitempty"`
	UserID       int64    `json:"user_id,omitempty"`
	Type         string   `json:"type,omitempty"` // alert|annotation
	Tags         []string `json:"tags,omitempty"`
	MatchAny     *bool    `json:"match_any,omitempty"`
	TimeoutMS    int      `json:"timeout_ms,omitempty"`
	UserAgent    string   `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaQueryAnnotations(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaAnnotationsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}
	in.AlertUID = strings.TrimSpace(in.AlertUID)
	in.DashboardUID = strings.TrimSpace(in.DashboardUID)
	in.Type = strings.TrimSpace(in.Type)
	if in.Type != "" && in.Type != "alert" && in.Type != "annotation" {
		return errorResult("type must be one of: alert, annotation"), nil
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	if in.From != 0 {
		q.Set("from", strconv.FormatInt(in.From, 10))
	}
	if in.To != 0 {
		q.Set("to", strconv.FormatInt(in.To, 10))
	}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}
	if in.AlertID != 0 {
		q.Set("alertId", strconv.FormatInt(in.AlertID, 10))
	}
	if in.AlertUID != "" {
		q.Set("alertUID", in.AlertUID)
	}
	if in.DashboardUID != "" {
		q.Set("dashboardUID", in.DashboardUID)
	}
	if in.PanelID != 0 {
		q.Set("panelId", strconv.FormatInt(in.PanelID, 10))
	}
	if in.UserID != 0 {
		q.Set("userId", strconv.FormatInt(in.UserID, 10))
	}
	if in.Type != "" {
		q.Set("type", in.Type)
	}
	for _, t := range in.Tags {
		t = strings.TrimSpace(t)
		if t != "" {
			q.Add("tags", t)
		}
	}
	if in.MatchAny != nil {
		q.Set("matchAny", strconv.FormatBool(*in.MatchAny))
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/annotations", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var items []any
	_ = json.Unmarshal(body, &items)
	return jsonResult(map[string]any{
		"items": items,
		"count": len(items),
	}), nil
}

type grafanaAnnotationTagsInput struct {
	grafanaBaseInput
	Tag       string `json:"tag,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

func (h *Handler) grafanaListAnnotationTags(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaAnnotationTagsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Limit <= 0 {
		in.Limit = 100
	}

	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, false, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	if strings.TrimSpace(in.Tag) != "" {
		q.Set("tag", strings.TrimSpace(in.Tag))
	}
	q.Set("limit", strconv.Itoa(in.Limit))

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/annotations/tags", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := grafanaAuthHint(status)
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var v any
	_ = json.Unmarshal(body, &v)
	return jsonResult(v), nil
}
