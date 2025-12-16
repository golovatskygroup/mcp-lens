package discovery

import (
	"context"
	"time"
)

// DiscoveryStrategy defines how API discovery should be performed
type DiscoveryStrategy string

const (
	// StrategyOpenAPIFirst tries to find OpenAPI spec first, falls back to reasoning
	StrategyOpenAPIFirst DiscoveryStrategy = "openapi_first"
	// StrategyFullDiscovery performs deep research with multiple focus areas
	StrategyFullDiscovery DiscoveryStrategy = "full_discovery"
	// StrategyEndpointsOnly discovers endpoints category by category
	StrategyEndpointsOnly DiscoveryStrategy = "endpoints_only"
)

// DiscoveryRequest contains parameters for API discovery
type DiscoveryRequest struct {
	ServiceName string            `json:"service_name"`
	Strategy    DiscoveryStrategy `json:"strategy"`
	FocusAreas  []string          `json:"focus_areas,omitempty"`
	CacheKey    string            `json:"cache_key,omitempty"`
}

// DiscoveryResult contains the discovered API information
type DiscoveryResult struct {
	ServiceName   string                 `json:"service_name"`
	Strategy      DiscoveryStrategy      `json:"strategy"`
	OpenAPIURL    string                 `json:"openapi_url,omitempty"`
	BaseURL       string                 `json:"base_url,omitempty"`
	Categories    []string               `json:"categories,omitempty"`
	Endpoints     []EndpointInfo         `json:"endpoints,omitempty"`
	Authentication map[string]string     `json:"authentication,omitempty"`
	RateLimits    *RateLimitInfo         `json:"rate_limits,omitempty"`
	Documentation string                 `json:"documentation,omitempty"`
	Recommendation string                `json:"recommendation,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	CachedAt      time.Time              `json:"cached_at,omitempty"`
	Sources       []string               `json:"sources,omitempty"`
}

// EndpointInfo describes a single API endpoint
type EndpointInfo struct {
	Category    string            `json:"category"`
	Method      string            `json:"method"`
	Path        string            `json:"path"`
	Description string            `json:"description,omitempty"`
	Parameters  map[string]string `json:"parameters,omitempty"`
}

// RateLimitInfo describes API rate limiting
type RateLimitInfo struct {
	RequestsPerSecond int    `json:"requests_per_second,omitempty"`
	RequestsPerMinute int    `json:"requests_per_minute,omitempty"`
	RequestsPerHour   int    `json:"requests_per_hour,omitempty"`
	Description       string `json:"description,omitempty"`
}

// PerplexityResponse represents a response from Perplexity
type PerplexityResponse struct {
	Query    string                 `json:"query"`
	Answer   string                 `json:"answer"`
	Sources  []string               `json:"sources,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DiscoveryClient is the main interface for API discovery
type DiscoveryClient interface {
	// DiscoverAPI performs API discovery using the specified strategy
	DiscoverAPI(ctx context.Context, req DiscoveryRequest) (*DiscoveryResult, error)

	// Search performs a simple Perplexity search
	Search(ctx context.Context, query string) (*PerplexityResponse, error)

	// Reason performs Perplexity reasoning
	Reason(ctx context.Context, query string) (*PerplexityResponse, error)

	// DeepResearch performs deep research with focus areas
	DeepResearch(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error)
}

// CacheStore defines the interface for caching discovery results
type CacheStore interface {
	Get(ctx context.Context, key string) (*DiscoveryResult, error)
	Set(ctx context.Context, key string, result *DiscoveryResult, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// PerplexityBackend defines the interface for Perplexity communication
type PerplexityBackend interface {
	Search(ctx context.Context, query string) (*PerplexityResponse, error)
	Reason(ctx context.Context, query string) (*PerplexityResponse, error)
	DeepResearch(ctx context.Context, query string, focusAreas []string) (*PerplexityResponse, error)
}
