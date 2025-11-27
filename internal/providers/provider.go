package providers

import (
	"context"
	"errors"
)

// Hotel represents a hotel from a provider.
type Hotel struct {
	HotelID  string  `json:"hotel_id"`
	Name     string  `json:"name"`
	City     string  `json:"city"`
	Currency string  `json:"currency"`
	Price    float64 `json:"price"`
	Nights   int     `json:"nights"`
}

// Provider defines the interface for hotel providers.
type Provider interface {
	// Search searches for hotels.
	Search(ctx context.Context, city, checkin string, nights, adults int) ([]Hotel, error)
}

// ErrProviderUnavailable is returned when a provider is unavailable.
var ErrProviderUnavailable = errors.New("provider unavailable")
