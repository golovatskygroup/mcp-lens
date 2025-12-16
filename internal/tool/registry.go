package tool

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ToolDefinition represents a tool with its metadata and code
type ToolDefinition struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Language    string                 `json:"language"`
	Code        string                 `json:"code"`
	InputSchema map[string]interface{} `json:"input_schema"`
	Version     int                    `json:"version"`
	Status      string                 `json:"status"` // active, disabled
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// ToolRegistry defines the interface for managing tools
type ToolRegistry interface {
	// Register registers a new tool
	Register(ctx context.Context, tool *ToolDefinition) error

	// Get retrieves a tool by name
	Get(ctx context.Context, name string) (*ToolDefinition, error)

	// List retrieves all tools
	List(ctx context.Context) ([]*ToolDefinition, error)

	// Update updates an existing tool (creates a new version)
	Update(ctx context.Context, name string, tool *ToolDefinition) error

	// Delete deletes a tool (soft delete - sets status to disabled)
	Delete(ctx context.Context, name string) error

	// Rollback rolls back a tool to a specific version
	Rollback(ctx context.Context, name string, version int) error

	// GetVersions retrieves all versions of a tool
	GetVersions(ctx context.Context, name string) ([]*ToolDefinition, error)
}

// SQLiteRegistry implements ToolRegistry with SQLite persistence
type SQLiteRegistry struct {
	db    *sql.DB
	cache *ToolCache
	mu    sync.RWMutex
}

// NewSQLiteRegistry creates a new SQLite-backed tool registry
func NewSQLiteRegistry(db *sql.DB, cache *ToolCache) (*SQLiteRegistry, error) {
	r := &SQLiteRegistry{
		db:    db,
		cache: cache,
	}

	// Initialize tables
	if err := r.initTables(); err != nil {
		return nil, fmt.Errorf("failed to initialize tables: %w", err)
	}

	// Load active tools into cache
	if err := r.loadCache(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	return r, nil
}

// initTables creates the necessary database tables
func (r *SQLiteRegistry) initTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tools (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL,
		language TEXT NOT NULL,
		code TEXT NOT NULL,
		input_schema TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		status TEXT NOT NULL DEFAULT 'active',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tool_versions (
		id TEXT PRIMARY KEY,
		tool_id TEXT NOT NULL,
		version INTEGER NOT NULL,
		description TEXT NOT NULL,
		language TEXT NOT NULL,
		code TEXT NOT NULL,
		input_schema TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(tool_id) REFERENCES tools(id),
		UNIQUE(tool_id, version)
	);

	CREATE TABLE IF NOT EXISTS tool_executions (
		id TEXT PRIMARY KEY,
		tool_id TEXT NOT NULL,
		status TEXT NOT NULL,
		input_params TEXT,
		result TEXT,
		error_message TEXT,
		execution_time_ms INTEGER,
		executed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(tool_id) REFERENCES tools(id)
	);

	CREATE INDEX IF NOT EXISTS idx_tools_name ON tools(name);
	CREATE INDEX IF NOT EXISTS idx_tools_status ON tools(status);
	CREATE INDEX IF NOT EXISTS idx_tool_versions_tool_id ON tool_versions(tool_id);
	CREATE INDEX IF NOT EXISTS idx_tool_executions_tool_id ON tool_executions(tool_id);
	`

	_, err := r.db.Exec(schema)
	return err
}

// loadCache loads all active tools into the cache
func (r *SQLiteRegistry) loadCache(ctx context.Context) error {
	tools, err := r.List(ctx)
	if err != nil {
		return err
	}

	for _, tool := range tools {
		if tool.Status == "active" {
			r.cache.Set(tool.Name, tool)
		}
	}

	return nil
}

// Register registers a new tool
func (r *SQLiteRegistry) Register(ctx context.Context, tool *ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate tool
	if err := validateTool(tool); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Generate ID if not provided
	if tool.ID == "" {
		tool.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	tool.CreatedAt = now
	tool.UpdatedAt = now
	tool.Version = 1
	tool.Status = "active"

	// Marshal input schema
	schemaJSON, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return fmt.Errorf("failed to marshal input schema: %w", err)
	}

	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert into tools table
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tools (id, name, description, language, code, input_schema, version, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, tool.ID, tool.Name, tool.Description, tool.Language, tool.Code, string(schemaJSON),
		tool.Version, tool.Status, tool.CreatedAt, tool.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert tool: %w", err)
	}

	// Insert into tool_versions table
	versionID := uuid.New().String()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tool_versions (id, tool_id, version, description, language, code, input_schema, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, versionID, tool.ID, tool.Version, tool.Description, tool.Language, tool.Code, string(schemaJSON), tool.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert tool version: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update cache
	r.cache.Set(tool.Name, tool)

	return nil
}

// Get retrieves a tool by name
func (r *SQLiteRegistry) Get(ctx context.Context, name string) (*ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check cache first
	if tool := r.cache.Get(name); tool != nil {
		return tool, nil
	}

	// Query database
	var tool ToolDefinition
	var schemaJSON string

	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, language, code, input_schema, version, status, created_at, updated_at
		FROM tools
		WHERE name = ? AND status = 'active'
	`, name).Scan(&tool.ID, &tool.Name, &tool.Description, &tool.Language, &tool.Code, &schemaJSON,
		&tool.Version, &tool.Status, &tool.CreatedAt, &tool.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query tool: %w", err)
	}

	// Unmarshal input schema
	if err := json.Unmarshal([]byte(schemaJSON), &tool.InputSchema); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input schema: %w", err)
	}

	// Update cache
	r.cache.Set(tool.Name, &tool)

	return &tool, nil
}

