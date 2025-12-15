package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type githubClient struct {
	token string
	c     *http.Client

	// Rate limit tracking
	mu             sync.RWMutex
	rateLimitRemaining int
	rateLimitReset     time.Time
}

var (
	ghClientOnce sync.Once
	ghClient     *githubClient
)

func newGitHubClient() *githubClient {
	ghClientOnce.Do(func() {
		// Prefer the commonly used env var, but also support the upstream var name.
		tok := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		if tok == "" {
			tok = strings.TrimSpace(os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN"))
		}

		ghClient = &githubClient{
			token:              tok,
			c:                  http.DefaultClient,
			rateLimitRemaining: -1, // Unknown
		}
	})
	return ghClient
}

// updateRateLimit updates rate limit info from response headers
func (g *githubClient) updateRateLimit(headers http.Header) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		if n, err := strconv.Atoi(remaining); err == nil {
			g.rateLimitRemaining = n
		}
	}
	if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
		if ts, err := strconv.ParseInt(reset, 10, 64); err == nil {
			g.rateLimitReset = time.Unix(ts, 0)
		}
	}
}

// GetRateLimitInfo returns current rate limit status
func (g *githubClient) GetRateLimitInfo() (remaining int, resetAt time.Time) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.rateLimitRemaining, g.rateLimitReset
}

func splitRepo(repo string) (owner string, name string, err error) {
	repo = strings.TrimSpace(repo)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repo: expected 'owner/repo'")
	}
	return parts[0], parts[1], nil
}

func (g *githubClient) do(ctx context.Context, method string, apiPath string, query url.Values, accept string) (int, http.Header, []byte, error) {
	base := "https://api.github.com"
	u := base + apiPath
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("User-Agent", "mcp-lens")
	if accept != "" {
		req.Header.Set("Accept", accept)
	} else {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	// Best-effort: if token is set, use it.
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	// Use the versioned header to keep behavior stable.
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := g.c.Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()

	// Update rate limit info from response
	g.updateRateLimit(resp.Header)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, resp.Header, nil, err
	}
	return resp.StatusCode, resp.Header, body, nil
}

func githubAuthHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "GitHub API returned 401. Likely missing/invalid token. Set GITHUB_TOKEN (classic PAT or fine-grained token with repo read access)."
	case http.StatusForbidden:
		return "GitHub API returned 403. Likely insufficient scopes, SSO requirement, or rate limit. Ensure token has required permissions (repo / pull requests read)."
	case http.StatusNotFound:
		return "GitHub API returned 404. This often means the repo/PR is private or you don't have access (GitHub hides private resources behind 404). Check token permissions and repo visibility."
	default:
		return ""
	}
}

func parseNextPage(linkHeader string) (int, bool) {
	// Link: <https://api.github.com/...&page=2>; rel="next", <...>; rel="last"
	for _, part := range strings.Split(linkHeader, ",") {
		p := strings.TrimSpace(part)
		if !strings.Contains(p, "rel=\"next\"") {
			continue
		}
		start := strings.Index(p, "<")
		end := strings.Index(p, ">")
		if start < 0 || end < 0 || end <= start+1 {
			continue
		}
		u := p[start+1 : end]
		parsed, err := url.Parse(u)
		if err != nil {
			continue
		}
		pageStr := parsed.Query().Get("page")
		if pageStr == "" {
			continue
		}
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			continue
		}
		return page, true
	}
	return 0, false
}

// --- Local tools ---

type prRefInput struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

type prFilesInput struct {
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

type prDiffInput struct {
	Repo       string   `json:"repo"`
	Number     int      `json:"number"`
	Offset     int      `json:"offset,omitempty"`
	MaxBytes   int      `json:"max_bytes,omitempty"`
	FileFilter []string `json:"file_filter,omitempty"` // Filter diff to specific file paths (glob patterns supported)
}

type prSummaryInput struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

type prFileDiffInput struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Path   string `json:"path"` // Specific file path to get diff for
}

type fileAtRefInput struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref"`
	Path string `json:"path"`
}

type prCommitsInput struct {
	Repo    string `json:"repo"`
	Number  int    `json:"number"`
	Page    int    `json:"page,omitempty"`
	PerPage int    `json:"per_page,omitempty"`
}

type prChecksInput struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
}

