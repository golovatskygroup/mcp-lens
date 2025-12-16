package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type jiraExportTasksInput struct {
	jiraBaseInput
	JQL               string   `json:"jql"`
	OutputDir         string   `json:"output_dir,omitempty"`
	MaxIssues         int      `json:"max_issues,omitempty"`
	Fields            []string `json:"fields,omitempty"`
	Expand            []string `json:"expand,omitempty"`
	IncludeConfluence *bool    `json:"include_confluence,omitempty"`
	ConfluenceBaseURL string   `json:"confluence_base_url,omitempty"`
}

type jiraExportLink struct {
	URL    string `json:"url"`
	Kind   string `json:"kind"`
	OK     bool   `json:"ok"`
	Note   string `json:"note,omitempty"`
	Output string `json:"output,omitempty"`
}

type jiraExportIssueResult struct {
	Key      string           `json:"key"`
	Summary  string           `json:"summary,omitempty"`
	Status   string           `json:"status,omitempty"`
	File     string           `json:"file"`
	Links    []jiraExportLink `json:"links,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}

func (h *Handler) jiraExportTasks(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in jiraExportTasksInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	in.JQL = strings.TrimSpace(in.JQL)
	if in.JQL == "" {
		return errorResult("jql is required"), nil
	}
	if in.MaxIssues == 0 {
		in.MaxIssues = 50
	}
	if in.MaxIssues < 1 || in.MaxIssues > 200 {
		return errorResult("max_issues must be between 1 and 200"), nil
	}
	if strings.TrimSpace(in.OutputDir) == "" {
		in.OutputDir = "tasks/jira-export"
	}
	in.OutputDir = strings.TrimSpace(in.OutputDir)
	if err := os.MkdirAll(in.OutputDir, 0o755); err != nil {
		return errorResult("failed to create output_dir: " + err.Error()), nil
	}

	// 1) Search issues by JQL.
	searchArgs, _ := json.Marshal(map[string]any{
		"client":      in.Client,
		"base_url":    in.BaseURL,
		"api_version": in.APIVersion,
		"jql":         in.JQL,
		"startAt":     0,
		"maxResults":  in.MaxIssues,
		"fields": func() []string {
			if len(in.Fields) > 0 {
				return in.Fields
			}
			return []string{"summary", "status", "assignee", "priority", "description"}
		}(),
		"expand": in.Expand,
	})
	searchRes, _ := h.jiraSearchIssues(ctx, searchArgs)
	if searchRes == nil || searchRes.IsError || len(searchRes.Content) == 0 {
		if searchRes != nil && len(searchRes.Content) > 0 {
			return errorResult(searchRes.Content[0].Text), nil
		}
		return errorResult("jira_search_issues failed"), nil
	}

	var searchPayload map[string]any
	if err := json.Unmarshal([]byte(searchRes.Content[0].Text), &searchPayload); err != nil {
		return errorResult("failed to parse jira_search_issues output: " + err.Error()), nil
	}
	issuesAny, _ := searchPayload["issues"].([]any)
	if len(issuesAny) == 0 {
		out := map[string]any{
			"output_dir": in.OutputDir,
			"count":      0,
			"issues":     []any{},
		}
		return jsonResult(out), nil
	}

	confluenceEnabled := true
	if in.IncludeConfluence != nil {
		confluenceEnabled = *in.IncludeConfluence
	}

	results := make([]jiraExportIssueResult, 0, len(issuesAny))
	unresolved := []map[string]any{}

	for _, it := range issuesAny {
		obj, _ := it.(map[string]any)
		key, _ := obj["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		issueArgs, _ := json.Marshal(map[string]any{
			"client":      in.Client,
			"base_url":    in.BaseURL,
			"api_version": in.APIVersion,
			"issue":       key,
			"fields": func() []string {
				if len(in.Fields) > 0 {
					return in.Fields
				}
				return []string{"summary", "status", "assignee", "priority", "description"}
			}(),
			"expand": in.Expand,
		})
		issueRes, _ := h.jiraGetIssue(ctx, issueArgs)
		if issueRes == nil || issueRes.IsError || len(issueRes.Content) == 0 {
			note := "jira_get_issue failed"
			if issueRes != nil && len(issueRes.Content) > 0 {
				note = issueRes.Content[0].Text
			}
			results = append(results, jiraExportIssueResult{Key: key, File: "", Warnings: []string{note}})
			continue
		}

		var issuePayload map[string]any
		if err := json.Unmarshal([]byte(issueRes.Content[0].Text), &issuePayload); err != nil {
			results = append(results, jiraExportIssueResult{Key: key, File: "", Warnings: []string{"failed to parse jira_get_issue output"}})
			continue
		}

		fields, _ := issuePayload["fields"].(map[string]any)
		summary, _ := fields["summary"].(string)
		status := extractJiraStatus(fields)
		descText, descRaw := extractJiraDescription(fields)

		links := extractURLs(descText)
		expandedLinks := make([]jiraExportLink, 0, len(links))
		expandedSnippets := make([]string, 0, len(links))

		for _, link := range links {
			kind, pageID, ok := classifyConfluenceURL(link)
			if ok && confluenceEnabled {
				htmlPath, textPath, err := h.exportConfluencePage(ctx, in.OutputDir, pageID, in.ConfluenceBaseURL)
				if err != nil {
					expandedLinks = append(expandedLinks, jiraExportLink{URL: link, Kind: kind, OK: false, Note: err.Error()})
					unresolved = append(unresolved, map[string]any{"issue": key, "url": link, "kind": kind, "reason": err.Error()})
					continue
				}
				expandedLinks = append(expandedLinks, jiraExportLink{URL: link, Kind: kind, OK: true, Output: htmlPath})
				expandedSnippets = append(expandedSnippets, fmt.Sprintf("- Confluence page %s:\n  - html: %s\n  - text: %s\n", pageID, htmlPath, textPath))
				continue
			}

			// Recognize Jira issue links and mark as not expanded (we have the issue already, but do not recurse).
			if kind, key2, ok := classifyJiraIssueURL(link); ok {
				expandedLinks = append(expandedLinks, jiraExportLink{URL: link, Kind: kind, OK: false, Note: "recognized Jira issue link (not expanded)"})
				unresolved = append(unresolved, map[string]any{"issue": key, "url": link, "kind": kind, "reason": "recognized but not expanded"})
				_ = key2
				continue
			}

			expandedLinks = append(expandedLinks, jiraExportLink{URL: link, Kind: "unknown", OK: false, Note: "no resolver available"})
			unresolved = append(unresolved, map[string]any{"issue": key, "url": link, "kind": "unknown", "reason": "no resolver available"})
		}

		issueFile := filepath.Join(in.OutputDir, fmt.Sprintf("%s.md", sanitizeFileName(key)))
		var md strings.Builder
		md.WriteString(fmt.Sprintf("# %s\n\n", key))
		if strings.TrimSpace(summary) != "" {
			md.WriteString(fmt.Sprintf("**Summary:** %s\n\n", summary))
		}
		if strings.TrimSpace(status) != "" {
			md.WriteString(fmt.Sprintf("**Status:** %s\n\n", status))
		}
		md.WriteString("## Description\n\n")
		if strings.TrimSpace(descText) == "" && descRaw != nil {
			md.WriteString("*(Description present, but could not be rendered as text; see description_json below.)*\n\n")
		} else {
			md.WriteString(descText)
			if !strings.HasSuffix(descText, "\n") {
				md.WriteString("\n")
			}
		}

		if len(expandedSnippets) > 0 {
			md.WriteString("\n## Expanded links\n\n")
			for _, sn := range expandedSnippets {
				md.WriteString(sn)
				if !strings.HasSuffix(sn, "\n") {
					md.WriteString("\n")
				}
			}
		}

		if descRaw != nil {
			b, _ := json.MarshalIndent(descRaw, "", "  ")
			md.WriteString("\n## description_json\n\n```json\n")
			md.Write(b)
			md.WriteString("\n```\n")
		}

		if err := os.WriteFile(issueFile, []byte(md.String()), 0o644); err != nil {
			results = append(results, jiraExportIssueResult{Key: key, Summary: summary, Status: status, File: "", Warnings: []string{"failed to write file: " + err.Error()}})
			continue
		}

		results = append(results, jiraExportIssueResult{
			Key:     key,
			Summary: summary,
			Status:  status,
			File:    issueFile,
			Links:   expandedLinks,
		})
	}

	indexFile := filepath.Join(in.OutputDir, "index.md")
	var idx strings.Builder
	idx.WriteString(fmt.Sprintf("# Jira export\n\nJQL: `%s`\n\n", in.JQL))
	idx.WriteString("## Issues\n\n")
	for _, r := range results {
		if r.File == "" {
			continue
		}
		line := fmt.Sprintf("- [%s](%s)", r.Key, filepath.Base(r.File))
		if strings.TrimSpace(r.Summary) != "" {
			line += " — " + r.Summary
		}
		idx.WriteString(line + "\n")
	}
	_ = os.WriteFile(indexFile, []byte(idx.String()), 0o644)

	out := map[string]any{
		"output_dir": in.OutputDir,
		"index":      indexFile,
		"count":      len(results),
		"issues":     results,
		"meta": map[string]any{
			"unresolved_links": unresolved,
		},
	}
	return jsonResult(out), nil
}

