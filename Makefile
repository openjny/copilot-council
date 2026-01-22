BINARIES := copilot-council
BUILD_DIR := build
MAIN_PATH := ./cmd/copilot-council

.PHONY: all build clean install test lint help

all: build

## build: Build the copilot-council binary
build:
	@echo "Building copilot-council..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARIES) $(MAIN_PATH)
	@echo "Build complete: $(BUILD_DIR)/$(BINARIES)"

## install: Install copilot-council to GOPATH/bin
install:
	@echo "Installing copilot-council..."
	go install $(MAIN_PATH)
	@echo "Installation complete"

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@go clean
	@echo "Clean complete"

## test: Run tests
test:
	@echo "Running tests..."
	go test -v ./...

## lint: Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found. Install it from https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run

## fmt: Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

## run: Run the application (example)
run: build
	@echo "Running copilot-council..."
	$(BUILD_DIR)/$(BINARIES) "What is the capital of France?"

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' Makefile
