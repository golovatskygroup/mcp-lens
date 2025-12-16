package discovery

import (
	"context"
	"testing"
)

func TestExtractURLs(t *testing.T) {
	extractor := NewOpenAPIExtractor()

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name: "GitHub raw URL",
			text: "The spec is at https://raw.githubusercontent.com/user/repo/main/openapi.json",
			expected: []string{
				"https://raw.githubusercontent.com/user/repo/main/openapi.json",
			},
		},
		{
			name: "Generic OpenAPI URL",
			text: "Documentation: https://api.example.com/openapi.json",
			expected: []string{
				"https://api.example.com/openapi.json",
			},
		},
		{
			name: "Multiple URLs",
			text: "Check https://api.example.com/v1/swagger.json or https://docs.example.com/api/spec.json",
			expected: []string{
				"https://api.example.com/v1/swagger.json",
				"https://docs.example.com/api/spec.json",
			},
		},
		{
			name:     "No URLs",
			text:     "This is just some text without any API URLs",
			expected: []string{},
		},
		{
			name: "YAML format",
			text: "The spec is at https://api.example.com/openapi.yaml",
			expected: []string{
				"https://api.example.com/openapi.yaml",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls := extractor.ExtractURLs(tt.text)

			if len(urls) != len(tt.expected) {
				t.Errorf("expected %d URLs, got %d", len(tt.expected), len(urls))
				return
			}

			for i, expectedURL := range tt.expected {
				if urls[i] != expectedURL {
					t.Errorf("expected URL %s, got %s", expectedURL, urls[i])
				}
			}
		})
	}
}

func TestExtractBaseURL(t *testing.T) {
	extractor := NewOpenAPIExtractor()

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "Base URL pattern",
			text:     "The base URL is: https://api.example.com",
			expected: "https://api.example.com",
		},
		{
			name:     "API endpoint pattern",
			text:     "API endpoint: https://api.example.com/v1",
			expected: "https://api.example.com/v1",
		},
		{
			name:     "URL with /api",
			text:     "Use https://example.com/api for all requests",
			expected: "https://example.com/api",
		},
		{
			name:     "No base URL",
			text:     "This text doesn't contain a base URL",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseURL := extractor.ExtractBaseURL(tt.text)

			if baseURL != tt.expected {
				t.Errorf("expected base URL %s, got %s", tt.expected, baseURL)
			}
		})
	}
}

func TestNormalizeGitHubURL(t *testing.T) {
	extractor := NewOpenAPIExtractor()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Convert blob to raw",
			input:    "https://github.com/user/repo/blob/main/openapi.json",
			expected: "https://raw.githubusercontent.com/user/repo/main/openapi.json",
		},
		{
			name:     "Already raw URL",
			input:    "https://raw.githubusercontent.com/user/repo/main/openapi.json",
			expected: "https://raw.githubusercontent.com/user/repo/main/openapi.json",
		},
		{
			name:     "Non-GitHub URL",
			input:    "https://api.example.com/openapi.json",
			expected: "https://api.example.com/openapi.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.NormalizeGitHubURL(tt.input)

			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	extractor := NewOpenAPIExtractor()
	ctx := context.Background()

	tests := []struct {
		name      string
		url       string
		shouldErr bool
	}{
		{
			name:      "Invalid scheme",
			url:       "ftp://example.com/openapi.json",
			shouldErr: true,
		},
		{
			name:      "Invalid URL format",
			url:       "not a url",
			shouldErr: true,
		},
		{
			name: "Valid HTTPS URL",
			url:  "https://raw.githubusercontent.com/OAI/OpenAPI-Specification/main/examples/v3.0/petstore.json",
			// This might succeed or fail depending on network, but it shouldn't panic
			shouldErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extractor.ValidateURL(ctx, tt.url)

			if tt.shouldErr && err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}
