# Multi-stage build for tcbroker
# Stage 1: Fast native cross-compilation on the build host
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=""

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY VERSION ./
COPY cmd ./cmd
COPY pkg ./pkg

RUN VER="${VERSION:-$(cat VERSION)}" && \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
      go build -buildvcs=false \
      -ldflags="-w -s -X main.version=${VER}" \
      -o tcbroker ./cmd/tcbroker

# Stage 2: Minimal runtime image
FROM alpine:3.21

# iproute2 provides the `tc` utility required for Linux Traffic Control operations
RUN apk add --no-cache iproute2

COPY --from=builder /build/tcbroker /usr/local/bin/tcbroker

WORKDIR /config

# TC operations require CAP_NET_ADMIN (run with --cap-add=NET_ADMIN --network host
# and mount /run/tcbroker:/run/tcbroker to persist runtime state)
ENTRYPOINT ["/usr/local/bin/tcbroker"]
CMD ["--help"]
