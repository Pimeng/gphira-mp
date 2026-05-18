# Phira MP Server - Makefile

APP_NAME      := gphira-mp
BENCH_NAME    := bench
BUILD_DIR     := build
BIN_DIR       := $(BUILD_DIR)/bin
DIST_DIR      := $(BUILD_DIR)/dist

VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS       := -s -w -X github.com/Pimeng/gphira-mp-next/internal/version.version=$(VERSION)

GO            := go
GOFLAGS       := -trimpath

# ------------------------------------------------------------------------------
# Build
# ------------------------------------------------------------------------------

.PHONY: all build server bench clean test

all: server bench

build: server bench

server:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) ./cmd/server

bench:
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BENCH_NAME) ./cmd/bench

# ------------------------------------------------------------------------------
# Development
# ------------------------------------------------------------------------------

.PHONY: run test lint fmt vet

run: server
	$(BIN_DIR)/$(APP_NAME) --config server_config.example.yml

test:
	$(GO) test -v -race -count=1 ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# ------------------------------------------------------------------------------
# Release
# ------------------------------------------------------------------------------

.PHONY: dist dist-linux dist-windows dist-darwin dist-freebsd

dist: dist-linux dist-windows dist-darwin dist-freebsd

dist-linux:
	@mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BENCH_NAME)-linux-amd64 ./cmd/bench

dist-windows:
	@mkdir -p $(DIST_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe ./cmd/server
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BENCH_NAME)-windows-amd64.exe ./cmd/bench

dist-darwin:
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 ./cmd/server
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BENCH_NAME)-darwin-amd64 ./cmd/bench

dist-freebsd:
	@mkdir -p $(DIST_DIR)
	GOOS=freebsd GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP_NAME)-freebsd-amd64 ./cmd/server
	GOOS=freebsd GOARCH=amd64 $(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BENCH_NAME)-freebsd-amd64 ./cmd/bench

# ------------------------------------------------------------------------------
# Docker
# ------------------------------------------------------------------------------

.PHONY: docker docker-push

docker:
	docker build -f docker/Dockerfile -t $(APP_NAME):$(VERSION) -t $(APP_NAME):latest .

docker-push:
	docker push $(APP_NAME):$(VERSION)
	docker push $(APP_NAME):latest

# ------------------------------------------------------------------------------
# Cleanup
# ------------------------------------------------------------------------------

.PHONY: clean

clean:
	rm -rf $(BUILD_DIR)
