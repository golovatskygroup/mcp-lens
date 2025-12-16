package discovery

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// executeOpenAPIFirst implements the openapi_first strategy
func (c *Client) executeOpenAPIFirst(ctx context.Context, req DiscoveryRequest) (*DiscoveryResult, error) {
	result := &DiscoveryResult{
		ServiceName: req.ServiceName,
		Strategy:    StrategyOpenAPIFirst,
		Metadata:    make(map[string]interface{}),
	}

	// Step 1: Search for OpenAPI specification
	searchQuery := fmt.Sprintf("%s OpenAPI specification swagger.json", req.ServiceName)
	searchResp, err := c.perplexity.Search(ctx, searchQuery)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	result.Sources = append(result.Sources, searchResp.Sources...)

	// Extract OpenAPI URLs from search results (without validation for speed)
	urls := c.extractor.ExtractURLs(searchResp.Answer)

	if len(urls) > 0 {
		// Found OpenAPI URL(s)
		result.OpenAPIURL = c.extractor.NormalizeGitHubURL(urls[0])
		if len(urls) > 1 {
			normalizedAlts := make([]string, len(urls)-1)
			for i, url := range urls[1:] {
				normalizedAlts[i] = c.extractor.NormalizeGitHubURL(url)
			}
			result.Metadata["alternative_urls"] = normalizedAlts
		}

		// Try to extract base URL from the answer
		baseURL := c.extractor.ExtractBaseURL(searchResp.Answer)
		if baseURL != "" {
			result.BaseURL = baseURL
		}

		result.Documentation = searchResp.Answer
		return result, nil
	}

	// Step 2: No URL found, use reasoning to get recommendations
	reasonQuery := fmt.Sprintf("How to get %s API schema for SDK generation? Provide specific steps and URLs if available.", req.ServiceName)
	reasonResp, err := c.perplexity.Reason(ctx, reasonQuery)
	if err != nil {
		return nil, fmt.Errorf("reasoning failed: %w", err)
	}

	result.Sources = append(result.Sources, reasonResp.Sources...)
	result.Recommendation = reasonResp.Answer
	result.Documentation = searchResp.Answer + "\n\n" + reasonResp.Answer

	// Try extracting URLs from reasoning response too
	urls = c.extractor.ExtractURLs(reasonResp.Answer)
	if len(urls) > 0 {
		result.OpenAPIURL = c.extractor.NormalizeGitHubURL(urls[0])
	}

	baseURL := c.extractor.ExtractBaseURL(reasonResp.Answer)
	if baseURL != "" {
		result.BaseURL = baseURL
	}

	return result, nil
}

// executeFullDiscovery implements the full_discovery strategy
func (c *Client) executeFullDiscovery(ctx context.Context, req DiscoveryRequest) (*DiscoveryResult, error) {
	result := &DiscoveryResult{
		ServiceName: req.ServiceName,
		Strategy:    StrategyFullDiscovery,
		Metadata:    make(map[string]interface{}),
	}

	// Default focus areas if not provided
	focusAreas := req.FocusAreas
	if len(focusAreas) == 0 {
		focusAreas = []string{
			"API endpoints",
			"Authentication methods",
			"Rate limits",
			"Response schemas",
			"Base URL",
		}
	}

	// Perform deep research
	query := fmt.Sprintf("%s complete API documentation", req.ServiceName)
	deepResp, err := c.perplexity.DeepResearch(ctx, query, focusAreas)
	if err != nil {
		return nil, fmt.Errorf("deep research failed: %w", err)
	}

	result.Sources = append(result.Sources, deepResp.Sources...)
	result.Documentation = deepResp.Answer

	// Extract OpenAPI URLs
	urls := c.extractor.ExtractURLs(deepResp.Answer)
	if len(urls) > 0 {
		result.OpenAPIURL = c.extractor.NormalizeGitHubURL(urls[0])
		if len(urls) > 1 {
			normalizedAlts := make([]string, len(urls)-1)
			for i, url := range urls[1:] {
				normalizedAlts[i] = c.extractor.NormalizeGitHubURL(url)
			}
			result.Metadata["alternative_urls"] = normalizedAlts
		}
	}

	// Extract base URL
	baseURL := c.extractor.ExtractBaseURL(deepResp.Answer)
	if baseURL != "" {
		result.BaseURL = baseURL
	}

	// Parse authentication info
	result.Authentication = c.extractAuthentication(deepResp.Answer)

	// Parse rate limits
	result.RateLimits = c.extractRateLimits(deepResp.Answer)

	// Extract categories from the answer
	result.Categories = c.extractCategories(deepResp.Answer)

	return result, nil
}

