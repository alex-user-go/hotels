package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alex-user-go/hotels/internal/handler"
	"github.com/alex-user-go/hotels/internal/middleware"
	"github.com/alex-user-go/hotels/internal/obs"
	"github.com/alex-user-go/hotels/internal/providers"
	"github.com/alex-user-go/hotels/internal/search"
	"github.com/alex-user-go/hotels/internal/search/cache"
	"github.com/alex-user-go/hotels/internal/search/ratelimit"
)

// Run initializes and runs the application.
func Run() error {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Initialize metrics
	metrics := obs.NewMetrics(logger)

	// Initialize providers (HTTP clients)
	providersList := []providers.Provider{
		providers.NewHTTPProvider("provider1", getEnv("PROVIDER1_URL", "http://localhost:9001"), 2*time.Second),
		providers.NewHTTPProvider("provider2", getEnv("PROVIDER2_URL", "http://localhost:9002"), 2*time.Second),
		providers.NewHTTPProvider("provider3", getEnv("PROVIDER3_URL", "http://localhost:9003"), 2*time.Second),
	}

	// Initialize aggregator
	aggregator := search.NewAggregator(
		providersList,
		2*time.Second,
		metrics,
		logger,
	)

	// Initialize cache
	searchCache := cache.NewCache(30 * time.Second)
	defer searchCache.Close()

	// Initialize rate limiter (10 requests per minute per IP)
	limiter := ratelimit.New(10, time.Minute)
	defer limiter.Close()

	// Initialize handler
	h := handler.New(aggregator, searchCache, limiter, metrics, logger)

	// Setup routes with logging middleware
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search", h.SearchHandler)
	mux.HandleFunc("GET /healthz", obs.HealthHandler(logger))
	mux.HandleFunc("GET /metrics", metrics.MetricsHandler())

	// Wrap with middleware
	wrappedHandler := middleware.Logging(logger)(mux)

	// Configure server
	srv := &http.Server{
		Addr:         ":8080",
		Handler:      wrappedHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("starting server", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	logger.Info("shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("server shutdown error", "error", err)
		return err
	}

	logger.Info("server stopped")
	return nil
}

// getEnv gets an environment variable with a default fallback.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
