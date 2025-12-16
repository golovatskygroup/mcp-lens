package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/httpcache"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type jiraClient struct {
	baseURL    string
	apiVersion int
	authHeader string
	c          *http.Client
}

type jiraConfig struct {
	baseURL    string
	apiVersion int
	authHeader string
}

type jiraClientEnvConfig struct {
	BaseURL          string `json:"base_url,omitempty"`
	APIVersion       int    `json:"api_version,omitempty"` // 2 or 3
	PAT              string `json:"pat,omitempty"`
	BearerToken      string `json:"bearer_token,omitempty"`
	Email            string `json:"email,omitempty"`
	APIToken         string `json:"api_token,omitempty"`
	OAuthAccessToken string `json:"oauth_access_token,omitempty"`
	CloudID          string `json:"cloud_id,omitempty"`
}

var (
	jiraClientsOnce sync.Once
	jiraClientsMap  map[string]jiraClientEnvConfig
)

func loadJiraClientsFromEnv() map[string]jiraClientEnvConfig {
	jiraClientsOnce.Do(func() {
		jiraClientsMap = map[string]jiraClientEnvConfig{}
		raw := strings.TrimSpace(os.Getenv("JIRA_CLIENTS_JSON"))
		if raw == "" {
			return
		}
		_ = json.Unmarshal([]byte(raw), &jiraClientsMap)
	})
	return jiraClientsMap
}

func jiraPublicClientsFromEnv() map[string]map[string]any {
	clients := loadJiraClientsFromEnv()
	if len(clients) == 0 {
		return nil
	}
	out := map[string]map[string]any{}
	for name, cfg := range clients {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if strings.TrimSpace(cfg.BaseURL) == "" && strings.TrimSpace(cfg.CloudID) != "" {
			// 3LO base URL (publicly safe).
			cfg.BaseURL = "https://api.atlassian.com/ex/jira/" + strings.TrimSpace(cfg.CloudID)
		}
		out[name] = map[string]any{
			"base_url":    strings.TrimSpace(cfg.BaseURL),
			"api_version": cfg.APIVersion,
		}
	}
	return out
}

