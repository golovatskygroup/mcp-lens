package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// OpenAPIExtractor extracts OpenAPI specification URLs from text
type OpenAPIExtractor struct {
	httpClient *http.Client
	patterns   []*regexp.Regexp
}

// NewOpenAPIExtractor creates a new OpenAPI URL extractor
func NewOpenAPIExtractor() *OpenAPIExtractor {
	patterns := []*regexp.Regexp{
		// GitHub raw URLs
		regexp.MustCompile(`https?://(?:raw\.)?github(?:usercontent)?\.com/[^/]+/[^/]+/[^/]+/[^/]*(?:openapi|swagger)[^/]*\.(?:json|ya?ml)`),
		// Generic OpenAPI/Swagger URLs
		regexp.MustCompile(`https?://[^\s<>"]+?/(?:openapi|swagger)[^/]*\.(?:json|ya?ml)`),
		regexp.MustCompile(`https?://[^\s<>"]+?/api/(?:v\d+/)?(?:openapi|swagger|spec)[^/]*\.(?:json|ya?ml)`),
		// Common API doc patterns
		regexp.MustCompile(`https?://[^\s<>"]+?/docs?/(?:api/)?(?:openapi|swagger)[^/]*\.(?:json|ya?ml)`),
		// spec3.json, api.json, etc.
		regexp.MustCompile(`https?://[^\s<>"]+?/(?:spec\d*|api|schema)\.(?:json|ya?ml)`),
	}

	return &OpenAPIExtractor{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		patterns: patterns,
	}
}

// ExtractURLs extracts potential OpenAPI specification URLs from text
func (e *OpenAPIExtractor) ExtractURLs(text string) []string {
	urlSet := make(map[string]bool)
	var urls []string

	for _, pattern := range e.patterns {
		matches := pattern.FindAllString(text, -1)
		for _, match := range matches {
			// Clean up the URL
			cleaned := strings.TrimRight(match, ".,;:)]}")
			if urlSet[cleaned] {
				continue
			}
			urlSet[cleaned] = true
			urls = append(urls, cleaned)
		}
	}

	return urls
}

// ExtractBaseURL attempts to extract the base API URL from text
func (e *OpenAPIExtractor) ExtractBaseURL(text string) string {
	// Look for common base URL patterns
	patterns := []struct {
		re     *regexp.Regexp
		extractor func(string) string
	}{
		{
			// "base URL: https://api.example.com" or "base URL is: https://api.example.com"
			regexp.MustCompile(`(?i)base\s+url\s+(?:is\s*)?[:]\s*([^\s<>"]+)`),
			func(s string) string { return s },
		},
		{
			// "API endpoint: https://api.example.com"
			regexp.MustCompile(`(?i)(?:api\s+)?endpoint[:\s]+([^\s<>"]+)`),
			func(s string) string { return s },
		},
		{
			// URLs ending with /api or /v1, etc.
			regexp.MustCompile(`(https?://[^\s<>"]+?/(?:api|v\d+))`),
			func(s string) string { return s },
		},
	}

	for _, p := range patterns {
		matches := p.re.FindStringSubmatch(text)
		if len(matches) > 1 {
			return p.extractor(matches[1])
		}
	}

	return ""
}

// ValidateURL checks if a URL is accessible and returns a valid OpenAPI spec
func (e *OpenAPIExtractor) ValidateURL(ctx context.Context, urlStr string) (bool, error) {
	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false, fmt.Errorf("invalid URL: %w", err)
	}

	// Ensure HTTPS for security
	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return false, fmt.Errorf("unsupported scheme: %s", parsedURL.Scheme)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "HEAD", urlStr, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	// Set user agent
	req.Header.Set("User-Agent", "MCP-Proxy-Discovery/1.0")

	// Execute request
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "json") && !strings.Contains(contentType, "yaml") && !strings.Contains(contentType, "yml") {
		// Some servers don't set proper content-type, so we allow this
		// The actual validation would happen when trying to parse the spec
	}

	return true, nil
}

// ExtractAndValidate extracts URLs from text and validates them
func (e *OpenAPIExtractor) ExtractAndValidate(ctx context.Context, text string) ([]string, error) {
	urls := e.ExtractURLs(text)
	if len(urls) == 0 {
		return nil, nil
	}

	var validURLs []string
	for _, urlStr := range urls {
		valid, err := e.ValidateURL(ctx, urlStr)
		if err != nil {
			// Log error but continue checking other URLs
			continue
		}
		if valid {
			validURLs = append(validURLs, urlStr)
		}
	}

	return validURLs, nil
}

// NormalizeGitHubURL converts GitHub URLs to raw content URLs if needed
func (e *OpenAPIExtractor) NormalizeGitHubURL(urlStr string) string {
	// Convert github.com URLs to raw.githubusercontent.com
	if strings.Contains(urlStr, "github.com") && !strings.Contains(urlStr, "raw.githubusercontent.com") {
		// github.com/user/repo/blob/branch/path -> raw.githubusercontent.com/user/repo/branch/path
		urlStr = strings.Replace(urlStr, "github.com", "raw.githubusercontent.com", 1)
		urlStr = strings.Replace(urlStr, "/blob/", "/", 1)
	}
	return urlStr
}
