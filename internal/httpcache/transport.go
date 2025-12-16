package httpcache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Enabled    bool
	TTL        time.Duration
	MaxEntries int
}

func ConfigFromEnv() Config {
	enabled := strings.TrimSpace(os.Getenv("MCP_LENS_HTTP_CACHE_ENABLED"))
	on := enabled == "1" || strings.EqualFold(enabled, "true") || strings.EqualFold(enabled, "yes")

	ttl := 60 * time.Second
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_HTTP_CACHE_TTL_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			ttl = time.Duration(n) * time.Second
		}
	}

	maxEntries := 512
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_HTTP_CACHE_MAX_ENTRIES")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxEntries = n
		}
	}

	return Config{Enabled: on, TTL: ttl, MaxEntries: maxEntries}
}

type Transport struct {
	base http.RoundTripper
	c    *Cache

	keyHeaders []string
}

func NewTransport(base http.RoundTripper, cfg Config) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if !cfg.Enabled {
		return base
	}
	return &Transport{
		base: base,
		c:    New(cfg.TTL, cfg.MaxEntries),
		keyHeaders: []string{
			"Authorization",
			"Cookie",
			"X-Grafana-Org-Id",
			"CF-Access-Client-Id",
			"CF-Access-Client-Secret",
			"Accept",
		},
	}
}

func NewTransportFromEnv(base http.RoundTripper) http.RoundTripper {
	return NewTransport(base, ConfigFromEnv())
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("httpcache: nil request")
	}
	if !strings.EqualFold(req.Method, http.MethodGet) {
		return t.base.RoundTrip(req)
	}

	key := req.Method + " " + req.URL.String() + " " + fingerprintHeaders(req.Header, t.keyHeaders)

	if ent, ok := t.c.Get(key); ok {
		ttl := t.c.TTL()
		if ttl > 0 && time.Since(ent.storedAt) < ttl {
			return cachedResponse(req, ent), nil
		}

		// TTL expired (or ttl==0): conditional revalidate if we have an ETag.
		if strings.TrimSpace(ent.etag) != "" {
			req2 := req.Clone(req.Context())
			req2.Header = cloneHeader(req.Header)
			req2.Header.Set("If-None-Match", ent.etag)

			resp, err := t.base.RoundTrip(req2)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusNotModified {
				t.c.Touch(key, time.Now())
				return cachedResponse(req, ent), nil
			}

			b, _ := io.ReadAll(resp.Body)
			ent2 := t.c.Put(key, resp.StatusCode, resp.Header, b, time.Now())
			return responseWithBody(req, resp, b, ent2.header), nil
		}
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	ent := t.c.Put(key, resp.StatusCode, resp.Header, b, time.Now())
	return responseWithBody(req, resp, b, ent.header), nil
}

func responseWithBody(req *http.Request, resp *http.Response, body []byte, header http.Header) *http.Response {
	r := &http.Response{
		StatusCode:    resp.StatusCode,
		Status:        resp.Status,
		Header:        cloneHeader(header),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
	}
	return r
}

func cachedResponse(req *http.Request, ent cacheEntry) *http.Response {
	status := ent.status
	if status == 0 {
		status = http.StatusOK
	}
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        cloneHeader(ent.header),
		Body:          io.NopCloser(bytes.NewReader(ent.body)),
		ContentLength: int64(len(ent.body)),
		Request:       req,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
	}
}
