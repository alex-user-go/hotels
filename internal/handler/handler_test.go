package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alex-user-go/hotels/internal/handler"
	"github.com/alex-user-go/hotels/internal/obs"
	"github.com/alex-user-go/hotels/internal/providers"
	"github.com/alex-user-go/hotels/internal/search"
	"github.com/alex-user-go/hotels/internal/search/cache"
	"github.com/alex-user-go/hotels/internal/search/ratelimit"
)

func TestHandler_SearchHandler(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupRateLimit func(*ratelimit.Limiter, string)
		wantStatus     int
		wantError      string
	}{
		{
			name:        "successful search",
			queryParams: "city=paris&checkin=2025-12-01&nights=2&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
				// Allow requests
			},
			wantStatus: http.StatusOK,
		},
		{
			name:        "missing city",
			queryParams: "checkin=2025-12-01&nights=2&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "city is required",
		},
		{
			name:        "missing checkin",
			queryParams: "city=paris&nights=2&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "checkin is required",
		},
		{
			name:        "invalid checkin format",
			queryParams: "city=paris&checkin=2025/12/01&nights=2&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "checkin must be in YYYY-MM-DD format",
		},
		{
			name:        "missing nights",
			queryParams: "city=paris&checkin=2025-12-01&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "nights is required",
		},
		{
			name:        "invalid nights (non-integer)",
			queryParams: "city=paris&checkin=2025-12-01&nights=abc&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "nights must be a positive integer",
		},
		{
			name:        "invalid nights (zero)",
			queryParams: "city=paris&checkin=2025-12-01&nights=0&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "nights must be a positive integer",
		},
		{
			name:        "invalid nights (negative)",
			queryParams: "city=paris&checkin=2025-12-01&nights=-1&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "nights must be a positive integer",
		},
		{
			name:        "missing adults",
			queryParams: "city=paris&checkin=2025-12-01&nights=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "adults is required",
		},
		{
			name:        "invalid adults (zero)",
			queryParams: "city=paris&checkin=2025-12-01&nights=2&adults=0",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
			},
			wantStatus: http.StatusBadRequest,
			wantError:  "adults must be a positive integer",
		},
		{
			name:        "rate limit exceeded",
			queryParams: "city=paris&checkin=2025-12-01&nights=2&adults=2",
			setupRateLimit: func(l *ratelimit.Limiter, ip string) {
				// Exhaust rate limit
				for i := 0; i < 10; i++ {
					l.Allow(ip)
				}
			},
			wantStatus: http.StatusTooManyRequests,
			wantError:  "rate limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup dependencies
			logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
			metrics := obs.NewMetrics(logger)
			searchCache := cache.NewCache(30 * time.Second)
			defer searchCache.Close()
			limiter := ratelimit.New(10, time.Minute)
			defer limiter.Close()

			// Create mock provider
			mockProvider := &mockProvider{}
			aggregator := search.NewAggregator([]providers.Provider{mockProvider}, 2*time.Second, metrics, logger)

			h := handler.New(aggregator, searchCache, limiter, metrics, logger)

			// Setup rate limiter
			ip := "192.168.1.1"
			tt.setupRateLimit(limiter, ip)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/search?"+tt.queryParams, nil)
			req.RemoteAddr = ip + ":12345"
			w := httptest.NewRecorder()

			// Execute
			h.SearchHandler(w, req)

			// Check status
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}

			// Check error message if expected
			if tt.wantError != "" {
				var errResp map[string]string
				if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
					t.Fatalf("failed to decode error response: %v", err)
				}
				if errResp["error"] != tt.wantError {
					t.Errorf("error = %q, want %q", errResp["error"], tt.wantError)
				}
			}

			// Check successful response
			if tt.wantStatus == http.StatusOK {
				var resp handler.SearchResponse
				if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
					t.Fatalf("failed to decode result: %v", err)
				}
				if resp.Stats.ProvidersTotal != 1 {
					t.Errorf("ProvidersTotal = %d, want 1", resp.Stats.ProvidersTotal)
				}
				if resp.Search.City == "" {
					t.Error("expected search.city to be set")
				}
				if resp.Stats.Cache == "" {
					t.Error("expected stats.cache to be set")
				}
				if resp.Stats.DurationMs < 0 {
					t.Errorf("duration_ms = %d, want >= 0", resp.Stats.DurationMs)
				}
			}
		})
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		wantIP     string
	}{
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195"},
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "203.0.113.195",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.195, 70.41.3.18, 150.172.238.178"},
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "203.0.113.195",
		},
		{
			name:       "X-Real-IP",
			headers:    map[string]string{"X-Real-IP": "203.0.113.50"},
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "203.0.113.50",
		},
		{
			name:       "X-Forwarded-For takes precedence",
			headers:    map[string]string{"X-Forwarded-For": "1.1.1.1", "X-Real-IP": "2.2.2.2"},
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "1.1.1.1",
		},
		{
			name:       "fallback to RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "192.168.1.1",
		},
		{
			name:       "RemoteAddr without port",
			headers:    map[string]string{},
			remoteAddr: "192.168.1.1",
			wantIP:     "192.168.1.1",
		},
		{
			name:       "IPv6 RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "[::1]:12345",
			wantIP:     "::1",
		},
		{
			name:       "X-Forwarded-For with whitespace",
			headers:    map[string]string{"X-Forwarded-For": "  203.0.113.195  "},
			remoteAddr: "192.168.1.1:12345",
			wantIP:     "203.0.113.195",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = tt.remoteAddr
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got := handler.ExtractIP(req)
			if got != tt.wantIP {
				t.Errorf("ExtractIP() = %q, want %q", got, tt.wantIP)
			}
		})
	}
}

