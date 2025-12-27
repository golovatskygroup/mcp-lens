package router

import (
	"encoding/json"
	"fmt"
	"os"
)

// CatalogConfig defines how to load the tool catalog.
type CatalogConfig struct {
	// Source: "inline" (from config) or "file" (from file path)
	Source string `yaml:"source" json:"source"`
	// InlineTools: tools defined directly in config (when source="inline")
	InlineTools []ToolCatalogItem `yaml:"inline_tools,omitempty" json:"inline_tools,omitempty"`
	// FilePath: path to JSON/YAML file with tools (when source="file")
	FilePath string `yaml:"file_path,omitempty" json:"file_path,omitempty"`
}

// RouterConfig holds router-specific configuration.
type RouterConfig struct {
	// ToolCatalog: how to load the tool catalog
	ToolCatalog CatalogConfig `yaml:"tool_catalog" json:"tool_catalog"`
	// TopKTools: number of top-K tools to include in prompt (0 = use all)
	TopKTools int `yaml:"top_k_tools" json:"top_k_tools"`
	// SchemaBudget: max characters per schema (0 = no compression)
	SchemaBudget int `yaml:"schema_budget" json:"schema_budget"`
	// TotalPromptBudget: max total characters for prompt (0 = no limit)
	TotalPromptBudget int `yaml:"total_prompt_budget" json:"total_prompt_budget"`
}

// LoadCatalogFromConfig loads a ToolCatalog based on the configuration.
func LoadCatalogFromConfig(cfg CatalogConfig) (*ToolCatalog, error) {
	switch cfg.Source {
	case "inline":
		if len(cfg.InlineTools) == 0 {
			return nil, fmt.Errorf("inline_tools is empty")
		}
		return NewToolCatalog(cfg.InlineTools), nil
	case "file":
		if cfg.FilePath == "" {
			return nil, fmt.Errorf("file_path is required when source=file")
		}
		return LoadCatalogFromFile(cfg.FilePath)
	default:
		return nil, fmt.Errorf("invalid catalog source: %s (must be 'inline' or 'file')", cfg.Source)
	}
}

// LoadCatalogFromFile loads a ToolCatalog from a JSON file.
func LoadCatalogFromFile(filePath string) (*ToolCatalog, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read catalog file: %w", err)
	}

	var tools []ToolCatalogItem
	if err := json.Unmarshal(data, &tools); err != nil {
		return nil, fmt.Errorf("failed to parse catalog JSON: %w", err)
	}

	if len(tools) == 0 {
		return nil, fmt.Errorf("catalog file contains no tools")
	}

	return NewToolCatalog(tools), nil
}
