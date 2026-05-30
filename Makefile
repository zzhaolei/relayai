# RelayAI Makefile
# Simplified build commands for different platforms

APP_NAME := RelayAI
BIN_DIR := bin
VITE_PORT := 9245

# Ensure Go bin directory is in PATH (for tools like wails3)
GOPATH := $(shell go env GOPATH)
export PATH := $(GOPATH)/bin:$(PATH)

# Detect current OS
UNAME_S := $(shell uname -s)

# Default target
.PHONY: help
help: ## Show this help message
	@echo "RelayAI Build Commands:"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2}'
	@echo ""

# Development
.PHONY: dev
dev: ## Start development mode with hot reload
	wails3 dev -config ./build/config.yml -port $(VITE_PORT)

.PHONY: dev-frontend
dev-frontend: ## Start frontend development server only
	cd frontend && npm run dev -- --port $(VITE_PORT) --strictPort

# Build for current platform
# macOS: builds .app bundle
# Others: builds executable only
.PHONY: build
build: ## Build for current platform (macOS: .app, others: executable)
ifeq ($(UNAME_S),Darwin)
	@echo "Building macOS .app bundle..."
	wails3 task darwin:package
else
	@echo "Building executable..."
	wails3 task build
endif

# Platform-specific builds
.PHONY: build-darwin
build-darwin: ## Build for macOS .app bundle
	@echo "Building macOS .app bundle..."
	wails3 task darwin:package

.PHONY: build-darwin-arm64
build-darwin-arm64: ## Build for macOS ARM64 (Apple Silicon)
	@echo "Building macOS ARM64 .app bundle..."
	wails3 task darwin:package ARCH=arm64

.PHONY: build-darwin-amd64
build-darwin-amd64: ## Build for macOS AMD64 (Intel)
	@echo "Building macOS AMD64 .app bundle..."
	wails3 task darwin:package ARCH=amd64

.PHONY: build-darwin-universal
build-darwin-universal: ## Build universal macOS .app bundle (ARM64 + AMD64)
	@echo "Building universal macOS .app bundle..."
	wails3 task darwin:package:universal

.PHONY: build-windows
build-windows: ## Build Windows executable
	@echo "Building Windows executable..."
	wails3 task windows:build

.PHONY: build-linux
build-linux: ## Build Linux executable
	@echo "Building Linux executable..."
	wails3 task linux:build

# Run
.PHONY: run
run: build ## Build and run the application
ifeq ($(UNAME_S),Darwin)
	@echo "Opening macOS app..."
	open $(BIN_DIR)/$(APP_NAME).app
else
	@echo "Running application..."
	./$(BIN_DIR)/$(APP_NAME)
endif

# Install dependencies
.PHONY: install
install: install-frontend ## Install all dependencies

.PHONY: install-frontend
install-frontend: ## Install frontend dependencies
	@echo "Installing frontend dependencies..."
	cd frontend && npm install

# Generate icons
.PHONY: generate-icons
generate-icons: ## Generate app iconset from appicon.png
	@echo "Generating iconset..."
	@mkdir -p build/darwin/icon.iconset
	@for size in 16 32 128 256 512; do \
		sips -s format png -z $$size $$size build/appicon.png --out build/darwin/icon.iconset/icon_$${size}x$${size}.png > /dev/null 2>&1; \
	done
	@sips -s format png -z 32 32 build/appicon.png --out build/darwin/icon.iconset/icon_16x16@2x.png > /dev/null 2>&1
	@sips -s format png -z 64 64 build/appicon.png --out build/darwin/icon.iconset/icon_32x32@2x.png > /dev/null 2>&1
	@sips -s format png -z 256 256 build/appicon.png --out build/darwin/icon.iconset/icon_128x128@2x.png > /dev/null 2>&1
	@sips -s format png -z 512 512 build/appicon.png --out build/darwin/icon.iconset/icon_256x256@2x.png > /dev/null 2>&1
	@cp build/appicon.png build/darwin/icon.iconset/icon_512x512@2x.png
	@echo "Icons ready"

.PHONY: generate-bindings
generate-bindings: ## Generate Go/JS bindings
	@echo "Generating bindings..."
	wails3 generate bindings -clean=true -ts

# Clean
.PHONY: clean
clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	rm -rf $(BIN_DIR)
	rm -rf frontend/dist
	rm -rf build/darwin/icon.iconset
	@echo "Clean complete"

.PHONY: clean-build
clean-build: clean build ## Clean and rebuild

# Docker cross-compilation
.PHONY: setup-docker
setup-docker: ## Setup Docker image for cross-compilation
	@echo "Setting up Docker cross-compilation..."
	wails3 task setup:docker

# Server mode
.PHONY: build-server
build-server: ## Build in server mode (no GUI)
	@echo "Building server..."
	wails3 task build:server

.PHONY: run-server
run-server: ## Run in server mode
	@echo "Running server..."
	wails3 task run:server

.PHONY: docker-build
docker-build: ## Build Docker image for server mode
	@echo "Building Docker image..."
	wails3 task build:docker

.PHONY: docker-run
docker-run: ## Run Docker container
	@echo "Running Docker container..."
	wails3 task run:docker

# Testing
.PHONY: test
test: ## Run Go tests
	@echo "Running tests..."
	go test ./... -v

.PHONY: test-short
test-short: ## Run Go tests (short mode)
	@echo "Running tests (short)..."
	go test ./... -short

# Code quality
.PHONY: fmt
fmt: ## Format and lint Go + frontend code
	@echo "Fixing Go code..."
	go fix ./...
	@echo "Formatting Go code..."
	gofmt -w .
	@echo "Formatting frontend code..."
	cd frontend && npm run format 2>/dev/null || true
	@echo "Linting frontend..."
	cd frontend && npm run lint 2>/dev/null || true
	@test -d frontend/dist || (echo "frontend/dist not found, building frontend..." && cd frontend && npm run build)
	@echo "Linting Go code..."
	go vet ./...

# Info
.PHONY: info
info: ## Show build information
	@echo "App Name:     $(APP_NAME)"
	@echo "Bin Dir:      $(BIN_DIR)"
	@echo "Vite Port:    $(VITE_PORT)"
	@echo "Platform:     $(UNAME_S) / $$(uname -m)"
	@echo "Go Version:   $$(go version 2>/dev/null || echo 'not installed')"
	@echo "Node Version: $$(node --version 2>/dev/null || echo 'not installed')"
	@echo "Wails3:       $$(wails3 version 2>&1 || echo 'not installed')"
