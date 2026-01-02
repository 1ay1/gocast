# Build stage
FROM golang:1.22-alpine AS builder

# Install git and ca-certificates (for HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.version=1.0.0" \
    -o gocast \
    ./cmd/gocast

# Runtime stage
FROM alpine:3.19

# Install ca-certificates for HTTPS support
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 gocast && \
    adduser -u 1000 -G gocast -s /bin/sh -D gocast

# Create directories
RUN mkdir -p /etc/gocast /var/log/gocast /var/lib/gocast && \
    chown -R gocast:gocast /etc/gocast /var/log/gocast /var/lib/gocast

# Copy binary from builder
COPY --from=builder /build/gocast /usr/local/bin/gocast

# Copy default configuration
COPY --from=builder /build/gocast.vibe /etc/gocast/gocast.vibe

# Set ownership
RUN chown gocast:gocast /usr/local/bin/gocast

# Switch to non-root user
USER gocast

# Set working directory
WORKDIR /var/lib/gocast

# Expose default ports
# 8000 - HTTP streaming
# 8443 - HTTPS streaming (optional)
EXPOSE 8000 8443

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8000/ || exit 1

# Default command
ENTRYPOINT ["/usr/local/bin/gocast"]
CMD ["-config", "/etc/gocast/gocast.vibe"]
