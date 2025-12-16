package discovery

import (
	"testing"
)

func TestExtractAuthentication(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name         string
		text         string
		expectedType string
	}{
		{
			name:         "Bearer token",
			text:         "Authentication uses Bearer token in the Authorization header",
			expectedType: "bearer",
		},
		{
			name:         "API Key",
			text:         "You need to pass an API key in the request",
			expectedType: "api_key",
		},
		{
			name:         "OAuth",
			text:         "This API uses OAuth 2.0 for authentication",
			expectedType: "oauth",
		},
		{
			name:         "Basic auth",
			text:         "Use basic auth with username and password",
			expectedType: "basic",
		},
		{
			name:         "No auth info",
			text:         "This API is public and requires no authentication",
			expectedType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth := client.extractAuthentication(tt.text)

			if tt.expectedType == "" {
				if len(auth) != 0 {
					t.Errorf("expected no auth, got %v", auth)
				}
				return
			}

			if auth["type"] != tt.expectedType {
				t.Errorf("expected auth type %s, got %s", tt.expectedType, auth["type"])
			}
		})
	}
}

func TestExtractCategories(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name: "Bullet list",
			text: `API Categories:
- Users
- Posts
- Comments
- Authentication`,
			expected: []string{"Users", "Posts", "Comments", "Authentication"},
		},
		{
			name: "Numbered list",
			text: `API Categories:
1. User Management
2. Content API
3. Analytics`,
			expected: []string{"User Management", "Content API", "Analytics"},
		},
		{
			name: "Asterisk list",
			text: `Categories:
* Products
* Orders
* Customers`,
			expected: []string{"Products", "Orders", "Customers"},
		},
		{
			name:     "No categories",
			text:     "This is just regular text without any categories",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			categories := client.extractCategories(tt.text)

			if len(categories) != len(tt.expected) {
				t.Errorf("expected %d categories, got %d", len(tt.expected), len(categories))
				return
			}

			for i, expected := range tt.expected {
				if categories[i] != expected {
					t.Errorf("expected category %s, got %s", expected, categories[i])
				}
			}
		})
	}
}

func TestParseEndpoints(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name     string
		category string
		text     string
		expected int
	}{
		{
			name:     "Multiple endpoints",
			category: "Users",
			text: `Available endpoints:
GET /users - List all users
POST /users - Create a new user
GET /users/{id} - Get user by ID
DELETE /users/{id} - Delete user`,
			expected: 4,
		},
		{
			name:     "Mixed case",
			category: "Posts",
			text: `Endpoints:
get /posts - List posts
post /posts - Create post`,
			expected: 2,
		},
		{
			name:     "No endpoints",
			category: "Empty",
			text:     "This text has no endpoint information",
			expected: 0,
		},
		{
			name:     "PATCH and PUT",
			category: "Resources",
			text: `Available operations:
PUT /resource/{id} - Replace resource
PATCH /resource/{id} - Update resource`,
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoints := client.parseEndpoints(tt.category, tt.text)

			if len(endpoints) != tt.expected {
				t.Errorf("expected %d endpoints, got %d", tt.expected, len(endpoints))
			}

			// Check that all endpoints have the correct category
			for _, endpoint := range endpoints {
				if endpoint.Category != tt.category {
					t.Errorf("expected category %s, got %s", tt.category, endpoint.Category)
				}
			}
		})
	}
}

func TestExtractRateLimits(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name        string
		text        string
		expectNil   bool
		expectedRPS int
		expectedRPM int
		expectedRPH int
	}{
		{
			name:        "Requests per second",
			text:        "Rate limit: 10 requests per second",
			expectNil:   false,
			expectedRPS: 10,
		},
		{
			name:        "Requests per minute",
			text:        "Rate limit: 60 requests per minute",
			expectNil:   false,
			expectedRPM: 60,
		},
		{
			name:        "Requests per hour",
			text:        "Rate limit: 1000 requests per hour",
			expectNil:   false,
			expectedRPH: 1000,
		},
		{
			name:      "No rate limit info",
			text:      "This text does not contain any relevant information",
			expectNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rateLimit := client.extractRateLimits(tt.text)

			if tt.expectNil {
				if rateLimit != nil {
					t.Errorf("expected nil, got %v", rateLimit)
				}
				return
			}

			if rateLimit == nil {
				t.Error("expected non-nil rate limit info")
				return
			}

			if tt.expectedRPS > 0 && rateLimit.RequestsPerSecond != tt.expectedRPS {
				t.Errorf("expected %d RPS, got %d", tt.expectedRPS, rateLimit.RequestsPerSecond)
			}

			if tt.expectedRPM > 0 && rateLimit.RequestsPerMinute != tt.expectedRPM {
				t.Errorf("expected %d RPM, got %d", tt.expectedRPM, rateLimit.RequestsPerMinute)
			}

			if tt.expectedRPH > 0 && rateLimit.RequestsPerHour != tt.expectedRPH {
				t.Errorf("expected %d RPH, got %d", tt.expectedRPH, rateLimit.RequestsPerHour)
			}
		})
	}
}
