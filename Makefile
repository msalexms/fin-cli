BINARY  := fin-cli
BIN_DIR := bin
PKG     := github.com/msalexms/fin-cli/internal/version
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build test lint fmt run clean release

build:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/fin-cli

test:
	go test ./... -race -count=1

lint:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...

fmt:
	gofmt -w .

run:
	go run ./cmd/fin-cli

clean:
	rm -rf $(BIN_DIR) dist

release:
	goreleaser release --clean
