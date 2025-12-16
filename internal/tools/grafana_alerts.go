package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type grafanaListAlertsInput struct {
	grafanaBaseInput
	TimeoutMS   int      `json:"timeout_ms,omitempty"`
	UserAgent   string   `json:"user_agent,omitempty"`
	State       []string `json:"state,omitempty"`        // ok|alerting|no_data|paused|pending|ALL
	Query       string   `json:"query,omitempty"`        // name like
	DashboardID []int    `json:"dashboard_id,omitempty"` // filter by dashboard id(s)
	FolderID    []int    `json:"folder_id,omitempty"`    // filter by folder id(s)
	Limit       int      `json:"limit,omitempty"`
}

type grafanaGetAlertInput struct {
	grafanaBaseInput
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	ID        int    `json:"id"`
}

type grafanaListAlertRulesInput struct {
	grafanaBaseInput
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

type grafanaGetAlertRuleInput struct {
	grafanaBaseInput
	TimeoutMS int    `json:"timeout_ms,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	UID       string `json:"uid"`
}

func (h *Handler) grafanaListAlerts(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaListAlertsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, true, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	for _, s := range in.State {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		q.Add("state", s)
	}
	if strings.TrimSpace(in.Query) != "" {
		q.Set("query", strings.TrimSpace(in.Query))
	}
	for _, id := range in.DashboardID {
		if id > 0 {
			q.Add("dashboardId", strconv.Itoa(id))
		}
	}
	for _, id := range in.FolderID {
		if id > 0 {
			q.Add("folderId", strconv.Itoa(id))
		}
	}
	if in.Limit > 0 {
		q.Set("limit", strconv.Itoa(in.Limit))
	}

	status, hdr, body, err := cl.do(ctx, http.MethodGet, "/api/alerts", q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}
	return jsonResult(map[string]any{
		"base_url": cl.baseURL,
		"org_id":   cl.orgID,
		"headers": map[string]any{
			"date":         hdr.Get("Date"),
			"content_type": hdr.Get("Content-Type"),
		},
		"alerts": mustUnmarshalAny(body),
	}), nil
}

func (h *Handler) grafanaGetAlert(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaGetAlertInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.ID <= 0 {
		return errorResult("id must be > 0"), nil
	}
	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, true, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/alerts/"+strconv.Itoa(in.ID), nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}

func (h *Handler) grafanaListAlertRules(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaListAlertRulesInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, true, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Prefer provisioning API when available.
	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/v1/provisioning/alert-rules", nil, nil)
	if err == nil && status >= 200 && status < 300 {
		return jsonResult(map[string]any{
			"api":       "provisioning",
			"base_url":  cl.baseURL,
			"org_id":    cl.orgID,
			"rules":     mustUnmarshalAny(body),
			"has_next":  false,
			"next_page": nil,
		}), nil
	}

	// Fallback: ruler API (namespace list), often used for unified alerting.
	status, _, body, err = cl.do(ctx, http.MethodGet, "/api/ruler/grafana/api/v1/rules", nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}
	return jsonResult(map[string]any{
		"api":      "ruler",
		"base_url": cl.baseURL,
		"org_id":   cl.orgID,
		"rules":    mustUnmarshalAny(body),
	}), nil
}

func (h *Handler) grafanaGetAlertRule(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in grafanaGetAlertRuleInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.UID) == "" {
		return errorResult("uid is required"), nil
	}
	cl, err := newGrafanaClient(in.Client, in.Base, in.OrgID, true, in.TimeoutMS, in.UserAgent, in.CFID, in.CFSecret)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	status, _, body, err := cl.do(ctx, http.MethodGet, "/api/v1/provisioning/alert-rules/"+url.PathEscape(strings.TrimSpace(in.UID)), nil, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status == http.StatusNotFound {
		return errorResult("alert rule not found via provisioning API (Grafana may be using a different alerting API; try grafana_list_alert_rules)"), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Grafana API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}
	return jsonResult(mustUnmarshalAny(body)), nil
}
