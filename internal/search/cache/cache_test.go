package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alex-user-go/hotels/internal/search/types"
)

func TestCache_Key(t *testing.T) {
	tests := []struct {
		name    string
		city    string
		checkin string
		nights  int
		adults  int
		want    string
	}{
		{
			name:    "basic key",
			city:    "paris",
			checkin: "2024-01-15",
			nights:  3,
			adults:  2,
			want:    "paris:2024-01-15:3:2",
		},
		{
			name:    "empty city",
			city:    "",
			checkin: "2024-01-15",
			nights:  1,
			adults:  1,
			want:    ":2024-01-15:1:1",
		},
		{
			name:    "zero values",
			city:    "london",
			checkin: "",
			nights:  0,
			adults:  0,
			want:    "london::0:0",
		},
	}

	cache := NewCache(time.Minute)
	defer cache.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cache.Key(tt.city, tt.checkin, tt.nights, tt.adults)
			if got != tt.want {
				t.Errorf("Key() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCache_GetOrFetch(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(c *Cache)
		key        string
		fetchFunc  func() (*types.Result, error)
		wantResult *types.Result
		wantHit    bool
		wantErr    bool
	}{
		{
			name:  "cache miss - successful fetch",
			setup: func(c *Cache) {},
			key:   "test-key",
			fetchFunc: func() (*types.Result, error) {
				return &types.Result{ProvidersTotal: 5}, nil
			},
			wantResult: &types.Result{ProvidersTotal: 5},
			wantHit:    false,
			wantErr:    false,
		},
		{
			name: "cache hit - returns cached value",
			setup: func(c *Cache) {
				c.mu.Lock()
				c.entries["cached-key"] = &cacheEntry{
					result:    &types.Result{ProvidersTotal: 10},
					expiresAt: time.Now().Add(time.Minute),
				}
				c.mu.Unlock()
			},
			key: "cached-key",
			fetchFunc: func() (*types.Result, error) {
				t.Error("fetch should not be called for cached entry")
				return nil, nil
			},
			wantResult: &types.Result{ProvidersTotal: 10},
			wantHit:    true,
			wantErr:    false,
		},
		{
			name:  "fetch error - not cached",
			setup: func(c *Cache) {},
			key:   "error-key",
			fetchFunc: func() (*types.Result, error) {
				return nil, errors.New("fetch failed")
			},
			wantResult: nil,
			wantHit:    false,
			wantErr:    true,
		},
		{
			name:  "fetch returns nil result - not cached",
			setup: func(c *Cache) {},
			key:   "nil-key",
			fetchFunc: func() (*types.Result, error) {
				return nil, nil
			},
			wantResult: nil,
			wantHit:    false,
			wantErr:    false,
		},
		{
			name: "expired entry - refetches",
			setup: func(c *Cache) {
				c.mu.Lock()
				c.entries["expired-key"] = &cacheEntry{
					result:    &types.Result{ProvidersTotal: 1},
					expiresAt: time.Now().Add(-time.Minute),
				}
				c.mu.Unlock()
			},
			key: "expired-key",
			fetchFunc: func() (*types.Result, error) {
				return &types.Result{ProvidersTotal: 99}, nil
			},
			wantResult: &types.Result{ProvidersTotal: 99},
			wantHit:    false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(time.Minute)
			defer cache.Close()

			tt.setup(cache)

			got, hit, err := cache.GetOrFetch(context.Background(), tt.key, tt.fetchFunc)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetOrFetch() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if hit != tt.wantHit {
				t.Errorf("GetOrFetch() hit = %v, want %v", hit, tt.wantHit)
			}

			if tt.wantResult == nil && got != nil {
				t.Errorf("GetOrFetch() = %v, want nil", got)
			} else if tt.wantResult != nil {
				if got == nil {
					t.Errorf("GetOrFetch() = nil, want %v", tt.wantResult)
				} else if got.ProvidersTotal != tt.wantResult.ProvidersTotal {
					t.Errorf("GetOrFetch() ProvidersTotal = %d, want %d", got.ProvidersTotal, tt.wantResult.ProvidersTotal)
				}
			}
		})
	}
}