func resolveJiraConfig(clientName string, baseOverride string, apiVersionOverride int) (jiraConfig, error) {
	apiVersion := apiVersionOverride

	clientName = strings.TrimSpace(clientName)
	clients := loadJiraClientsFromEnv()
	if clientName == "" {
		clientName = strings.TrimSpace(os.Getenv("JIRA_DEFAULT_CLIENT"))
	}

	// If a client alias is provided and exists, use it as the base config.
	var clientCfg jiraClientEnvConfig
	var clientCfgOK bool
	if clientName != "" && len(clients) > 0 {
		clientCfg, clientCfgOK = clients[clientName]
	}

	baseURL := strings.TrimSpace(baseOverride)
	if baseURL == "" {
		if clientCfgOK && strings.TrimSpace(clientCfg.BaseURL) != "" {
			baseURL = strings.TrimSpace(clientCfg.BaseURL)
		} else {
			baseURL = strings.TrimSpace(os.Getenv("JIRA_BASE_URL"))
		}
	}

	// OAuth 2.0 (3LO) mode uses api.atlassian.com base with cloudId.
	oauthToken := strings.TrimSpace(os.Getenv("JIRA_OAUTH_ACCESS_TOKEN"))
	cloudID := strings.TrimSpace(os.Getenv("JIRA_CLOUD_ID"))
	if clientCfgOK {
		if oauthToken == "" {
			oauthToken = strings.TrimSpace(clientCfg.OAuthAccessToken)
		}
		if cloudID == "" {
			cloudID = strings.TrimSpace(clientCfg.CloudID)
		}
	}
	if baseURL == "" && oauthToken != "" && cloudID != "" {
		baseURL = "https://api.atlassian.com/ex/jira/" + cloudID
	}

	if baseURL == "" {
		if clientName != "" && !clientCfgOK {
			return jiraConfig{}, fmt.Errorf("unknown Jira client %q: not found in JIRA_CLIENTS_JSON", clientName)
		}
		return jiraConfig{}, fmt.Errorf("missing Jira base URL: set JIRA_BASE_URL (site or DC base), or configure JIRA_CLIENTS_JSON, or set JIRA_OAUTH_ACCESS_TOKEN + JIRA_CLOUD_ID (3LO)")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	if apiVersion == 0 {
		// Default to v2 for Server/Data Center (many instances redirect /rest/api/3 to HTML login),
		// and v3 for Jira Cloud. Override per-call with input.api_version.
		if isJiraCloudBaseURL(baseURL) {
			apiVersion = 3
		} else {
			apiVersion = 2
		}
	}
	if clientCfgOK && clientCfg.APIVersion != 0 && apiVersionOverride == 0 {
		// If caller didn't override api_version explicitly, allow per-client default.
		apiVersion = clientCfg.APIVersion
	}
	if apiVersion != 2 && apiVersion != 3 {
		return jiraConfig{}, fmt.Errorf("api_version must be 2 or 3")
	}

	// Auth precedence:
	// 1) OAuth access token (3LO) / generic bearer token.
	if oauthToken == "" {
		oauthToken = strings.TrimSpace(os.Getenv("JIRA_BEARER_TOKEN"))
		if oauthToken == "" && clientCfgOK {
			oauthToken = strings.TrimSpace(clientCfg.BearerToken)
		}
	}
	if oauthToken == "" {
		oauthToken = strings.TrimSpace(os.Getenv("JIRA_PAT"))
		if oauthToken == "" && clientCfgOK {
			oauthToken = strings.TrimSpace(clientCfg.PAT)
		}
	}
	if oauthToken != "" {
		return jiraConfig{
			baseURL:    baseURL,
			apiVersion: apiVersion,
			authHeader: "Bearer " + oauthToken,
		}, nil
	}

	// 2) Basic auth: email + API token (Jira Cloud recommended for scripts).
	email := strings.TrimSpace(os.Getenv("JIRA_EMAIL"))
	apiToken := strings.TrimSpace(os.Getenv("JIRA_API_TOKEN"))
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
		return jiraConfig{
			baseURL:    baseURL,
			apiVersion: apiVersion,
			authHeader: "Basic " + enc,
		}, nil
	}

	if clientName != "" && clientCfgOK {
		return jiraConfig{}, fmt.Errorf("missing Jira auth for client %q: configure pat/bearer_token or email+api_token (or oauth_access_token+cloud_id) in JIRA_CLIENTS_JSON", clientName)
	}
	return jiraConfig{}, fmt.Errorf("missing Jira auth: set JIRA_PAT (Data Center/Server) or JIRA_EMAIL + JIRA_API_TOKEN (Cloud), or JIRA_OAUTH_ACCESS_TOKEN (+ JIRA_CLOUD_ID), or configure JIRA_CLIENTS_JSON")
}

