.DEFAULT_GOAL := help

SHELL     := /bin/bash
BIN_DIR   := bin
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -X github.com/jamesonstone/ghostgc/internal/version.Version=$(VERSION)
GO        ?= go
SIZE_DIRS := cmd internal fixtures
MAX_LINES := 300

.PHONY: help build test race cover vet fmt fmt-check lint size check install uninstall run clean

help:
	@printf '%s\n' 'ghostgc developer workflow'
	@printf '%s\n' ''
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

build: ## build the ghostgc executable into ./bin
	@mkdir -p $(BIN_DIR)
	@rm -f $(BIN_DIR)/ghostgcd
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/ghostgc  ./cmd/cli

test: ## run the unit and integration tests
	$(GO) test ./...

race: ## run the tests under the race detector
	$(GO) test -race -count=1 ./...

cover: ## run the tests and report coverage
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

vet: ## run go vet
	$(GO) vet ./...

fmt: ## format the source
	gofmt -w $(SIZE_DIRS)

fmt-check: ## fail if anything is unformatted
	@out=$$(gofmt -l $(SIZE_DIRS)); \
	if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

lint: ## run golangci-lint
	golangci-lint run ./...

# docs/references/rules/source-file-size.md caps handwritten source and test
# files at 300 physical lines. Enforcing it here keeps the gate observable
# rather than something a reviewer has to remember.
size: ## fail if any source or test file exceeds the 300-line limit
	@over=$$(find $(SIZE_DIRS) -type f \( -name '*.go' -o -name '*.c' -o -name '*.sh' \) \
		| xargs wc -l | grep -v ' total$$' \
		| awk '$$1 > $(MAX_LINES) {print $$1 " " $$2}'); \
	if [ -n "$$over" ]; then \
		echo "files above the $(MAX_LINES) line limit:"; echo "$$over"; exit 1; \
	fi; \
	echo "source file size: all files within $(MAX_LINES) lines"

check: fmt-check vet size test ## everything required before delivery

install: build ## install ghostgc into ~/.local/bin
	@mkdir -p $(HOME)/.local/bin
	install -m 0755 $(BIN_DIR)/ghostgc  $(HOME)/.local/bin/ghostgc
	@echo "Installed to $(HOME)/.local/bin. Run 'ghostgc service install' to start the daemon at login."

uninstall: ## remove the service registration and installed executable
	-$(HOME)/.local/bin/ghostgc service uninstall
	rm -f $(HOME)/.local/bin/ghostgcd $(HOME)/.local/bin/ghostgc

run: build ## run the daemon in the foreground with debug logging
	$(BIN_DIR)/ghostgc daemon --log-level debug

clean: ## remove build output
	rm -rf $(BIN_DIR) coverage.out