type prReviewBundleInput struct {
	Repo         string `json:"repo"`
	Number       int    `json:"number"`
	FilesPage    int    `json:"files_page,omitempty"`
	FilesPerPage int    `json:"files_per_page,omitempty"`
	IncludeDiff  bool   `json:"include_diff,omitempty"`
	DiffOffset   int    `json:"diff_offset,omitempty"`
	MaxDiffBytes int    `json:"max_diff_bytes,omitempty"`
	IncludeCommits bool `json:"include_commits,omitempty"`
	CommitsPage  int    `json:"commits_page,omitempty"`
	CommitsPerPage int `json:"commits_per_page,omitempty"`
	IncludeChecks bool `json:"include_checks,omitempty"`
}

type fetchCompletePRDiffInput struct {
	Repo       string   `json:"repo"`
	Number     int      `json:"number"`
	FileFilter []string `json:"file_filter,omitempty"`
	OutputDir  string   `json:"output_dir,omitempty"`
}

type fetchCompletePRFilesInput struct {
	Repo      string `json:"repo"`
	Number    int    `json:"number"`
	OutputDir string `json:"output_dir,omitempty"`
}

func (h *Handler) getPullRequestDetails(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prRefInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}
	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, in.Number), nil, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	// Extract a stable subset with compact nested objects.
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	out := map[string]any{
		"repo":       in.Repo,
		"number":     in.Number,
		"title":      raw["title"],
		"state":      raw["state"],
		"draft":      raw["draft"],
		"html_url":   raw["html_url"],
		"user":       compactUser(raw["user"]),
		"base":       compactRef(raw["base"]),
		"head":       compactRef(raw["head"]),
		"merged":     raw["merged"],
		"mergeable":  raw["mergeable"],
		"rebaseable": raw["rebaseable"],
		"created_at": raw["created_at"],
		"updated_at": raw["updated_at"],
	}
	return jsonResult(out), nil
}

// compactUser extracts only essential user fields
func compactUser(u any) map[string]any {
	user, ok := u.(map[string]any)
	if !ok {
		return nil
	}
	return map[string]any{
		"login":      user["login"],
		"id":         user["id"],
		"avatar_url": user["avatar_url"],
		"html_url":   user["html_url"],
	}
}

// compactRef extracts only essential ref fields (base/head)
func compactRef(r any) map[string]any {
	ref, ok := r.(map[string]any)
	if !ok {
		return nil
	}
	result := map[string]any{
		"ref": ref["ref"],
		"sha": ref["sha"],
	}
	// Include compact repo info
	if repo, ok := ref["repo"].(map[string]any); ok {
		result["repo"] = map[string]any{
			"full_name": repo["full_name"],
			"html_url":  repo["html_url"],
		}
	}
	// Include compact user info
	if user, ok := ref["user"].(map[string]any); ok {
		result["user"] = compactUser(user)
	}
	return result
}

func (h *Handler) listPullRequestFiles(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prFilesInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 30
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
	status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, in.Number), q, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var files []map[string]any
	_ = json.Unmarshal(body, &files)

	// Compact files to essential fields only
	compactFiles := make([]map[string]any, 0, len(files))
	for _, f := range files {
		compactFiles = append(compactFiles, map[string]any{
			"filename":    f["filename"],
			"status":      f["status"],
			"additions":   f["additions"],
			"deletions":   f["deletions"],
			"changes":     f["changes"],
			"patch":       f["patch"], // Keep patch for review context
			"raw_url":     f["raw_url"],
			"blob_url":    f["blob_url"],
			"contents_url": f["contents_url"],
		})
	}

	nextPage, hasNext := parseNextPage(headers.Get("Link"))
	out := map[string]any{
		"repo":       in.Repo,
		"number":     in.Number,
		"page":       in.Page,
		"per_page":   in.PerPage,
		"has_next":   hasNext,
		"next_page":  nextPage,
		"file_count": len(compactFiles),
		"files":      compactFiles,
	}
	return jsonResult(out), nil
}

