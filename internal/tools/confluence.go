package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/httpcache"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type confluenceClient struct {
	baseURL       string
	isCloudSite   bool // https://<site>.atlassian.net/wiki
	isCloudBridge bool // https://api.atlassian.com/ex/confluence/<cloudId>
	authHeader    string
	c             *http.Client
}

var errConfluenceHTMLOrRedirect = errors.New("confluence api returned html/redirect (likely login page)")

type confluenceConfig struct {
	baseURL       string
	isCloudSite   bool
	isCloudBridge bool
	authHeader    string
}

type confluenceClientEnvConfig struct {
	BaseURL          string `json:"base_url,omitempty"`
	PAT              string `json:"pat,omitempty"`
	BearerToken      string `json:"bearer_token,omitempty"`
	Email            string `json:"email,omitempty"`
	APIToken         string `json:"api_token,omitempty"`
	Username         string `json:"username,omitempty"`
	Password         string `json:"password,omitempty"`
	OAuthAccessToken string `json:"oauth_access_token,omitempty"`
	CloudID          string `json:"cloud_id,omitempty"`
}

var (
	confluenceClientsOnce sync.Once
	confluenceClientsMap  map[string]confluenceClientEnvConfig
)

func loadConfluenceClientsFromEnv() map[string]confluenceClientEnvConfig {
	confluenceClientsOnce.Do(func() {
		confluenceClientsMap = map[string]confluenceClientEnvConfig{}
		raw := strings.TrimSpace(os.Getenv("CONFLUENCE_CLIENTS_JSON"))
		if raw == "" {
			return
		}
		_ = json.Unmarshal([]byte(raw), &confluenceClientsMap)
	})
	return confluenceClientsMap
}

func confluencePublicClientsFromEnv() map[string]map[string]any {
	clients := loadConfluenceClientsFromEnv()
	if len(clients) == 0 {
		return nil
	}
	out := map[string]map[string]any{}
	for name, cfg := range clients {
		if strings.TrimSpace(name) == "" {
			continue
		}
		baseURL := strings.TrimSpace(cfg.BaseURL)
		if baseURL == "" && strings.TrimSpace(cfg.CloudID) != "" {
			baseURL = "https://api.atlassian.com/ex/confluence/" + strings.TrimSpace(cfg.CloudID)
		}
		out[name] = map[string]any{
			"base_url": baseURL,
		}
	}
	return out
}

func isConfluenceCloudSiteBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, ".atlassian.net")
}

func isConfluenceCloudBridgeBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.HasPrefix(u, "https://api.atlassian.com/ex/confluence/")
}

func normalizeConfluenceBaseURL(baseURL string) (string, bool, bool) {
	baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if baseURL == "" {
		return "", false, false
	}

	isCloudSite := isConfluenceCloudSiteBaseURL(baseURL)
	isCloudBridge := isConfluenceCloudBridgeBaseURL(baseURL)

	// Cloud site API base typically lives under /wiki.
	if isCloudSite && !strings.HasSuffix(strings.ToLower(baseURL), "/wiki") {
		baseURL = baseURL + "/wiki"
	}

	return baseURL, isCloudSite, isCloudBridge
}

