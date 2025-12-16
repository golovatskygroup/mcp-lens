package router

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type OutputOptions struct {
	View          string   `json:"view,omitempty"` // full|summary|metadata|errors_only
	IncludeFields []string `json:"include_fields,omitempty"`
	ExcludeFields []string `json:"exclude_fields,omitempty"`
	MaxItems      int      `json:"max_items,omitempty"`
	MaxDepth      int      `json:"max_depth,omitempty"`
	Redact        []string `json:"redact,omitempty"`
}

func ApplyOutputShaping(toolName string, v any, opts *OutputOptions) (any, error) {
	if opts == nil {
		return v, nil
	}

	out := v
	if view := strings.ToLower(strings.TrimSpace(opts.View)); view != "" && view != "full" {
		out = applyViewPreset(toolName, out, view)
	}

	var err error
	if len(opts.IncludeFields) > 0 {
		out, err = applyInclude(out, opts.IncludeFields)
		if err != nil {
			return nil, err
		}
	}
	if len(opts.ExcludeFields) > 0 {
		for _, p := range opts.ExcludeFields {
			if err := removePath(&out, p); err != nil {
				return nil, err
			}
		}
	}
	if len(opts.Redact) > 0 {
		for _, p := range opts.Redact {
			if err := setPath(&out, p, "[REDACTED]"); err != nil {
				return nil, err
			}
		}
	}

	if opts.MaxDepth > 0 {
		out = truncateDepth(out, 0, opts.MaxDepth)
	}
	if opts.MaxItems > 0 {
		out = truncateItems(out, opts.MaxItems)
	}
	return out, nil
}

func applyViewPreset(toolName string, v any, view string) any {
	view = strings.ToLower(strings.TrimSpace(view))

	switch view {
	case "errors_only":
		switch vv := v.(type) {
		case map[string]any:
			out := map[string]any{}
			for _, k := range []string{"ok", "status", "error", "errors", "message"} {
				if val, ok := vv[k]; ok {
					out[k] = val
				}
			}
			return out
		case []any:
			out := make([]any, 0, len(vv))
			for _, it := range vv {
				out = append(out, applyViewPreset(toolName, it, view))
			}
			return out
		default:
			return v
		}
	case "metadata":
		paths := viewPresetPaths(toolName, view)
		if len(paths) == 0 {
			paths = []string{"id", "uid", "key", "name", "title", "url", "html_url", "web_url", "number", "repo"}
		}
		out, err := applyInclude(v, paths)
		if err == nil {
			return out
		}
		return v
	case "summary":
		paths := viewPresetPaths(toolName, view)
		if len(paths) == 0 {
			paths = []string{
				"id", "uid", "key", "name", "title", "summary", "description",
				"url", "html_url", "web_url", "state", "status", "created", "updated",
			}
		}
		out, err := applyInclude(v, paths)
		if err == nil {
			return out
		}
		return v
	default:
		return v
	}
}

func viewPresetPaths(toolName string, view string) []string {
	name := strings.ToLower(strings.TrimSpace(toolName))
	switch view {
	case "summary":
		switch name {
		case "grafana_get_dashboard":
			return []string{
				"meta.slug",
				"dashboard.uid",
				"dashboard.title",
				"dashboard.tags",
				"dashboard.time",
				"dashboard.templating.list",
			}
		}
	case "metadata":
		switch name {
		case "grafana_get_dashboard":
			return []string{"dashboard.uid", "dashboard.title", "meta.slug"}
		}
	}
	return nil
}

func applyInclude(v any, paths []string) (any, error) {
	if v == nil {
		return nil, nil
	}

	switch v.(type) {
	case map[string]any, []any:
		// ok
	default:
		// For non-objects, include acts as no-op.
		return v, nil
	}

	out := map[string]any{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		val, ok, err := getPath(v, p)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := setPathIntoMap(out, p, val); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return map[string]any{}, nil
	}
	return out, nil
}

type pathSeg struct {
	field string
	index *int
}

func parsePath(p string) ([]pathSeg, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil, fmt.Errorf("empty path")
	}

	if strings.HasPrefix(p, "/") {
		parts := strings.Split(strings.TrimPrefix(p, "/"), "/")
		segs := make([]pathSeg, 0, len(parts))
		for _, part := range parts {
			part = strings.ReplaceAll(part, "~1", "/")
			part = strings.ReplaceAll(part, "~0", "~")
			if part == "" {
				continue
			}
			if idx, err := strconv.Atoi(part); err == nil {
				i := idx
				segs = append(segs, pathSeg{index: &i})
				continue
			}
			segs = append(segs, pathSeg{field: part})
		}
		return segs, nil
	}

	segs := []pathSeg{}
	for len(p) > 0 {
		// field
		fieldEnd := strings.IndexAny(p, ".[")
		field := p
		if fieldEnd >= 0 {
			field = p[:fieldEnd]
		}
		if field != "" && field != "." {
			segs = append(segs, pathSeg{field: field})
		}
		p = p[len(field):]
		if strings.HasPrefix(p, ".") {
			p = p[1:]
			continue
		}
		if strings.HasPrefix(p, "[") {
			end := strings.Index(p, "]")
			if end < 0 {
				return nil, fmt.Errorf("invalid path (missing ]): %q", p)
			}
			raw := p[1:end]
			idx, err := strconv.Atoi(raw)
			if err != nil || idx < 0 {
				return nil, fmt.Errorf("invalid array index %q", raw)
			}
			i := idx
			segs = append(segs, pathSeg{index: &i})
			p = p[end+1:]
			if strings.HasPrefix(p, ".") {
				p = p[1:]
			}
			continue
		}
		if p != "" && fieldEnd < 0 {
			break
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("invalid path: %q", p)
	}
	return segs, nil
}

