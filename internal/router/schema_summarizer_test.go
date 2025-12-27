package router

import (
	"encoding/json"
	"testing"
)

func TestSummarizeSchema(t *testing.T) {
	tests := []struct {
		name      string
		schema    json.RawMessage
		maxChars  int
		wantValid bool
		wantSize  bool // wantSize means we want to check size constraint
	}{
		{
			name:      "simple schema",
			schema:    json.RawMessage(`{"type":"object","description":"Test"}`),
			maxChars:  100,
			wantValid: true,
			wantSize:  true,
		},
		{
			name: "complex schema with properties",
			schema: json.RawMessage(`{
				"type": "object",
				"description": "A complex schema",
				"properties": {
					"name": {"type": "string", "description": "Name field"},
					"age": {"type": "number"}
				},
				"required": ["name"]
			}`),
			maxChars:  200,
			wantValid: true,
			wantSize:  true,
		},
		{
			name: "empty schema",
			schema: json.RawMessage(`{}`),
			maxChars:  100,
			wantValid: true,
			wantSize:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SummarizeSchema(tt.schema, tt.maxChars)
			if err != nil {
				if tt.wantValid {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}

			// Verify it's valid JSON
			var test map[string]any
			if err := json.Unmarshal(result, &test); err != nil {
				t.Errorf("result is not valid JSON: %v", err)
			}

			// Verify size constraint if requested
			if tt.wantSize && len(result) > tt.maxChars+50 { // Allow some overhead
				t.Errorf("result exceeds maxChars: got %d, max %d", len(result), tt.maxChars)
			}
		})
	}
}

func TestSummarizeSchema_Truncation(t *testing.T) {
	// Create a very large schema
	largeDesc := make([]byte, 1000)
	for i := range largeDesc {
		largeDesc[i] = 'a'
	}
	schema := json.RawMessage(`{
		"type": "object",
		"description": "` + string(largeDesc) + `",
		"properties": {
			"field1": {"type": "string", "description": "` + string(largeDesc) + `"}
		}
	}`)

	result, err := SummarizeSchema(schema, 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) > 250 { // Allow overhead
		t.Errorf("schema should be truncated, got %d bytes", len(result))
	}

	// Should still be valid JSON
	var test map[string]any
	if err := json.Unmarshal(result, &test); err != nil {
		t.Errorf("truncated schema is not valid JSON: %v", err)
	}
}

func TestEstimateSchemaTokens(t *testing.T) {
	tests := []struct {
		schema json.RawMessage
		want   int
	}{
		{json.RawMessage(`{"type":"object"}`), 4}, // ~16 chars / 4 = 4 tokens
		{json.RawMessage(`{}`), 0},                // ~2 chars / 4 = 0 tokens (rounds down)
	}

	for _, tt := range tests {
		t.Run(string(tt.schema), func(t *testing.T) {
			got := EstimateSchemaTokens(tt.schema)
			// Allow some variance in estimation
			if got < tt.want-1 || got > tt.want+1 {
				t.Errorf("EstimateSchemaTokens() = %d, want ~%d", got, tt.want)
			}
		})
	}
}
