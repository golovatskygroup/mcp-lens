package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"unicode"

	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

type confluenceXhtmlToTextInput struct {
	XHTML         string `json:"xhtml"`
	MaxChars      *int   `json:"max_chars,omitempty"`
	PreserveLinks bool   `json:"preserve_links,omitempty"`
}

func (h *Handler) confluenceXhtmlToText(_ context.Context, args json.RawMessage) (*mcp.CallToolResult, error) {
	var in confluenceXhtmlToTextInput
	if err := json.Unmarshal(args, &in); err != nil {
		return errorResult("Invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(in.XHTML) == "" {
		return errorResult("xhtml is required"), nil
	}

	maxChars := 20_000
	if in.MaxChars != nil {
		maxChars = *in.MaxChars
	}
	if maxChars < 0 {
		return errorResult("max_chars must be >= 0"), nil
	}
	if maxChars > 2_000_000 {
		return errorResult("max_chars is too large (max 2000000)"), nil
	}

	text, truncated, err := confluenceXhtmlToText(in.XHTML, in.PreserveLinks, maxChars)
	if err != nil {
		return errorResult(err.Error()), nil
	}

	return jsonResult(map[string]any{
		"text":      text,
		"truncated": truncated,
		"chars":     len([]rune(text)),
	}), nil
}

func confluenceXhtmlToText(input string, preserveLinks bool, maxChars int) (string, bool, error) {
	// Confluence storage often embeds plain text in CDATA blocks (e.g. code macros).
	// Make it HTML-parser-friendly by stripping CDATA wrappers.
	clean := stripCDATA(input)

	nodes, err := xhtml.ParseFragment(strings.NewReader(clean), &xhtml.Node{Type: xhtml.ElementNode, DataAtom: atom.Div, Data: "div"})
	if err != nil {
		return "", false, fmt.Errorf("failed to parse XHTML: %w", err)
	}

	w := &textWriter{preserveLinks: preserveLinks}
	for _, n := range nodes {
		w.walk(n, false)
	}
	out := w.finalize()

	// Best-effort truncation on rune boundary.
	if maxChars > 0 {
		r := []rune(out)
		if len(r) > maxChars {
			out = string(r[:maxChars])
			return strings.TrimSpace(out), true, nil
		}
	}

	return strings.TrimSpace(out), false, nil
}

func stripCDATA(s string) string {
	// Minimal, deterministic CDATA unwrap (no regex; avoids surprising backtracking).
	const open = "<![CDATA["
	const close = "]]>"
	for {
		i := strings.Index(s, open)
		if i < 0 {
			return s
		}
		j := strings.Index(s[i+len(open):], close)
		if j < 0 {
			return s
		}
		j = i + len(open) + j
		payload := s[i+len(open) : j]
		s = s[:i] + payload + s[j+len(close):]
	}
}

type textWriter struct {
	sb            strings.Builder
	preserveLinks bool
	listDepth     int
	needSpace     bool
	lastNL        bool
	trailingNL    int
}

func (w *textWriter) walk(n *xhtml.Node, inPre bool) {
	switch n.Type {
	case xhtml.TextNode:
		w.writeText(n.Data, inPre)
		return
	case xhtml.ElementNode:
		tag := strings.ToLower(strings.TrimSpace(n.Data))

		// Namespaced Confluence tags are kept as-is by the HTML tokenizer (e.g. "ac:structured-macro").
		if tag == "br" {
			w.newline(1)
			return
		}

		blockBefore, blockAfter := isBlockTag(tag)
		if blockBefore {
			w.newline(1)
		}

		nextInPre := inPre || tag == "pre" || tag == "code" || strings.Contains(tag, "plain-text-body")

		if tag == "ul" || tag == "ol" {
			w.listDepth++
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				w.walk(c, nextInPre)
			}
			if w.listDepth > 0 {
				w.listDepth--
			}
			w.newline(1)
			return
		}

		if tag == "li" {
			w.newline(1)
			indent := ""
			if w.listDepth > 1 {
				indent = strings.Repeat("  ", w.listDepth-1)
			}
			w.sb.WriteString(indent)
			w.sb.WriteString("- ")
			w.needSpace = false
			w.lastNL = false
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				w.walk(c, nextInPre)
			}
			w.newline(1)
			return
		}

		// Confluence link targets inside storage format.
		if tag == "ri:url" {
			if v := getAttr(n, "ri:value"); v != "" {
				w.writeText(v, inPre)
			}
		}
		if tag == "ri:page" {
			if v := getAttr(n, "ri:content-title"); v != "" {
				w.writeText(v, inPre)
			}
		}
		if tag == "ri:attachment" {
			if v := getAttr(n, "ri:filename"); v != "" {
				w.writeText(v, inPre)
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			w.walk(c, nextInPre)
		}

		if tag == "a" && w.preserveLinks {
			if href := getAttr(n, "href"); href != "" {
				w.writeText(" ("+href+")", false)
			}
		}

		if blockAfter {
			w.newline(2)
		}
	}
}

