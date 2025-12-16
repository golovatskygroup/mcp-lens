package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Item struct {
	ID         string    `json:"id"`
	Path       string    `json:"artifact_path"`
	SHA256     string    `json:"sha256"`
	Bytes      int       `json:"bytes"`
	Mime       string    `json:"mime"`
	Tool       string    `json:"tool"`
	ArgsDigest string    `json:"args_digest,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	Preview    any       `json:"preview,omitempty"`
}

type Manifest struct {
	Artifacts []Item `json:"artifacts"`
}

type Store struct {
	dir           string
	inlineMax     int
	previewMax    int
	keepIndex     bool
	mu            sync.Mutex
	byID          map[string]Item
	orderedRecent []string
}

type Config struct {
	Dir            string
	InlineMaxBytes int
	PreviewBytes   int
	KeepIndex      bool
}

func ConfigFromEnv() Config {
	dir := strings.TrimSpace(os.Getenv("MCP_LENS_ARTIFACT_DIR"))
	if dir == "" {
		dir = os.TempDir()
	}
	inlineMax := 64 * 1024
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_ARTIFACT_INLINE_MAX_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			inlineMax = n
		}
	}
	previewBytes := 8 * 1024
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_ARTIFACT_PREVIEW_BYTES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			previewBytes = n
		}
	}
	keepIndex := true
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_ARTIFACT_KEEP_INDEX")); v != "" {
		keepIndex = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
	}
	return Config{Dir: dir, InlineMaxBytes: inlineMax, PreviewBytes: previewBytes, KeepIndex: keepIndex}
}

func NewFromEnv() (*Store, error) { return New(ConfigFromEnv()) }

func New(cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Dir) == "" {
		cfg.Dir = os.TempDir()
	}
	if cfg.InlineMaxBytes < 0 {
		cfg.InlineMaxBytes = 0
	}
	if cfg.PreviewBytes <= 0 {
		cfg.PreviewBytes = 8 * 1024
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, fmt.Errorf("artifacts: create dir: %w", err)
	}
	return &Store{
		dir:        cfg.Dir,
		inlineMax:  cfg.InlineMaxBytes,
		previewMax: cfg.PreviewBytes,
		keepIndex:  cfg.KeepIndex,
		byID:       map[string]Item{},
	}, nil
}

func (s *Store) InlineMaxBytes() int { return s.inlineMax }

// StoreBytes stores raw bytes as an artifact (always writes to disk) and returns a lightweight reference object.
// This is useful for non-JSON payloads like logs or HTML exports.
func (s *Store) StoreBytes(tool string, args json.RawMessage, mime string, ext string, b []byte) (replacement any, created *Item, err error) {
	if strings.TrimSpace(tool) == "" {
		tool = "tool"
	}
	if strings.TrimSpace(mime) == "" {
		mime = "application/octet-stream"
	}
	ext = strings.TrimSpace(ext)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])
	id := sha

	now := time.Now().UTC()
	primary := primaryIDFromArgs(args)
	name := sanitizeFileComponent(tool)
	if name == "" {
		name = "tool"
	}
	if primary == "" {
		primary = "query"
	}
	filename := fmt.Sprintf("%s-%s-%s-%s%s", name, sanitizeFileComponent(primary), now.Format("20060102T150405Z"), sha[:12], ext)
	path := filepath.Join(s.dir, filename)

	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, nil, fmt.Errorf("artifacts: write: %w", err)
	}

	item := Item{
		ID:         id,
		Path:       path,
		SHA256:     sha,
		Bytes:      len(b),
		Mime:       mime,
		Tool:       tool,
		ArgsDigest: argsDigest(args),
		CreatedAt:  now,
	}
	if strings.HasPrefix(mime, "text/") {
		item.Preview = string(bytesPreview(b, s.previewMax))
	}

	if s.keepIndex {
		s.mu.Lock()
		s.byID[id] = item
		s.orderedRecent = append(s.orderedRecent, id)
		s.mu.Unlock()
	}

	replacement = map[string]any{
		"artifact_id":   item.ID,
		"artifact_uri":  ArtifactURI(item.ID),
		"artifact_path": item.Path,
		"sha256":        item.SHA256,
		"bytes":         item.Bytes,
		"mime":          item.Mime,
		"preview":       item.Preview,
	}
	return replacement, &item, nil
}

func (s *Store) MaybeStore(tool string, args json.RawMessage, value any) (replacement any, created *Item, err error) {
	// Measure size of what we'd return inline.
	b, err := json.Marshal(value)
	if err != nil {
		// If it can't be marshaled, return as-is.
		return value, nil, nil
	}
	if s.inlineMax <= 0 || len(b) <= s.inlineMax {
		return value, nil, nil
	}

	pretty := b
	if json.Valid(b) {
		var tmp any
		if json.Unmarshal(b, &tmp) == nil {
			if pb, err := json.MarshalIndent(tmp, "", "  "); err == nil {
				pretty = pb
			}
		}
	}

	sum := sha256.Sum256(pretty)
	sha := hex.EncodeToString(sum[:])
	id := sha

	now := time.Now().UTC()
	primary := primaryIDFromArgs(args)
	name := sanitizeFileComponent(tool)
	if name == "" {
		name = "tool"
	}
	if primary == "" {
		primary = "query"
	}
	filename := fmt.Sprintf("%s-%s-%s-%s.json", name, sanitizeFileComponent(primary), now.Format("20060102T150405Z"), sha[:12])
	path := filepath.Join(s.dir, filename)

	if err := os.WriteFile(path, pretty, 0o600); err != nil {
		return value, nil, fmt.Errorf("artifacts: write: %w", err)
	}

	item := Item{
		ID:         id,
		Path:       path,
		SHA256:     sha,
		Bytes:      len(pretty),
		Mime:       "application/json",
		Tool:       tool,
		ArgsDigest: argsDigest(args),
		CreatedAt:  now,
		Preview:    previewValue(pretty, s.previewMax),
	}

	if s.keepIndex {
		s.mu.Lock()
		s.byID[id] = item
		s.orderedRecent = append(s.orderedRecent, id)
		s.mu.Unlock()
	}

	replacement = map[string]any{
		"artifact_id":   item.ID,
		"artifact_uri":  ArtifactURI(item.ID),
		"artifact_path": item.Path,
		"sha256":        item.SHA256,
		"bytes":         item.Bytes,
		"mime":          item.Mime,
		"preview":       item.Preview,
	}
	return replacement, &item, nil
}

func (s *Store) List() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, 0, len(s.byID))
	for _, id := range s.orderedRecent {
		if it, ok := s.byID[id]; ok {
			out = append(out, it)
		}
	}
	return out
}

func (s *Store) Get(id string) (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	it, ok := s.byID[id]
	return it, ok
}

func (s *Store) Read(id string) ([]byte, string, bool) {
	it, ok := s.Get(id)
	if !ok {
		return nil, "", false
	}
	b, err := os.ReadFile(it.Path)
	if err != nil {
		return nil, "", false
	}
	return b, it.Mime, true
}

func argsDigest(args json.RawMessage) string {
	if len(args) == 0 {
		return ""
	}
	var obj any
	if json.Unmarshal(args, &obj) != nil {
		return ""
	}
	b, _ := json.Marshal(obj)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

func previewValue(b []byte, max int) any {
	if max <= 0 {
		return nil
	}
	if len(b) <= max {
		var out any
		if json.Unmarshal(b, &out) == nil {
			return out
		}
		return string(b)
	}
	cut := b[:max]
	// Prefer valid JSON preview when possible.
	var out any
	if json.Unmarshal(cut, &out) == nil {
		return out
	}
	return string(cut) + "…"
}

func bytesPreview(b []byte, max int) []byte {
	if max <= 0 || len(b) == 0 {
		return nil
	}
	if len(b) <= max {
		return b
	}
	return append(append([]byte{}, b[:max]...), []byte("…")...)
}

var reSafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitizeFileComponent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = reSafe.ReplaceAllString(s, "_")
	s = strings.Trim(s, "._-")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func primaryIDFromArgs(args json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(args, &m) != nil || m == nil {
		return ""
	}

	// Common identifiers across tools.
	if v := strings.TrimSpace(getString(m, "uid")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getString(m, "id")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getString(m, "issue")); v != "" {
		return v
	}
	if v := strings.TrimSpace(getString(m, "repo")); v != "" {
		if n, ok := getInt(m, "number"); ok && n > 0 {
			return fmt.Sprintf("%s-%d", v, n)
		}
		return v
	}
	if v := strings.TrimSpace(getString(m, "name")); v != "" {
		return v
	}
	return ""
}

func getString(m map[string]any, k string) string {
	if v, ok := m[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getInt(m map[string]any, k string) (int, bool) {
	switch v := m[k].(type) {
	case float64:
		return int(v), true
	case int:
		return v, true
	default:
		return 0, false
	}
}
