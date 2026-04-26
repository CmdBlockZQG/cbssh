APP := cbssh
CMD := ./cmd/cbssh
BIN_DIR := bin
DIST_DIR := dist
DEV_DIR := .tmp/cbssh
DEV_CONFIG := $(DEV_DIR)/config.toml
DEV_STATE := $(DEV_DIR)/state.json

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X main.version=$(VERSION)
ARGS ?=
CONFIG ?=
STATE ?=

RUN_FLAGS := $(if $(CONFIG),--config $(CONFIG),) $(if $(STATE),--state $(STATE),)

.PHONY: help run dev dev-init build install test test-race vet fmt tidy clean clean-dist dist release \
	build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

help:
	@printf "cbssh development targets\n\n"
	@printf "Usage:\n"
	@printf "  make <target> [ARGS='...'] [CONFIG=...] [STATE=...] [VERSION=...]\n\n"
	@printf "Targets:\n"
	@printf "  run                 Run cbssh with the default user config\n"
	@printf "  dev                 Run cbssh with local debug config/state under .tmp/cbssh\n"
	@printf "  dev-init            Create local debug config/state paths\n"
	@printf "  build               Build local binary into bin/cbssh\n"
	@printf "  dist                Build linux/darwin amd64/arm64 binaries into dist/\n"
	@printf "  release             Alias of dist\n"
	@printf "  test                Run tests\n"
	@printf "  test-race           Run tests with race detector\n"
	@printf "  vet                 Run go vet\n"
	@printf "  fmt                 Format Go files\n"
	@printf "  tidy                Run go mod tidy\n"
	@printf "  clean               Remove local build outputs\n\n"
	@printf "Examples:\n"
	@printf "  make run ARGS='ls'\n"
	@printf "  make dev ARGS='config validate'\n"
	@printf "  make build VERSION=0.1.0\n"
	@printf "  make dist VERSION=0.1.0\n"

run:
	$(GO) run $(CMD) $(RUN_FLAGS) $(ARGS)

dev: dev-init
	$(GO) run $(CMD) --config $(DEV_CONFIG) --state $(DEV_STATE) $(ARGS)

dev-init:
	@mkdir -p $(DEV_DIR)
	@$(GO) run $(CMD) --config $(DEV_CONFIG) --state $(DEV_STATE) config init >/dev/null
	@printf "Debug config: %s\n" "$(DEV_CONFIG)"
	@printf "Debug state:  %s\n" "$(DEV_STATE)"

build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP) $(CMD)

install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" $(CMD)

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR) $(DIST_DIR) $(DEV_DIR)

clean-dist:
	rm -rf $(DIST_DIR)

dist: clean-dist build-linux-amd64 build-linux-arm64 build-darwin-amd64 build-darwin-arm64

release: dist

build-linux-amd64:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_linux_amd64 $(CMD)

build-linux-arm64:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_linux_arm64 $(CMD)

build-darwin-amd64:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_darwin_amd64 $(CMD)

build-darwin-arm64:
	@mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(APP)_darwin_arm64 $(CMD)
