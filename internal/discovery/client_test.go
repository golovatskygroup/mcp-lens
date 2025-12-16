package discovery

import (
	"context"
	"testing"
	"time"
)

// MockPerplexityClient is a mock implementation for testing
type MockPerplexityClient struct {
	searchFunc       func(ctx context.Context, query string) (*PerplexityResponse, error)
	reasonFunc       func(ctx context.Context, query string) (*PerplexityResponse, error)
	deepResearchFunc func(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error)
}

func (m *MockPerplexityClient) Search(ctx context.Context, query string) (*PerplexityResponse, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, query)
	}
	return &PerplexityResponse{
		Query:  query,
		Answer: "Mock search result",
	}, nil
}

func (m *MockPerplexityClient) Reason(ctx context.Context, query string) (*PerplexityResponse, error) {
	if m.reasonFunc != nil {
		return m.reasonFunc(ctx, query)
	}
	return &PerplexityResponse{
		Query:  query,
		Answer: "Mock reasoning result",
	}, nil
}

func (m *MockPerplexityClient) DeepResearch(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error) {
	if m.deepResearchFunc != nil {
		return m.deepResearchFunc(ctx, query, focusAreas)
	}
	return &PerplexityResponse{
		Query:  query,
		Answer: "Mock deep research result",
	}, nil
}

