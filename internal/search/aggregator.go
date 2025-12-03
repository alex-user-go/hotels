package search

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alex-user-go/hotels/internal/obs"
	"github.com/alex-user-go/hotels/internal/providers"
	"github.com/alex-user-go/hotels/internal/search/types"
)

// Aggregator aggregates results from multiple providers.
type Aggregator struct {
	providers []providers.Provider
	timeout   time.Duration
	metrics   *obs.Metrics
	logger    *slog.Logger
}

// NewAggregator creates a new Aggregator.
func NewAggregator(providers []providers.Provider, timeout time.Duration, metrics *obs.Metrics, logger *slog.Logger) *Aggregator {
	return &Aggregator{
		providers: providers,
		timeout:   timeout,
		metrics:   metrics,
		logger:    logger,
	}
}

// Search queries all providers concurrently and aggregates results.
func (a *Aggregator) Search(ctx context.Context, city, checkin string, nights, adults int) (*types.Result, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	var (
		mu        sync.Mutex
		wg        sync.WaitGroup
		hotelMap  = make(map[string]types.Hotel)
		succeeded int
		failed    int
		errors    []error
	)

	for _, provider := range a.providers {
		wg.Go(func() {
			hotels, err := provider.Search(ctx, city, checkin, nights, adults)
			if err != nil {
				mu.Lock()
				failed++
				errors = append(errors, err)
				mu.Unlock()
				a.metrics.IncProviderErrors()
				return
			}

			mu.Lock()
			succeeded++
			for _, h := range hotels {
				normalized := normalizeHotel(h)
				if normalized == nil {
					continue
				}

				// Dedup by hotel_id, keep lowest price
				if existing, ok := hotelMap[normalized.HotelID]; ok {
					if normalized.Price < existing.Price {
						hotelMap[normalized.HotelID] = *normalized
					}
				} else {
					hotelMap[normalized.HotelID] = *normalized
				}
			}
			mu.Unlock()
		})
	}

	// Wait for all providers to complete
	wg.Wait()

	// Log provider errors if any
	if len(errors) > 0 {
		a.logger.Error("provider search errors",
			"city", city,
			"failed_count", failed,
			"errors", errors)

		// If all providers failed, return error
		if failed == len(a.providers) {
			return nil, errors[0]
		}
	}

	// Convert map to slice and sort by price
	hotels := make([]types.Hotel, 0, len(hotelMap))
	for _, h := range hotelMap {
		hotels = append(hotels, h)
	}
	sort.Slice(hotels, func(i, j int) bool {
		return hotels[i].Price < hotels[j].Price
	})

	return &types.Result{
		Hotels:             hotels,
		ProvidersTotal:     len(a.providers),
		ProvidersSucceeded: succeeded,
		ProvidersFailed:    failed,
	}, nil
}

func normalizeHotel(h providers.Hotel) *types.Hotel {
	// Drop invalid data
	hotelID := strings.TrimSpace(h.HotelID)
	if hotelID == "" {
		return nil
	}

	name := strings.TrimSpace(h.Name)
	if name == "" {
		return nil
	}

	if h.Price <= 0 {
		return nil
	}

	currency := strings.ToUpper(strings.TrimSpace(h.Currency))
	if currency == "" {
		currency = "EUR"
	}

	return &types.Hotel{
		HotelID:  hotelID,
		Name:     name,
		Currency: currency,
		Price:    h.Price,
	}
}
