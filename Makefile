.PHONY: build test run clean help providers server stop

# Build all binaries
build:
	@echo "Building binaries..."
	@go build -o server ./cmd/server
	@go build -o provider ./cmd/provider
	@echo "✓ Build complete"

# Run tests with race detector
test:
	@go test -race -v ./...

# Run tests with coverage
test-coverage:
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"

# Start all services (foreground - Ctrl+C to stop)
run: build
	@echo "Starting services..."
	@trap 'pkill -P $$; exit' INT; \
	PORT=9001 PROVIDER_TYPE=mock1 ./provider & \
	PORT=9002 PROVIDER_TYPE=mock2 ./provider & \
	PORT=9003 PROVIDER_TYPE=mock3 ./provider & \
	sleep 1; \
	./server & \
	sleep 1; \
	echo "✓ Services started:"; \
	echo "  - Providers: http://localhost:9001-9003"; \
	echo "  - Server: http://localhost:8080"; \
	echo "  - Press Ctrl+C to stop all services"; \
	wait

# Start only provider servers in background
providers: build
	@echo "Starting providers..."
	@PORT=9001 PROVIDER_TYPE=mock1 ./provider &
	@PORT=9002 PROVIDER_TYPE=mock2 ./provider &
	@PORT=9003 PROVIDER_TYPE=mock3 ./provider &
	@sleep 1
	@echo "✓ Providers started on ports 9001-9003"

# Start only main server (foreground)
server: build
	@./server

# Stop all services
stop:
	@echo "Stopping services..."
	@-pkill -f "./provider" 2>/dev/null || true
	@-pkill -f "./server" 2>/dev/null || true
	@echo "✓ Services stopped"

# Clean build artifacts
clean:
	@rm -f server provider coverage.out coverage.html
	@echo "✓ Clean complete"

# Format code
fmt:
	@go fmt ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  make build         - Build all binaries"
	@echo "  make test          - Run tests with race detector"
	@echo "  make test-coverage - Run tests with coverage report"
	@echo "  make run           - Start all services (Ctrl+C to stop)"
	@echo "  make providers     - Start only provider servers in background"
	@echo "  make server        - Start only main server (foreground)"
	@echo "  make stop          - Stop all services"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make fmt           - Format code"
	@echo "  make help          - Show this help"
