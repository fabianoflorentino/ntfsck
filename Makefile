BINARY_NAME := ntfsck
GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X github.com/fabianoflorentino/ntfsck/internal/cli.version=$(VERSION)"

.PHONY: all build install tidy fmt vet test test-race cover lint lint-fix audit clean help

all: fmt vet test build

## build: Build the ntfsck binary into bin/
build:
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	$(GO) build -trimpath $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/ntfsck

## install: Download and tidy module dependencies
install:
	$(GO) mod download
	$(GO) mod tidy

## tidy: Sync go.mod and go.sum
tidy:
	$(GO) mod tidy

## fmt: Format every package
fmt:
	$(GO) fmt ./...

## vet: Run go vet
vet:
	$(GO) vet ./...

## test: Run the test suite
test:
	$(GO) test ./...

## test-race: Run the test suite under the race detector
test-race:
	$(GO) test -race ./...

## cover: Report coverage per function
cover:
	$(GO) test -coverprofile=coverage.out -covermode=atomic ./...
	$(GO) tool cover -func=coverage.out

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## lint-fix: Run golangci-lint with auto-fixes
lint-fix:
	golangci-lint run --fix ./...

## audit: Check dependencies for known vulnerabilities
audit:
	govulncheck ./...

## clean: Remove build artifacts
clean:
	rm -rf bin/ coverage.out

## help: List the available targets
help:
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'
