# Multi-stage build for hotel search aggregator

# Stage 1: Build
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binaries
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags "-static"' -o provider ./cmd/provider

# Stage 2: Runtime
FROM alpine:latest

# Install ca-certificates for HTTPS calls
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/server .
COPY --from=builder /build/provider .

# Create non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

# Expose ports
EXPOSE 8080

# Run server by default
CMD ["./server"]
