package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type githubListWorkflowRunsInput struct {
	Repo    string `json:"repo"`
	Branch  string `json:"branch,omitempty"`
	Event   string `json:"event,omitempty"`
	Status  string `json:"status,omitempty"`
	HeadSHA string `json:"head_sha,omitempty"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

type githubListWorkflowJobsInput struct {
	Repo    string `json:"repo"`
	RunID   int64  `json:"run_id"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

type githubDownloadJobLogsInput struct {
	Repo  string `json:"repo"`
	JobID int64  `json:"job_id"`
}

type workflowRunSummary struct {
	ID         int64  `json:"id"`
	Name       string `json:"name,omitempty"`
	Event      string `json:"event,omitempty"`
	Status     string `json:"status,omitempty"`
	Conclusion string `json:"conclusion,omitempty"`
	HeadSHA    string `json:"head_sha,omitempty"`
	HeadBranch string `json:"head_branch,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	HTMLURL    string `json:"html_url,omitempty"`
}

type workflowJobSummary struct {
	ID          int64  `json:"id"`
	Name        string `json:"name,omitempty"`
	Status      string `json:"status,omitempty"`
	Conclusion  string `json:"conclusion,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
	RunnerName  string `json:"runner_name,omitempty"`
	HTMLURL     string `json:"html_url,omitempty"`
}

func (h *Handler) githubListWorkflowRuns(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in githubListWorkflowRunsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Repo) == "" {
		return errorResult("repo is required"), nil
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 20
	}
	if in.PerPage > 100 {
		in.PerPage = 100
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(in.Page))
	q.Set("per_page", strconv.Itoa(in.PerPage))
	if v := strings.TrimSpace(in.Branch); v != "" {
		q.Set("branch", v)
	}
	if v := strings.TrimSpace(in.Event); v != "" {
		q.Set("event", v)
	}
	if v := strings.TrimSpace(in.Status); v != "" {
		q.Set("status", v)
	}
	if v := strings.TrimSpace(in.HeadSHA); v != "" {
		q.Set("head_sha", v)
	}

	gh := newGitHubClient()
	status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/actions/runs", owner, repo), q, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status != http.StatusOK {
		hint := githubAuthHint(status)
		if hint != "" {
			return errorResult(fmt.Sprintf("GitHub API error: %d. %s", status, hint)), nil
		}
		return errorResult(fmt.Sprintf("GitHub API error: %d", status)), nil
	}

	var raw struct {
		TotalCount   int                  `json:"total_count"`
		WorkflowRuns []workflowRunSummary `json:"workflow_runs"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return errorResult("Failed to parse response: " + err.Error()), nil
	}

	nextPage, hasNext := parseNextPage(headers.Get("Link"))
	out := map[string]any{
		"total_count":   raw.TotalCount,
		"workflow_runs": raw.WorkflowRuns,
		"has_next":      hasNext,
	}
	if hasNext {
		out["next_page"] = nextPage
	}
	return jsonResult(out), nil
}

func (h *Handler) githubListWorkflowJobs(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in githubListWorkflowJobsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Repo) == "" {
		return errorResult("repo is required"), nil
	}
	if in.RunID <= 0 {
		return errorResult("run_id must be > 0"), nil
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 50
	}
	if in.PerPage > 100 {
		in.PerPage = 100
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("page", strconv.Itoa(in.Page))
	q.Set("per_page", strconv.Itoa(in.PerPage))

	gh := newGitHubClient()
	status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs", owner, repo, in.RunID), q, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status != http.StatusOK {
		hint := githubAuthHint(status)
		if hint != "" {
			return errorResult(fmt.Sprintf("GitHub API error: %d. %s", status, hint)), nil
		}
		return errorResult(fmt.Sprintf("GitHub API error: %d", status)), nil
	}

	var raw struct {
		TotalCount int                  `json:"total_count"`
		Jobs       []workflowJobSummary `json:"jobs"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return errorResult("Failed to parse response: " + err.Error()), nil
	}

	nextPage, hasNext := parseNextPage(headers.Get("Link"))
	out := map[string]any{
		"total_count": raw.TotalCount,
		"jobs":        raw.Jobs,
		"has_next":    hasNext,
	}
	if hasNext {
		out["next_page"] = nextPage
	}
	return jsonResult(out), nil
}

func (h *Handler) githubDownloadJobLogs(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in githubDownloadJobLogsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Repo) == "" {
		return errorResult("repo is required"), nil
	}
	if in.JobID <= 0 {
		return errorResult("job_id must be > 0"), nil
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()
	status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/actions/jobs/%d/logs", owner, repo, in.JobID), nil, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}

	var logBytes []byte
	mime := "text/plain"

	switch status {
	case http.StatusFound, http.StatusMovedPermanently, http.StatusTemporaryRedirect, http.StatusPermanentRedirect:
		loc := strings.TrimSpace(headers.Get("Location"))
		if loc == "" {
			return errorResult("GitHub API returned redirect without Location header"), nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc, nil)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		req.Header.Set("User-Agent", "mcp-lens")
		resp, err := gh.c.Do(req)
		if err != nil {
			return errorResult(err.Error()), nil
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return errorResult(fmt.Sprintf("failed to download logs: status %d", resp.StatusCode)), nil
		}
		if ct := strings.TrimSpace(resp.Header.Get("Content-Type")); ct != "" {
			mime = strings.Split(ct, ";")[0]
		}
		logBytes, err = ioReadAllLimit(resp.Body, 10*1024*1024) // 10MB safety limit
		if err != nil {
			return errorResult(err.Error()), nil
		}
	case http.StatusOK:
		// Some GitHub Enterprise installs may return logs directly.
		logBytes = body
	default:
		hint := githubAuthHint(status)
		if hint != "" {
			return errorResult(fmt.Sprintf("GitHub API error: %d. %s", status, hint)), nil
		}
		return errorResult(fmt.Sprintf("GitHub API error: %d", status)), nil
	}

	if h.artifacts == nil {
		return errorResult("artifact store is not configured"), nil
	}
	repl, item, err := h.artifacts.StoreBytes("github_download_job_logs", args, mime, "log", logBytes)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	out := map[string]any{
		"job_id":   in.JobID,
		"artifact": repl,
		"bytes":    item.Bytes,
		"mime":     item.Mime,
		"sha256":   item.SHA256,
	}
	return jsonResult(out), nil
}

func ioReadAllLimit(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = 1
	}
	lr := &io.LimitedReader{R: r, N: limit}
	b, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if lr.N <= 0 {
		return nil, fmt.Errorf("response too large (limit %d bytes)", limit)
	}
	return b, nil
}
