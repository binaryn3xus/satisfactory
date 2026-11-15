# syntax=docker/dockerfile:1

# =============================================================================
# Stage 1: Build ficsit-portal static binary
# =============================================================================
FROM golang:1.27-bookworm AS builder

ARG TARGETARCH=amd64

WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ficsit-portal ./cmd/ficsit-portal

# =============================================================================
# Stage 2: Minimal Non-Root Runtime
# =============================================================================
FROM alpine:3.21

LABEL org.opencontainers.image.title="satisfactory-portal"
LABEL org.opencontainers.image.description="Satisfactory Save, SMM Mod Profile & Blueprint Web Portal with Prometheus Metrics"
LABEL org.opencontainers.image.source="https://github.com/binaryn3xus/satisfactory"
LABEL org.opencontainers.image.licenses="MIT"

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -g 1000 ficsit \
    && adduser -u 1000 -G ficsit -D -h /config ficsit

COPY --from=builder /out/ficsit-portal /usr/local/bin/ficsit-portal

USER 1000:1000
WORKDIR /config

EXPOSE 8080/tcp

HEALTHCHECK --interval=15s --timeout=5s --start-period=5s --retries=3 \
    CMD ["wget", "-qO-", "http://127.0.0.1:8080/healthz"]

ENTRYPOINT ["/usr/local/bin/ficsit-portal"]