func (h *Handler) getPullRequestDiff(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prDiffInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}
	if in.Offset < 0 {
		return errorResult("offset must be >= 0"), nil
	}
	if in.MaxBytes <= 0 {
		in.MaxBytes = 16_000 // ~4000 tokens (1 token ≈ 4 chars). Safe default to avoid huge responses.
	}
	// Hard cap to prevent accidental huge responses
	if in.MaxBytes > 64_000 {
		in.MaxBytes = 64_000 // Max ~16k tokens per chunk
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, in.Number), nil, "application/vnd.github.v3.diff")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	// Apply file filtering if specified
	diffContent := string(body)
	if len(in.FileFilter) > 0 {
		diffContent = filterDiffByPatterns(diffContent, in.FileFilter)
		body = []byte(diffContent)
	}

	// Apply offset/max_bytes chunking.
	if in.Offset > len(body) {
		in.Offset = len(body)
	}

	end := in.Offset + in.MaxBytes
	if end > len(body) {
		end = len(body)
	}

	chunk := body[in.Offset:end]
	hasNext := end < len(body)

	// Estimate tokens: ~4 chars per token for English/code text
	estimatedTokens := len(chunk) / 4
	totalEstimatedTokens := len(body) / 4

	// Calculate pagination info
	totalParts := (len(body) + in.MaxBytes - 1) / in.MaxBytes
	currentPart := (in.Offset / in.MaxBytes) + 1

	out := map[string]any{
		"repo":      in.Repo,
		"number":    in.Number,
		"offset":    in.Offset,
		"max_bytes": in.MaxBytes,
		"chunk_len": len(chunk),
		"has_next":  hasNext,
		"next_offset": func() any {
			if !hasNext {
				return nil
			}
			return end
		}(),
		"diff_chunk": string(chunk),
		"total_len":  len(body),
		"format":     "unified",
		"unit":       "bytes",
		// Token estimates for LLM context awareness
		"estimated_tokens":       estimatedTokens,
		"total_estimated_tokens": totalEstimatedTokens,
		// Pagination info
		"pagination": map[string]any{
			"current_part": currentPart,
			"total_parts":  totalParts,
			"hint":         fmt.Sprintf("Part %d of %d. Use offset=%d to get next part.", currentPart, totalParts, end),
		},
	}

	// Include filter info if applied
	if len(in.FileFilter) > 0 {
		out["file_filter"] = in.FileFilter
		out["filtered"] = true
	}

	return jsonResult(out), nil
}

func (h *Handler) getFileAtRef(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in fileAtRefInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Ref == "" || in.Path == "" {
		return errorResult("repo, ref and path are required"), nil
	}
	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("ref", in.Ref)

	// Path must be escaped per-segment, but must preserve slashes.
	escapedPath := strings.Join(func() []string {
		segs := strings.Split(in.Path, "/")
		out := make([]string, 0, len(segs))
		for _, s := range segs {
			out = append(out, url.PathEscape(s))
		}
		return out
	}(), "/")

	gh := newGitHubClient()
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/contents/%s", owner, repo, escapedPath), q, "application/vnd.github.v3.raw")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	out := map[string]any{
		"repo": in.Repo,
		"ref":  in.Ref,
		"path": in.Path,
		"raw":  string(body),
	}
	return jsonResult(out), nil
}

func (h *Handler) listPullRequestCommits(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prCommitsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}
	if in.Page <= 0 {
		in.Page = 1
	}
	if in.PerPage <= 0 {
		in.PerPage = 30
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
	status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d/commits", owner, repo, in.Number), q, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var commits []map[string]any
	_ = json.Unmarshal(body, &commits)

	// Compact commits to essential fields only
	compactCommits := make([]map[string]any, 0, len(commits))
	for _, c := range commits {
		cc := map[string]any{
			"sha":      c["sha"],
			"html_url": c["html_url"],
		}
		// Extract commit message and author info
		if commit, ok := c["commit"].(map[string]any); ok {
			cc["message"] = commit["message"]
			if author, ok := commit["author"].(map[string]any); ok {
				cc["author"] = map[string]any{
					"name":  author["name"],
					"email": author["email"],
					"date":  author["date"],
				}
			}
		}
		// Include compact author user info if available
		if author, ok := c["author"].(map[string]any); ok {
			cc["author_user"] = compactUser(author)
		}
		compactCommits = append(compactCommits, cc)
	}

	nextPage, hasNext := parseNextPage(headers.Get("Link"))
	out := map[string]any{
		"repo":         in.Repo,
		"number":       in.Number,
		"page":         in.Page,
		"per_page":     in.PerPage,
		"has_next":     hasNext,
		"next_page":    nextPage,
		"commit_count": len(compactCommits),
		"commits":      compactCommits,
	}
	return jsonResult(out), nil
}

