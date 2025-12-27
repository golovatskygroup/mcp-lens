package router

import (
	"encoding/json"
	"fmt"
)

// SummarizeSchema compresses a JSON Schema to reduce token usage.
// It keeps essential structure (name, description, required fields, property types)
// while removing verbose descriptions, examples, and metadata.
func SummarizeSchema(schema json.RawMessage, maxChars int) (json.RawMessage, error) {
	if len(schema) == 0 {
		return schema, nil
	}

	var raw map[string]any
	if err := json.Unmarshal(schema, &raw); err != nil {
		// If not a valid JSON object, return as-is
		return schema, nil
	}

	// Create compressed version
	compressed := make(map[string]any)

	// Keep type
	if t, ok := raw["type"]; ok {
		compressed["type"] = t
	}

	// Keep description but truncate if too long
	if desc, ok := raw["description"].(string); ok {
		if len(desc) > 200 {
			desc = desc[:200] + "..."
		}
		compressed["description"] = desc
	}

	// Keep required fields
	if req, ok := raw["required"]; ok {
		compressed["required"] = req
	}

	// Compress properties
	if props, ok := raw["properties"].(map[string]any); ok {
		compressedProps := make(map[string]any)
		for name, prop := range props {
			compressedProp := compressProperty(prop)
			compressedProps[name] = compressedProp
		}
		compressed["properties"] = compressedProps
	}

	// Keep enum if present (useful for constrained values)
	if enum, ok := raw["enum"]; ok {
		compressed["enum"] = enum
	}

	// Keep oneOf/anyOf/allOf structure but compress nested schemas
	if oneOf, ok := raw["oneOf"]; ok {
		compressed["oneOf"] = compressSchemaArray(oneOf, maxChars/2)
	}
	if anyOf, ok := raw["anyOf"]; ok {
		compressed["anyOf"] = compressSchemaArray(anyOf, maxChars/2)
	}
	if allOf, ok := raw["allOf"]; ok {
		compressed["allOf"] = compressSchemaArray(allOf, maxChars/2)
	}

	result, err := json.Marshal(compressed)
	if err != nil {
		return schema, err
	}

	// If compressed version is still too long, truncate string representation
	if len(result) > maxChars {
		// Try to truncate description further
		if desc, ok := compressed["description"].(string); ok && len(desc) > 50 {
			compressed["description"] = desc[:50] + "..."
			result, _ = json.Marshal(compressed)
		}
		// If still too long, return a minimal version
		if len(result) > maxChars {
			minimal := map[string]any{
				"type":        compressed["type"],
				"description": "Schema truncated due to size limit",
			}
			if req, ok := compressed["required"]; ok {
				minimal["required"] = req
			}
			result, _ = json.Marshal(minimal)
		}
	}

	return json.RawMessage(result), nil
}

func compressProperty(prop any) map[string]any {
	propMap, ok := prop.(map[string]any)
	if !ok {
		return map[string]any{"type": "any"}
	}

	result := make(map[string]any)

	// Keep type
	if t, ok := propMap["type"]; ok {
		result["type"] = t
	}

	// Keep description but truncate
	if desc, ok := propMap["description"].(string); ok {
		if len(desc) > 100 {
			desc = desc[:100] + "..."
		}
		result["description"] = desc
	}

	// Keep enum
	if enum, ok := propMap["enum"]; ok {
		result["enum"] = enum
	}

	// Keep items for arrays
	if items, ok := propMap["items"]; ok {
		if itemsMap, ok := items.(map[string]any); ok {
			result["items"] = compressProperty(itemsMap)
		} else {
			result["items"] = items
		}
	}

	// Keep properties for objects (nested)
	if props, ok := propMap["properties"].(map[string]any); ok {
		compressedProps := make(map[string]any)
		for name, p := range props {
			compressedProps[name] = compressProperty(p)
		}
		result["properties"] = compressedProps
	}

	// Keep required for nested objects
	if req, ok := propMap["required"]; ok {
		result["required"] = req
	}

	return result
}

func compressSchemaArray(arr any, maxChars int) []any {
	arrSlice, ok := arr.([]any)
	if !ok {
		return []any{}
	}

	result := make([]any, 0, len(arrSlice))
	for _, item := range arrSlice {
		if _, ok := item.(map[string]any); ok {
			compressed, _ := SummarizeSchema(json.RawMessage(fmt.Sprintf(`{"type":"object"}`)), maxChars/len(arrSlice))
			var compressedMap map[string]any
			if err := json.Unmarshal(compressed, &compressedMap); err == nil {
				result = append(result, compressedMap)
			}
		} else {
			result = append(result, item)
		}
	}
	return result
}

// EstimateSchemaTokens provides a rough estimate of token count for a schema.
// Uses a simple heuristic: ~4 characters per token.
func EstimateSchemaTokens(schema json.RawMessage) int {
	return len(schema) / 4
}

// TruncateSchemaString truncates a schema JSON string to fit within character limit.
func TruncateSchemaString(schema json.RawMessage, maxChars int) json.RawMessage {
	if len(schema) <= maxChars {
		return schema
	}

	// Try to parse and truncate intelligently
	var raw map[string]any
	if err := json.Unmarshal(schema, &raw); err == nil {
		// Remove verbose fields
		delete(raw, "examples")
		delete(raw, "default")
		if desc, ok := raw["description"].(string); ok && len(desc) > 100 {
			raw["description"] = desc[:100] + "..."
		}
		truncated, _ := json.Marshal(raw)
		if len(truncated) <= maxChars {
			return json.RawMessage(truncated)
		}
	}

	// Fallback: hard truncate
	if maxChars < 50 {
		return json.RawMessage(`{"type":"object","description":"Schema truncated"}`)
	}
	truncated := string(schema)
	if len(truncated) > maxChars-3 {
		truncated = truncated[:maxChars-3] + "..."
	}
	return json.RawMessage(truncated)
}
