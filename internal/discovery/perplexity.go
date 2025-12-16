package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// PerplexityClient implements communication with Perplexity API
type PerplexityClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	useMCP     bool // If true, use MCP tools instead of direct API
}

// NewPerplexityClient creates a new Perplexity client
func NewPerplexityClient() *PerplexityClient {
	apiKey := os.Getenv("PERPLEXITY_API_KEY")
	useMCP := apiKey == ""

	return &PerplexityClient{
		apiKey:  apiKey,
		baseURL: "https://api.perplexity.ai",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		useMCP: useMCP,
	}
}

// NewPerplexityClientWithKey creates a client with explicit API key
func NewPerplexityClientWithKey(apiKey string) *PerplexityClient {
	return &PerplexityClient{
		apiKey:  apiKey,
		baseURL: "https://api.perplexity.ai",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		useMCP: false,
	}
}

// Search performs a Perplexity search
func (c *PerplexityClient) Search(ctx context.Context, query string) (*PerplexityResponse, error) {
	if c.useMCP {
		return c.searchViaMCP(ctx, query)
	}
	return c.searchViaAPI(ctx, query, "llama-3.1-sonar-small-128k-online")
}

// Reason performs Perplexity reasoning (uses a more powerful model)
func (c *PerplexityClient) Reason(ctx context.Context, query string) (*PerplexityResponse, error) {
	if c.useMCP {
		return c.reasonViaMCP(ctx, query)
	}
	return c.searchViaAPI(ctx, query, "llama-3.1-sonar-large-128k-online")
}

// DeepResearch performs deep research with focus areas
func (c *PerplexityClient) DeepResearch(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error) {
	if c.useMCP {
		return c.deepResearchViaMCP(ctx, query, focusAreas)
	}

	// For API mode, enhance the query with focus areas
	enhancedQuery := query
	if len(focusAreas) > 0 {
		enhancedQuery = fmt.Sprintf("%s\n\nFocus on:\n- %s", query, strings.Join(focusAreas, "\n- "))
	}

	return c.searchViaAPI(ctx, enhancedQuery, "llama-3.1-sonar-huge-128k-online")
}

// searchViaAPI performs a search using Perplexity's HTTP API
func (c *PerplexityClient) searchViaAPI(ctx context.Context, query, model string) (*PerplexityResponse, error) {
	reqBody := map[string]interface{}{
		"model": model,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": query,
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	var apiResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Citations []string `json:"citations"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &PerplexityResponse{
		Query:   query,
		Answer:  apiResp.Choices[0].Message.Content,
		Sources: apiResp.Citations,
	}, nil
}

// searchViaMCP performs search using MCP tools
func (c *PerplexityClient) searchViaMCP(ctx context.Context, query string) (*PerplexityResponse, error) {
	// This would integrate with MCP tools if available
	// For now, return an error indicating MCP is not yet implemented
	return nil, fmt.Errorf("MCP integration not yet implemented - please set PERPLEXITY_API_KEY environment variable")
}

// reasonViaMCP performs reasoning using MCP tools
func (c *PerplexityClient) reasonViaMCP(ctx context.Context, query string) (*PerplexityResponse, error) {
	return nil, fmt.Errorf("MCP integration not yet implemented - please set PERPLEXITY_API_KEY environment variable")
}

// deepResearchViaMCP performs deep research using MCP tools
func (c *PerplexityClient) deepResearchViaMCP(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error) {
	return nil, fmt.Errorf("MCP integration not yet implemented - please set PERPLEXITY_API_KEY environment variable")
}