// executeEndpointsOnly implements the endpoints_only strategy
func (c *Client) executeEndpointsOnly(ctx context.Context, req DiscoveryRequest) (*DiscoveryResult, error) {
	result := &DiscoveryResult{
		ServiceName: req.ServiceName,
		Strategy:    StrategyEndpointsOnly,
		Metadata:    make(map[string]interface{}),
		Endpoints:   []EndpointInfo{},
	}

	// Step 1: Search for API categories
	categoriesQuery := fmt.Sprintf("List all %s API categories and endpoint groups", req.ServiceName)
	categoriesResp, err := c.perplexity.Search(ctx, categoriesQuery)
	if err != nil {
		return nil, fmt.Errorf("categories search failed: %w", err)
	}

	result.Sources = append(result.Sources, categoriesResp.Sources...)
	categories := c.extractCategories(categoriesResp.Answer)
	result.Categories = categories

	// Step 2: For each category, discover endpoints
	for _, category := range categories {
		endpointsQuery := fmt.Sprintf("%s %s API endpoints with HTTP methods and paths", req.ServiceName, category)
		endpointsResp, err := c.perplexity.Reason(ctx, endpointsQuery)
		if err != nil {
			// Log error but continue with other categories
			if result.Metadata["errors"] == nil {
				result.Metadata["errors"] = []string{}
			}
			result.Metadata["errors"] = append(result.Metadata["errors"].([]string), fmt.Sprintf("%s: %v", category, err))
			continue
		}

		result.Sources = append(result.Sources, endpointsResp.Sources...)

		// Parse endpoints from the response
		endpoints := c.parseEndpoints(category, endpointsResp.Answer)
		result.Endpoints = append(result.Endpoints, endpoints...)
	}

	// Extract base URL if mentioned
	baseURL := c.extractor.ExtractBaseURL(categoriesResp.Answer)
	if baseURL != "" {
		result.BaseURL = baseURL
	}

	result.Documentation = categoriesResp.Answer

	return result, nil
}

// extractAuthentication parses authentication information from text
func (c *Client) extractAuthentication(text string) map[string]string {
	auth := make(map[string]string)

	lowerText := strings.ToLower(text)

	// Check for different auth types
	if strings.Contains(lowerText, "bearer token") || strings.Contains(lowerText, "bearer auth") {
		auth["type"] = "bearer"
	} else if strings.Contains(lowerText, "api key") {
		auth["type"] = "api_key"
	} else if strings.Contains(lowerText, "oauth") {
		auth["type"] = "oauth"
	} else if strings.Contains(lowerText, "basic auth") {
		auth["type"] = "basic"
	}

	// Try to extract header name
	if strings.Contains(lowerText, "authorization:") {
		auth["header"] = "Authorization"
	} else if strings.Contains(lowerText, "x-api-key") {
		auth["header"] = "X-API-Key"
	}

	return auth
}

// extractRateLimits parses rate limit information from text
func (c *Client) extractRateLimits(text string) *RateLimitInfo {
	info := &RateLimitInfo{}

	// Simple regex-based extraction (can be improved)
	lines := strings.Split(text, "\n")
	foundRateLimit := false
	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "rate limit") || strings.Contains(lowerLine, "requests per") {
			if info.Description == "" {
				info.Description = strings.TrimSpace(line)
			}
			foundRateLimit = true

			// Try to extract numbers with regex
			if strings.Contains(lowerLine, "per second") {
				// Extract number before "requests per second" or "per second"
				matches := regexp.MustCompile(`(\d+)\s*(?:requests\s+)?per\s+second`).FindStringSubmatch(lowerLine)
				if len(matches) > 1 {
					var num int
					fmt.Sscanf(matches[1], "%d", &num)
					if num > 0 {
						info.RequestsPerSecond = num
					}
				}
			} else if strings.Contains(lowerLine, "per minute") {
				matches := regexp.MustCompile(`(\d+)\s*(?:requests\s+)?per\s+minute`).FindStringSubmatch(lowerLine)
				if len(matches) > 1 {
					var num int
					fmt.Sscanf(matches[1], "%d", &num)
					if num > 0 {
						info.RequestsPerMinute = num
					}
				}
			} else if strings.Contains(lowerLine, "per hour") {
				matches := regexp.MustCompile(`(\d+)\s*(?:requests\s+)?per\s+hour`).FindStringSubmatch(lowerLine)
				if len(matches) > 1 {
					var num int
					fmt.Sscanf(matches[1], "%d", &num)
					if num > 0 {
						info.RequestsPerHour = num
					}
				}
			}
		}
	}

	if !foundRateLimit {
		return nil
	}

	return info
}

// extractCategories extracts API categories from text
func (c *Client) extractCategories(text string) []string {
	var categories []string
	seen := make(map[string]bool)

	// Look for common category patterns
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for bullet points or numbered lists
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			category := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			category = strings.TrimSpace(category)
			if category != "" && !seen[category] && len(category) < 50 {
				categories = append(categories, category)
				seen[category] = true
			}
		} else if len(line) > 0 && line[0] >= '0' && line[0] <= '9' {
			// Numbered list
			parts := strings.SplitN(line, ".", 2)
			if len(parts) == 2 {
				category := strings.TrimSpace(parts[1])
				if category != "" && !seen[category] && len(category) < 50 {
					categories = append(categories, category)
					seen[category] = true
				}
			}
		}
	}

	return categories
}

// parseEndpoints extracts endpoint information from text
func (c *Client) parseEndpoints(category, text string) []EndpointInfo {
	var endpoints []EndpointInfo

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Look for HTTP method patterns: GET /path, POST /path, etc.
		for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH"} {
			if strings.Contains(strings.ToUpper(line), method) {
				parts := strings.Fields(line)
				for i, part := range parts {
					if strings.ToUpper(part) == method && i+1 < len(parts) {
						path := parts[i+1]
						if strings.HasPrefix(path, "/") {
							endpoint := EndpointInfo{
								Category: category,
								Method:   method,
								Path:     path,
							}

							// Try to extract description (rest of the line)
							if i+2 < len(parts) {
								endpoint.Description = strings.Join(parts[i+2:], " ")
							}

							endpoints = append(endpoints, endpoint)
							break
						}
					}
				}
			}
		}
	}

	return endpoints
}