func (h *Handler) getPullRequestChecks(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prChecksInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}
	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	// Need head SHA; reuse PR details.
	details, _ := h.getPullRequestDetails(ctx, mustMarshal(prRefInput{Repo: in.Repo, Number: in.Number}))
	d := extractJSON(details)
	m, ok := d.(map[string]any)
	if !ok {
		return errorResult("failed to parse PR details for checks"), nil
	}
	head, ok := m["head"].(map[string]any)
	if !ok {
		return errorResult("failed to read head from PR details"), nil
	}
	sha, _ := head["sha"].(string)
	if strings.TrimSpace(sha) == "" {
		return errorResult("failed to read head.sha from PR details"), nil
	}

	q := url.Values{}
	q.Set("ref", sha)

	gh := newGitHubClient()
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs", owner, repo, sha), q, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var checks map[string]any
	_ = json.Unmarshal(body, &checks)

	// Compact check runs to essential fields only
	compactChecks := map[string]any{
		"total_count": checks["total_count"],
	}
	if checkRuns, ok := checks["check_runs"].([]any); ok {
		compactRuns := make([]map[string]any, 0, len(checkRuns))
		for _, cr := range checkRuns {
			if run, ok := cr.(map[string]any); ok {
				compactRun := map[string]any{
					"id":           run["id"],
					"name":         run["name"],
					"status":       run["status"],
					"conclusion":   run["conclusion"],
					"started_at":   run["started_at"],
					"completed_at": run["completed_at"],
					"html_url":     run["html_url"],
				}
				// Include app name only
				if app, ok := run["app"].(map[string]any); ok {
					compactRun["app_name"] = app["name"]
				}
				compactRuns = append(compactRuns, compactRun)
			}
		}
		compactChecks["check_runs"] = compactRuns
	}

	out := map[string]any{
		"repo":     in.Repo,
		"number":   in.Number,
		"head_sha": sha,
		"checks":   compactChecks,
	}
	return jsonResult(out), nil
}

func (h *Handler) preparePullRequestReviewBundle(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prReviewBundleInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}
	if in.FilesPage <= 0 {
		in.FilesPage = 1
	}
	if in.FilesPerPage <= 0 {
		in.FilesPerPage = 30
	}
	if in.FilesPerPage > 100 {
		in.FilesPerPage = 100
	}
	if in.DiffOffset < 0 {
		return errorResult("diff_offset must be >= 0"), nil
	}
	if in.MaxDiffBytes <= 0 {
		in.MaxDiffBytes = 16_000 // ~4000 tokens (1 token ≈ 4 chars). Safe default.
	}
	if in.MaxDiffBytes > 64_000 {
		in.MaxDiffBytes = 64_000 // Hard cap at ~16k tokens
	}
	if in.IncludeCommits {
		if in.CommitsPage <= 0 {
			in.CommitsPage = 1
		}
		if in.CommitsPerPage <= 0 {
			in.CommitsPerPage = 30
		}
		if in.CommitsPerPage > 100 {
			in.CommitsPerPage = 100
		}
	}

	// Compose by calling our own local tool handlers with proper error tracking.
	var errors []string

	details, detailsErr := h.getPullRequestDetails(ctx, mustMarshal(prRefInput{Repo: in.Repo, Number: in.Number}))
	if detailsErr != nil || (details != nil && details.IsError) {
		errors = append(errors, "failed to fetch PR details")
	}

	files, filesErr := h.listPullRequestFiles(ctx, mustMarshal(prFilesInput{Repo: in.Repo, Number: in.Number, Page: in.FilesPage, PerPage: in.FilesPerPage}))
	if filesErr != nil || (files != nil && files.IsError) {
		errors = append(errors, "failed to fetch PR files")
	}

	bundle := map[string]any{
		"repo":    in.Repo,
		"number":  in.Number,
		"details": extractJSON(details),
		"files":   extractJSON(files),
	}

	if in.IncludeDiff {
		d, dErr := h.getPullRequestDiff(ctx, mustMarshal(prDiffInput{Repo: in.Repo, Number: in.Number, Offset: in.DiffOffset, MaxBytes: in.MaxDiffBytes}))
		if dErr != nil || (d != nil && d.IsError) {
			errors = append(errors, "failed to fetch PR diff")
		}
		bundle["diff"] = extractJSON(d)
	}

	if in.IncludeCommits {
		c, cErr := h.listPullRequestCommits(ctx, mustMarshal(prCommitsInput{Repo: in.Repo, Number: in.Number, Page: in.CommitsPage, PerPage: in.CommitsPerPage}))
		if cErr != nil || (c != nil && c.IsError) {
			errors = append(errors, "failed to fetch PR commits")
		}
		bundle["commits"] = extractJSON(c)
	}

	if in.IncludeChecks {
		ch, chErr := h.getPullRequestChecks(ctx, mustMarshal(prChecksInput{Repo: in.Repo, Number: in.Number}))
		if chErr != nil || (ch != nil && ch.IsError) {
			errors = append(errors, "failed to fetch PR checks")
		}
		bundle["checks"] = extractJSON(ch)
	}

	// Include errors in response if any occurred
	if len(errors) > 0 {
		bundle["errors"] = errors
		bundle["partial"] = true
	}

	return jsonResult(bundle), nil
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func extractJSON(res *mcp.CallToolResult) any {
	if res == nil || len(res.Content) == 0 {
		return nil
	}
	var v any
	_ = json.Unmarshal([]byte(res.Content[0].Text), &v)
	return v
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errorResult(err.Error())
	}
	return textResult(string(b))
}

