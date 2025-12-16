package discovery

import (
	"context"
	"testing"
	"time"
)

func TestInMemoryCache(t *testing.T) {
	cache := NewInMemoryCache(10)
	ctx := context.Background()

	t.Run("Set and Get", func(t *testing.T) {
		result := &DiscoveryResult{
			ServiceName: "test-service",
			Strategy:    StrategyOpenAPIFirst,
			OpenAPIURL:  "https://api.example.com/openapi.json",
		}

		err := cache.Set(ctx, "test-key", result, 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		retrieved, err := cache.Get(ctx, "test-key")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if retrieved.ServiceName != result.ServiceName {
			t.Errorf("expected service name %s, got %s", result.ServiceName, retrieved.ServiceName)
		}

		if retrieved.OpenAPIURL != result.OpenAPIURL {
			t.Errorf("expected OpenAPI URL %s, got %s", result.OpenAPIURL, retrieved.OpenAPIURL)
		}
	})

	t.Run("Cache Miss", func(t *testing.T) {
		_, err := cache.Get(ctx, "nonexistent-key")
		if err == nil {
			t.Error("expected error for cache miss, got nil")
		}
	})

	t.Run("TTL Expiration", func(t *testing.T) {
		result := &DiscoveryResult{
			ServiceName: "test-service-ttl",
		}

		err := cache.Set(ctx, "ttl-key", result, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Should be available immediately
		_, err = cache.Get(ctx, "ttl-key")
		if err != nil {
			t.Errorf("Get failed before expiration: %v", err)
		}

		// Wait for expiration
		time.Sleep(100 * time.Millisecond)

		// Should be expired now
		_, err = cache.Get(ctx, "ttl-key")
		if err == nil {
			t.Error("expected error for expired cache entry, got nil")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		result := &DiscoveryResult{
			ServiceName: "test-service-delete",
		}

		err := cache.Set(ctx, "delete-key", result, 1*time.Hour)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		err = cache.Delete(ctx, "delete-key")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err = cache.Get(ctx, "delete-key")
		if err == nil {
			t.Error("expected error after delete, got nil")
		}
	})

	t.Run("Size Limit", func(t *testing.T) {
		smallCache := NewInMemoryCache(3)

		// Add 5 entries (exceeds limit)
		for i := 0; i < 5; i++ {
			result := &DiscoveryResult{
				ServiceName: "test-service",
			}
			key := string(rune('a' + i))
			err := smallCache.Set(ctx, key, result, 1*time.Hour)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		// Cache should have at most 3 entries
		if smallCache.Size() > 3 {
			t.Errorf("cache size %d exceeds limit of 3", smallCache.Size())
		}
	})

	t.Run("Clear", func(t *testing.T) {
		result := &DiscoveryResult{
			ServiceName: "test-service",
		}

		cache.Set(ctx, "key1", result, 1*time.Hour)
		cache.Set(ctx, "key2", result, 1*time.Hour)
		cache.Set(ctx, "key3", result, 1*time.Hour)

		cache.Clear()

		if cache.Size() != 0 {
			t.Errorf("expected cache size 0 after Clear, got %d", cache.Size())
		}
	})

	t.Run("Concurrent Access", func(t *testing.T) {
		concurrentCache := NewInMemoryCache(100)

		// Launch multiple goroutines
		done := make(chan bool)
		for i := 0; i < 10; i++ {
			go func(id int) {
				result := &DiscoveryResult{
					ServiceName: "concurrent-test",
				}
				key := string(rune('a' + id))
				concurrentCache.Set(ctx, key, result, 1*time.Hour)
				concurrentCache.Get(ctx, key)
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}

		// Should not panic or deadlock
	})
}
