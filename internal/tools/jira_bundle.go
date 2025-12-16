package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type jiraGetIssueBundleInput struct {
	jiraBaseInput
	Issue string `json:"issue"`

	// Issue payload tuning
	Fields           []string `json:"fields,omitempty"`
	Expand           []string `json:"expand,omitempty"`
	IncludeChangelog bool     `json:"include_changelog,omitempty"`

	// Comments payload tuning
	IncludeComments bool   `json:"include_comments,omitempty"`
	CommentsStartAt int    `json:"comments_startAt,omitempty"`
	CommentsMax     int    `json:"comments_maxResults,omitempty"`
	CommentsExpand  string `json:"comments_expand,omitempty"` // e.g. "renderedBody"
}

func (h *Handler) jiraGetIssueBundle(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraGetIssueBundleInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Issue) == "" {
		return errorResult("issue is required"), nil
	}
	if in.CommentsMax == 0 {
		in.CommentsMax = 50
	}
	if in.CommentsMax < 0 {
		return errorResult("comments_maxResults must be >= 0"), nil
	}
	if in.CommentsStartAt < 0 {
		return errorResult("comments_startAt must be >= 0"), nil
	}

	cl, err := newJiraClient(in.Client, in.BaseURL, in.APIVersion)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Issue
	issueExpand := make([]string, 0, len(in.Expand)+1)
	issueExpand = append(issueExpand, in.Expand...)
	if in.IncludeChangelog {
		issueExpand = append(issueExpand, "changelog")
	}
	q := url.Values{}
	if len(in.Fields) > 0 {
		q.Set("fields", strings.Join(in.Fields, ","))
	}
	if len(issueExpand) > 0 {
		q.Set("expand", strings.Join(dedupeStrings(issueExpand), ","))
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

	out := map[string]any{
		"base_url":    cl.baseURL,
		"api_version": cl.apiVersion,
		"issue":       mustUnmarshalAny(body),
	}

	// Comments
	if in.IncludeComments {
		qc := url.Values{}
		qc.Set("startAt", strconv.Itoa(in.CommentsStartAt))
		qc.Set("maxResults", strconv.Itoa(in.CommentsMax))
		if strings.TrimSpace(in.CommentsExpand) != "" {
			qc.Set("expand", strings.TrimSpace(in.CommentsExpand))
		}

		status, hdr, body, err := cl.do(ctx, http.MethodGet, "/issue/"+url.PathEscape(in.Issue)+"/comment", qc, nil, nil)
		if err != nil {
			if errors.Is(err, errJiraHTMLOrRedirect) {
				return errorResult(fmt.Sprintf("Jira API returned HTML/redirect (likely login). status=%d location=%s\n%s", status, hdr.Get("Location"), jiraAuthHint(status, body))), nil
			}
			return errorResult(err.Error()), nil
		}
		if status < 200 || status >= 300 {
			return errorResult(fmt.Sprintf("Jira API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), jiraAuthHint(status, body))), nil
		}
		commentsAny := mustUnmarshalAny(body)
		out["comments"] = commentsAny

		// best-effort pagination hints
		if m, ok := commentsAny.(map[string]any); ok {
			startAt, _ := m["startAt"].(float64)
			maxResults, _ := m["maxResults"].(float64)
			total, _ := m["total"].(float64)
			next := int(startAt) + int(maxResults)
			if int(maxResults) > 0 && next < int(total) {
				out["comments_has_next"] = true
				out["comments_next_startAt"] = next
			} else {
				out["comments_has_next"] = false
			}
		}
	}

	return jsonResult(out), nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