func TestParseSearchParams(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantError string
	}{
		{
			name:      "valid params",
			query:     "city=paris&checkin=2025-12-01&nights=2&adults=2",
			wantError: "",
		},
		{
			name:      "empty city",
			query:     "city=&checkin=2025-12-01&nights=2&adults=2",
			wantError: "city is required",
		},
		{
			name:      "whitespace city",
			query:     "city=%20%20%20&checkin=2025-12-01&nights=2&adults=2",
			wantError: "city is required",
		},
		{
			name:      "invalid date format",
			query:     "city=paris&checkin=12/01/2025&nights=2&adults=2",
			wantError: "checkin must be in YYYY-MM-DD format",
		},
		{
			name:      "negative nights",
			query:     "city=paris&checkin=2025-12-01&nights=-5&adults=2",
			wantError: "nights must be a positive integer",
		},
		{
			name:      "non-integer adults",
			query:     "city=paris&checkin=2025-12-01&nights=2&adults=two",
			wantError: "adults must be a positive integer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/search?"+tt.query, nil)
			params, err := handler.ParseSearchParams(req)

			if tt.wantError != "" {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.wantError)
				} else if err.Error() != tt.wantError {
					t.Errorf("error = %q, want %q", err.Error(), tt.wantError)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if params == nil {
					t.Error("params should not be nil")
				}
			}
		})
	}
}

// mockProvider is a simple mock provider for testing.
type mockProvider struct{}

func (m *mockProvider) Name() string {
	return "mock"
}

func (m *mockProvider) Search(ctx context.Context, city, checkin string, nights, adults int) ([]providers.Hotel, error) {
	return []providers.Hotel{
		{HotelID: "1", Name: "Test Hotel", Price: 100.0, Currency: "EUR"},
	}, nil
}

// failingProvider always returns an error.
type failingProvider struct{}

func (f *failingProvider) Name() string {
	return "failing"
}

func (f *failingProvider) Search(ctx context.Context, city, checkin string, nights, adults int) ([]providers.Hotel, error) {
	return nil, errors.New("provider error")
}

func TestHandler_SearchHandler_ProviderError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	metrics := obs.NewMetrics(logger)
	searchCache := cache.NewCache(30 * time.Second)
	defer searchCache.Close()
	limiter := ratelimit.New(10, time.Minute)
	defer limiter.Close()

	// All providers fail
	failProvider := &failingProvider{}
	aggregator := search.NewAggregator([]providers.Provider{failProvider}, 2*time.Second, metrics, logger)

	h := handler.New(aggregator, searchCache, limiter, metrics, logger)

	req := httptest.NewRequest(http.MethodGet, "/search?city=paris&checkin=2025-12-01&nights=2&adults=2", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	h.SearchHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var errResp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp["error"] != "search failed" {
		t.Errorf("error = %q, want %q", errResp["error"], "search failed")
	}
}
