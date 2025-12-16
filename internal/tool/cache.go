package tool

import (
	"sync"
	"time"
)

// ToolCache provides in-memory caching for tools
type ToolCache struct {
	mu      sync.RWMutex
	tools   map[string]*cachedTool
	ttl     time.Duration
	maxSize int
}

type cachedTool struct {
	tool      *ToolDefinition
	expiresAt time.Time
}

// NewToolCache creates a new tool cache
func NewToolCache(ttl time.Duration, maxSize int) *ToolCache {
	cache := &ToolCache{
		tools:   make(map[string]*cachedTool),
		ttl:     ttl,
		maxSize: maxSize,
	}

	// Start cleanup goroutine
	go cache.cleanup()

	return cache
}

// Get retrieves a tool from cache
func (c *ToolCache) Get(name string) *ToolDefinition {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cached, exists := c.tools[name]
	if !exists {
		return nil
	}

	// Check if expired
	if time.Now().After(cached.expiresAt) {
		return nil
	}

	return cached.tool
}

// Set stores a tool in cache
func (c *ToolCache) Set(name string, tool *ToolDefinition) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check size limit
	if len(c.tools) >= c.maxSize {
		// Evict oldest entry
		c.evictOldest()
	}

	c.tools[name] = &cachedTool{
		tool:      tool,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Delete removes a tool from cache
func (c *ToolCache) Delete(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.tools, name)
}

// Clear removes all tools from cache
func (c *ToolCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tools = make(map[string]*cachedTool)
}

// Size returns the current cache size
func (c *ToolCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.tools)
}

// evictOldest removes the oldest entry from cache
func (c *ToolCache) evictOldest() {
	var oldestName string
	var oldestTime time.Time

	for name, cached := range c.tools {
		if oldestName == "" || cached.expiresAt.Before(oldestTime) {
			oldestName = name
			oldestTime = cached.expiresAt
		}
	}

	if oldestName != "" {
		delete(c.tools, oldestName)
	}
}

// cleanup periodically removes expired entries
func (c *ToolCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for name, cached := range c.tools {
			if now.After(cached.expiresAt) {
				delete(c.tools, name)
			}
		}
		c.mu.Unlock()
	}
}
