package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements token bucket rate limiting per key.
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    int           // tokens per window
	window  time.Duration // time window
	done    chan struct{}
}

type bucket struct {
	tokens    int
	lastReset time.Time
}

// New creates a new Limiter.
func New(rate int, window time.Duration) *Limiter {
	l := &Limiter{
		buckets: make(map[string]*bucket),
		rate:    rate,
		window:  window,
		done:    make(chan struct{}),
	}

	// Start background cleanup
	go l.cleanup()

	return l
}

// Close stops the background cleanup goroutine.
func (l *Limiter) Close() {
	close(l.done)
}

// Allow checks if a request for the given key is allowed.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Get or create bucket
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{
			tokens:    l.rate,
			lastReset: now,
		}
		l.buckets[key] = b
	}

	// Reset bucket if window has passed
	if now.Sub(b.lastReset) >= l.window {
		b.tokens = l.rate
		b.lastReset = now
	}

	// Check and consume token
	if b.tokens > 0 {
		b.tokens--
		return true
	}

	return false
}

// cleanup periodically removes stale buckets.
func (l *Limiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for key, b := range l.buckets {
				// Remove buckets inactive for 2x window
				if now.Sub(b.lastReset) > 2*l.window {
					delete(l.buckets, key)
				}
			}
			l.mu.Unlock()
		case <-l.done:
			return
		}
	}
}