func newJiraClient(clientName string, baseOverride string, apiVersionOverride int) (*jiraClient, error) {
	cfg, err := resolveJiraConfig(clientName, baseOverride, apiVersionOverride)
	if err != nil {
		return nil, err
	}
	return &jiraClient{
		baseURL:    cfg.baseURL,
		apiVersion: cfg.apiVersion,
		authHeader: cfg.authHeader,
		c: &http.Client{
			Timeout:   30 * time.Second,
			Transport: httpcache.NewTransportFromEnv(nil),
			// Do not follow redirects automatically. Jira DC commonly redirects /rest/api/3 to login pages.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (j *jiraClient) apiBase() string {
	return j.baseURL + "/rest/api/" + strconv.Itoa(j.apiVersion)
}

func isJiraCloudBaseURL(baseURL string) bool {
	u := strings.ToLower(strings.TrimSpace(baseURL))
	return strings.Contains(u, ".atlassian.net") || strings.HasPrefix(u, "https://api.atlassian.com/ex/jira/")
}

var errJiraHTMLOrRedirect = errors.New("jira api returned html/redirect (likely login page)")

func looksLikeHTML(b []byte) bool {
	s := strings.TrimSpace(strings.ToLower(string(b)))
	if s == "" {
		return false
	}
	return strings.HasPrefix(s, "<!doctype html") || strings.HasPrefix(s, "<html") || (strings.Contains(s, "<html") && strings.Contains(s, "<body"))
}

func (j *jiraClient) do(ctx context.Context, method string, apiPath string, query url.Values, headers map[string]string, body []byte) (int, http.Header, []byte, error) {
	u := j.apiBase() + apiPath
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "mcp-lens")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if j.authHeader != "" {
		req.Header.Set("Authorization", j.authHeader)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := j.c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return resp.StatusCode, resp.Header, b, errJiraHTMLOrRedirect
	}
	if strings.Contains(ct, "text/html") || looksLikeHTML(b) {
		return resp.StatusCode, resp.Header, b, errJiraHTMLOrRedirect
	}
	return resp.StatusCode, resp.Header, b, nil
}

func jiraAuthHint(status int, body []byte) string {
	switch status {
	case http.StatusUnauthorized:
		return "Jira API returned 401. Check auth env vars: (Cloud) JIRA_EMAIL + JIRA_API_TOKEN, (DC/Server) JIRA_PAT, (3LO) JIRA_OAUTH_ACCESS_TOKEN (+ JIRA_CLOUD_ID)."
	case http.StatusForbidden:
		return "Jira API returned 403. Likely missing permissions/scopes or blocked operation for this user/app."
	case http.StatusNotFound:
		return "Jira API returned 404. Issue/project may not exist or your user/app lacks access (some Jira instances mask permission issues as 404)."
	case http.StatusTooManyRequests:
		// Cloud: points-based rate limiting.
		return "Jira API returned 429 (rate limited). Respect Retry-After and retry with backoff."
	default:
		// Best-effort hint for common auth denial responses.
		if bytes.Contains(bytes.ToLower(body), []byte("captcha")) {
			return "Jira reported CAPTCHA/authentication denial; interactive login may be required to clear it."
		}
		return ""
	}
}

// --- Tool inputs ---

type jiraBaseInput struct {
	Client     string `json:"client,omitempty"`
	BaseURL    string `json:"base_url,omitempty"`
	APIVersion int    `json:"api_version,omitempty"` // 2 or 3 (default depends on base URL)
}

type jiraGetIssueInput struct {
	jiraBaseInput
	Issue  string   `json:"issue"`
	Fields []string `json:"fields,omitempty"`
	Expand []string `json:"expand,omitempty"`
}

type jiraSearchIssuesInput struct {
	jiraBaseInput
	JQL        string   `json:"jql"`
	StartAt    int      `json:"startAt,omitempty"`
	MaxResults int      `json:"maxResults,omitempty"`
	Fields     []string `json:"fields,omitempty"`
	Expand     []string `json:"expand,omitempty"`
	// validateQuery: "strict" (default), "warn", "none"
	ValidateQuery string `json:"validateQuery,omitempty"`
}

type jiraIssueCommentsInput struct {
	jiraBaseInput
	Issue      string `json:"issue"`
	StartAt    int    `json:"startAt,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
	OrderBy    string `json:"orderBy,omitempty"`
	Expand     string `json:"expand,omitempty"`
}

type jiraIssueTransitionsInput struct {
	jiraBaseInput
	Issue  string `json:"issue"`
	Expand string `json:"expand,omitempty"`
}

type jiraAddCommentInput struct {
	jiraBaseInput
	Issue  string `json:"issue"`
	Body   string `json:"body"`
	Format string `json:"format,omitempty"` // "text" (default) or "adf"
}

type jiraTransitionIssueInput struct {
	jiraBaseInput
	Issue        string         `json:"issue"`
	TransitionID string         `json:"transition_id"`
	Comment      string         `json:"comment,omitempty"`
	Fields       map[string]any `json:"fields,omitempty"`
	Update       map[string]any `json:"update,omitempty"`
}

type jiraCreateIssueInput struct {
	jiraBaseInput
	Fields map[string]any `json:"fields"`
	Update map[string]any `json:"update,omitempty"`
}

type jiraUpdateIssueInput struct {
	jiraBaseInput
	Issue  string         `json:"issue"`
	Fields map[string]any `json:"fields,omitempty"`
	Update map[string]any `json:"update,omitempty"`
}

type jiraAddAttachmentInput struct {
	jiraBaseInput
	Issue    string `json:"issue"`
	FilePath string `json:"file_path"`
}

type jiraListProjectsInput struct {
	jiraBaseInput
	StartAt    int    `json:"startAt,omitempty"`
	MaxResults int    `json:"maxResults,omitempty"`
	OrderBy    string `json:"orderBy,omitempty"`
	Query      string `json:"query,omitempty"`
}

// --- Tool handlers ---

func (h *Handler) jiraGetMyself(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraBaseInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/myself", nil, nil, nil)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
	}
	return jsonResult(map[string]any{
		"base_url":    cl.baseURL,
		"api_version": cl.apiVersion,
		"headers": map[string]any{
			"date":                  hdr.Get("Date"),
			"retry_after":           hdr.Get("Retry-After"),
			"x_ratelimit_limit":     hdr.Get("X-RateLimit-Limit"),
			"x_ratelimit_remaining": hdr.Get("X-RateLimit-Remaining"),
			"x_ratelimit_reset":     hdr.Get("X-RateLimit-Reset"),
			"ratelimit_reason":      hdr.Get("RateLimit-Reason"),
		},
		"myself": mustUnmarshalAny(body),
	}), nil
}

func (h *Handler) jiraGetIssue(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraGetIssueInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	q := url.Values{}
	if len(in.Fields) > 0 {
		q.Set("fields", strings.Join(in.Fields, ","))
	}
	if len(in.Expand) > 0 {
		q.Set("expand", strings.Join(in.Expand, ","))
	}
	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/issue/"+url.PathEscape(in.Issue), q, nil, nil)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}

func (h *Handler) jiraSearchIssues(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraSearchIssuesInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.JQL) == "" {
		return errorResult("jql is required"), nil
	}
	if in.MaxResults == 0 {
		in.MaxResults = 50
	}
	if in.MaxResults < 1 {
		return errorResult("maxResults must be positive"), nil
	}
	if in.StartAt < 0 {
		return errorResult("startAt must be >= 0"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("jql", in.JQL)
	q.Set("startAt", strconv.Itoa(in.StartAt))
	q.Set("maxResults", strconv.Itoa(in.MaxResults))
	if len(in.Fields) > 0 {
		q.Set("fields", strings.Join(in.Fields, ","))
	}
	if len(in.Expand) > 0 {
		q.Set("expand", strings.Join(in.Expand, ","))
	}
	if strings.TrimSpace(in.ValidateQuery) != "" {
		q.Set("validateQuery", strings.TrimSpace(in.ValidateQuery))
	}

	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/search", q, nil, nil)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}

func (h *Handler) jiraGetIssueComments(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraIssueCommentsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	if in.MaxResults == 0 {
		in.MaxResults = 50
	}
	if in.MaxResults < 1 {
		return errorResult("maxResults must be positive"), nil
	}
	if in.StartAt < 0 {
		return errorResult("startAt must be >= 0"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("startAt", strconv.Itoa(in.StartAt))
	q.Set("maxResults", strconv.Itoa(in.MaxResults))
	if strings.TrimSpace(in.OrderBy) != "" {
		q.Set("orderBy", strings.TrimSpace(in.OrderBy))
	}
	if strings.TrimSpace(in.Expand) != "" {
		q.Set("expand", strings.TrimSpace(in.Expand))
	}

	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/issue/"+url.PathEscape(in.Issue)+"/comment", q, nil, nil)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}

func (h *Handler) jiraGetIssueTransitions(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraIssueTransitionsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	if strings.TrimSpace(in.Expand) != "" {
		q.Set("expand", strings.TrimSpace(in.Expand))
	}

	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/issue/"+url.PathEscape(in.Issue)+"/transitions", q, nil, nil)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}

func jiraADFDocFromText(text string) map[string]any {
	return map[string]any{
		"type":    "doc",
		"version": 1,
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
			},
		},
	}
}

func (h *Handler) jiraAddComment(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraAddCommentInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	if strings.TrimSpace(in.Body) == "" {
		return errorResult("body is required"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	format := strings.ToLower(strings.TrimSpace(in.Format))
	if format == "" {
		format = "text"
	}

	var payload map[string]any
	if cl.apiVersion == 3 || format == "adf" {
		payload = map[string]any{"body": jiraADFDocFromText(in.Body)}
	} else {
		payload = map[string]any{"body": in.Body}
	}
	b, _ := json.Marshal(payload)

	status, hdr, respBody, err := cl.do(ctx, http.MethodPost, "/issue/"+url.PathEscape(in.Issue)+"/comment", nil, nil, b)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, respBody))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(respBody)), jiraAuthHint(status, respBody))), nil
	}
	return jsonResult(mustUnmarshalAny(respBody)), nil
}

func (h *Handler) jiraTransitionIssue(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraTransitionIssueInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	if strings.TrimSpace(in.TransitionID) == "" {
		return errorResult("transition_id is required"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	payload := map[string]any{
		"transition": map[string]any{"id": in.TransitionID},
	}
	if len(in.Fields) > 0 {
		payload["fields"] = in.Fields
	}
	if len(in.Update) > 0 {
		payload["update"] = in.Update
	}
	if strings.TrimSpace(in.Comment) != "" {
		// Jira transition supports adding a comment as an update.
		var commentBody any = in.Comment
		if cl.apiVersion == 3 {
			commentBody = jiraADFDocFromText(in.Comment)
		}
		payload["update"] = mergeUpdate(payload["update"], map[string]any{
			"comment": []any{map[string]any{"add": map[string]any{"body": commentBody}}},
		})
	}

	b, _ := json.Marshal(payload)

	status, hdr, respBody, err := cl.do(ctx, http.MethodPost, "/issue/"+url.PathEscape(in.Issue)+"/transitions", nil, nil, b)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, respBody))), nil
		}
		return errorResult(err.Error()), nil
	}
	// Jira often returns 204 No Content for successful transitions.
	if status == http.StatusNoContent {
		return jsonResult(map[string]any{"ok": true, "status": status}), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(respBody)), jiraAuthHint(status, respBody))), nil
	}
	return jsonResult(mustUnmarshalAny(respBody)), nil
}

func mergeUpdate(existing any, add map[string]any) map[string]any {
	out := map[string]any{}
	if m, ok := existing.(map[string]any); ok {
		for k, v := range m {
			out[k] = v
		}
	}
	for k, v := range add {
		out[k] = v
	}
	return out
}

func (h *Handler) jiraCreateIssue(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraCreateIssueInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if len(in.Fields) == 0 {
		return errorResult("fields is required"), nil
	}
	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	payload := map[string]any{"fields": in.Fields}
	if len(in.Update) > 0 {
		payload["update"] = in.Update
	}
	b, _ := json.Marshal(payload)

	status, hdr, respBody, err := cl.do(ctx, http.MethodPost, "/issue", nil, nil, b)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, respBody))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(respBody)), jiraAuthHint(status, respBody))), nil
	}
	return jsonResult(mustUnmarshalAny(respBody)), nil
}

func (h *Handler) jiraUpdateIssue(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraUpdateIssueInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	if len(in.Fields) == 0 && len(in.Update) == 0 {
		return errorResult("fields or update is required"), nil
	}
	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	payload := map[string]any{}
	if len(in.Fields) > 0 {
		payload["fields"] = in.Fields
	}
	if len(in.Update) > 0 {
		payload["update"] = in.Update
	}
	b, _ := json.Marshal(payload)

	status, hdr, respBody, err := cl.do(ctx, http.MethodPut, "/issue/"+url.PathEscape(in.Issue), nil, nil, b)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, respBody))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status == http.StatusNoContent {
		return jsonResult(map[string]any{"ok": true, "status": status}), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(respBody)), jiraAuthHint(status, respBody))), nil
	}
	return jsonResult(mustUnmarshalAny(respBody)), nil
}

func (h *Handler) jiraAddAttachment(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraAddAttachmentInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	if strings.TrimSpace(in.FilePath) == "" {
		return errorResult("file_path is required"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	f, err := os.Open(in.FilePath)
	if err != nil {
		return errorResult("failed to open file: " + err.Error()), nil
	}
	defer f.Close()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(in.FilePath))
	if err != nil {
		return errorResult("failed to create multipart: " + err.Error()), nil
	}
	if _, err := io.Copy(part, f); err != nil {
		return errorResult("failed to read file: " + err.Error()), nil
	}
	_ = w.Close()

	// Attachment upload requires multipart and the special CSRF header.
	u := cl.apiBase() + "/issue/" + url.PathEscape(in.Issue) + "/attachments"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, &buf)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	req.Header.Set("User-Agent", "mcp-lens")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-Atlassian-Token", "no-check")
	if cl.authHeader != "" {
		req.Header.Set("Authorization", cl.authHeader)
	}

	resp, err := cl.c.Do(req)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", resp.StatusCode, strings.TrimSpace(string(respBody)), jiraAuthHint(resp.StatusCode, respBody))), nil
	}
	return jsonResult(mustUnmarshalAny(respBody)), nil
}

func (h *Handler) jiraListProjects(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraListProjectsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Cloud v3: /project/search. v2/DC: /project (no pagination) or /project/search (varies).
	if cl.apiVersion == 3 {
		q := url.Values{}
		if in.StartAt < 0 {
			return errorResult("startAt must be >= 0"), nil
		}
		if in.MaxResults == 0 {
			in.MaxResults = 50
		}
		if in.MaxResults < 1 {
			return errorResult("maxResults must be positive"), nil
		}
		q.Set("startAt", strconv.Itoa(in.StartAt))
		q.Set("maxResults", strconv.Itoa(in.MaxResults))
		if strings.TrimSpace(in.OrderBy) != "" {
			q.Set("orderBy", strings.TrimSpace(in.OrderBy))
		}
		if strings.TrimSpace(in.Query) != "" {
			q.Set("query", strings.TrimSpace(in.Query))
		}

		status, hdr, body, err := cl.do(ctx, http.MethodGet, "/project/search", q, nil, nil)
		if err != nil {
			if errors.Is(err, errJiraHTMLOrRedirect) {
				return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
			}
			return errorResult(err.Error()), nil
		}
		if status < 200 || status >= 300 {
			return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
		}
		return jsonResult(mustUnmarshalAny(body)), nil
	}

	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/project", nil, nil, nil)
	if err != nil {
		if errors.Is(err, errJiraHTMLOrRedirect) {
			return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
		}
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}

func mustUnmarshalAny(b []byte) any {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return map[string]any{"raw": string(b)}
	}
	return v
}

// Optional helper for callers that want to implement backoff on 429.
func parseRetryAfterSeconds(h http.Header) (time.Duration, bool) {
	ra := strings.TrimSpace(h.Get("Retry-After"))
	if ra == "" {
		return 0, false
	}
	sec, err := strconv.Atoi(ra)
	if err != nil || sec <= 0 {
		return 0, false
	}
	return time.Duration(sec) * time.Second, true
}
