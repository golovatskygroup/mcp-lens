package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenRouterClientFinishReasonAndMaxTokens(t *testing.T) {
	var gotMaxTokens any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotMaxTokens = body["max_tokens"]

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message":       map[string]any{"content": "hello"},
					"finish_reason": "length",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	cl := &OpenRouterClient{
		baseURL:          srv.URL,
		apiKey:           "x",
		model:            "m",
		maxTokensSummary: 123,
		c:                &http.Client{Timeout: 5 * time.Second},
	}

	content, finish, err := cl.ChatCompletionTextWithFinishReason(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("ChatCompletionTextWithFinishReason: %v", err)
	}
	if content != "hello" {
		t.Fatalf("content: %q", content)
	}
	if finish != "length" {
		t.Fatalf("finish: %q", finish)
	}
	if gotMaxTokens != float64(123) { // JSON numbers decode to float64
		t.Fatalf("expected max_tokens=123, got %v", gotMaxTokens)
	}
}

func TestSummarizeFallbackOnLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message":       map[string]any{"content": strings.Repeat("x", 10)},
					"finish_reason": "length",
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	cl := &OpenRouterClient{
		baseURL: srv.URL,
		apiKey:  "x",
		model:   "m",
		c:       &http.Client{Timeout: 5 * time.Second},
	}

	res := RouterResult{ExecutedSteps: []ExecutedStep{{Name: "t", OK: true, Result: map[string]any{"k": "v"}}}}
	out, err := Summarize(context.Background(), cl, "task", res)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !strings.Contains(out, "Summary truncated") {
		t.Fatalf("expected fallback summary, got %q", out)
	}
}
