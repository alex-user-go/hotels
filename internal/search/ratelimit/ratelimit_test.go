package ratelimit_test

import (
	"testing"
	"time"

	"github.com/alex-user-go/hotels/internal/search/ratelimit"
)

func TestLimiter_Allow(t *testing.T) {
	tests := []struct {
		name       string
		rate       int
		window     time.Duration
		key        string
		calls      int
		wantPassed int
	}{
		{
			name:       "all requests within limit",
			rate:       5,
			window:     time.Minute,
			key:        "user1",
			calls:      5,
			wantPassed: 5,
		},
		{
			name:       "exceed rate limit",
			rate:       3,
			window:     time.Minute,
			key:        "user2",
			calls:      5,
			wantPassed: 3,
		},
		{
			name:       "single request",
			rate:       10,
			window:     time.Minute,
			key:        "user3",
			calls:      1,
			wantPassed: 1,
		},
		{
			name:       "zero rate blocks all",
			rate:       0,
			window:     time.Minute,
			key:        "user4",
			calls:      3,
			wantPassed: 0,
		},
		{
			name:       "empty key",
			rate:       2,
			window:     time.Minute,
			key:        "",
			calls:      3,
			wantPassed: 2,
		},
		{
			name:       "negative rate blocks all",
			rate:       -5,
			window:     time.Minute,
			key:        "user5",
			calls:      3,
			wantPassed: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := ratelimit.New(tt.rate, tt.window)
			defer l.Close()

			passed := 0
			for i := 0; i < tt.calls; i++ {
				if l.Allow(tt.key) {
					passed++
				}
			}

			if passed != tt.wantPassed {
				t.Errorf("Allow() passed %d requests, want %d", passed, tt.wantPassed)
			}
		})
	}
}

func TestLimiter_Allow_WindowReset(t *testing.T) {
	l := ratelimit.New(2, 50*time.Millisecond)
	defer l.Close()

	key := "user1"

	// Use all tokens
	if !l.Allow(key) {
		t.Error("first request should be allowed")
	}
	if !l.Allow(key) {
		t.Error("second request should be allowed")
	}
	if l.Allow(key) {
		t.Error("third request should be blocked")
	}

	// Wait for window to reset
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !l.Allow(key) {
		t.Error("request after window reset should be allowed")
	}
	if !l.Allow(key) {
		t.Error("second request after window reset should be allowed")
	}
}

func TestLimiter_Allow_MultipleKeys(t *testing.T) {
	l := ratelimit.New(2, time.Minute)
	defer l.Close()

	tests := []struct {
		key        string
		wantPassed int
	}{
		{key: "user1", wantPassed: 2},
		{key: "user2", wantPassed: 2},
		{key: "user3", wantPassed: 2},
	}

	for _, tt := range tests {
		passed := 0
		for i := 0; i < 3; i++ {
			if l.Allow(tt.key) {
				passed++
			}
		}
		if passed != tt.wantPassed {
			t.Errorf("key %s: passed %d requests, want %d", tt.key, passed, tt.wantPassed)
		}
	}
}

func TestLimiter_Concurrent(t *testing.T) {
	l := ratelimit.New(100, time.Minute)
	defer l.Close()

	key := "user1"
	start := make(chan struct{})
	results := make(chan bool, 200)

	// Launch 200 concurrent requests
	for i := 0; i < 200; i++ {
		go func() {
			<-start
			results <- l.Allow(key)
		}()
	}

	// Release all goroutines at once
	close(start)

	// Collect results
	count := 0
	for i := 0; i < 200; i++ {
		if <-results {
			count++
		}
	}

	if count != 100 {
		t.Errorf("concurrent test: %d requests passed, want 100", count)
	}
}
