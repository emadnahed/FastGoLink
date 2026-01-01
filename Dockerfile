# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first for layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o /app/bin/gourl \
    ./cmd/api

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata curl

# Create non-root user
RUN addgroup -g 1000 gourl && \
    adduser -u 1000 -G gourl -s /bin/sh -D gourl

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bin/gourl /app/gourl

# Copy migrations if needed
COPY --from=builder /app/migrations /app/migrations

# Copy API documentation
COPY --from=builder /app/docs /app/docs

# Set ownership
RUN chown -R gourl:gourl /app

# Switch to non-root user
USER gourl

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/gourl"]
