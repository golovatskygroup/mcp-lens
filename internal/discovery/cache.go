package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// InMemoryCache implements a simple in-memory cache with TTL
type InMemoryCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
}

type cacheEntry struct {
	result    *DiscoveryResult
	expiresAt time.Time
}

// NewInMemoryCache creates a new in-memory cache
func NewInMemoryCache(maxSize int) *InMemoryCache {
	cache := &InMemoryCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves a result from cache
func (c *InMemoryCache) Get(ctx context.Context, key string) (*DiscoveryResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, fmt.Errorf("cache miss")
	}

	if time.Now().After(entry.expiresAt) {
		return nil, fmt.Errorf("cache expired")
	}

	// Deep copy to prevent mutations
	data, err := json.Marshal(entry.result)
	if err != nil {
		return nil, fmt.Errorf("marshal cached result: %w", err)
	}

	var result DiscoveryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal cached result: %w", err)
	}

	return &result, nil
}

// Set stores a result in cache
func (c *InMemoryCache) Set(ctx context.Context, key string, result *DiscoveryResult, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check size limit
	if len(c.entries) >= c.maxSize {
		// Remove oldest entry
		c.evictOldest()
	}

	// Deep copy to prevent mutations
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	var resultCopy DiscoveryResult
	if err := json.Unmarshal(data, &resultCopy); err != nil {
		return fmt.Errorf("unmarshal result: %w", err)
	}

	resultCopy.CachedAt = time.Now()

	c.entries[key] = &cacheEntry{
		result:    &resultCopy,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes an entry from cache
func (c *InMemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
	return nil
}

// evictOldest removes the oldest entry (must be called with lock held)
func (c *InMemoryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.result.CachedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.result.CachedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// cleanupLoop periodically removes expired entries
func (c *InMemoryCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries
func (c *InMemoryCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// Size returns the current cache size
func (c *InMemoryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Clear removes all entries
func (c *InMemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}
