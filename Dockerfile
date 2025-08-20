# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates for building
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o vpn-commander .

# Final stage
FROM scratch

# Import ca-certificates from builder stage
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Import timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /app/vpn-commander /vpn-commander

# Create non-root user (user ID 1000)
USER 1000:1000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD ["/vpn-commander", "--health-check"] || exit 1

# Expose port (if needed for health checks)
EXPOSE 8080

# Set entrypoint
ENTRYPOINT ["/vpn-commander"]