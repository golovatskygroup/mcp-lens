package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type OpenRouterClient struct {
	baseURL          string
	apiKey           string
	model            string
	maxTokensPlan    int
	maxTokensSummary int
	c                *http.Client
}

func NewOpenRouterClientFromEnv() (*OpenRouterClient, error) {
	apiKey := strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	model := strings.TrimSpace(os.Getenv("MCP_LENS_ROUTER_MODEL"))
	if apiKey == "" || model == "" {
		return nil, errors.New("missing OPENROUTER_API_KEY or MCP_LENS_ROUTER_MODEL")
	}

	baseURL := strings.TrimSpace(os.Getenv("MCP_LENS_ROUTER_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}

	timeout := 30 * time.Second
	if ms := strings.TrimSpace(os.Getenv("MCP_LENS_ROUTER_TIMEOUT_MS")); ms != "" {
		if v, err := strconv.Atoi(ms); err == nil && v > 0 {
			timeout = time.Duration(v) * time.Millisecond
		}
	}

	maxPlan := 0
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_ROUTER_MAX_TOKENS_PLAN")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxPlan = n
		}
	}
	maxSummary := 0
	if v := strings.TrimSpace(os.Getenv("MCP_LENS_ROUTER_MAX_TOKENS_SUMMARY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxSummary = n
		}
	}

	return &OpenRouterClient{
		baseURL:          baseURL,
		apiKey:           apiKey,
		model:            model,
		maxTokensPlan:    maxPlan,
		maxTokensSummary: maxSummary,
		c:                &http.Client{Timeout: timeout},
	}, nil
}

func (cl *OpenRouterClient) chatCompletion(ctx context.Context, system string, user string, kind string) (string, string, error) {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}

	body := map[string]any{
		"model": cl.model,
		"messages": []msg{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		"temperature": 0,
	}
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "plan":
		if cl.maxTokensPlan > 0 {
			body["max_tokens"] = cl.maxTokensPlan
		}
	case "summary":
		if cl.maxTokensSummary > 0 {
			body["max_tokens"] = cl.maxTokensSummary
		}
	}

	b, err := json.Marshal(body)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cl.baseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+cl.apiKey)
	req.Header.Set("Content-Type", "application/json")
	// Optional, but helps OpenRouter attribution/routing.
	req.Header.Set("HTTP-Referer", "https://github.com/golovatskygroup/mcp-lens")
	req.Header.Set("X-Title", "mcp-lens")

	resp, err := cl.c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("openrouter error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", "", err
	}
	if len(parsed.Choices) == 0 {
		return "", "", errors.New("openrouter: empty choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", "", errors.New("openrouter: empty message content")
	}
	return content, strings.TrimSpace(parsed.Choices[0].FinishReason), nil
}

// ChatCompletionJSON asks the model to return strict JSON in the assistant content.
func (cl *OpenRouterClient) ChatCompletionJSON(ctx context.Context, system string, user string) ([]byte, error) {
	content, _, err := cl.chatCompletion(ctx, system, user, "plan")
	if err != nil {
		return nil, err
	}

	// The model should return pure JSON. We still defensively try to extract a JSON object/array
	// from the assistant content (handles occasional leading/trailing prose or markdown fences).
	startObj := strings.Index(content, "{")
	startArr := strings.Index(content, "[")
	start := startObj
	if start < 0 || (startArr >= 0 && startArr < start) {
		start = startArr
	}

	var end int
	if start == startArr {
		end = strings.LastIndex(content, "]")
	} else {
		end = strings.LastIndex(content, "}")
	}
	if start >= 0 && end > start {
		content = content[start : end+1]
	}

	return []byte(strings.TrimSpace(content)), nil
}

// ChatCompletionText returns assistant content as plain text (no JSON extraction).
func (cl *OpenRouterClient) ChatCompletionText(ctx context.Context, system string, user string) (string, error) {
	content, _, err := cl.chatCompletion(ctx, system, user, "summary")
	return content, err
}

func (cl *OpenRouterClient) ChatCompletionTextWithFinishReason(ctx context.Context, system string, user string) (string, string, error) {
	return cl.chatCompletion(ctx, system, user, "summary")
}

func (cl *OpenRouterClient) ChatCompletionJSONWithFinishReason(ctx context.Context, system string, user string) ([]byte, string, error) {
	content, finish, err := cl.chatCompletion(ctx, system, user, "plan")
	if err != nil {
		return nil, finish, err
	}

	startObj := strings.Index(content, "{")
	startArr := strings.Index(content, "[")
	start := startObj
	if start < 0 || (startArr >= 0 && startArr < start) {
		start = startArr
	}

	var end int
	if start == startArr {
		end = strings.LastIndex(content, "]")
	} else {
		end = strings.LastIndex(content, "}")
	}
	if start >= 0 && end > start {
		content = content[start : end+1]
	}
	return []byte(strings.TrimSpace(content)), finish, nil
}
