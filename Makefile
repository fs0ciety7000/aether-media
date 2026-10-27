VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
PKG     := github.com/fs0ciety7000/aether-media/internal/version
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT)

.PHONY: all web build test lint sqlc dev-api dev-web clean

all: web build

web:
	cd web && npm ci && npm run build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/aether ./cmd/aether

test:
	go test -race ./...
	cd web && npm run check && npm run check:contrast

lint:
	golangci-lint run

sqlc:
	sqlc generate

# Développement : API sur :8096, Vite sur :5173 (proxy /api → :8096).
dev-api:
	AETHER_DATA_DIR=./.dev AETHER_LOG_LEVEL=debug go run ./cmd/aether

dev-web:
	cd web && npm run dev

clean:
	rm -rf bin .dev
	find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
