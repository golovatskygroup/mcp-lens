package discovery_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/golovatskygroup/mcp-lens/internal/discovery"
)

// Example demonstrates basic usage of the discovery client
func Example() {
	// Create client with default settings
	client := discovery.NewClient()

	// Discover GitHub API using openapi_first strategy
	result, err := client.DiscoverAPI(context.Background(), discovery.DiscoveryRequest{
		ServiceName: "GitHub",
		Strategy:    discovery.StrategyOpenAPIFirst,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Service: %s\n", result.ServiceName)
	fmt.Printf("Strategy: %s\n", result.Strategy)
	if result.OpenAPIURL != "" {
		fmt.Printf("OpenAPI URL: %s\n", result.OpenAPIURL)
	}
	if result.Recommendation != "" {
		fmt.Printf("Recommendation: %s\n", result.Recommendation)
	}
}

// ExampleClient_DiscoverAPI_openAPIFirst demonstrates the openapi_first strategy
func ExampleClient_DiscoverAPI_openAPIFirst() {
	client := discovery.NewClient(
		discovery.WithPerplexityAPIKey("your-api-key"),
	)

	result, err := client.DiscoverAPI(context.Background(), discovery.DiscoveryRequest{
		ServiceName: "Stripe",
		Strategy:    discovery.StrategyOpenAPIFirst,
	})
	if err != nil {
		log.Fatal(err)
	}

	if result.OpenAPIURL != "" {
		fmt.Println("Found OpenAPI specification URL")
	} else {
		fmt.Println("Recommendation provided instead")
	}
}

// ExampleClient_DiscoverAPI_fullDiscovery demonstrates the full_discovery strategy
func ExampleClient_DiscoverAPI_fullDiscovery() {
	client := discovery.NewClient()

	result, err := client.DiscoverAPI(context.Background(), discovery.DiscoveryRequest{
		ServiceName: "Slack",
		Strategy:    discovery.StrategyFullDiscovery,
		FocusAreas: []string{
			"API endpoints",
			"Authentication",
			"Rate limits",
			"Webhooks",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Base URL: %s\n", result.BaseURL)
	fmt.Printf("Categories: %d\n", len(result.Categories))
	if result.Authentication != nil {
		fmt.Printf("Auth Type: %s\n", result.Authentication["type"])
	}
	if result.RateLimits != nil {
		fmt.Printf("Rate Limits: %s\n", result.RateLimits.Description)
	}
}

// ExampleClient_DiscoverAPI_endpointsOnly demonstrates the endpoints_only strategy
func ExampleClient_DiscoverAPI_endpointsOnly() {
	client := discovery.NewClient()

	result, err := client.DiscoverAPI(context.Background(), discovery.DiscoveryRequest{
		ServiceName: "Twilio",
		Strategy:    discovery.StrategyEndpointsOnly,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d categories\n", len(result.Categories))
	fmt.Printf("Found %d endpoints\n", len(result.Endpoints))

	// Print first few endpoints
	for i, endpoint := range result.Endpoints {
		if i >= 3 {
			break
		}
		fmt.Printf("%s %s - %s\n", endpoint.Method, endpoint.Path, endpoint.Description)
	}
}

// ExampleClient_Search demonstrates simple Perplexity search
func ExampleClient_Search() {
	client := discovery.NewClient()

	resp, err := client.Search(context.Background(), "GitHub API authentication methods")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Query: %s\n", resp.Query)
	fmt.Printf("Answer: %s\n", resp.Answer)
	fmt.Printf("Sources: %d\n", len(resp.Sources))
}

// ExampleClient_Reason demonstrates Perplexity reasoning
func ExampleClient_Reason() {
	client := discovery.NewClient()

	resp, err := client.Reason(context.Background(),
		"What are the best practices for using the Stripe API?")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Answer: %s\n", resp.Answer)
}

// ExampleClient_DeepResearch demonstrates deep research with focus areas
func ExampleClient_DeepResearch() {
	client := discovery.NewClient()

	resp, err := client.DeepResearch(context.Background(),
		"Complete AWS S3 API documentation",
		[]string{
			"Bucket operations",
			"Object operations",
			"Access control",
			"Multipart uploads",
		})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Answer length: %d characters\n", len(resp.Answer))
	fmt.Printf("Sources: %d\n", len(resp.Sources))
}

// ExampleNewClient demonstrates various client configuration options
func ExampleNewClient() {
	// Default client
	client1 := discovery.NewClient()
	_ = client1

	// Client with API key
	client2 := discovery.NewClient(
		discovery.WithPerplexityAPIKey("your-api-key"),
	)
	_ = client2

	// Client with custom cache TTL
	client3 := discovery.NewClient(
		discovery.WithCacheTTL(6 * time.Hour),
	)
	_ = client3

	// Client with custom cache store
	customCache := discovery.NewInMemoryCache(50)
	client4 := discovery.NewClient(
		discovery.WithCache(customCache),
	)
	_ = client4

	// Client with multiple options
	client5 := discovery.NewClient(
		discovery.WithPerplexityAPIKey("your-api-key"),
		discovery.WithCacheTTL(12 * time.Hour),
	)
	_ = client5

	fmt.Println("Created 5 client configurations")
	// Output: Created 5 client configurations
}

// ExampleOpenAPIExtractor demonstrates URL extraction
func ExampleOpenAPIExtractor() {
	extractor := discovery.NewOpenAPIExtractor()

	text := "The API specification is at https://api.example.com/openapi.json"

	// Extract URLs
	urls := extractor.ExtractURLs(text)
	fmt.Printf("Found %d URLs\n", len(urls))
	if len(urls) > 0 {
		fmt.Printf("First URL: %s\n", urls[0])
	}

	// Normalize GitHub URL example
	githubURL := "https://github.com/user/repo/blob/main/openapi.yaml"
	normalized := extractor.NormalizeGitHubURL(githubURL)
	fmt.Printf("Normalized: %s\n", normalized)

	// Output:
	// Found 1 URLs
	// First URL: https://api.example.com/openapi.json
	// Normalized: https://raw.githubusercontent.com/user/repo/main/openapi.yaml
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
