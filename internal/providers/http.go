package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// HTTPProvider queries a real HTTP endpoint for hotel data.
type HTTPProvider struct {
	name       string
	baseURL    string
	httpClient *http.Client
}

// NewHTTPProvider creates a new HTTPProvider.
func NewHTTPProvider(name, baseURL string, timeout time.Duration) *HTTPProvider {
	return &HTTPProvider{
		name:    name,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Name returns the provider name.
func (p *HTTPProvider) Name() string {
	return p.name
}

// Search searches for hotels by making an HTTP GET request.
func (p *HTTPProvider) Search(ctx context.Context, city, checkin string, nights, adults int) ([]Hotel, error) {
	// Build URL with query parameters
	u, err := url.Parse(p.baseURL + "/search")
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	q := u.Query()
	q.Set("city", city)
	q.Set("checkin", checkin)
	q.Set("nights", fmt.Sprintf("%d", nights))
	q.Set("adults", fmt.Sprintf("%d", adults))
	u.RawQuery = q.Encode()

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() // Explicitly ignore close error
	}()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("provider returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse JSON response
	var hotels []Hotel
	if err := json.NewDecoder(resp.Body).Decode(&hotels); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return hotels, nil
}
