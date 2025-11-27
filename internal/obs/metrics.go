package obs

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync/atomic"
)

// Metrics tracks application metrics using atomic counters.
type Metrics struct {
	requests       atomic.Int64
	cacheHits      atomic.Int64
	providerErrors atomic.Int64
	logger         *slog.Logger
}

// NewMetrics creates a new Metrics instance.
func NewMetrics(logger *slog.Logger) *Metrics {
	return &Metrics{
		logger: logger,
	}
}

// IncRequests increments the total request counter.
func (m *Metrics) IncRequests() {
	m.requests.Add(1)
}

// IncCacheHits increments the cache hits counter.
func (m *Metrics) IncCacheHits() {
	m.cacheHits.Add(1)
}

// IncProviderErrors increments the provider errors counter.
func (m *Metrics) IncProviderErrors() {
	m.providerErrors.Add(1)
}

// Snapshot returns current metric values.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		Requests:       m.requests.Load(),
		CacheHits:      m.cacheHits.Load(),
		ProviderErrors: m.providerErrors.Load(),
	}
}

// MetricsSnapshot represents a point-in-time snapshot of metrics.
type MetricsSnapshot struct {
	Requests       int64
	CacheHits      int64
	ProviderErrors int64
}

// HealthHandler returns a handler for /healthz requests.
func HealthHandler(logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			logger.Error("failed to write health response", "error", err)
		}
	}
}

// MetricsHandler returns a handler for /metrics requests in Prometheus format.
func (m *Metrics) MetricsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := m.Snapshot()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)

		// Write metrics in Prometheus format
		if _, err := fmt.Fprintf(w, "# HELP requests_total Total number of requests\n"); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "# TYPE requests_total counter\n"); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "requests_total %d\n", snapshot.Requests); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}

		if _, err := fmt.Fprintf(w, "# HELP cache_hits_total Total number of cache hits\n"); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "# TYPE cache_hits_total counter\n"); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "cache_hits_total %d\n", snapshot.CacheHits); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}

		if _, err := fmt.Fprintf(w, "# HELP provider_errors_total Total number of provider errors\n"); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "# TYPE provider_errors_total counter\n"); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
		if _, err := fmt.Fprintf(w, "provider_errors_total %d\n", snapshot.ProviderErrors); err != nil {
			m.logger.Error("failed to write metrics", "error", err)
			return
		}
	}
}
