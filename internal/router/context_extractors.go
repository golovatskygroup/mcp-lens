package router

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type ContextExtractor interface {
	Name() string
	TryExtract(input string) (map[string]any, bool)
}

func DefaultContextExtractors() []ContextExtractor {
	return []ContextExtractor{
		grafanaDashboardURLExtractor{},
		githubPRURLExtractor{},
		jiraIssueURLExtractor{},
		confluencePageURLExtractor{},
	}
}

func ExtractStructuredContext(input string) map[string]any {
	out := map[string]any{}
	for _, ex := range DefaultContextExtractors() {
		m, ok := ex.TryExtract(input)
		if !ok || len(m) == 0 {
			continue
		}
		for k, v := range m {
			if _, exists := out[k]; exists {
				continue
			}
			out[k] = v
		}
	}
	return out
}

var urlRe = regexp.MustCompile(`https?://[^\s]+`)

func findURLs(input string) []string {
	return urlRe.FindAllString(input, -1)
}

func sanitizeURLToken(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "`\"'")
	s = strings.TrimRight(s, ".,);]\"'")
	return s
}

type grafanaDashboardURLExtractor struct{}

func (grafanaDashboardURLExtractor) Name() string { return "grafana_dashboard_url" }

func (grafanaDashboardURLExtractor) TryExtract(input string) (map[string]any, bool) {
	for _, raw := range findURLs(input) {
		u, err := url.Parse(sanitizeURLToken(raw))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Host) == "" {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		uid := ""
		for i := 0; i < len(parts); i++ {
			if parts[i] == "d" && i+1 < len(parts) {
				uid = parts[i+1]
				break
			}
		}
		if strings.TrimSpace(uid) == "" {
			continue
		}
		orgID := 0
		if v := strings.TrimSpace(u.Query().Get("orgId")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				orgID = n
			}
		}
		baseURL := u.Scheme + "://" + u.Host
		return map[string]any{
			"grafana_base_url":      baseURL,
			"grafana_org_id":        orgID,
			"grafana_dashboard_uid": uid,
			"grafana_dashboard_url": sanitizeURLToken(raw),
		}, true
	}
	return nil, false
}

type githubPRURLExtractor struct{}

func (githubPRURLExtractor) Name() string { return "github_pr_url" }

func (githubPRURLExtractor) TryExtract(input string) (map[string]any, bool) {
	for _, raw := range findURLs(input) {
		u, err := url.Parse(sanitizeURLToken(raw))
		if err != nil || strings.ToLower(u.Host) != "github.com" {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) < 4 || parts[2] != "pull" {
			continue
		}
		num, err := strconv.Atoi(parts[3])
		if err != nil || num <= 0 {
			continue
		}
		repo := parts[0] + "/" + parts[1]
		return map[string]any{
			"github_repo":      repo,
			"github_pr_number": num,
			"github_pr_url":    sanitizeURLToken(raw),
		}, true
	}
	return nil, false
}

type jiraIssueURLExtractor struct{}

func (jiraIssueURLExtractor) Name() string { return "jira_issue_url" }

var jiraKeyRe = regexp.MustCompile(`(?i)^[A-Z][A-Z0-9]+-\d+$`)

func (jiraIssueURLExtractor) TryExtract(input string) (map[string]any, bool) {
	for _, raw := range findURLs(input) {
		u, err := url.Parse(sanitizeURLToken(raw))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Host) == "" {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := 0; i < len(parts); i++ {
			if parts[i] == "browse" && i+1 < len(parts) && jiraKeyRe.MatchString(parts[i+1]) {
				base := u.Scheme + "://" + u.Host
				return map[string]any{
					"jira_issue_key": parts[i+1],
					"jira_base_url":  base,
				}, true
			}
		}
	}
	return nil, false
}

type confluencePageURLExtractor struct{}

func (confluencePageURLExtractor) Name() string { return "confluence_page_url" }

func (confluencePageURLExtractor) TryExtract(input string) (map[string]any, bool) {
	for _, raw := range findURLs(input) {
		u, err := url.Parse(sanitizeURLToken(raw))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || strings.TrimSpace(u.Host) == "" {
			continue
		}
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i := 0; i < len(parts); i++ {
			if parts[i] == "pages" && i+1 < len(parts) {
				id := parts[i+1]
				if _, err := strconv.ParseInt(id, 10, 64); err != nil {
					continue
				}
				base := u.Scheme + "://" + u.Host
				if strings.Contains(strings.ToLower(strings.Trim(u.Path, "/")), "wiki/") || strings.HasPrefix(strings.ToLower(strings.Trim(u.Path, "/")), "wiki") {
					// Confluence Cloud commonly uses /wiki as a base path.
					base += "/wiki"
				}
				return map[string]any{
					"confluence_page_id":  id,
					"confluence_base_url": base,
				}, true
			}
		}
	}
	return nil, false
}