// List retrieves all tools
func (r *SQLiteRegistry) List(ctx context.Context) ([]*ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, description, language, code, input_schema, version, status, created_at, updated_at
		FROM tools
		WHERE status = 'active'
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query tools: %w", err)
	}
	defer rows.Close()

	var tools []*ToolDefinition
	for rows.Next() {
		var tool ToolDefinition
		var schemaJSON string

		err := rows.Scan(&tool.ID, &tool.Name, &tool.Description, &tool.Language, &tool.Code, &schemaJSON,
			&tool.Version, &tool.Status, &tool.CreatedAt, &tool.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool: %w", err)
		}

		// Unmarshal input schema
		if err := json.Unmarshal([]byte(schemaJSON), &tool.InputSchema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input schema: %w", err)
		}

		tools = append(tools, &tool)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return tools, nil
}

// Update updates an existing tool (creates a new version)
func (r *SQLiteRegistry) Update(ctx context.Context, name string, tool *ToolDefinition) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get existing tool directly from DB without calling Get (to avoid deadlock)
	var existing ToolDefinition
	var schemaJSON string
	err := r.db.QueryRowContext(ctx, `
		SELECT id, name, description, language, code, input_schema, version, status, created_at, updated_at
		FROM tools
		WHERE name = ? AND status = 'active'
	`, name).Scan(&existing.ID, &existing.Name, &existing.Description, &existing.Language, &existing.Code, &schemaJSON,
		&existing.Version, &existing.Status, &existing.CreatedAt, &existing.UpdatedAt)

	if err == sql.ErrNoRows {
		return fmt.Errorf("tool not found: %s", name)
	}
	if err != nil {
		return fmt.Errorf("failed to query existing tool: %w", err)
	}

	// Validate tool
	if err := validateTool(tool); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Increment version
	tool.ID = existing.ID
	tool.Version = existing.Version + 1
	tool.UpdatedAt = time.Now()
	tool.Status = "active"

	// Marshal input schema
	schemaBytes, err := json.Marshal(tool.InputSchema)
	if err != nil {
		return fmt.Errorf("failed to marshal input schema: %w", err)
	}
	schemaJSON = string(schemaBytes)

	// Begin transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update tools table
	_, err = tx.ExecContext(ctx, `
		UPDATE tools
		SET description = ?, language = ?, code = ?, input_schema = ?, version = ?, updated_at = ?
		WHERE id = ?
	`, tool.Description, tool.Language, tool.Code, string(schemaJSON), tool.Version, tool.UpdatedAt, tool.ID)
	if err != nil {
		return fmt.Errorf("failed to update tool: %w", err)
	}

	// Insert new version
	versionID := uuid.New().String()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO tool_versions (id, tool_id, version, description, language, code, input_schema, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, versionID, tool.ID, tool.Version, tool.Description, tool.Language, tool.Code, string(schemaJSON), tool.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to insert tool version: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Invalidate cache
	r.cache.Delete(name)

	return nil
}

