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
	baseURL string
	apiKey  string
	model   string
	c       *http.Client
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

	return &OpenRouterClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		c:       &http.Client{Timeout: timeout},
	}, nil
}

func (cl *OpenRouterClient) chatCompletion(ctx context.Context, system string, user string) (string, error) {
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

	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cl.baseURL, "/")+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+cl.apiKey)
	req.Header.Set("Content-Type", "application/json")
	// Optional, but helps OpenRouter attribution/routing.
	req.Header.Set("HTTP-Referer", "https://github.com/golovatskygroup/mcp-lens")
	req.Header.Set("X-Title", "mcp-lens")

	resp, err := cl.c.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("openrouter: empty choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("openrouter: empty message content")
	}
	return content, nil
}

// ChatCompletionJSON asks the model to return strict JSON in the assistant content.
func (cl *OpenRouterClient) ChatCompletionJSON(ctx context.Context, system string, user string) ([]byte, error) {
	content, err := cl.chatCompletion(ctx, system, user)
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
	return cl.chatCompletion(ctx, system, user)
}
