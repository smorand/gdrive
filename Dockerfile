# Multi-stage Dockerfile for gdrive MCP server
# Stage 1: Build the Go binary

FROM golang:1.25 AS builder

WORKDIR /app

# Copy go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# - CGO_ENABLED=0 for static binary
# - -ldflags="-s -w" strips debug info for smaller binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w" \
    -o /gdrive \
    ./cmd/gdrive

# Stage 2: Final minimal image

FROM alpine:latest

# Install ca-certificates for HTTPS requests to Google APIs
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user for security
RUN adduser -D -g '' appuser

WORKDIR /app

# Copy binary from builder
COPY --from=builder /gdrive /app/gdrive

# Change ownership to appuser
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose the default MCP server port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command: start MCP server
# Environment variables expected:
# - PORT (optional, defaults to 8080)
# - SECRET_NAME (required for OAuth credentials from Secret Manager)
# - SECRET_PROJECT (required for Secret Manager project)
# - BASE_URL (required for OAuth redirect URLs)
ENTRYPOINT ["/app/gdrive", "mcp"]
