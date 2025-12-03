package types

// Result represents aggregated search results.
type Result struct {
	Hotels             []Hotel `json:"hotels"`
	ProvidersTotal     int     `json:"-"`
	ProvidersSucceeded int     `json:"-"`
	ProvidersFailed    int     `json:"-"`
}

// Hotel represents a normalized hotel.
type Hotel struct {
	HotelID  string  `json:"hotel_id"`
	Name     string  `json:"name"`
	Currency string  `json:"currency"`
	Price    float64 `json:"price"`
}
