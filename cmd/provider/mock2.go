package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Mock2 is the second mock provider with 150ms base latency and 15% failure rate.
type Mock2 struct {
	rng    *rand.Rand
	logger *slog.Logger
}

// NewMock2 creates a new Mock2 provider.
func NewMock2() *Mock2 {
	return &Mock2{
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

// search simulates searching for hotels with random latency and potential failures.
func (p *Mock2) search(ctx context.Context, city, _ string, nights, _ int) ([]hotel, error) {
	// Simulate random latency (75ms to 300ms)
	latency := time.Duration(75+p.rng.Intn(225)) * time.Millisecond

	select {
	case <-time.After(latency):
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}

	// Simulate 15% failure rate
	if p.rng.Float64() < 0.15 {
		return nil, errProviderUnavailable
	}

	// Generate hotels
	return p.generateHotels(city, nights), nil
}

func (p *Mock2) generateHotels(city string, nights int) []hotel {
	city = strings.ToLower(strings.TrimSpace(city))

	hotels := []hotel{
		{
			HotelID:  "H001",
			Name:     "Grand Hotel",
			City:     city,
			Currency: "EUR",
			Price:    p.randomPrice(100, 200),
			Nights:   nights,
		},
		{
			HotelID:  "H002",
			Name:     "City Center Inn",
			City:     city,
			Currency: "eur",
			Price:    p.randomPrice(80, 150),
			Nights:   nights,
		},
		{
			HotelID:  "H003",
			Name:     "Budget Stay",
			City:     city,
			Currency: "EUR",
			Price:    p.randomPrice(50, 100),
			Nights:   nights,
		},
		{
			HotelID:  "H005",
			Name:     "Seaside Resort",
			City:     city,
			Currency: "EUR",
			Price:    p.randomPrice(150, 300),
			Nights:   nights,
		},
	}

	// Sometimes return invalid data (missing hotel_id)
	if p.rng.Float64() < 0.3 {
		hotels = append(hotels, hotel{
			HotelID:  "", // Invalid - should be filtered out
			Name:     "Mystery Hotel",
			City:     city,
			Currency: "EUR",
			Price:    100,
			Nights:   nights,
		})
	}

	return hotels
}

func (p *Mock2) randomPrice(min, max float64) float64 {
	price := min + p.rng.Float64()*(max-min)
	return float64(int(price*100)) / 100
}

// ServeHTTP handles HTTP requests for this provider.
func (p *Mock2) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	checkin := strings.TrimSpace(r.URL.Query().Get("checkin"))
	nightsStr := r.URL.Query().Get("nights")
	adultsStr := r.URL.Query().Get("adults")

	if city == "" || checkin == "" || nightsStr == "" || adultsStr == "" {
		http.Error(w, "missing required parameters", http.StatusBadRequest)
		return
	}

	nights, err := strconv.Atoi(nightsStr)
	if err != nil || nights <= 0 {
		http.Error(w, "invalid nights", http.StatusBadRequest)
		return
	}

	adults, err := strconv.Atoi(adultsStr)
	if err != nil || adults <= 0 {
		http.Error(w, "invalid adults", http.StatusBadRequest)
		return
	}

	// Use the search method
	hotels, err := p.search(r.Context(), city, checkin, nights, adults)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(hotels); err != nil {
		p.logger.Error("failed to encode response", "error", err)
		return
	}
}
