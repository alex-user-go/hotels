package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// hotel represents a hotel returned by the mock providers.
type hotel struct {
	HotelID  string  `json:"hotel_id"`
	Name     string  `json:"name"`
	City     string  `json:"city"`
	Currency string  `json:"currency"`
	Price    float64 `json:"price"`
	Nights   int     `json:"nights"`
}

var errProviderUnavailable = errors.New("provider unavailable")

func main() {
	port := getEnv("PORT", "9001")
	providerType := getEnv("PROVIDER_TYPE", "mock1")

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	var handler http.Handler

	switch providerType {
	case "mock1":
		handler = NewMock1()
		logger.Info("starting provider", "type", "provider1", "port", port)
	case "mock2":
		handler = NewMock2()
		logger.Info("starting provider", "type", "provider2", "port", port)
	case "mock3":
		handler = NewMock3()
		logger.Info("starting provider", "type", "provider3", "port", port)
	default:
		logger.Error("unknown provider type", "type", providerType)
		os.Exit(1)
	}

	// Setup routes
	mux := http.NewServeMux()
	mux.Handle("/search", handler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			logger.Error("failed to write healthz response", "error", err)
		}
	})

	// Configure server
	addr := ":" + port
	srv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		logger.Info("server listening", "addr", addr)
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
		os.Exit(1)
	}

	logger.Info("server stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