func TestClientWithMock(t *testing.T) {
	ctx := context.Background()

	t.Run("DiscoverAPI with cache", func(t *testing.T) {
		mockPerplexity := &MockPerplexityClient{
			searchFunc: func(ctx context.Context, query string) (*PerplexityResponse, error) {
				return &PerplexityResponse{
					Query:  query,
					Answer: "The OpenAPI spec is at https://api.example.com/openapi.json",
					Sources: []string{"https://docs.example.com"},
				}, nil
			},
		}

		client := NewClient()
		client.perplexity = mockPerplexity

		req := DiscoveryRequest{
			ServiceName: "TestAPI",
			Strategy:    StrategyOpenAPIFirst,
		}

		// First call - should hit the mock
		result1, err := client.DiscoverAPI(ctx, req)
		if err != nil {
			t.Fatalf("DiscoverAPI failed: %v", err)
		}

		if result1.ServiceName != "TestAPI" {
			t.Errorf("expected service name TestAPI, got %s", result1.ServiceName)
		}

		if result1.OpenAPIURL != "https://api.example.com/openapi.json" {
			t.Errorf("expected OpenAPI URL, got %s", result1.OpenAPIURL)
		}

		// Second call with same params - should hit cache
		result2, err := client.DiscoverAPI(ctx, req)
		if err != nil {
			t.Fatalf("DiscoverAPI failed on cached call: %v", err)
		}

		if !result2.CachedAt.IsZero() {
			// Verify it came from cache
			if result2.ServiceName != result1.ServiceName {
				t.Error("cached result differs from original")
			}
		}
	})

	t.Run("OpenAPIFirst strategy - found URL", func(t *testing.T) {
		mockPerplexity := &MockPerplexityClient{
			searchFunc: func(ctx context.Context, query string) (*PerplexityResponse, error) {
				return &PerplexityResponse{
					Query:  query,
					Answer: "The specification is available at https://github.com/example/repo/blob/main/openapi.yaml",
				}, nil
			},
		}

		client := NewClient()
		client.perplexity = mockPerplexity

		req := DiscoveryRequest{
			ServiceName: "GitHub",
			Strategy:    StrategyOpenAPIFirst,
			CacheKey:    "test-no-cache", // Unique key to avoid cache
		}

		result, err := client.DiscoverAPI(ctx, req)
		if err != nil {
			t.Fatalf("DiscoverAPI failed: %v", err)
		}

		if result.OpenAPIURL == "" {
			t.Error("expected OpenAPI URL to be found")
		}
	})

	t.Run("OpenAPIFirst strategy - no URL, fallback to reason", func(t *testing.T) {
		mockPerplexity := &MockPerplexityClient{
			searchFunc: func(ctx context.Context, query string) (*PerplexityResponse, error) {
				return &PerplexityResponse{
					Query:  query,
					Answer: "The API documentation is available on the website",
				}, nil
			},
			reasonFunc: func(ctx context.Context, query string) (*PerplexityResponse, error) {
				return &PerplexityResponse{
					Query:  query,
					Answer: "To get the API schema, contact support or check the developer portal at https://dev.example.com",
				}, nil
			},
		}

		client := NewClient()
		client.perplexity = mockPerplexity

		req := DiscoveryRequest{
			ServiceName: "NoSpecAPI",
			Strategy:    StrategyOpenAPIFirst,
			CacheKey:    "test-no-spec",
		}

		result, err := client.DiscoverAPI(ctx, req)
		if err != nil {
			t.Fatalf("DiscoverAPI failed: %v", err)
		}

		if result.Recommendation == "" {
			t.Error("expected recommendation when no URL found")
		}
	})

	t.Run("FullDiscovery strategy", func(t *testing.T) {
		mockPerplexity := &MockPerplexityClient{
			deepResearchFunc: func(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error) {
				return &PerplexityResponse{
					Query: query,
					Answer: `Complete API Documentation:
Base URL: https://api.example.com/v1
Authentication: Bearer token
Rate limit: 100 requests per minute

Categories:
- Users
- Posts
- Comments`,
					Sources: []string{"https://docs.example.com"},
				}, nil
			},
		}

		client := NewClient()
		client.perplexity = mockPerplexity

		req := DiscoveryRequest{
			ServiceName: "CompleteAPI",
			Strategy:    StrategyFullDiscovery,
			CacheKey:    "test-full-discovery",
		}

		result, err := client.DiscoverAPI(ctx, req)
		if err != nil {
			t.Fatalf("DiscoverAPI failed: %v", err)
		}

		if result.BaseURL != "https://api.example.com/v1" {
			t.Errorf("expected base URL, got %s", result.BaseURL)
		}

		if len(result.Categories) == 0 {
			t.Error("expected categories to be extracted")
		}

		if result.Authentication["type"] != "bearer" {
			t.Error("expected bearer authentication to be detected")
		}
	})

	t.Run("EndpointsOnly strategy", func(t *testing.T) {
		callCount := 0
		mockPerplexity := &MockPerplexityClient{
			searchFunc: func(ctx context.Context, query string) (*PerplexityResponse, error) {
				return &PerplexityResponse{
					Query: query,
					Answer: `API Categories:
- Users
- Posts`,
				}, nil
			},
			reasonFunc: func(ctx context.Context, query string) (*PerplexityResponse, error) {
				callCount++
				if callCount == 1 {
					return &PerplexityResponse{
						Query: query,
						Answer: `Users endpoints:
GET /users - List users
POST /users - Create user`,
					}, nil
				}
				return &PerplexityResponse{
					Query: query,
					Answer: `Posts endpoints:
GET /posts - List posts
POST /posts - Create post`,
				}, nil
			},
		}

		client := NewClient()
		client.perplexity = mockPerplexity

		req := DiscoveryRequest{
			ServiceName: "EndpointsAPI",
			Strategy:    StrategyEndpointsOnly,
			CacheKey:    "test-endpoints-only",
		}

		result, err := client.DiscoverAPI(ctx, req)
		if err != nil {
			t.Fatalf("DiscoverAPI failed: %v", err)
		}

		if len(result.Categories) != 2 {
			t.Errorf("expected 2 categories, got %d", len(result.Categories))
		}

		if len(result.Endpoints) == 0 {
			t.Error("expected endpoints to be discovered")
		}
	})

	t.Run("ClearCache", func(t *testing.T) {
		client := NewClient()

		// Add to cache
		result := &DiscoveryResult{
			ServiceName: "CacheTest",
		}
		client.cache.Set(ctx, "test-clear-cache", result, 1*time.Hour)

		// Clear specific key
		err := client.ClearCache(ctx, "test-clear-cache")
		if err != nil {
			t.Fatalf("ClearCache failed: %v", err)
		}

		// Should be gone
		_, err = client.cache.Get(ctx, "test-clear-cache")
		if err == nil {
			t.Error("expected cache miss after clear")
		}
	})

	t.Run("Invalid strategy", func(t *testing.T) {
		client := NewClient()

		req := DiscoveryRequest{
			ServiceName: "Test",
			Strategy:    "invalid_strategy",
		}

		_, err := client.DiscoverAPI(ctx, req)
		if err == nil {
			t.Error("expected error for invalid strategy")
		}
	})
}