// Delete deletes a tool (soft delete)
func (r *SQLiteRegistry) Delete(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	result, err := r.db.ExecContext(ctx, `
		UPDATE tools
		SET status = 'disabled', updated_at = ?
		WHERE name = ?
	`, time.Now(), name)
	if err != nil {
		return fmt.Errorf("failed to delete tool: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("tool not found: %s", name)
	}

	// Invalidate cache
	r.cache.Delete(name)

	return nil
}

// Rollback rolls back a tool to a specific version
func (r *SQLiteRegistry) Rollback(ctx context.Context, name string, version int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get tool ID
	var toolID string
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM tools WHERE name = ?
	`, name).Scan(&toolID)
	if err == sql.ErrNoRows {
		return fmt.Errorf("tool not found: %s", name)
	}
	if err != nil {
		return fmt.Errorf("failed to query tool: %w", err)
	}

	// Get version data
	var description, language, code, schemaJSON string
	err = r.db.QueryRowContext(ctx, `
		SELECT description, language, code, input_schema
		FROM tool_versions
		WHERE tool_id = ? AND version = ?
	`, toolID, version).Scan(&description, &language, &code, &schemaJSON)
	if err == sql.ErrNoRows {
		return fmt.Errorf("version %d not found for tool %s", version, name)
	}
	if err != nil {
		return fmt.Errorf("failed to query tool version: %w", err)
	}

	// Update tool with version data
	_, err = r.db.ExecContext(ctx, `
		UPDATE tools
		SET description = ?, language = ?, code = ?, input_schema = ?, updated_at = ?
		WHERE id = ?
	`, description, language, code, schemaJSON, time.Now(), toolID)
	if err != nil {
		return fmt.Errorf("failed to rollback tool: %w", err)
	}

	// Invalidate cache
	r.cache.Delete(name)

	return nil
}

// GetVersions retrieves all versions of a tool
func (r *SQLiteRegistry) GetVersions(ctx context.Context, name string) ([]*ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get tool ID
	var toolID string
	err := r.db.QueryRowContext(ctx, `
		SELECT id FROM tools WHERE name = ?
	`, name).Scan(&toolID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query tool: %w", err)
	}

	// Query versions
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, version, description, language, code, input_schema, created_at
		FROM tool_versions
		WHERE tool_id = ?
		ORDER BY version DESC
	`, toolID)
	if err != nil {
		return nil, fmt.Errorf("failed to query tool versions: %w", err)
	}
	defer rows.Close()

	var versions []*ToolDefinition
	for rows.Next() {
		var tool ToolDefinition
		var schemaJSON string

		err := rows.Scan(&tool.ID, &tool.Version, &tool.Description, &tool.Language, &tool.Code, &schemaJSON, &tool.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool version: %w", err)
		}

		// Unmarshal input schema
		if err := json.Unmarshal([]byte(schemaJSON), &tool.InputSchema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input schema: %w", err)
		}

		tool.Name = name
		versions = append(versions, &tool)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return versions, nil
}

// LogExecution logs a tool execution
func (r *SQLiteRegistry) LogExecution(ctx context.Context, toolID, status string, inputParams, result interface{}, errorMsg string, executionTimeMs int64) error {
	// Marshal params and result
	paramsJSON, _ := json.Marshal(inputParams)
	resultJSON, _ := json.Marshal(result)

	executionID := uuid.New().String()
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO tool_executions (id, tool_id, status, input_params, result, error_message, execution_time_ms, executed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, executionID, toolID, status, string(paramsJSON), string(resultJSON), errorMsg, executionTimeMs, time.Now())

	return err
}

// validateTool validates a tool definition
func validateTool(tool *ToolDefinition) error {
	if tool == nil {
		return fmt.Errorf("tool cannot be nil")
	}

	if strings.TrimSpace(tool.Name) == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	if strings.TrimSpace(tool.Description) == "" {
		return fmt.Errorf("tool description cannot be empty")
	}

	if strings.TrimSpace(tool.Language) == "" {
		return fmt.Errorf("tool language cannot be empty")
	}

	if strings.TrimSpace(tool.Code) == "" {
		return fmt.Errorf("tool code cannot be empty")
	}

	if tool.InputSchema == nil {
		return fmt.Errorf("tool input schema cannot be nil")
	}

	// Validate name format (snake_case, no special characters)
	if !isValidToolName(tool.Name) {
		return fmt.Errorf("invalid tool name: must be snake_case with alphanumeric characters and underscores")
	}

	// Check for forbidden names (mutating operations)
	forbiddenPrefixes := []string{"create_", "update_", "delete_", "remove_", "destroy_"}
	for _, prefix := range forbiddenPrefixes {
		if strings.HasPrefix(tool.Name, prefix) {
			return fmt.Errorf("tool name cannot start with mutating prefix: %s", prefix)
		}
	}

	return nil
}

// isValidToolName checks if a tool name is valid (snake_case)
func isValidToolName(name string) bool {
	if name == "" {
		return false
	}

	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		// Allow uppercase only at start for acronyms
		if i == 0 && r >= 'A' && r <= 'Z' {
			continue
		}
		return false
	}

	return true
}
