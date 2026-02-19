.PHONY: build install clean test lint help

# Build variables
BINARY_NAME=xcli
BUILD_DIR=bin
VERSION?=dev
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.buildVersion=$(VERSION) -X main.buildCommit=$(COMMIT) -X main.buildDate=$(DATE)"

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOINSTALL=$(GOCMD) install
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

cc-frontend: ## Build the Command Center frontend
	@echo "Building CC frontend..."
	@cd pkg/cc/frontend && pnpm install --frozen-lockfile && pnpm build
	@echo "✓ CC frontend built"

build: cc-frontend ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)
	@echo "✓ Built: $(BUILD_DIR)/$(BINARY_NAME)"

install: cc-frontend ## Install the binary to GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	$(GOINSTALL) $(LDFLAGS) ./cmd/$(BINARY_NAME)
	@echo "✓ Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

clean: ## Remove build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	@echo "✓ Cleaned"

test: ## Run tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-coverage: test ## Run tests with coverage report
	$(GOCMD) tool cover -html=coverage.out

lint: ## Run linters
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, installing..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run --timeout=5m

tidy: ## Tidy go modules
	$(GOMOD) tidy
	$(GOMOD) verify

deps: ## Download dependencies
	$(GOMOD) download

run: build ## Build and run
	./$(BUILD_DIR)/$(BINARY_NAME)

dev: ## Run in development mode
	$(GOCMD) run ./cmd/$(BINARY_NAME)

cc-dev: build ## Run CC backend + Vite HMR (open localhost:5173)
	@trap 'kill 0' EXIT; \
		./$(BUILD_DIR)/$(BINARY_NAME) cc --no-open & \
		cd pkg/cc/frontend && pnpm dev & \
		wait

.DEFAULT_GOAL := help
