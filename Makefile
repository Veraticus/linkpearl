# Linkpearl Makefile

BINARY_NAME=linkpearl
MAIN_PACKAGE=./cmd/linkpearl
GO=go
GOTEST=$(GO) test
GOVET=$(GO) vet
GOFMT=gofmt
GOLINT=golangci-lint

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME)"

# Platform detection
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: all build clean test test-verbose test-coverage test-integration bench fmt lint vet run help

# Default target
all: build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_PACKAGE)

# Build for specific platforms
build-linux:
	@echo "Building for Linux..."
	GOOS=linux GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 $(MAIN_PACKAGE)

build-darwin:
	@echo "Building for macOS..."
	GOOS=darwin GOARCH=amd64 $(GO) build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 $(MAIN_PACKAGE)
	GOOS=darwin GOARCH=arm64 $(GO) build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 $(MAIN_PACKAGE)

build-all: build-linux build-darwin

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GO) clean
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) ./...

# Run tests with race detector
test-race:
	@echo "Running tests with race detector..."
	$(GOTEST) -race ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests (verbose)..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -cover ./...

# Generate coverage report
coverage-report:
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -tags=integration ./...

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. ./...

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

# Run linter
lint:
	@echo "Running linter..."
	@if command -v $(GOLINT) >/dev/null; then \
		$(GOLINT) run; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

# Run the application
run: build
	./$(BINARY_NAME) $(ARGS)

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	$(GO) install $(MAIN_PACKAGE)

# Update dependencies
deps:
	@echo "Updating dependencies..."
	$(GO) mod download
	$(GO) mod tidy

# Verify dependencies
verify:
	@echo "Verifying dependencies..."
	$(GO) mod verify

# Development workflow - format, vet, and test
check: fmt vet test

# CI workflow - all checks
ci: deps verify fmt vet lint test test-coverage

# Update all Nix hashes to point to current HEAD
update-nix:
	@echo "Updating all Nix hashes to current HEAD..."
	@./scripts/update-nix-hashes.sh $(ARGS)

# Help
help:
	@echo "Linkpearl Makefile targets:"
	@echo "  make build          - Build the binary"
	@echo "  make build-all      - Build for all platforms"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make test-verbose   - Run tests with verbose output"
	@echo "  make test-coverage  - Run tests with coverage"
	@echo "  make test-integration - Run integration tests"
	@echo "  make bench          - Run benchmarks"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Run linter"
	@echo "  make vet            - Run go vet"
	@echo "  make run            - Build and run the application"
	@echo "  make install        - Install the binary"
	@echo "  make deps           - Update dependencies"
	@echo "  make check          - Run fmt, vet, and test"
	@echo "  make ci             - Run full CI workflow"
	@echo "  make update-nix     - Update all Nix hashes to current HEAD (use ARGS=-f to force)"
	@echo "  make help           - Show this help message"