func getPath(v any, p string) (val any, ok bool, err error) {
	segs, err := parsePath(p)
	if err != nil {
		return nil, false, err
	}
	cur := v
	for _, s := range segs {
		if s.index != nil {
			arr, ok := cur.([]any)
			if !ok {
				return nil, false, nil
			}
			if *s.index < 0 || *s.index >= len(arr) {
				return nil, false, nil
			}
			cur = arr[*s.index]
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false, nil
		}
		nxt, ok := obj[s.field]
		if !ok {
			return nil, false, nil
		}
		cur = nxt
	}
	return cur, true, nil
}

func setPathIntoMap(dst map[string]any, p string, val any) error {
	segs, err := parsePath(p)
	if err != nil {
		return err
	}
	cur := dst
	for i, s := range segs {
		last := i == len(segs)-1
		if s.index != nil {
			return fmt.Errorf("include path cannot create arrays (%q)", p)
		}
		if last {
			cur[s.field] = val
			return nil
		}
		nxt, ok := cur[s.field]
		if !ok {
			m := map[string]any{}
			cur[s.field] = m
			cur = m
			continue
		}
		m, ok := nxt.(map[string]any)
		if !ok {
			m = map[string]any{}
			cur[s.field] = m
		}
		cur = m
	}
	return nil
}

func removePath(v *any, p string) error {
	segs, err := parsePath(p)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return nil
	}

	cur := *v
	for i := 0; i < len(segs)-1; i++ {
		s := segs[i]
		if s.index != nil {
			arr, ok := cur.([]any)
			if !ok || *s.index < 0 || *s.index >= len(arr) {
				return nil
			}
			cur = arr[*s.index]
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		nxt, ok := obj[s.field]
		if !ok {
			return nil
		}
		cur = nxt
	}

	last := segs[len(segs)-1]
	if last.index != nil {
		arr, ok := cur.([]any)
		if !ok || *last.index < 0 || *last.index >= len(arr) {
			return nil
		}
		arr[*last.index] = nil
		return nil
	}
	obj, ok := cur.(map[string]any)
	if !ok {
		return nil
	}
	delete(obj, last.field)
	return nil
}

func setPath(v *any, p string, val any) error {
	segs, err := parsePath(p)
	if err != nil {
		return err
	}
	if len(segs) == 0 {
		return nil
	}

	cur := *v
	for i := 0; i < len(segs)-1; i++ {
		s := segs[i]
		if s.index != nil {
			arr, ok := cur.([]any)
			if !ok || *s.index < 0 || *s.index >= len(arr) {
				return nil
			}
			cur = arr[*s.index]
			continue
		}
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		nxt, ok := obj[s.field]
		if !ok {
			return nil
		}
		cur = nxt
	}

	last := segs[len(segs)-1]
	if last.index != nil {
		arr, ok := cur.([]any)
		if !ok || *last.index < 0 || *last.index >= len(arr) {
			return nil
		}
		arr[*last.index] = val
		return nil
	}
	obj, ok := cur.(map[string]any)
	if !ok {
		return nil
	}
	obj[last.field] = val
	return nil
}

func truncateDepth(v any, depth int, maxDepth int) any {
	if maxDepth <= 0 {
		return v
	}
	if depth >= maxDepth {
		switch v.(type) {
		case map[string]any, []any:
			return "<truncated>"
		default:
			return v
		}
	}
	switch vv := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, val := range vv {
			out[k] = truncateDepth(val, depth+1, maxDepth)
		}
		return out
	case []any:
		out := make([]any, 0, len(vv))
		for _, it := range vv {
			out = append(out, truncateDepth(it, depth+1, maxDepth))
		}
		return out
	default:
		return v
	}
}

func truncateItems(v any, maxItems int) any {
	if maxItems <= 0 {
		return v
	}
	switch vv := v.(type) {
	case []any:
		if len(vv) <= maxItems {
			return v
		}
		return vv[:maxItems]
	case map[string]any:
		out := map[string]any{}
		for k, val := range vv {
			out[k] = truncateItems(val, maxItems)
		}
		return out
	default:
		return v
	}
}

func JSONSize(v any) int {
	b, _ := json.Marshal(v)
	return len(b)
}
