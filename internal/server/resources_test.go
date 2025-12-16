package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golovatskygroup/mcp-lens/internal/proxy"
	"github.com/golovatskygroup/mcp-lens/pkg/mcp"
)

func TestResourcesListRead(t *testing.T) {
	t.Setenv("MCP_LENS_ARTIFACT_DIR", t.TempDir())
	t.Setenv("MCP_LENS_ARTIFACT_INLINE_MAX_BYTES", "10")

	s := New(context.Background(), proxy.Config{})

	st := s.handler.ArtifactStore()
	if st == nil {
		t.Fatalf("expected artifact store")
	}

	repl, item, err := st.MaybeStore("t", json.RawMessage(`{"id":"1"}`), map[string]any{"big": string(make([]byte, 2000))})
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	if item == nil || repl == nil {
		t.Fatalf("expected stored artifact")
	}

	// list
	listReq := &mcp.Request{JSONRPC: "2.0", ID: 1, Method: "resources/list"}
	listResp := s.handleRequest(listReq)
	if listResp == nil || listResp.Error != nil {
		t.Fatalf("list resp error: %+v", listResp)
	}

	// read
	readParams, _ := json.Marshal(map[string]any{"uri": "artifact://" + item.ID})
	readReq := &mcp.Request{JSONRPC: "2.0", ID: 2, Method: "resources/read", Params: readParams}
	readResp := s.handleRequest(readReq)
	if readResp == nil || readResp.Error != nil {
		t.Fatalf("read resp error: %+v", readResp)
	}
}
