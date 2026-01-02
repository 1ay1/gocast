# GoCast Makefile
# Modern Icecast replacement written in Go

BINARY_NAME=gocast
VERSION=1.0.0
BUILD_DIR=build
GO=go
GOFLAGS=-ldflags="-s -w -X main.version=$(VERSION)"

# Default target
.PHONY: all
all: build

# Build the binary
.PHONY: build
build:
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME) ./cmd/gocast

# Build with race detector
.PHONY: build-race
build-race:
	$(GO) build -race -o $(BINARY_NAME) ./cmd/gocast

# Build for all platforms
.PHONY: build-all
build-all: build-linux build-darwin build-windows

.PHONY: build-linux
build-linux:
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/gocast
	GOOS=linux GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/gocast

.PHONY: build-darwin
build-darwin:
	@mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/gocast
	GOOS=darwin GOARCH=arm64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/gocast

.PHONY: build-windows
build-windows:
	@mkdir -p $(BUILD_DIR)
	GOOS=windows GOARCH=amd64 $(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/gocast

# Run tests
.PHONY: test
test:
	$(GO) test -v ./...

# Run tests with coverage
.PHONY: test-coverage
test-coverage:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
.PHONY: bench
bench:
	$(GO) test -bench=. -benchmem ./...

# Run the server
.PHONY: run
run: build
	./$(BINARY_NAME)

# Run with custom config
.PHONY: run-config
run-config: build
	./$(BINARY_NAME) -config $(CONFIG)

# Check configuration
.PHONY: check-config
check-config: build
	./$(BINARY_NAME) -check -config gocast.vibe

# Format code
.PHONY: fmt
fmt:
	$(GO) fmt ./...

# Lint code
.PHONY: lint
lint:
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

# Vet code
.PHONY: vet
vet:
	$(GO) vet ./...

# Tidy dependencies
.PHONY: tidy
tidy:
	$(GO) mod tidy

# Clean build artifacts
.PHONY: clean
clean:
	rm -f $(BINARY_NAME)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html

# Install binary to GOPATH/bin
.PHONY: install
install:
	$(GO) install $(GOFLAGS) ./cmd/gocast

# Uninstall binary
.PHONY: uninstall
uninstall:
	rm -f $(GOPATH)/bin/$(BINARY_NAME)

# Docker build
.PHONY: docker-build
docker-build:
	docker build -t gocast:$(VERSION) -t gocast:latest .

# Docker run
.PHONY: docker-run
docker-run:
	docker run -p 8000:8000 -v $(PWD)/gocast.vibe:/etc/gocast/gocast.vibe gocast:latest

# Generate documentation
.PHONY: docs
docs:
	@which godoc > /dev/null || (echo "Installing godoc..." && go install golang.org/x/tools/cmd/godoc@latest)
	@echo "Documentation server starting at http://localhost:6060/pkg/github.com/gocast/gocast/"
	godoc -http=:6060

# Show help
.PHONY: help
help:
	@echo "GoCast - Modern Icecast Replacement"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@echo "  build          Build the binary"
	@echo "  build-race     Build with race detector"
	@echo "  build-all      Build for all platforms"
	@echo "  build-linux    Build for Linux (amd64, arm64)"
	@echo "  build-darwin   Build for macOS (amd64, arm64)"
	@echo "  build-windows  Build for Windows (amd64)"
	@echo "  test           Run tests"
	@echo "  test-coverage  Run tests with coverage report"
	@echo "  bench          Run benchmarks"
	@echo "  run            Build and run the server"
	@echo "  run-config     Run with custom config (CONFIG=path)"
	@echo "  check-config   Check configuration file"
	@echo "  fmt            Format code"
	@echo "  lint           Lint code with golangci-lint"
	@echo "  vet            Run go vet"
	@echo "  tidy           Tidy go modules"
	@echo "  clean          Clean build artifacts"
	@echo "  install        Install to GOPATH/bin"
	@echo "  docker-build   Build Docker image"
	@echo "  docker-run     Run Docker container"
	@echo "  docs           Start documentation server"
	@echo "  help           Show this help"
