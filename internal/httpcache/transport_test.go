package httpcache

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
)

func TestTransportETagRevalidate304(t *testing.T) {
	t.Setenv("MCP_LENS_HTTP_CACHE_ENABLED", "1")
	t.Setenv("MCP_LENS_HTTP_CACHE_TTL_SECONDS", "0")
	t.Setenv("MCP_LENS_HTTP_CACHE_MAX_ENTRIES", "32")

	var gotIfNoneMatch atomic.Value
	var hitCount atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		if inm := r.Header.Get("If-None-Match"); inm != "" {
			gotIfNoneMatch.Store(inm)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(srv.Close)

	cl := &http.Client{Transport: NewTransportFromEnv(nil)}

	req1, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/x", nil)
	resp1, err := cl.Do(req1)
	if err != nil {
		t.Fatalf("first request: %v", err)
	}
	b1, _ := io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()
	if string(b1) != "hello" {
		t.Fatalf("unexpected body: %q", string(b1))
	}

	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/x", nil)
	resp2, err := cl.Do(req2)
	if err != nil {
		t.Fatalf("second request: %v", err)
	}
	b2, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if string(b2) != "hello" {
		t.Fatalf("expected cached body after 304, got %q", string(b2))
	}
	if v := gotIfNoneMatch.Load(); v == nil || v.(string) != `"v1"` {
		t.Fatalf("expected If-None-Match to be sent, got %v", v)
	}
	if n := hitCount.Load(); n != 2 {
		t.Fatalf("expected 2 server hits, got %d", n)
	}
}

func TestTransportCacheKeySeparatesAuth(t *testing.T) {
	t.Setenv("MCP_LENS_HTTP_CACHE_ENABLED", "1")
	t.Setenv("MCP_LENS_HTTP_CACHE_TTL_SECONDS", "60")
	t.Setenv("MCP_LENS_HTTP_CACHE_MAX_ENTRIES", "32")

	var hitCount atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitCount.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	cl := &http.Client{Transport: NewTransportFromEnv(nil)}

	req1, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req1.Header.Set("Authorization", "Bearer A")
	resp1, err := cl.Do(req1)
	if err != nil {
		t.Fatalf("request 1: %v", err)
	}
	_, _ = io.ReadAll(resp1.Body)
	_ = resp1.Body.Close()

	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req2.Header.Set("Authorization", "Bearer B")
	resp2, err := cl.Do(req2)
	if err != nil {
		t.Fatalf("request 2: %v", err)
	}
	_, _ = io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()

	if n := hitCount.Load(); n != 2 {
		t.Fatalf("expected 2 server hits for different auth, got %d", n)
	}
}

func TestConfigFromEnvDisabled(t *testing.T) {
	_ = os.Unsetenv("MCP_LENS_HTTP_CACHE_ENABLED")
	cfg := ConfigFromEnv()
	if cfg.Enabled {
		t.Fatalf("expected disabled by default")
	}
}