func getAttr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func isBlockTag(tag string) (before bool, after bool) {
	switch tag {
	case "p", "div", "section", "article", "header", "footer",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"pre", "blockquote",
		"table", "thead", "tbody", "tfoot", "tr", "th", "td",
		"ac:structured-macro", "ac:rich-text-body", "ac:plain-text-body":
		return true, true
	default:
		return false, false
	}
}

func (w *textWriter) writeText(s string, inPre bool) {
	if s == "" {
		return
	}
	// Normalize HTML entities in case we injected any ourselves.
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\u00a0", " ")

	if inPre {
		// Preserve whitespace as much as possible.
		if w.needSpace {
			w.sb.WriteByte(' ')
			w.trailingNL = 0
			w.lastNL = false
		}
		w.sb.WriteString(s)
		w.needSpace = false
		w.lastNL = strings.HasSuffix(s, "\n")
		w.trailingNL = countTrailingNewlines(s)
		return
	}

	for _, r := range s {
		if unicode.IsSpace(r) {
			w.needSpace = true
			continue
		}
		if w.needSpace && w.sb.Len() > 0 && !w.lastNL {
			w.sb.WriteByte(' ')
		}
		w.needSpace = false
		w.lastNL = false
		w.trailingNL = 0
		w.sb.WriteRune(r)
	}
}

func (w *textWriter) newline(n int) {
	if n <= 0 {
		return
	}
	w.needSpace = false
	// Avoid accumulating lots of blank lines.
	if w.trailingNL >= n {
		w.lastNL = true
		return
	}
	for i := 0; i < n-w.trailingNL; i++ {
		w.sb.WriteByte('\n')
		w.trailingNL++
	}
	w.lastNL = true
}

func (w *textWriter) finalize() string {
	// Trim trailing spaces on each line while preserving indentation.
	raw := w.sb.String()
	lines := strings.Split(raw, "\n")
	for i := range lines {
		lines[i] = strings.TrimRightFunc(lines[i], unicode.IsSpace)
	}
	joined := strings.Join(lines, "\n")
	// Collapse excessive blank lines to max 2.
	joined = collapseBlankLines(joined, 2)
	return joined
}

func collapseBlankLines(s string, max int) string {
	if max < 1 {
		max = 1
	}
	// max blank lines => max consecutive '\n' is (max + 1)
	maxNewlines := max + 1
	nl := 0
	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		if r == '\n' {
			nl++
			if nl > maxNewlines {
				continue
			}
			out.WriteRune(r)
			continue
		}
		nl = 0
		out.WriteRune(r)
	}
	return out.String()
}

func countTrailingNewlines(s string) int {
	n := 0
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] != '\n' {
			break
		}
		n++
	}
	return n
}
