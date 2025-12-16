package artifacts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreMaybeStoreLargeResult(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{Dir: dir, InlineMaxBytes: 10, PreviewBytes: 64, KeepIndex: true})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	v := map[string]any{"a": "this is definitely larger than 10 bytes"}
	args, _ := json.Marshal(map[string]any{"uid": "abc"})

	repl, item, err := s.MaybeStore("grafana_get_dashboard", args, v)
	if err != nil {
		t.Fatalf("MaybeStore: %v", err)
	}
	if item == nil {
		t.Fatalf("expected artifact to be created")
	}
	if _, ok := repl.(map[string]any); !ok {
		t.Fatalf("expected replacement map")
	}
	if item.Path == "" || item.SHA256 == "" {
		t.Fatalf("expected path and sha")
	}
	if _, err := os.Stat(item.Path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if filepath.Dir(item.Path) != dir {
		t.Fatalf("expected artifact in temp dir")
	}
	if got, ok := s.Get(item.ID); !ok || got.SHA256 != item.SHA256 {
		t.Fatalf("expected artifact indexed")
	}
}

func TestParseArtifactURI(t *testing.T) {
	id, ok := ParseArtifactURI("artifact://abc")
	if !ok || id != "abc" {
		t.Fatalf("unexpected parse: %v %q", ok, id)
	}
}
