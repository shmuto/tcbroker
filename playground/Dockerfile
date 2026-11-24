# Stage 1: Build tcbroker static binary with embedded mini-NPB WebUI
FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY VERSION ./
COPY cmd ./cmd
COPY pkg ./pkg

RUN VERSION=$(cat VERSION) && \
    CGO_ENABLED=0 GOOS=linux go build -buildvcs=false \
      -ldflags="-w -s -X main.version=${VERSION}" \
      -o tcbroker ./cmd/tcbroker

# Stage 2: Self-contained Linux TC playground (ip netns + tcpdump + traffic gen)
FROM alpine:3.21

RUN apk add --no-cache \
      bash \
      curl \
      iproute2 \
      iputils \
      python3 \
      tcpdump

COPY --from=builder /build/tcbroker /usr/local/bin/tcbroker
COPY playground/config.yaml /playground/config.yaml
COPY playground/entrypoint.sh /playground/entrypoint.sh
RUN chmod +x /playground/entrypoint.sh

WORKDIR /playground
EXPOSE 8080

ENTRYPOINT ["/playground/entrypoint.sh"]
