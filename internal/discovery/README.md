# Discovery Package

The `discovery` package implements Perplexity-powered API discovery for the MCP Proxy. It automatically discovers API specifications, endpoints, and documentation using various search strategies.

## Features

- **Multiple Discovery Strategies**:
  - `openapi_first`: Quick OpenAPI spec search with reasoning fallback
  - `full_discovery`: Comprehensive deep research with multiple focus areas
  - `endpoints_only`: Category-by-category endpoint discovery

- **OpenAPI URL Extraction**: Automatic detection and normalization of OpenAPI/Swagger URLs
- **Base URL Detection**: Extracts API base URLs from documentation
- **Authentication Parsing**: Identifies auth methods (Bearer, API Key, OAuth, Basic)
- **Rate Limit Detection**: Extracts rate limiting information
- **In-Memory Caching**: 24-hour TTL cache with configurable size limit
- **GitHub URL Normalization**: Converts GitHub blob URLs to raw content URLs

## Usage

### Basic Usage

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/golovatskygroup/mcp-lens/internal/discovery"
)

func main() {
    // Create client with default settings
    client := discovery.NewClient()

    // Or with custom options
    client = discovery.NewClient(
        discovery.WithPerplexityAPIKey("your-api-key"),
        discovery.WithCacheTTL(12 * time.Hour),
    )

    // Discover API using openapi_first strategy
    result, err := client.DiscoverAPI(context.Background(), discovery.DiscoveryRequest{
        ServiceName: "GitHub",
        Strategy:    discovery.StrategyOpenAPIFirst,
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("OpenAPI URL: %s\n", result.OpenAPIURL)
    fmt.Printf("Base URL: %s\n", result.BaseURL)
    fmt.Printf("Documentation: %s\n", result.Documentation)
}
```

### Strategy Examples

#### OpenAPI First

Quickly finds OpenAPI specs or provides recommendations:

```go
result, err := client.DiscoverAPI(ctx, discovery.DiscoveryRequest{
    ServiceName: "Stripe",
    Strategy:    discovery.StrategyOpenAPIFirst,
})
// Returns: OpenAPI URL or recommendation on how to get the schema
```

#### Full Discovery

Performs comprehensive research with multiple focus areas:

```go
result, err := client.DiscoverAPI(ctx, discovery.DiscoveryRequest{
    ServiceName: "Slack",
    Strategy:    discovery.StrategyFullDiscovery,
    FocusAreas:  []string{
        "Web API endpoints",
        "Authentication methods",
        "Rate limits",
        "Webhook events",
    },
})
// Returns: Detailed information including endpoints, auth, rate limits
```

#### Endpoints Only

Discovers endpoints category by category:

```go
result, err := client.DiscoverAPI(ctx, discovery.DiscoveryRequest{
    ServiceName: "Twilio",
    Strategy:    discovery.StrategyEndpointsOnly,
})
// Returns: Categories and endpoints with HTTP methods and paths
```

### Direct Perplexity API Usage

```go
// Simple search
resp, err := client.Search(ctx, "GitHub API OpenAPI specification")

// Reasoning (more powerful model)
resp, err := client.Reason(ctx, "How to authenticate with GitHub API")

// Deep research with focus areas
resp, err := client.DeepResearch(ctx, "Complete Stripe API documentation", []string{
    "Payment intents",
    "Customers",
    "Subscriptions",
})
```

## Configuration

### Environment Variables

- `PERPLEXITY_API_KEY`: (Optional) Perplexity API key. If not set, attempts to use MCP tools.

### Client Options

```go
type ClientOption func(*Client)

// WithCache sets a custom cache store
func WithCache(cache CacheStore) ClientOption

// WithCacheTTL sets the cache TTL duration
func WithCacheTTL(ttl time.Duration) ClientOption

// WithPerplexityAPIKey sets the Perplexity API key
func WithPerplexityAPIKey(apiKey string) ClientOption
```

## Architecture

### Components

- **Client**: Main discovery client implementing the `DiscoveryClient` interface
- **PerplexityClient**: Handles communication with Perplexity API
- **OpenAPIExtractor**: Extracts and validates OpenAPI specification URLs
- **InMemoryCache**: Thread-safe caching with TTL and size limits
- **Strategies**: Three discovery strategies (openapi_first, full_discovery, endpoints_only)

### Flow Diagrams

#### OpenAPI First Strategy

```
Search("{ServiceName} OpenAPI specification swagger.json")
       ↓
Found URL? → YES → Normalize & Return
       ↓ NO
Reason("How to get {ServiceName} API schema for SDK generation")
       ↓
Return recommendation
```

#### Full Discovery Strategy

```
DeepResearch("{ServiceName} complete API documentation")
    with focus_areas: ["API endpoints", "Authentication", "Rate limits", "Response schemas"]
       ↓
Extract URLs, base URL, auth, rate limits, categories
       ↓
Return comprehensive result
```

#### Endpoints Only Strategy

```
Search("List all {ServiceName} API categories")
       ↓
Extract categories
       ↓
For each category:
    Reason("{ServiceName} {Category} API endpoints")
    Parse endpoints (method, path, description)
       ↓
Aggregate & Return all endpoints
```

## Data Structures

### DiscoveryResult

```go
type DiscoveryResult struct {
    ServiceName    string                 // Service name
    Strategy       DiscoveryStrategy      // Strategy used
    OpenAPIURL     string                 // OpenAPI spec URL (if found)
    BaseURL        string                 // API base URL
    Categories     []string               // API categories
    Endpoints      []EndpointInfo         // Discovered endpoints
    Authentication map[string]string      // Auth info (type, header)
    RateLimits     *RateLimitInfo         // Rate limiting info
    Documentation  string                 // Full documentation text
    Recommendation string                 // Recommendation (if no spec found)
    Metadata       map[string]interface{} // Additional metadata
    CachedAt       time.Time              // Cache timestamp
    Sources        []string               // Source URLs
}
```

### EndpointInfo

```go
type EndpointInfo struct {
    Category    string            // Endpoint category
    Method      string            // HTTP method (GET, POST, etc.)
    Path        string            // Endpoint path
    Description string            // Description
    Parameters  map[string]string // Parameters
}
```

## Testing

The package includes comprehensive unit tests with mocked Perplexity responses:

```bash
go test ./internal/discovery/... -v
```

Test coverage includes:
- URL extraction and validation
- Base URL detection
- GitHub URL normalization
- Authentication parsing
- Rate limit extraction
- Category extraction
- Endpoint parsing
- Cache functionality
- All three discovery strategies

## Performance

- **Caching**: Results are cached for 24 hours (configurable)
- **Concurrency**: Thread-safe cache with proper locking
- **Network**: HTTP client with 60s timeout
- **Validation**: Optional URL validation with HEAD requests

## Error Handling

The package returns detailed errors for:
- Perplexity API failures
- Network timeouts
- Invalid strategies
- Cache errors (non-fatal)

## Future Enhancements

1. **MCP Integration**: Implement MCP tool-based Perplexity access
2. **Persistent Cache**: Add Redis/file-based caching option
3. **Schema Validation**: Validate extracted OpenAPI specs
4. **Parallel Discovery**: Run multiple strategies in parallel
5. **Smart Retry**: Exponential backoff for failed requests

## License

Part of the MCP Proxy project.
