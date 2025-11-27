package search_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alex-user-go/hotels/internal/obs"
	"github.com/alex-user-go/hotels/internal/providers"
	"github.com/alex-user-go/hotels/internal/search"
)

// mockProvider is a test provider that returns predefined results.
type mockProvider struct {
	name   string
	hotels []providers.Hotel
	err    error
	delay  time.Duration
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Search(ctx context.Context, city, checkin string, nights, adults int) ([]providers.Hotel, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
	}
	return m.hotels, m.err
}

func TestAggregator_Search_Merging(t *testing.T) {
	providers := []providers.Provider{
		&mockProvider{
			name: "provider1",
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Hotel A", Currency: "EUR", Price: 100},
				{HotelID: "H002", Name: "Hotel B", Currency: "EUR", Price: 150},
			},
		},
		&mockProvider{
			name: "provider2",
			hotels: []providers.Hotel{
				{HotelID: "H003", Name: "Hotel C", Currency: "EUR", Price: 120},
				{HotelID: "H004", Name: "Hotel D", Currency: "EUR", Price: 200},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 2*time.Second, metrics, logger)

	result, err := agg.Search(context.Background(), "paris", "2025-12-01", 2, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ProvidersTotal != 2 {
		t.Errorf("expected 2 total providers, got %d", result.ProvidersTotal)
	}

	if result.ProvidersSucceeded != 2 {
		t.Errorf("expected 2 succeeded providers, got %d", result.ProvidersSucceeded)
	}

	if result.ProvidersFailed != 0 {
		t.Errorf("expected 0 failed providers, got %d", result.ProvidersFailed)
	}

	if len(result.Hotels) != 4 {
		t.Fatalf("expected 4 hotels, got %d", len(result.Hotels))
	}

	// Verify sorted by price ascending
	if result.Hotels[0].Price != 100 {
		t.Errorf("expected first hotel price 100, got %v", result.Hotels[0].Price)
	}
	if result.Hotels[3].Price != 200 {
		t.Errorf("expected last hotel price 200, got %v", result.Hotels[3].Price)
	}
}

func TestAggregator_Search_Deduplication(t *testing.T) {
	providers := []providers.Provider{
		&mockProvider{
			name: "provider1",
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Hotel A", Currency: "EUR", Price: 150},
				{HotelID: "H002", Name: "Hotel B", Currency: "EUR", Price: 200},
			},
		},
		&mockProvider{
			name: "provider2",
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Hotel A", Currency: "EUR", Price: 120}, // Duplicate, lower price
				{HotelID: "H003", Name: "Hotel C", Currency: "EUR", Price: 180},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 2*time.Second, metrics, logger)

	result, err := agg.Search(context.Background(), "paris", "2025-12-01", 2, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 3 unique hotels (H001 deduplicated)
	if len(result.Hotels) != 3 {
		t.Fatalf("expected 3 unique hotels, got %d", len(result.Hotels))
	}

	// H001 should have the lower price (120)
	var h001Found bool
	for _, h := range result.Hotels {
		if h.HotelID == "H001" {
			h001Found = true
			if h.Price != 120 {
				t.Errorf("expected H001 price 120 (lowest), got %v", h.Price)
			}
		}
	}
	if !h001Found {
		t.Error("H001 not found in results")
	}
}

func TestAggregator_Search_Timeout(t *testing.T) {
	providers := []providers.Provider{
		&mockProvider{
			name:  "fast-provider",
			delay: 50 * time.Millisecond,
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Hotel A", Currency: "EUR", Price: 100},
			},
		},
		&mockProvider{
			name:  "slow-provider",
			delay: 2 * time.Second, // Will timeout
			hotels: []providers.Hotel{
				{HotelID: "H002", Name: "Hotel B", Currency: "EUR", Price: 150},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 500*time.Millisecond, metrics, logger) // 500ms timeout

	result, err := agg.Search(context.Background(), "paris", "2025-12-01", 2, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Fast provider should succeed, slow provider should fail
	if result.ProvidersSucceeded != 1 {
		t.Errorf("expected 1 succeeded provider, got %d", result.ProvidersSucceeded)
	}

	if result.ProvidersFailed != 1 {
		t.Errorf("expected 1 failed provider (timeout), got %d", result.ProvidersFailed)
	}

	// Should only have hotel from fast provider
	if len(result.Hotels) != 1 {
		t.Fatalf("expected 1 hotel, got %d", len(result.Hotels))
	}

	if result.Hotels[0].HotelID != "H001" {
		t.Errorf("expected H001, got %s", result.Hotels[0].HotelID)
	}
}

func TestAggregator_Search_PartialFailure(t *testing.T) {
	providerErr := errors.New("provider unavailable")
	providers := []providers.Provider{
		&mockProvider{
			name: "success-provider",
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Hotel A", Currency: "EUR", Price: 100},
			},
		},
		&mockProvider{
			name: "failed-provider",
			err:  providerErr,
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 2*time.Second, metrics, logger)

	result, err := agg.Search(context.Background(), "paris", "2025-12-01", 2, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ProvidersSucceeded != 1 {
		t.Errorf("expected 1 succeeded provider, got %d", result.ProvidersSucceeded)
	}

	if result.ProvidersFailed != 1 {
		t.Errorf("expected 1 failed provider, got %d", result.ProvidersFailed)
	}

	// Should still return results from successful provider
	if len(result.Hotels) != 1 {
		t.Fatalf("expected 1 hotel, got %d", len(result.Hotels))
	}
}

func TestAggregator_Search_AllProvidersFail(t *testing.T) {
	providerErr := errors.New("all providers down")
	providers := []providers.Provider{
		&mockProvider{name: "provider1", err: providerErr},
		&mockProvider{name: "provider2", err: providerErr},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 2*time.Second, metrics, logger)

	result, err := agg.Search(context.Background(), "paris", "2025-12-01", 2, 2)
	if err == nil {
		t.Fatal("expected error when all providers fail, got nil")
	}

	if result != nil {
		t.Errorf("expected nil result when all providers fail, got %v", result)
	}
}

func TestAggregator_Search_InvalidDataFiltered(t *testing.T) {
	providers := []providers.Provider{
		&mockProvider{
			name: "provider1",
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Valid Hotel", Currency: "EUR", Price: 100},
				{HotelID: "", Name: "Invalid - No ID", Currency: "EUR", Price: 150},   // Filtered
				{HotelID: "H003", Name: "", Currency: "EUR", Price: 120},              // Filtered
				{HotelID: "H004", Name: "Invalid Price", Currency: "EUR", Price: 0},   // Filtered
				{HotelID: "H005", Name: "Invalid Price", Currency: "EUR", Price: -50}, // Filtered
				{HotelID: "H006", Name: "Valid Hotel 2", Currency: "usd", Price: 200}, // Valid, currency normalized
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 2*time.Second, metrics, logger)

	result, err := agg.Search(context.Background(), "paris", "2025-12-01", 2, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 2 valid hotels
	if len(result.Hotels) != 2 {
		t.Fatalf("expected 2 valid hotels, got %d", len(result.Hotels))
	}

	// Check currency normalization
	for _, h := range result.Hotels {
		if h.HotelID == "H006" && h.Currency != "USD" {
			t.Errorf("expected currency USD (normalized), got %s", h.Currency)
		}
	}
}

func TestAggregator_Search_ContextCancellation(t *testing.T) {
	providers := []providers.Provider{
		&mockProvider{
			name:  "slow-provider",
			delay: 2 * time.Second,
			hotels: []providers.Hotel{
				{HotelID: "H001", Name: "Hotel A", Currency: "EUR", Price: 100},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metrics := obs.NewMetrics(logger)
	agg := search.NewAggregator(providers, 10*time.Second, metrics, logger)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := agg.Search(ctx, "paris", "2025-12-01", 2, 2)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}

	if result != nil {
		t.Errorf("expected nil result from cancelled context, got %v", result)
	}
}
