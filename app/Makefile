GO = go
GOFLAGS = -v

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
LDFLAGS = -ldflags "-s -w -X 'github.com/mqtt-home/mqtt-sonos/version.Version=$(VERSION)' -X 'github.com/mqtt-home/mqtt-sonos/version.GitCommit=$(GIT_COMMIT)' -X 'github.com/mqtt-home/mqtt-sonos/version.BuildTime=$(BUILD_TIME)'"

BUILD_DIR = build
CONFIG_DIR = $(PWD)/../production/config
WEB_DIR = web

BINARY_NAME = mqtt-sonos
DOCKER_IMAGE_NAME = pharndt/mqtt-sonos:latest

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@echo "mqtt-sonos — available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

.PHONY: build
build: build-frontend build-backend ## Build the frontend and backend

.PHONY: version
version: ## Print version/build metadata
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

.PHONY: build-frontend
build-frontend: ## Build the web frontend
	@echo "Building the frontend..."
	@cd $(WEB_DIR) && pnpm install && pnpm run build

.PHONY: build-backend
build-backend: ## Build the Go backend binary
	@echo "Building the backend..."
	@$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

.PHONY: test
test: ## Run the Go tests
	@echo "Running tests..."
	@$(GO) test ./...

.PHONY: dev
dev: build-frontend build-backend ## Build everything and run with the dev config
	@echo "Starting the application..."
	@$(BUILD_DIR)/$(BINARY_NAME) $(CONFIG_DIR)/config.json

.PHONY: dev-frontend
dev-frontend: ## Run the frontend dev server (hot reload)
	@echo "Starting frontend dev server..."
	@cd $(WEB_DIR) && pnpm run dev

.PHONY: dev-backend
dev-backend: build-backend ## Build and run only the backend
	@echo "Starting backend dev server..."
	@$(BUILD_DIR)/$(BINARY_NAME) $(CONFIG_DIR)/config.json

.PHONY: run
run: build ## Build everything and run
	@echo "Running the application..."
	@$(BUILD_DIR)/$(BINARY_NAME) $(CONFIG_DIR)/config.json

.PHONY: docker
docker: ## Build the Docker image
	@echo "Building Docker image..."
	@docker build -t $(DOCKER_IMAGE_NAME) .

.PHONY: clean
clean: ## Remove build artifacts and node_modules
	@echo "Cleaning up..."
	@rm -rf $(BUILD_DIR)
	@cd $(WEB_DIR) && rm -rf dist node_modules

.PHONY: deps
deps: ## Download and tidy Go dependencies
	@echo "Downloading Go dependencies..."
	@$(GO) mod download
	@$(GO) mod tidy
