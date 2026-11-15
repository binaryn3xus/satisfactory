# syntax=docker/dockerfile:1

# =============================================================================
# Stage 1: Build ficsit-ctl and fetch ficsit-cli
# =============================================================================
FROM golang:1.27-bookworm AS builder

ARG FICSIT_CLI_VERSION=v0.7.1
ARG TARGETARCH=amd64

WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /out/ficsit-ctl ./cmd/ficsit-ctl

RUN curl -fsSL "https://github.com/satisfactorymodding/ficsit-cli/releases/download/${FICSIT_CLI_VERSION}/ficsit_linux_${TARGETARCH}" \
    -o /out/ficsit-cli && \
    chmod +x /out/ficsit-cli

# =============================================================================
# Stage 2: Satisfactory Dedicated Server Runtime
# =============================================================================
FROM steamcmd/steamcmd:ubuntu-24

LABEL org.opencontainers.image.title="satisfactory-server"
LABEL org.opencontainers.image.description="Satisfactory Dedicated Server with built-in ficsit-cli Mod Manager and Strict World-Protection Gate"
LABEL org.opencontainers.image.source="https://github.com/binaryn3xus/satisfactory"
LABEL org.opencontainers.image.licenses="MIT"

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gosu \
        jq \
        libatomic1 \
        libcurl4 \
        libglib2.0-0 \
        libpulse0 \
        libsdl2-2.0-0 \
        procps \
        tzdata \
        xdg-user-dirs \
    && rm -rf /var/lib/apt/lists/* \
    && userdel -r ubuntu 2>/dev/null || true \
    && groupadd -f -g 1000 steam \
    && if ! id -u steam >/dev/null 2>&1; then \
         useradd -o -u 1000 -g 1000 -m -d /home/steam -s /bin/bash steam; \
       else \
         usermod -o -u 1000 -g 1000 steam; \
       fi \
    && mkdir -p /config /certs /home/steam \
    && chown -R 1000:1000 /config /home/steam

COPY --from=builder /out/ficsit-ctl /usr/local/bin/ficsit-ctl
COPY --from=builder /out/ficsit-cli /usr/local/bin/ficsit-cli
RUN ln -s /usr/local/bin/ficsit-cli /usr/local/bin/ficsit

COPY --chmod=0755 images/satisfactory-server/entrypoint.sh /usr/local/bin/entrypoint.sh

WORKDIR /config
VOLUME ["/config"]

EXPOSE 7777/udp 7777/tcp 8888/tcp

HEALTHCHECK --interval=30s --timeout=10s --start-period=180s --retries=3 \
    CMD ["ficsit-ctl", "healthcheck"]

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