// getPullRequestSummary returns a compact summary of PR changes without full diff
func (h *Handler) getPullRequestSummary(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prSummaryInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()

	// Get PR details
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, in.Number), nil, "application/vnd.github+json")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	var prData map[string]any
	_ = json.Unmarshal(body, &prData)

	// Get all files to compute summary (paginate through all)
	var allFiles []map[string]any
	page := 1
	for {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", "100")

		status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, in.Number), q, "application/vnd.github+json")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		if status < 200 || status >= 300 {
			hint := githubAuthHint(status)
			return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
		}

		var files []map[string]any
		_ = json.Unmarshal(body, &files)

		if len(files) == 0 {
			break
		}

		allFiles = append(allFiles, files...)

		_, hasNext := parseNextPage(headers.Get("Link"))
		if !hasNext {
			break
		}
		page++
	}

	// Compute summary statistics
	totalAdditions := 0
	totalDeletions := 0
	totalChanges := 0
	directories := make(map[string]int)
	fileTypes := make(map[string]int)
	filesByStatus := make(map[string][]string)

	for _, f := range allFiles {
		filename, _ := f["filename"].(string)
		additions, _ := f["additions"].(float64)
		deletions, _ := f["deletions"].(float64)
		changes, _ := f["changes"].(float64)
		fileStatus, _ := f["status"].(string)

		totalAdditions += int(additions)
		totalDeletions += int(deletions)
		totalChanges += int(changes)

		// Extract directory
		dir := filepath.Dir(filename)
		if dir == "." {
			dir = "/"
		}
		directories[dir]++

		// Extract file extension
		ext := filepath.Ext(filename)
		if ext == "" {
			ext = "(no extension)"
		}
		fileTypes[ext]++

		// Group by status
		filesByStatus[fileStatus] = append(filesByStatus[fileStatus], filename)
	}

	// Build summary response
	summary := map[string]any{
		"repo":   in.Repo,
		"number": in.Number,
		"title":  prData["title"],
		"state":  prData["state"],
		"author": func() string {
			if user, ok := prData["user"].(map[string]any); ok {
				if login, ok := user["login"].(string); ok {
					return login
				}
			}
			return ""
		}(),
		"statistics": map[string]any{
			"total_files":     len(allFiles),
			"total_additions": totalAdditions,
			"total_deletions": totalDeletions,
			"total_changes":   totalChanges,
		},
		"files_by_status": filesByStatus,
		"directories":     directories,
		"file_types":      fileTypes,
		"files": func() []map[string]any {
			// Return compact file list
			compact := make([]map[string]any, 0, len(allFiles))
			for _, f := range allFiles {
				compact = append(compact, map[string]any{
					"filename":  f["filename"],
					"status":    f["status"],
					"additions": f["additions"],
					"deletions": f["deletions"],
				})
			}
			return compact
		}(),
	}

	// Add rate limit info
	remaining, resetAt := gh.GetRateLimitInfo()
	if remaining >= 0 {
		summary["rate_limit"] = map[string]any{
			"remaining": remaining,
			"reset_at":  resetAt.Format(time.RFC3339),
		}
	}

	return jsonResult(summary), nil
}

