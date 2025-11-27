package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/alex-user-go/hotels/internal/search"
)

// Cache provides in-memory caching with TTL and request collapsing (singleflight).
type Cache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	ttl      time.Duration
	inflight map[string]*inflightRequest
	done     chan struct{}
}

type cacheEntry struct {
	result    *search.Result
	expiresAt time.Time
}

type inflightRequest struct {
	done   chan struct{}
	result *search.Result
	err    error
}

// NewCache creates a new Cache with the specified TTL.
func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		entries:  make(map[string]*cacheEntry),
		ttl:      ttl,
		inflight: make(map[string]*inflightRequest),
		done:     make(chan struct{}),
	}

	// Start background cleanup
	go c.cleanup()

	return c
}

// Close stops the background cleanup goroutine.
func (c *Cache) Close() {
	close(c.done)
}

// Key generates a cache key from search parameters.
func (c *Cache) Key(city, checkin string, nights, adults int) string {
	return fmt.Sprintf("%s:%s:%d:%d", city, checkin, nights, adults)
}

// GetOrFetch retrieves from cache or executes the fetch function.
// Concurrent requests for the same key are collapsed (singleflight pattern).
// Returns the result and a boolean indicating if it was a cache hit.
func (c *Cache) GetOrFetch(ctx context.Context, key string, fetch func() (*search.Result, error)) (*search.Result, bool, error) {
	c.mu.Lock()

	// Check cache
	if entry, ok := c.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.Unlock()
		return entry.result, true, nil
	}

	// Check for existing in-flight request
	if inflight, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-inflight.done:
			return inflight.result, false, inflight.err
		case <-ctx.Done():
			return nil, false, context.Cause(ctx)
		}
	}

	// Create new in-flight request
	inflight := &inflightRequest{
		done: make(chan struct{}),
	}
	c.inflight[key] = inflight
	c.mu.Unlock()

	// Execute fetch (outside of lock)
	result, err := fetch()

	// Store result
	c.mu.Lock()
	inflight.result = result
	inflight.err = err
	if err == nil && result != nil {
		c.entries[key] = &cacheEntry{
			result:    result,
			expiresAt: time.Now().Add(c.ttl),
		}
	}
	delete(c.inflight, key)
	c.mu.Unlock()

	// Notify all waiters
	close(inflight.done)

	return result, false, err
}

// Invalidate removes a specific key from the cache.
func (c *Cache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// Clear removes all entries from the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	c.entries = make(map[string]*cacheEntry)
	c.mu.Unlock()
}

// cleanup periodically removes expired entries.
func (c *Cache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for key, entry := range c.entries {
				if now.After(entry.expiresAt) {
					delete(c.entries, key)
				}
			}
			c.mu.Unlock()
		case <-c.done:
			return
		}
	}
}
