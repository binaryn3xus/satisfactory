set shell := ["bash", "-euo", "pipefail", "-c"]

# Default recipe: list available commands
default:
    @just --list

# Run all Go unit tests
test:
    go test -v -race ./...

# Build local Go binaries into ./bin
build:
    mkdir -p bin
    CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ficsit-ctl ./cmd/ficsit-ctl
    @if [ -d ./cmd/ficsit-portal ]; then \
        CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/ficsit-portal ./cmd/ficsit-portal; \
    fi

# Bundle Web Save Inspector JavaScript asset into cmd/ficsit-portal/assets
bundle-inspector:
    cd web/inspector && npx --yes esbuild src/index.js --bundle --minify --format=iife --global-name=FicsitInspectorBundle --outfile=../../cmd/ficsit-portal/assets/inspector.bundle.js

# Build the satisfactory-server Docker image
docker-build-server tag="latest":
    docker build -f images/satisfactory-server/Dockerfile -t ghcr.io/binaryn3xus/satisfactory-server:{{tag}} .

# Build the satisfactory-portal Docker image
docker-build-portal tag="latest":
    docker build -f images/satisfactory-portal/Dockerfile -t ghcr.io/binaryn3xus/satisfactory-portal:{{tag}} .

# Lint the Helm chart (when charts/satisfactory is present)
chart-lint:
    helm lint charts/satisfactory
