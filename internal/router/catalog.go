package router

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolCatalog provides explicit tool listing without runtime discovery.
// It maintains a static list of available tools with their schemas and metadata.
type ToolCatalog struct {
	tools map[string]ToolCatalogItem
	// Cached compressed schemas to avoid recomputation
	compressedSchemas map[string]json.RawMessage
}

// NewToolCatalog creates a new catalog from a list of tools.
func NewToolCatalog(tools []ToolCatalogItem) *ToolCatalog {
	catalog := &ToolCatalog{
		tools:             make(map[string]ToolCatalogItem),
		compressedSchemas: make(map[string]json.RawMessage),
	}
	for _, tool := range tools {
		catalog.tools[tool.Name] = tool
	}
	return catalog
}

// ListTools returns all tools in the catalog.
func (c *ToolCatalog) ListTools() []ToolCatalogItem {
	result := make([]ToolCatalogItem, 0, len(c.tools))
	for _, tool := range c.tools {
		result = append(result, tool)
	}
	return result
}

// GetTool returns a tool by name, or false if not found.
func (c *ToolCatalog) GetTool(name string) (ToolCatalogItem, bool) {
	tool, ok := c.tools[name]
	return tool, ok
}

// TopKByQuery selects the top K most relevant tools for a query.
// Uses simple keyword matching for now (can be enhanced with embeddings later).
func (c *ToolCatalog) TopKByQuery(query string, k int) []ToolCatalogItem {
	if k <= 0 {
		return []ToolCatalogItem{}
	}
	if k >= len(c.tools) {
		return c.ListTools()
	}

	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)

	// Score each tool based on keyword matches
	type scoredTool struct {
		tool  ToolCatalogItem
		score int
	}
	scores := make([]scoredTool, 0, len(c.tools))

	for _, tool := range c.tools {
		score := 0
		nameLower := strings.ToLower(tool.Name)
		descLower := strings.ToLower(tool.Description)
		categoryLower := strings.ToLower(tool.Category)

		// Score based on exact matches in name, description, category
		for _, word := range queryWords {
			if strings.Contains(nameLower, word) {
				score += 10 // Name matches are most important
			}
			if strings.Contains(descLower, word) {
				score += 5
			}
			if strings.Contains(categoryLower, word) {
				score += 3
			}
		}

		// Boost exact name match
		if nameLower == queryLower {
			score += 50
		}

		// Boost if query is a prefix of the tool name
		if strings.HasPrefix(nameLower, queryLower) {
			score += 20
		}

		scores = append(scores, scoredTool{tool: tool, score: score})
	}

	// Sort by score (descending) - simple bubble sort for small lists
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[i].score < scores[j].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	// Return top K
	result := make([]ToolCatalogItem, 0, k)
	for i := 0; i < k && i < len(scores); i++ {
		if scores[i].score > 0 {
			result = append(result, scores[i].tool)
		}
	}

	return result
}

// GetCompressedSchema returns a compressed version of a tool's schema.
// Results are cached to avoid recomputation.
func (c *ToolCatalog) GetCompressedSchema(toolName string, maxChars int) (json.RawMessage, error) {
	// Check cache first
	if cached, ok := c.compressedSchemas[toolName]; ok {
		return cached, nil
	}

	tool, ok := c.GetTool(toolName)
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	compressed, err := SummarizeSchema(tool.InputSchema, maxChars)
	if err != nil {
		return nil, err
	}

	// Cache the result
	c.compressedSchemas[toolName] = compressed
	return compressed, nil
}

// GetTopKWithCompressedSchemas returns top K tools with compressed schemas.
// This is the main method used by the router to minimize context usage.
func (c *ToolCatalog) GetTopKWithCompressedSchemas(query string, k int, schemaMaxChars int) ([]ToolCatalogItem, error) {
	topK := c.TopKByQuery(query, k)
	result := make([]ToolCatalogItem, 0, len(topK))

	for _, tool := range topK {
		compressedSchema, err := c.GetCompressedSchema(tool.Name, schemaMaxChars)
		if err != nil {
			// If compression fails, use original schema
			compressedSchema = tool.InputSchema
		}
		tool.InputSchema = compressedSchema
		result = append(result, tool)
	}

	return result, nil
}