func resolveConfluenceConfig(clientName string, baseOverride string) (confluenceConfig, error) {
	clientName = strings.TrimSpace(clientName)
	clients := loadConfluenceClientsFromEnv()
	if clientName == "" {
		clientName = strings.TrimSpace(os.Getenv("CONFLUENCE_DEFAULT_CLIENT"))
	}

	var clientCfg confluenceClientEnvConfig
	var clientCfgOK bool
	if clientName != "" && len(clients) > 0 {
		clientCfg, clientCfgOK = clients[clientName]
	}

	baseURL := strings.TrimSpace(baseOverride)
	if baseURL == "" {
		if clientCfgOK && strings.TrimSpace(clientCfg.BaseURL) != "" {
			baseURL = strings.TrimSpace(clientCfg.BaseURL)
		} else {
			baseURL = strings.TrimSpace(os.Getenv("CONFLUENCE_BASE_URL"))
		}
	}

	oauthToken := strings.TrimSpace(os.Getenv("CONFLUENCE_OAUTH_ACCESS_TOKEN"))
	cloudID := strings.TrimSpace(os.Getenv("CONFLUENCE_CLOUD_ID"))
	if clientCfgOK {
		if oauthToken == "" {
			oauthToken = strings.TrimSpace(clientCfg.OAuthAccessToken)
		}
		if cloudID == "" {
			cloudID = strings.TrimSpace(clientCfg.CloudID)
		}
	}

	if baseURL == "" && oauthToken != "" && cloudID != "" {
		baseURL = "https://api.atlassian.com/ex/confluence/" + cloudID
	}

	if baseURL == "" {
		if clientName != "" && !clientCfgOK {
			return confluenceConfig{}, fmt.Errorf("unknown Confluence client %q: not found in CONFLUENCE_CLIENTS_JSON", clientName)
		}
		return confluenceConfig{}, fmt.Errorf("missing Confluence base URL: set CONFLUENCE_BASE_URL or configure CONFLUENCE_CLIENTS_JSON (or set CONFLUENCE_OAUTH_ACCESS_TOKEN + CONFLUENCE_CLOUD_ID)")
	}

	baseURL, isCloudSite, isCloudBridge := normalizeConfluenceBaseURL(baseURL)

	// Auth precedence:
	// 1) OAuth access token / bearer token / PAT.
	bearer := oauthToken
	if bearer == "" {
		bearer = strings.TrimSpace(os.Getenv("CONFLUENCE_BEARER_TOKEN"))
		if bearer == "" && clientCfgOK {
			bearer = strings.TrimSpace(clientCfg.BearerToken)
		}
	}
	if bearer == "" {
		bearer = strings.TrimSpace(os.Getenv("CONFLUENCE_PAT"))
		if bearer == "" && clientCfgOK {
			bearer = strings.TrimSpace(clientCfg.PAT)
		}
	}
	if bearer != "" {
		return confluenceConfig{
			baseURL:       baseURL,
			isCloudSite:   isCloudSite,
			isCloudBridge: isCloudBridge,
			authHeader:    "Bearer " + bearer,
		}, nil
	}

	// 2) Basic auth: email + API token (Cloud) or username+password (DC/Server).
	email := strings.TrimSpace(os.Getenv("CONFLUENCE_EMAIL"))
	apiToken := strings.TrimSpace(os.Getenv("CONFLUENCE_API_TOKEN"))
	if clientCfgOK {
		if email == "" {
			email = strings.TrimSpace(clientCfg.Email)
		}
		if apiToken == "" {
			apiToken = strings.TrimSpace(clientCfg.APIToken)
		}
	}
	if email != "" && apiToken != "" {
		enc := base64.StdEncoding.EncodeToString([]byte(email + ":" + apiToken))
		return confluenceConfig{
			baseURL:       baseURL,
			isCloudSite:   isCloudSite,
			isCloudBridge: isCloudBridge,
			authHeader:    "Basic " + enc,
		}, nil
	}

	username := strings.TrimSpace(os.Getenv("CONFLUENCE_USERNAME"))
	password := strings.TrimSpace(os.Getenv("CONFLUENCE_PASSWORD"))
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
		return confluenceConfig{
			baseURL:       baseURL,
			isCloudSite:   isCloudSite,
			isCloudBridge: isCloudBridge,
			authHeader:    "Basic " + enc,
		}, nil
	}

	if clientName != "" && clientCfgOK {
		return confluenceConfig{}, fmt.Errorf("missing Confluence auth for client %q: configure bearer_token/pat/oauth_access_token (or email+api_token) in CONFLUENCE_CLIENTS_JSON", clientName)
	}
	return confluenceConfig{}, fmt.Errorf("missing Confluence auth: set CONFLUENCE_PAT/CONFLUENCE_BEARER_TOKEN, or CONFLUENCE_EMAIL + CONFLUENCE_API_TOKEN, or CONFLUENCE_OAUTH_ACCESS_TOKEN (+ CONFLUENCE_CLOUD_ID), or configure CONFLUENCE_CLIENTS_JSON")
}

