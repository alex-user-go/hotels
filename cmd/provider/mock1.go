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

// Mock1 is the first mock provider with 100ms base latency and 10% failure rate.
type Mock1 struct {
	rng    *rand.Rand
	logger *slog.Logger
}

// NewMock1 creates a new Mock1 provider.
func NewMock1() *Mock1 {
	return &Mock1{
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
		logger: slog.New(slog.NewTextHandler(os.Stdout, nil)),
	}
}

// search simulates searching for hotels with random latency and potential failures.
func (p *Mock1) search(ctx context.Context, city, _ string, nights, adults int) ([]hotel, error) {
	// Simulate random latency (50ms to 200ms)
	latency := time.Duration(50+p.rng.Intn(150)) * time.Millisecond

	select {
	case <-time.After(latency):
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}

	// Simulate 10% failure rate
	if p.rng.Float64() < 0.1 {
		return nil, errProviderUnavailable
	}

	// Generate hotels
	return p.generateHotels(city, nights, adults), nil
}

func (p *Mock1) generateHotels(city string, nights, _ int) []hotel {
	city = strings.ToLower(strings.TrimSpace(city))

	return []hotel{
		{
			HotelID:  "H001",
			Name:     "Grand Hotel",
			City:     city,
			Currency: "EUR",
			Price:    p.calculatePrice(100, 200, nights),
			Nights:   nights,
		},
		{
			HotelID:  "H002",
			Name:     "City Center Inn",
			City:     city,
			Currency: "eur", // Inconsistent casing
			Price:    p.calculatePrice(80, 150, nights),
			Nights:   nights,
		},
		{
			HotelID:  "H003",
			Name:     "Budget Stay",
			City:     city,
			Currency: "EUR",
			Price:    p.calculatePrice(50, 100, nights),
			Nights:   nights,
		},
		{
			HotelID:  "H004",
			Name:     "Luxury Palace",
			City:     city,
			Currency: "EUR",
			Price:    p.calculatePrice(200, 400, nights),
			Nights:   nights,
		},
	}
}

func (p *Mock1) randomPrice(min, max float64) float64 {
	price := min + p.rng.Float64()*(max-min)
	return float64(int(price*100)) / 100
}

// calculatePrice calculates the total price based on per-night rate and number of nights.
func (p *Mock1) calculatePrice(minPerNight, maxPerNight float64, nights int) float64 {
	perNightPrice := p.randomPrice(minPerNight, maxPerNight)
	totalPrice := perNightPrice * float64(nights)
	return float64(int(totalPrice*100)) / 100
}

// ServeHTTP handles HTTP requests for this provider.
func (p *Mock1) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
