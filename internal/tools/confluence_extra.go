package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type confluencePageChildrenInput struct {
	Client  string `json:"client,omitempty"`
	BaseURL string `json:"base_url,omitempty"`

	PageID string `json:"page_id"`
	Start  int    `json:"start,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Expand string `json:"expand,omitempty"` // v1 expand, e.g. "page"
}

type confluencePageAttachmentsInput struct {
	Client  string `json:"client,omitempty"`
	BaseURL string `json:"base_url,omitempty"`

	PageID string `json:"page_id"`
	Start  int    `json:"start,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Expand string `json:"expand,omitempty"` // v1 expand, e.g. "metadata,version"
}

type confluenceDownloadAttachmentInput struct {
	Client  string `json:"client,omitempty"`
	BaseURL string `json:"base_url,omitempty"`

	DownloadURL string `json:"download_url"`
	Name        string `json:"name,omitempty"`
}

func (h *Handler) confluenceGetPageChildren(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluencePageChildrenInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.PageID) == "" {
		return errorResult("page_id is required"), nil
	}
	if in.Start < 0 {
		return errorResult("start must be >= 0"), nil
	}
	if in.Limit == 0 {
		in.Limit = 25
	}
	if in.Limit < 1 || in.Limit > 250 {
		return errorResult("limit must be between 1 and 250"), nil
	}
	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("start", strconv.Itoa(in.Start))
	q.Set("limit", strconv.Itoa(in.Limit))
	if strings.TrimSpace(in.Expand) != "" {
		q.Set("expand", strings.TrimSpace(in.Expand))
	} else {
		q.Set("expand", "page")
	}

	u := cl.apiV1Base() + "/content/" + url.PathEscape(in.PageID) + "/child"
	status, _, body, err := cl.do(ctx, http.MethodGet, u, q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}

	respAny := mustUnmarshalAny(body)
	out := map[string]any{
		"base_url": cl.baseURL,
		"result":   respAny,
	}

	// Best-effort next pagination (v1 uses _links.next, relative).
	if m, ok := respAny.(map[string]any); ok {
		if links, ok := m["_links"].(map[string]any); ok {
			if next, ok := links["next"].(string); ok && strings.TrimSpace(next) != "" {
				out["has_next"] = true
				out["next_url"] = cl.baseURL + next
				// Try parse start from next URL.
				if cursor, start, ok := parseNextCursorFromRelative(next); ok && cursor != "" {
					out["next_cursor"] = cursor
				} else if start != nil {
					out["next_start"] = *start
				}
			}
		}
	}

	return jsonResult(out), nil
}

func (h *Handler) confluenceListPageAttachments(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluencePageAttachmentsInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.PageID) == "" {
		return errorResult("page_id is required"), nil
	}
	if in.Start < 0 {
		return errorResult("start must be >= 0"), nil
	}
	if in.Limit == 0 {
		in.Limit = 25
	}
	if in.Limit < 1 || in.Limit > 250 {
		return errorResult("limit must be between 1 and 250"), nil
	}
	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	q := url.Values{}
	q.Set("start", strconv.Itoa(in.Start))
	q.Set("limit", strconv.Itoa(in.Limit))
	if strings.TrimSpace(in.Expand) != "" {
		q.Set("expand", strings.TrimSpace(in.Expand))
	}

	u := cl.apiV1Base() + "/content/" + url.PathEscape(in.PageID) + "/child/attachment"
	status, _, body, err := cl.do(ctx, http.MethodGet, u, q, nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if status < 200 || status >= 300 {
		return errorResult(fmt.Sprintf("Confluence API error (%d): %s", status, strings.TrimSpace(string(body)))), nil
	}

	respAny := mustUnmarshalAny(body)
	out := map[string]any{
		"base_url": cl.baseURL,
		"result":   respAny,
	}
	if m, ok := respAny.(map[string]any); ok {
		if links, ok := m["_links"].(map[string]any); ok {
			if next, ok := links["next"].(string); ok && strings.TrimSpace(next) != "" {
				out["has_next"] = true
				out["next_url"] = cl.baseURL + next
				if _, start, ok := parseNextCursorFromRelative(next); ok && start != nil {
					out["next_start"] = *start
				}
			}
		}
	}

	return jsonResult(out), nil
}

func (h *Handler) confluenceDownloadAttachment(ctx context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluenceDownloadAttachmentInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	dl := strings.TrimSpace(in.DownloadURL)
	if dl == "" {
		return errorResult("download_url is required"), nil
	}
	cl, err := newConfluenceClient(in.Client, in.BaseURL)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	u, err := url.Parse(dl)
	if err != nil {
		return errorResult("invalid download_url: " + err.Error()), nil
	}
	if !u.IsAbs() {
		u, err = url.Parse(cl.baseURL + "/" + strings.TrimLeft(dl, "/"))
		if err != nil {
			return errorResult("invalid download_url: " + err.Error()), nil
		}
	}
	// Basic safety: keep downloads scoped to the configured base host.
	base, _ := url.Parse(cl.baseURL)
	if base != nil && !strings.EqualFold(u.Host, base.Host) {
		return errorResult("download_url must match Confluence base_url host"), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	req.Header.Set("User-Agent", "mcp-lens")
	if cl.authHeader != "" {
		req.Header.Set("Authorization", cl.authHeader)
	}

	resp, err := cl.c.Do(req)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return errorResult(fmt.Sprintf("failed to download attachment: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))), nil
	}

	if h.artifacts == nil {
		return errorResult("artifact store is not configured"), nil
	}

	ct := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if ct == "" {
		ct = "application/octet-stream"
	} else {
		ct = strings.Split(ct, ";")[0]
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 20*1024*1024)) // 20MB cap
	if err != nil {
		return errorResult(err.Error()), nil
	}
	if len(b) >= 20*1024*1024 {
		return errorResult("attachment too large (limit 20MB)"), nil
	}

	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = path.Base(u.Path)
	}
	ext := path.Ext(name)
	repl, item, err := h.artifacts.StoreBytes("confluence_download_attachment", args, ct, strings.TrimPrefix(ext, "."), b)
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"download_url": u.String(),
		"artifact":     repl,
		"bytes":        item.Bytes,
		"mime":         item.Mime,
		"sha256":       item.SHA256,
	}), nil
}