func newConfluenceClient(clientName string, baseOverride string) (*confluenceClient, error) {
	cfg, err := resolveConfluenceConfig(clientName, baseOverride)
	if err != nil {
		return nil, err
	}
	return &confluenceClient{
		baseURL:       cfg.baseURL,
		isCloudSite:   cfg.isCloudSite,
		isCloudBridge: cfg.isCloudBridge,
		authHeader:    cfg.authHeader,
		c: &http.Client{
			Timeout:   30 * time.Second,
			Transport: httpcache.NewTransportFromEnv(nil),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *confluenceClient) apiV1Base() string {
	// Cloud site: https://<site>.atlassian.net/wiki/rest/api
	// Cloud OAuth bridge: https://api.atlassian.com/ex/confluence/<cloudId>/rest/api
	// DC/Server: <base>/rest/api
	return c.baseURL + "/rest/api"
}

func (c *confluenceClient) apiV2Base() (string, error) {
	// Only supported on Cloud site base URLs: https://<site>.atlassian.net/wiki/api/v2
	if !c.isCloudSite {
		return "", errors.New("confluence api v2 is only supported for Confluence Cloud site base URLs (e.g. https://<site>.atlassian.net/wiki)")
	}
	return c.baseURL + "/api/v2", nil
}

func (c *confluenceClient) do(ctx context.Context, method string, fullURL string, query url.Values, headers map[string]string) (int, http.Header, []byte, error) {
	u := fullURL
	if len(query) > 0 {
		if strings.Contains(u, "?") {
			u += "&" + query.Encode()
		} else {
			u += "?" + query.Encode()
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, nil, nil, err
	}

	req.Header.Set("Accept", "application/json")
	if c.authHeader != "" {
		req.Header.Set("Authorization", c.authHeader)
	}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	resp, err := c.c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	if looksLikeHTML(b) {
		return resp.StatusCode, resp.Header, b, fmt.Errorf("confluence api returned html/redirect (likely login page): %w", errConfluenceHTMLOrRedirect)
	}
	return resp.StatusCode, resp.Header, b, nil
}

type confluenceListSpacesInput struct {
	Client  string `json:"client,omitempty"`
	BaseURL string `json:"base_url,omitempty"`

	UseV2  *bool  `json:"use_v2,omitempty"` // Cloud only; default true
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"` // Cloud v2
	Start  int    `json:"start,omitempty"`  // v1 (DC/Server + bridge)
}

func parseNextLinkURL(linkHeader string) string {
	// RFC 5988: <url>; rel="next", ...
	parts := strings.Split(linkHeader, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if !strings.Contains(p, `rel="next"`) && !strings.Contains(p, `rel=next`) {
			continue
		}
		start := strings.Index(p, "<")
		end := strings.Index(p, ">")
		if start >= 0 && end > start+1 {
			return p[start+1 : end]
		}
	}
	return ""
}

func parseNextCursorFromRelative(nextURL string) (cursor string, start *int, ok bool) {
	nextURL = strings.TrimSpace(nextURL)
	if nextURL == "" {
		return "", nil, false
	}
	u, err := url.Parse(nextURL)
	if err != nil {
		return "", nil, false
	}
	q := u.Query()
	if c := strings.TrimSpace(q.Get("cursor")); c != "" {
		return c, nil, true
	}
	if s := strings.TrimSpace(q.Get("start")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return "", &n, true
		}
	}
	return "", nil, false
}

func (h *Handler) confluenceListSpaces(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluenceListSpacesInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Limit <= 0 {
		in.Limit = 25
	}
	if in.Limit > 250 {
		in.Limit = 250
	}
	if in.Start < 0 {
		return errorResult("start must be >= 0"), nil
	}

	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	useV2 := true
	if in.UseV2 != nil {
		useV2 = *in.UseV2
	}

	if cl.isCloudSite && useV2 {
		base, err := cl.apiV2Base()
		if err != nil {
			return errorResult(err.Error()), nil
		}

		q := url.Values{}
		q.Set("limit", strconv.Itoa(in.Limit))
		if strings.TrimSpace(in.Cursor) != "" {
			q.Set("cursor", strings.TrimSpace(in.Cursor))
		}

		status, headers, body, err := cl.do(ctx, http.MethodGet, base+"/spaces", q, nil)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		if status < 200 || status >= 300 {
			return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
		}

		var data map[string]any
		_ = json.Unmarshal(body, &data)

		var nextURL string
		if links, ok := data["_links"].(map[string]any); ok {
			if s, ok := links["next"].(string); ok {
				nextURL = strings.TrimSpace(s)
			}
		}
		if nextURL == "" {
			nextURL = strings.TrimSpace(parseNextLinkURL(headers.Get("Link")))
		}

		nextCursor, _, ok := parseNextCursorFromRelative(nextURL)
		hasNext := ok && strings.TrimSpace(nextCursor) != ""

		out := map[string]any{
			"limit":       in.Limit,
			"cursor":      strings.TrimSpace(in.Cursor),
			"has_next":    hasNext,
			"next_cursor": nextCursor,
			"next_url":    nextURL,
			"data":        data,
		}
		return jsonResult(out), nil
	}

	// v1: /rest/api/space (DC/Server + Cloud OAuth bridge; also works on Cloud site).
	q := url.Values{}
	q.Set("limit", strconv.Itoa(in.Limit))
	if in.Start > 0 {
		q.Set("start", strconv.Itoa(in.Start))
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, cl.apiV1Base()+"/space", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}

	var data map[string]any
	_ = json.Unmarshal(body, &data)

	var nextURL string
	if links, ok := data["_links"].(map[string]any); ok {
		if s, ok := links["next"].(string); ok {
			nextURL = strings.TrimSpace(s)
		}
	}

	_, nextStartPtr, ok := parseNextCursorFromRelative(nextURL)
	nextStart := 0
	hasNext := false
	if ok && nextStartPtr != nil {
		nextStart = *nextStartPtr
		hasNext = true
	}

	out := map[string]any{
		"limit":    in.Limit,
		"start":    in.Start,
		"has_next": hasNext,
		"next_start": func() any {
			if !hasNext {
				return nil
			}
			return nextStart
		}(),
		"next_url": nextURL,
		"data":     data,
	}
	return jsonResult(out), nil
}

type confluenceGetPageInput struct {
	ID         string   `json:"id"`
	BodyFormat string   `json:"body_format,omitempty"` // storage|view|export_view
	Expand     []string `json:"expand,omitempty"`      // v1 expand (additional)
	Client     string   `json:"client,omitempty"`
	BaseURL    string   `json:"base_url,omitempty"`
	UseV2      *bool    `json:"use_v2,omitempty"` // Cloud only; default true
}

func confluenceExpandForBodyFormat(bodyFormat string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(bodyFormat)) {
	case "", "storage":
		return "body.storage", nil
	case "view":
		return "body.view", nil
	case "export_view":
		return "body.export_view", nil
	default:
		return "", fmt.Errorf("unsupported body_format %q (use storage|view|export_view)", bodyFormat)
	}
}

func (h *Handler) confluenceGetPage(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluenceGetPageInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" {
		return errorResult("id is required"), nil
	}

	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	useV2 := true
	if in.UseV2 != nil {
		useV2 = *in.UseV2
	}

	bodyFormat := strings.ToLower(strings.TrimSpace(in.BodyFormat))
	if cl.isCloudSite && useV2 && (bodyFormat == "" || bodyFormat == "storage") {
		base, err := cl.apiV2Base()
		if err != nil {
			return errorResult(err.Error()), nil
		}

		q := url.Values{}
		q.Set("body-format", "storage")

		status, _, body, err := cl.do(ctx, http.MethodGet, base+"/pages/"+url.PathEscape(in.ID), q, nil)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		if status < 200 || status >= 300 {
			return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
		}
		var page map[string]any
		_ = json.Unmarshal(body, &page)
		return jsonResult(page), nil
	}

	expandBody, err := confluenceExpandForBodyFormat(in.BodyFormat)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	expands := []string{expandBody, "space", "version"}
	for _, e := range in.Expand {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		expands = append(expands, e)
	}
	q := url.Values{}
	q.Set("expand", strings.Join(expands, ","))

	status, _, body, err := cl.do(ctx, http.MethodGet, cl.apiV1Base()+"/content/"+url.PathEscape(in.ID), q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}

	var page map[string]any
	_ = json.Unmarshal(body, &page)
	return jsonResult(page), nil
}

