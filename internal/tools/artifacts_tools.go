package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/golovatskygroup/mcp-lens/internal/artifacts"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

type artifactSaveTextInput struct {
	Name string `json:"name,omitempty"`
	Text string `json:"text"`
	Mime string `json:"mime,omitempty"` // default text/markdown
}

type artifactAppendTextInput struct {
	ArtifactID  string `json:"artifact_id,omitempty"`
	ArtifactURI string `json:"artifact_uri,omitempty"`
	Text        string `json:"text"`
}

type artifactListInput struct {
	Limit int `json:"limit,omitempty"`
}

type artifactSearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

func (h *Handler) artifactSaveText(_ context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if h.artifacts == nil {
		return errorResult("artifact store is not configured"), nil
	}
	var in artifactSaveTextInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Text) == "" {
		return errorResult("text is required"), nil
	}
	mime := strings.TrimSpace(in.Mime)
	if mime == "" {
		mime = "text/markdown"
	}
	ext := "txt"
	if strings.Contains(mime, "markdown") {
		ext = "md"
	}
	if strings.TrimSpace(in.Name) != "" {
		if e := path.Ext(strings.TrimSpace(in.Name)); e != "" {
			ext = strings.TrimPrefix(e, ".")
		}
	}
	repl, item, err := h.artifacts.StoreBytes("artifact_save_text", args, mime, ext, []byte(in.Text))
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"artifact": repl,
		"bytes":    item.Bytes,
		"mime":     item.Mime,
		"sha256":   item.SHA256,
	}), nil
}

func (h *Handler) artifactAppendText(_ context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if h.artifacts == nil {
		return errorResult("artifact store is not configured"), nil
	}
	var in artifactAppendTextInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.Text) == "" {
		return errorResult("text is required"), nil
	}
	id := strings.TrimSpace(in.ArtifactID)
	if id == "" && strings.TrimSpace(in.ArtifactURI) != "" {
		id = strings.TrimPrefix(strings.TrimSpace(in.ArtifactURI), "artifact://")
	}
	if id == "" {
		return errorResult("artifact_id or artifact_uri is required"), nil
	}
	b, mime, ok := h.artifacts.Read(id)
	if !ok {
		return errorResult("artifact not found: " + id), nil
	}
	orig := string(b)
	sep := ""
	if orig != "" && !strings.HasSuffix(orig, "\n") {
		sep = "\n"
	}
	newText := orig + sep + in.Text
	repl, item, err := h.artifacts.StoreBytes("artifact_append_text", args, mime, "txt", []byte(newText))
	if err != nil {
		return errorResult(err.Error()), nil
	}
	return jsonResult(map[string]any{
		"previous_artifact_id": id,
		"artifact":             repl,
		"bytes":                item.Bytes,
		"mime":                 item.Mime,
		"sha256":               item.SHA256,
	}), nil
}

func (h *Handler) artifactList(_ context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if h.artifacts == nil {
		return errorResult("artifact store is not configured"), nil
	}
	var in artifactListInput
	_ = json.Unmarshal(args, &in)
	items := h.artifacts.List()
	if in.Limit <= 0 || in.Limit > len(items) {
		in.Limit = len(items)
	}
	out := make([]artifacts.Item, 0, in.Limit)
	for i := 0; i < in.Limit; i++ {
		out = append(out, items[i])
	}
	return jsonResult(map[string]any{"artifacts": out}), nil
}

func (h *Handler) artifactSearch(_ context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	if h.artifacts == nil {
		return errorResult("artifact store is not configured"), nil
	}
	var in artifactSearchInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	q := strings.ToLower(strings.TrimSpace(in.Query))
	if q == "" {
		return errorResult("query is required"), nil
	}
	if in.Limit <= 0 {
		in.Limit = 20
	}
	if in.Limit > 200 {
		in.Limit = 200
	}
	items := h.artifacts.List()
	matches := make([]artifacts.Item, 0, minIntArtifacts(in.Limit, len(items)))
	for _, it := range items {
		if len(matches) >= in.Limit {
			break
		}
		hay := strings.ToLower(fmt.Sprintf("%s %s %s %s", it.ID, it.Path, it.Mime, it.Tool))
		if strings.Contains(hay, q) {
			matches = append(matches, it)
		}
	}
	return jsonResult(map[string]any{"artifacts": matches, "match_count": len(matches)}), nil
}

func minIntArtifacts(a, b int) int {
	if a < b {
		return a
	}
	return b
}
