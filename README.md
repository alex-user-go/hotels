# Hotel Search Aggregator Service

An HTTP service that aggregates hotel search results from multiple providers with caching, rate limiting, and observability.

## Features

- Multi-provider aggregation with concurrent queries
- In-memory cache with request collapsing (30s TTL)
- Rate limiting (10 requests/minute per IP)
- Automatic deduplication by hotel ID (keeps lowest price)
- Prometheus metrics and health checks
- Graceful degradation on provider failures

## Quick Start

### Using Make

```bash
# Start all services
make run

# Test the API
curl "http://localhost:8080/search?city=paris&checkin=2025-12-01&nights=2&adults=2"

# Stop all services
make stop
```

### Using Docker Compose

See [docker-compose.yml](docker-compose.yml) and [Dockerfile](Dockerfile) for container deployment.

## API

### Search Hotels

```bash
GET /search?city=<string>&checkin=YYYY-MM-DD&nights=<int>&adults=<int>
```

**Example:**

```bash
curl "http://localhost:8080/search?city=paris&checkin=2025-12-01&nights=2&adults=2"
```

**Response:**

```json
{
  "search": {
    "city": "paris",
    "checkin": "2025-12-01",
    "nights": 2,
    "adults": 2
  },
  "stats": {
    "providers_total": 3,
    "providers_succeeded": 3,
    "providers_failed": 0,
    "cache": "miss",
    "duration_ms": 150
  },
  "hotels": [
    {
      "hotel_id": "H003",
      "name": "Budget Stay",
      "currency": "EUR",
      "price": 120.50
    },
    {
      "hotel_id": "H002",
      "name": "City Center Inn",
      "currency": "EUR",
      "price": 180.00
    }
  ]
}
```

### Health Check

```bash
curl http://localhost:8080/healthz
```

### Metrics

```bash
curl http://localhost:8080/metrics
```

## Development

### Build

```bash
make build
```

### Test

```bash
make test               # Run tests
make test-coverage      # Run tests with coverage
```

### Run Locally

```bash
# Option 1: All services
make run

# Option 2: Separate terminals
make providers  # Terminal 1
make server     # Terminal 2
```

### Available Commands

```bash
make help          # Show all commands
make build         # Build binaries
make test          # Run tests
make run           # Start all services
make stop          # Stop all services
make clean         # Clean artifacts
make fmt           # Format code
```

## Configuration

### Environment Variables

**Main Service:**
- `PROVIDER1_URL` - Provider 1 URL (default: http://localhost:9001)
- `PROVIDER2_URL` - Provider 2 URL (default: http://localhost:9002)
- `PROVIDER3_URL` - Provider 3 URL (default: http://localhost:9003)

**Mock Providers:**
- `PORT` - Server port (default: 9001)
- `PROVIDER_TYPE` - Mock type: mock1, mock2, or mock3

### Service Defaults

- Cache TTL: 30 seconds
- Rate Limit: 10 requests/minute per IP
- Provider Timeout: 2 seconds
- Server Port: 8080

## Testing Scenarios

### Cache Behavior

```bash
# First request (cache miss)
curl "http://localhost:8080/search?city=paris&checkin=2025-12-01&nights=2&adults=2"

# Second request (cache hit - faster)
curl "http://localhost:8080/search?city=paris&checkin=2025-12-01&nights=2&adults=2"
```

### Rate Limiting

```bash
# Send 11 requests - 11th will fail with 429
for i in {1..11}; do
  curl "http://localhost:8080/search?city=paris&checkin=2025-12-01&nights=2&adults=2"
done
```

## Mock Provider Behavior

**Provider 1 (Mock1):**
- Latency: 50-200ms
- Failure Rate: 10%
- Hotels: H001-H004
- Price: Based on nights × per-night rate

**Provider 2 (Mock2):**
- Latency: 75-300ms
- Failure Rate: 15%
- Hotels: H001-H003, H005
- Special: 30% chance of invalid data

**Provider 3 (Mock3):**
- Latency: 60-240ms
- Failure Rate: 10%
- Hotels: H001-H003, H006
- Special: 50% chance of duplicate H001

## Known Limitations

Simplified/not implemented according to PDF specification:

- **Input validation**: No date validation or bounds checking on nights/adults parameters
- **Mock providers**: Only Mock1 uses nights parameter; Mock2/Mock3 use static pricing
- **Cache/rate limiter**: No memory limits or cleanup; could grow unbounded over time
- **Configuration**: Timeout, cache TTL, and rate limits are hardcoded (not configurable via env vars)
- **Testing**: Unit tests only; no integration or load tests

## License

MIT