type confluenceGetPageByTitleInput struct {
	SpaceKey   string   `json:"space_key"`
	Title      string   `json:"title"`
	BodyFormat string   `json:"body_format,omitempty"` // storage|view|export_view
	Expand     []string `json:"expand,omitempty"`
	Client     string   `json:"client,omitempty"`
	BaseURL    string   `json:"base_url,omitempty"`
	Limit      int      `json:"limit,omitempty"` // default 5
}

func (h *Handler) confluenceGetPageByTitle(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluenceGetPageByTitleInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.SpaceKey = strings.TrimSpace(in.SpaceKey)
	in.Title = strings.TrimSpace(in.Title)
	if in.SpaceKey == "" || in.Title == "" {
		return errorResult("space_key and title are required"), nil
	}
	if in.Limit <= 0 {
		in.Limit = 5
	}
	if in.Limit > 25 {
		in.Limit = 25
	}

	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	expandBody, err := confluenceExpandForBodyFormat(in.BodyFormat)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	expands := []string{expandBody, "space", "version"}
	for _, e := range in.Expand {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		expands = append(expands, e)
	}

	q := url.Values{}
	q.Set("type", "page")
	q.Set("spaceKey", in.SpaceKey)
	q.Set("title", in.Title)
	q.Set("limit", strconv.Itoa(in.Limit))
	q.Set("expand", strings.Join(expands, ","))

	status, _, body, err := cl.do(ctx, http.MethodGet, cl.apiV1Base()+"/content", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}

	var data map[string]any
	_ = json.Unmarshal(body, &data)

	results, _ := data["results"].([]any)
	if len(results) == 0 {
		return errorResult("page not found"), nil
	}

	out := map[string]any{
		"space_key": in.SpaceKey,
		"title":     in.Title,
		"count":     len(results),
		"page":      results[0],
		"data":      data,
	}
	return jsonResult(out), nil
}

