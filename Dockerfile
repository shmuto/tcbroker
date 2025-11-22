# Multi-stage build for tcbroker
# Stage 1: Build the binary
FROM golang:1.24-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git make

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -buildvcs=false -ldflags="-w -s" -o tcbroker ./cmd/tcbroker

# Stage 2: Runtime image
FROM alpine:latest

# Install iproute2 for tc command
RUN apk add --no-cache iproute2

# Copy binary from builder
COPY --from=builder /build/tcbroker /usr/local/bin/tcbroker

# Create non-root user (though tc requires root privileges)
RUN addgroup -g 1000 tcbroker && \
    adduser -D -u 1000 -G tcbroker tcbroker

WORKDIR /config

# tcbroker requires root for tc operations, but we set the user for better security practice
# User can override with --user root when running container
USER root

ENTRYPOINT ["/usr/local/bin/tcbroker"]
CMD ["--help"]