func extractJiraStatus(fields map[string]any) string {
	statusObj, _ := fields["status"].(map[string]any)
	if statusObj == nil {
		return ""
	}
	if name, ok := statusObj["name"].(string); ok {
		return strings.TrimSpace(name)
	}
	return ""
}

func extractJiraDescription(fields map[string]any) (text string, raw any) {
	desc, ok := fields["description"]
	if !ok || desc == nil {
		return "", nil
	}
	switch v := desc.(type) {
	case string:
		return strings.TrimSpace(v), nil
	default:
		// Try to render Atlassian Document Format (best-effort), and also keep raw.
		raw = v
		text = strings.TrimSpace(renderADFText(v))
		return text, raw
	}
}

func renderADFText(v any) string {
	var sb strings.Builder
	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if t, ok := n["type"].(string); ok && (t == "text" || t == "inlineCard") {
				if txt, ok := n["text"].(string); ok && txt != "" {
					sb.WriteString(txt)
				}
				if attrs, ok := n["attrs"].(map[string]any); ok {
					if urlStr, ok := attrs["url"].(string); ok && urlStr != "" {
						if sb.Len() > 0 && !strings.HasSuffix(sb.String(), " ") {
							sb.WriteString(" ")
						}
						sb.WriteString(urlStr)
					}
				}
			}
			if content, ok := n["content"].([]any); ok {
				for _, c := range content {
					walk(c)
				}
				if t, ok := n["type"].(string); ok {
					switch t {
					case "paragraph", "heading", "listItem":
						sb.WriteString("\n")
					}
				}
			}
		case []any:
			for _, it := range n {
				walk(it)
			}
		}
	}
	walk(v)
	return normalizeWhitespace(sb.String())
}