// getPullRequestFileDiff returns diff for a specific file in a PR
func (h *Handler) getPullRequestFileDiff(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in prFileDiffInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 || in.Path == "" {
		return errorResult("repo, number, and path are required"), nil
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()

	// Get full diff
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, in.Number), nil, "application/vnd.github.v3.diff")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	// Parse and filter diff to specific file
	fileDiff := filterDiffByFile(string(body), in.Path)

	if fileDiff == "" {
		return errorResult(fmt.Sprintf("File '%s' not found in PR diff", in.Path)), nil
	}

	out := map[string]any{
		"repo":   in.Repo,
		"number": in.Number,
		"path":   in.Path,
		"diff":   fileDiff,
		"format": "unified",
	}

	return jsonResult(out), nil
}

// filterDiffByFile extracts diff for a specific file from unified diff output
func filterDiffByFile(fullDiff string, targetPath string) string {
	lines := strings.Split(fullDiff, "\n")
	var result strings.Builder
	capturing := false
	targetPathNormalized := strings.TrimPrefix(targetPath, "/")

	// Regex to match diff file headers
	diffHeaderRegex := regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// Check for diff header
		if matches := diffHeaderRegex.FindStringSubmatch(line); matches != nil {
			aPath := matches[1]
			bPath := matches[2]

			// Check if this is our target file
			if aPath == targetPathNormalized || bPath == targetPathNormalized ||
				strings.HasSuffix(aPath, "/"+targetPathNormalized) || strings.HasSuffix(bPath, "/"+targetPathNormalized) {
				capturing = true
				result.WriteString(line)
				result.WriteString("\n")
			} else if capturing {
				// We were capturing but hit a new file - stop
				break
			}
		} else if capturing {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// filterDiffByPatterns filters diff to only include files matching any of the given patterns
func filterDiffByPatterns(fullDiff string, patterns []string) string {
	if len(patterns) == 0 {
		return fullDiff
	}

	lines := strings.Split(fullDiff, "\n")
	var result strings.Builder
	capturing := false

	diffHeaderRegex := regexp.MustCompile(`^diff --git a/(.+) b/(.+)$`)

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if matches := diffHeaderRegex.FindStringSubmatch(line); matches != nil {
			bPath := matches[2]

			// Check if file matches any pattern
			capturing = false
			for _, pattern := range patterns {
				matched, _ := filepath.Match(pattern, bPath)
				if matched {
					capturing = true
					break
				}
				// Also try matching just the filename
				matched, _ = filepath.Match(pattern, filepath.Base(bPath))
				if matched {
					capturing = true
					break
				}
			}

			if capturing {
				result.WriteString(line)
				result.WriteString("\n")
			}
		} else if capturing {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// fetchCompletePRDiff fetches the complete PR diff and saves to file
func (h *Handler) fetchCompletePRDiff(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in fetchCompletePRDiffInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()

	// Fetch complete diff in one request (GitHub returns full diff)
	status, _, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d", owner, repo, in.Number), nil, "application/vnd.github.v3.diff")
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		hint := githubAuthHint(status)
		return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
	}

	// Apply file filtering if specified
	diffContent := string(body)
	if len(in.FileFilter) > 0 {
		diffContent = filterDiffByPatterns(diffContent, in.FileFilter)
	}

	// Determine output directory
	outputDir := in.OutputDir
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	// Create filename: pr-{owner}-{repo}-{number}-diff.txt
	safeRepo := strings.ReplaceAll(in.Repo, "/", "-")
	filename := fmt.Sprintf("pr-%s-%d-diff.txt", safeRepo, in.Number)
	filePath := filepath.Join(outputDir, filename)

	// Write to file
	if err := os.WriteFile(filePath, []byte(diffContent), 0644); err != nil {
		return errorResult(fmt.Sprintf("Failed to write diff file: %s", err.Error())), nil
	}

	// Count files in diff
	fileCount := strings.Count(diffContent, "diff --git")

	// Calculate statistics
	additions := strings.Count(diffContent, "\n+") - strings.Count(diffContent, "\n+++")
	deletions := strings.Count(diffContent, "\n-") - strings.Count(diffContent, "\n---")

	out := map[string]any{
		"repo":        in.Repo,
		"number":      in.Number,
		"saved_to":    filePath,
		"total_bytes": len(diffContent),
		"file_count":  fileCount,
		"statistics": map[string]any{
			"additions": additions,
			"deletions": deletions,
		},
		"format":   "unified",
		"complete": true,
	}

	if len(in.FileFilter) > 0 {
		out["file_filter"] = in.FileFilter
		out["filtered"] = true
	}

	// Add rate limit info
	remaining, resetAt := gh.GetRateLimitInfo()
	if remaining >= 0 {
		out["rate_limit"] = map[string]any{
			"remaining": remaining,
			"reset_at":  resetAt.Format(time.RFC3339),
		}
	}

	return jsonResult(out), nil
}

// fetchCompletePRFiles fetches all changed files in PR (all pages) and saves to file
func (h *Handler) fetchCompletePRFiles(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in fetchCompletePRFilesInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if in.Repo == "" || in.Number <= 0 {
		return errorResult("repo and positive number are required"), nil
	}

	owner, repo, err := splitRepo(in.Repo)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	gh := newGitHubClient()

	// Fetch all pages of files
	var allFiles []map[string]any
	page := 1
	for {
		q := url.Values{}
		q.Set("page", strconv.Itoa(page))
		q.Set("per_page", "100")

		status, headers, body, err := gh.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/%s/pulls/%d/files", owner, repo, in.Number), q, "application/vnd.github+json")
		if err != nil {
			return errorResult(err.Error()), nil
		}
		if status < 200 || status >= 300 {
			hint := githubAuthHint(status)
			return errorResult(fmt.Sprintf("GitHub API error (%d): %s\n%s", status, strings.TrimSpace(string(body)), hint)), nil
		}

		var files []map[string]any
		_ = json.Unmarshal(body, &files)

		if len(files) == 0 {
			break
		}

		// Compact files to essential fields
		for _, f := range files {
			allFiles = append(allFiles, map[string]any{
				"filename":  f["filename"],
				"status":    f["status"],
				"additions": f["additions"],
				"deletions": f["deletions"],
				"changes":   f["changes"],
				"patch":     f["patch"],
			})
		}

		_, hasNext := parseNextPage(headers.Get("Link"))
		if !hasNext {
			break
		}
		page++
	}

	// Determine output directory
	outputDir := in.OutputDir
	if outputDir == "" {
		outputDir = os.TempDir()
	}

	// Create filename: pr-{owner}-{repo}-{number}-files.json
	safeRepo := strings.ReplaceAll(in.Repo, "/", "-")
	filename := fmt.Sprintf("pr-%s-%d-files.json", safeRepo, in.Number)
	filePath := filepath.Join(outputDir, filename)

	// Write to file as JSON
	fileContent, _ := json.MarshalIndent(allFiles, "", "  ")
	if err := os.WriteFile(filePath, fileContent, 0644); err != nil {
		return errorResult(fmt.Sprintf("Failed to write files list: %s", err.Error())), nil
	}

	// Calculate statistics
	totalAdditions := 0
	totalDeletions := 0
	filesByStatus := make(map[string]int)
	for _, f := range allFiles {
		if add, ok := f["additions"].(float64); ok {
			totalAdditions += int(add)
		}
		if del, ok := f["deletions"].(float64); ok {
			totalDeletions += int(del)
		}
		if status, ok := f["status"].(string); ok {
			filesByStatus[status]++
		}
	}

	out := map[string]any{
		"repo":        in.Repo,
		"number":      in.Number,
		"saved_to":    filePath,
		"total_files": len(allFiles),
		"total_pages": page,
		"statistics": map[string]any{
			"total_additions": totalAdditions,
			"total_deletions": totalDeletions,
			"files_by_status": filesByStatus,
		},
		"complete": true,
	}

	// Add rate limit info
	remaining, resetAt := gh.GetRateLimitInfo()
	if remaining >= 0 {
		out["rate_limit"] = map[string]any{
			"remaining": remaining,
			"reset_at":  resetAt.Format(time.RFC3339),
		}
	}

	return jsonResult(out), nil
}