type confluenceSearchCQLInput struct {
	CQL                   string `json:"cql"`
	Limit                 int    `json:"limit,omitempty"`
	Cursor                string `json:"cursor,omitempty"`
	Start                 int    `json:"start,omitempty"`
	IncludeArchivedSpaces *bool  `json:"include_archived_spaces,omitempty"`

	Client  string `json:"client,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
}

func (h *Handler) confluenceSearchCQL(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluenceSearchCQLInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.CQL = strings.TrimSpace(in.CQL)
	if in.CQL == "" {
		return errorResult("cql is required"), nil
	}
	if in.Limit <= 0 {
		in.Limit = 25
	}
	if in.Limit > 250 {
		in.Limit = 250
	}
	if in.Start < 0 {
		return errorResult("start must be >= 0"), nil
	}

	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("cql", in.CQL)
	q.Set("limit", strconv.Itoa(in.Limit))
	if strings.TrimSpace(in.Cursor) != "" {
		q.Set("cursor", strings.TrimSpace(in.Cursor))
	} else if in.Start > 0 {
		q.Set("start", strconv.Itoa(in.Start))
	}
	if in.IncludeArchivedSpaces != nil {
		q.Set("includeArchivedSpaces", strconv.FormatBool(*in.IncludeArchivedSpaces))
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, cl.apiV1Base()+"/search", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}

	var data map[string]any
	_ = json.Unmarshal(body, &data)

	var nextURL string
	if links, ok := data["_links"].(map[string]any); ok {
		if s, ok := links["next"].(string); ok {
			nextURL = strings.TrimSpace(s)
		}
	}
	nextCursor, nextStartPtr, ok := parseNextCursorFromRelative(nextURL)
	hasNext := ok && (strings.TrimSpace(nextCursor) != "" || nextStartPtr != nil)

	out := map[string]any{
		"cql":      in.CQL,
		"limit":    in.Limit,
		"cursor":   strings.TrimSpace(in.Cursor),
		"start":    in.Start,
		"has_next": hasNext,
		"next_url": nextURL,
		"next_cursor": func() any {
			if strings.TrimSpace(nextCursor) == "" {
				return nil
			}
			return nextCursor
		}(),
		"next_start": func() any {
			if nextStartPtr == nil {
				return nil
			}
			return *nextStartPtr
		}(),
		"data": data,
	}
	return jsonResult(out), nil
}
