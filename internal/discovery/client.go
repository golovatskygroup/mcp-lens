package discovery

import (
	"context"
	"fmt"
	"time"
)

// Client implements the DiscoveryClient interface
type Client struct {
	perplexity PerplexityBackend
	extractor  *OpenAPIExtractor
	cache      CacheStore
	cacheTTL   time.Duration
}

// ClientOption configures the Client
type ClientOption func(*Client)

// WithCache sets a custom cache store
func WithCache(cache CacheStore) ClientOption {
	return func(c *Client) {
		c.cache = cache
	}
}

// WithCacheTTL sets the cache TTL duration
func WithCacheTTL(ttl time.Duration) ClientOption {
	return func(c *Client) {
		c.cacheTTL = ttl
	}
}

// WithPerplexityAPIKey sets the Perplexity API key
func WithPerplexityAPIKey(apiKey string) ClientOption {
	return func(c *Client) {
		c.perplexity = NewPerplexityClientWithKey(apiKey)
	}
}

// NewClient creates a new discovery client
func NewClient(opts ...ClientOption) *Client {
	client := &Client{
		perplexity: NewPerplexityClient(),
		extractor:  NewOpenAPIExtractor(),
		cache:      NewInMemoryCache(100),
		cacheTTL:   24 * time.Hour, // Default 24 hours
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// DiscoverAPI performs API discovery using the specified strategy
func (c *Client) DiscoverAPI(ctx context.Context, req DiscoveryRequest) (*DiscoveryResult, error) {
	// Generate cache key
	cacheKey := req.CacheKey
	if cacheKey == "" {
		cacheKey = fmt.Sprintf("%s:%s", req.ServiceName, req.Strategy)
	}

	// Try to get from cache
	if cached, err := c.cache.Get(ctx, cacheKey); err == nil {
		return cached, nil
	}

	// Execute strategy
	var result *DiscoveryResult
	var err error

	switch req.Strategy {
	case StrategyOpenAPIFirst:
		result, err = c.executeOpenAPIFirst(ctx, req)
	case StrategyFullDiscovery:
		result, err = c.executeFullDiscovery(ctx, req)
	case StrategyEndpointsOnly:
		result, err = c.executeEndpointsOnly(ctx, req)
	default:
		return nil, fmt.Errorf("unknown strategy: %s", req.Strategy)
	}

	if err != nil {
		return nil, err
	}

	// Cache the result
	if err := c.cache.Set(ctx, cacheKey, result, c.cacheTTL); err != nil {
		// Log error but don't fail - caching is optional
		_ = err
	}

	return result, nil
}

// Search performs a simple Perplexity search
func (c *Client) Search(ctx context.Context, query string) (*PerplexityResponse, error) {
	return c.perplexity.Search(ctx, query)
}

// Reason performs Perplexity reasoning
func (c *Client) Reason(ctx context.Context, query string) (*PerplexityResponse, error) {
	return c.perplexity.Reason(ctx, query)
}

// DeepResearch performs deep research with focus areas
func (c *Client) DeepResearch(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error) {
	return c.perplexity.DeepResearch(ctx, query, focusAreas)
}

// ClearCache clears the discovery cache
func (c *Client) ClearCache(ctx context.Context, key string) error {
	if key == "" {
		// Clear all cache if no key specified
		if cache, ok := c.cache.(*InMemoryCache); ok {
			cache.Clear()
			return nil
		}
	}
	return c.cache.Delete(ctx, key)
}