func TestCache_GetOrFetch_ContextCancellation(t *testing.T) {
	cache := NewCache(time.Minute)
	defer cache.Close()

	ctx, cancel := context.WithCancel(context.Background())

	fetchStarted := make(chan struct{})
	fetchDone := make(chan struct{})

	// Start a slow fetch
	go func() {
		_, _, _ = cache.GetOrFetch(context.Background(), "slow-key", func() (*types.Result, error) {
			close(fetchStarted)
			<-fetchDone
			return &types.Result{ProvidersTotal: 1}, nil
		})
	}()

	<-fetchStarted

	// Cancel context before fetch completes
	cancel()

	// Try to get the same key with cancelled context
	_, _, err := cache.GetOrFetch(ctx, "slow-key", func() (*types.Result, error) {
		t.Error("fetch should not be called - should wait for inflight")
		return nil, nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	close(fetchDone)
}

func TestCache_GetOrFetch_Singleflight(t *testing.T) {
	cache := NewCache(time.Minute)
	defer cache.Close()

	var fetchCount atomic.Int32
	fetchStarted := make(chan struct{})
	fetchContinue := make(chan struct{})

	var wg sync.WaitGroup
	const numGoroutines = 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, _, err := cache.GetOrFetch(context.Background(), "shared-key", func() (*types.Result, error) {
				if fetchCount.Add(1) == 1 {
					close(fetchStarted)
					<-fetchContinue
				}
				return &types.Result{ProvidersTotal: 42}, nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result == nil || result.ProvidersTotal != 42 {
				t.Errorf("unexpected result: %v", result)
			}
		}()
	}

	<-fetchStarted
	close(fetchContinue)
	wg.Wait()

	if count := fetchCount.Load(); count != 1 {
		t.Errorf("fetch called %d times, expected 1 (singleflight)", count)
	}
}

func TestCache_Invalidate(t *testing.T) {
	tests := []struct {
		name       string
		setupKeys  []string
		invalidate string
		wantKeys   []string
	}{
		{
			name:       "invalidate existing key",
			setupKeys:  []string{"a", "b", "c"},
			invalidate: "b",
			wantKeys:   []string{"a", "c"},
		},
		{
			name:       "invalidate non-existing key",
			setupKeys:  []string{"a", "b"},
			invalidate: "x",
			wantKeys:   []string{"a", "b"},
		},
		{
			name:       "invalidate from empty cache",
			setupKeys:  []string{},
			invalidate: "a",
			wantKeys:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(time.Minute)
			defer cache.Close()

			for _, key := range tt.setupKeys {
				cache.mu.Lock()
				cache.entries[key] = &cacheEntry{
					result:    &types.Result{},
					expiresAt: time.Now().Add(time.Minute),
				}
				cache.mu.Unlock()
			}

			cache.Invalidate(tt.invalidate)

			cache.mu.RLock()
			defer cache.mu.RUnlock()

			if len(cache.entries) != len(tt.wantKeys) {
				t.Errorf("cache has %d entries, want %d", len(cache.entries), len(tt.wantKeys))
			}

			for _, key := range tt.wantKeys {
				if _, ok := cache.entries[key]; !ok {
					t.Errorf("expected key %q to exist", key)
				}
			}
		})
	}
}

func TestCache_Clear(t *testing.T) {
	tests := []struct {
		name      string
		setupKeys []string
	}{
		{
			name:      "clear populated cache",
			setupKeys: []string{"a", "b", "c"},
		},
		{
			name:      "clear empty cache",
			setupKeys: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewCache(time.Minute)
			defer cache.Close()

			for _, key := range tt.setupKeys {
				cache.mu.Lock()
				cache.entries[key] = &cacheEntry{
					result:    &types.Result{},
					expiresAt: time.Now().Add(time.Minute),
				}
				cache.mu.Unlock()
			}

			cache.Clear()

			cache.mu.RLock()
			defer cache.mu.RUnlock()

			if len(cache.entries) != 0 {
				t.Errorf("cache has %d entries after Clear(), want 0", len(cache.entries))
			}
		})
	}
}

func TestCache_NilResultNotCached(t *testing.T) {
	cache := NewCache(time.Minute)
	defer cache.Close()

	callCount := 0

	// First call - returns nil
	result, hit, err := cache.GetOrFetch(context.Background(), "nil-key", func() (*types.Result, error) {
		callCount++
		return nil, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result, got %v", result)
	}
	if hit {
		t.Error("expected cache miss, got hit")
	}

	// Second call - should fetch again (nil not cached)
	result, hit, err = cache.GetOrFetch(context.Background(), "nil-key", func() (*types.Result, error) {
		callCount++
		return &types.Result{ProvidersTotal: 1}, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.ProvidersTotal != 1 {
		t.Errorf("unexpected result: %v", result)
	}
	if hit {
		t.Error("expected cache miss, got hit")
	}

	if callCount != 2 {
		t.Errorf("fetch called %d times, expected 2", callCount)
	}
}

func TestCache_ErrorNotCached(t *testing.T) {
	cache := NewCache(time.Minute)
	defer cache.Close()

	fetchErr := errors.New("temporary error")
	callCount := 0

	// First call - returns error
	_, hit, err := cache.GetOrFetch(context.Background(), "error-key", func() (*types.Result, error) {
		callCount++
		return nil, fetchErr
	})
	if err != fetchErr {
		t.Errorf("expected fetchErr, got %v", err)
	}
	if hit {
		t.Error("expected cache miss on error, got hit")
	}

	// Second call - should fetch again (error not cached)
	result, hit, err := cache.GetOrFetch(context.Background(), "error-key", func() (*types.Result, error) {
		callCount++
		return &types.Result{ProvidersTotal: 1}, nil
	})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result == nil || result.ProvidersTotal != 1 {
		t.Errorf("unexpected result: %v", result)
	}
	if hit {
		t.Error("expected cache miss, got hit")
	}

	if callCount != 2 {
		t.Errorf("fetch called %d times, expected 2", callCount)
	}
}