var reURL = regexp.MustCompile(`https?://[^\s<>"')\]]+`)

func extractURLs(text string) []string {
	matches := reURL.FindAllString(text, -1)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimRight(m, ".,);]")
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

func classifyConfluenceURL(raw string) (kind string, pageID string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	if v := strings.TrimSpace(u.Query().Get("pageId")); v != "" {
		if _, err := strconv.ParseInt(v, 10, 64); err == nil {
			return "confluence_page", v, true
		}
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "pages" && i+1 < len(parts) {
			id := parts[i+1]
			if _, err := strconv.ParseInt(id, 10, 64); err == nil {
				return "confluence_page", id, true
			}
		}
	}
	return "", "", false
}

func classifyJiraIssueURL(raw string) (kind string, issueKey string, ok bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(parts); i++ {
		if parts[i] == "browse" && i+1 < len(parts) {
			key := strings.TrimSpace(parts[i+1])
			if key != "" {
				return "jira_issue", key, true
			}
		}
	}
	return "", "", false
}

func (h *Handler) exportConfluencePage(ctx context.Context, outputDir string, pageID string, baseOverride string) (htmlPath string, textPath string, err error) {
	pageID = strings.TrimSpace(pageID)
	if pageID == "" {
		return "", "", fmt.Errorf("missing confluence page id")
	}

	confluenceDir := filepath.Join(outputDir, "confluence")
	descDir := filepath.Join(confluenceDir, "descriptions")
	if err := os.MkdirAll(descDir, 0o755); err != nil {
		return "", "", err
	}

	reqArgs, _ := json.Marshal(map[string]any{
		"id":          pageID,
		"base_url":    strings.TrimSpace(baseOverride),
		"body_format": "view",
		"expand":      []string{"body.view", "body.storage", "version"},
		"use_v2":      false,
	})
	res, _ := h.confluenceGetPage(ctx, reqArgs)
	if res == nil || res.IsError || len(res.Content) == 0 {
		if res != nil && len(res.Content) > 0 {
			return "", "", fmt.Errorf("%s", res.Content[0].Text)
		}
		return "", "", fmt.Errorf("confluence_get_page failed")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(res.Content[0].Text), &payload); err != nil {
		return "", "", fmt.Errorf("failed to parse confluence response: %w", err)
	}

	title := extractConfluenceTitle(payload)
	bodyView := extractConfluenceBody(payload, "view")
	bodyStorage := extractConfluenceBody(payload, "storage")
	if strings.TrimSpace(bodyView) == "" && strings.TrimSpace(bodyStorage) == "" {
		return "", "", fmt.Errorf("confluence page has no body.view/body.storage")
	}

	htmlPath = filepath.Join(confluenceDir, fmt.Sprintf("%s.html", sanitizeFileName(pageID)))
	textPath = filepath.Join(descDir, fmt.Sprintf("%s.md", sanitizeFileName(pageID)))

	var htmlOut strings.Builder
	htmlOut.WriteString("<!doctype html><meta charset=\"utf-8\">")
	if title != "" {
		htmlOut.WriteString("<title>" + html.EscapeString(title) + "</title>")
	}
	if title != "" {
		htmlOut.WriteString("<h1>" + html.EscapeString(title) + "</h1>\n")
	}
	if strings.TrimSpace(bodyView) != "" {
		htmlOut.WriteString(bodyView)
	} else {
		htmlOut.WriteString(bodyStorage)
	}

	if err := os.WriteFile(htmlPath, []byte(htmlOut.String()), 0o644); err != nil {
		return "", "", err
	}

	text := htmlToText(bodyStorage)
	if strings.TrimSpace(text) == "" {
		text = htmlToText(bodyView)
	}
	if err := os.WriteFile(textPath, []byte(text), 0o644); err != nil {
		return "", "", err
	}

	return htmlPath, textPath, nil
}

func extractConfluenceTitle(p map[string]any) string {
	if s, ok := p["title"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func extractConfluenceBody(p map[string]any, repr string) string {
	body, _ := p["body"].(map[string]any)
	if body == nil {
		return ""
	}
	part, _ := body[repr].(map[string]any)
	if part == nil {
		return ""
	}
	val, _ := part["value"].(string)
	return strings.TrimSpace(val)
}

func htmlToText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Drop script/style blocks.
	reScript := regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	reStyle := regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")

	reBreaks := regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr)>`)
	s = reBreaks.ReplaceAllString(s, "\n")
	s = strings.ReplaceAll(s, "<br>", "\n")
	s = strings.ReplaceAll(s, "<br/>", "\n")
	s = strings.ReplaceAll(s, "<br />", "\n")

	reTags := regexp.MustCompile(`(?s)<[^>]+>`)
	s = reTags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return normalizeWhitespace(s)
}

func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n") + "\n"
}

func sanitizeFileName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "item"
	}
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, " ", "_")
	keep := func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '_' || r == '-' || r == '.':
			return r
		default:
			return '_'
		}
	}
	s = strings.Map(keep, s)
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.Trim(s, "._-")
}
