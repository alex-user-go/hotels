package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alex-user-go/hotels/internal/middleware"
	"github.com/alex-user-go/hotels/internal/obs"
	"github.com/alex-user-go/hotels/internal/search"
	"github.com/alex-user-go/hotels/internal/search/cache"
	"github.com/alex-user-go/hotels/internal/search/ratelimit"
	"github.com/alex-user-go/hotels/internal/search/types"
)

// Handler handles HTTP requests.
type Handler struct {
	aggregator  *search.Aggregator
	cache       *cache.Cache
	rateLimiter *ratelimit.Limiter
	metrics     *obs.Metrics
	logger      *slog.Logger
}

// New creates a new Handler.
func New(
	aggregator *search.Aggregator,
	searchCache *cache.Cache,
	rateLimiter *ratelimit.Limiter,
	metrics *obs.Metrics,
	logger *slog.Logger,
) *Handler {
	return &Handler{
		aggregator:  aggregator,
		cache:       searchCache,
		rateLimiter: rateLimiter,
		metrics:     metrics,
		logger:      logger,
	}
}

// SearchResponse represents the complete API response.
type SearchResponse struct {
	Search SearchInfo    `json:"search"`
	Stats  SearchStats   `json:"stats"`
	Hotels []types.Hotel `json:"hotels"`
}

// SearchInfo contains the search parameters.
type SearchInfo struct {
	City    string `json:"city"`
	Checkin string `json:"checkin"`
	Nights  int    `json:"nights"`
	Adults  int    `json:"adults"`
}

// SearchStats contains search statistics.
type SearchStats struct {
	ProvidersTotal     int    `json:"providers_total"`
	ProvidersSucceeded int    `json:"providers_succeeded"`
	ProvidersFailed    int    `json:"providers_failed"`
	Cache              string `json:"cache"`
	DurationMs         int64  `json:"duration_ms"`
}

// SearchHandler handles /search requests.
func (h *Handler) SearchHandler(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	h.metrics.IncRequests()
	requestID := middleware.RequestID(r.Context())

	// Check rate limit
	ip := ExtractIP(r)
	if !h.rateLimiter.Allow(ip) {
		h.logger.Warn("rate limit exceeded", "request_id", requestID, "ip", ip)
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	// Parse and validate query parameters
	params, err := ParseSearchParams(r)
	if err != nil {
		h.logger.Debug("invalid request parameters", "request_id", requestID, "error", err, "ip", ip)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Generate cache key and fetch from cache
	key := h.cache.Key(params.City, params.Checkin, params.Nights, params.Adults)

	// Get or fetch from cache
	result, cacheHit, err := h.cache.GetOrFetch(r.Context(), key, func() (*types.Result, error) {
		return h.aggregator.Search(r.Context(), params.City, params.Checkin, params.Nights, params.Adults)
	})

	if err != nil {
		h.logger.Error("search failed",
			"request_id", requestID,
			"error", err,
			"city", params.City,
			"checkin", params.Checkin,
			"ip", ip,
		)
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	// Calculate duration
	duration := time.Since(startTime).Milliseconds()

	// Build response
	cacheStatus := "miss"
	if cacheHit {
		cacheStatus = "hit"
		h.metrics.IncCacheHits()
	}

	response := SearchResponse{
		Search: SearchInfo{
			City:    params.City,
			Checkin: params.Checkin,
			Nights:  params.Nights,
			Adults:  params.Adults,
		},
		Stats: SearchStats{
			ProvidersTotal:     result.ProvidersTotal,
			ProvidersSucceeded: result.ProvidersSucceeded,
			ProvidersFailed:    result.ProvidersFailed,
			Cache:              cacheStatus,
			DurationMs:         duration,
		},
		Hotels: result.Hotels,
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		// Can't change status after WriteHeader, just log
		h.logger.Error("failed to encode response", "error", err)
	}
}

// SearchParams holds validated search parameters.
type SearchParams struct {
	City    string
	Checkin string
	Nights  int
	Adults  int
}

// ParseSearchParams parses and validates search parameters from the request.
func ParseSearchParams(r *http.Request) (*SearchParams, error) {
	query := r.URL.Query()

	// City - required, non-empty
	city := strings.TrimSpace(query.Get("city"))
	if city == "" {
		return nil, fmt.Errorf("city is required")
	}

	// Checkin - required, YYYY-MM-DD format
	checkin := strings.TrimSpace(query.Get("checkin"))
	if checkin == "" {
		return nil, fmt.Errorf("checkin is required")
	}
	if _, err := time.Parse("2006-01-02", checkin); err != nil {
		return nil, fmt.Errorf("checkin must be in YYYY-MM-DD format")
	}

	// Nights - required, positive integer
	nightsStr := query.Get("nights")
	if nightsStr == "" {
		return nil, fmt.Errorf("nights is required")
	}
	nights, err := strconv.Atoi(nightsStr)
	if err != nil || nights <= 0 {
		return nil, fmt.Errorf("nights must be a positive integer")
	}

	// Adults - required, positive integer
	adultsStr := query.Get("adults")
	if adultsStr == "" {
		return nil, fmt.Errorf("adults is required")
	}
	adults, err := strconv.Atoi(adultsStr)
	if err != nil || adults <= 0 {
		return nil, fmt.Errorf("adults must be a positive integer")
	}

	return &SearchParams{
		City:    city,
		Checkin: checkin,
		Nights:  nights,
		Adults:  adults,
	}, nil
}

// ExtractIP extracts the client IP from the request.
// Checks X-Forwarded-For, X-Real-IP, then falls back to RemoteAddr.
func ExtractIP(r *http.Request) string {
	// Check X-Forwarded-For (first IP in the list)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	// Fallback to RemoteAddr (strip port)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
