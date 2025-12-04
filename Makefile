.PHONY: test test-unit test-integration bench coverage vet lint clean build

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod

# Test parameters
TEST_FLAGS=-v -race -cover
COVERAGE_FILE=coverage.out

# Directories
INTERNAL_DIR=internal
PKG_DIR=pkg
TEST_DIRS=$(INTERNAL_DIR)/... $(PKG_DIR)/...

# Default target
all: test build

# Run all tests
test: test-unit

# Run unit tests
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) $(TEST_FLAGS) ./$(INTERNAL_DIR)/states/... ./$(PKG_DIR)/...

# Run integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) $(TEST_FLAGS) ./examples/...

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./$(INTERNAL_DIR)/states/... ./$(PKG_DIR)/...

# Generate coverage report
coverage:
	@echo "Generating coverage report..."
	$(GOTEST) -coverprofile=$(COVERAGE_FILE) ./$(INTERNAL_DIR)/... ./$(PKG_DIR)/...
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./$(INTERNAL_DIR)/... ./$(PKG_DIR)/...

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	$(GOMOD) tidy

# Build the project
build:
	@echo "Building..."
	$(GOBUILD) ./...

# Clean
clean:
	@echo "Cleaning..."
	rm -f $(COVERAGE_FILE) coverage.html
	rm -rf bin/
	rm -rf dist/

# Install dependencies
deps:
	@echo "Installing test dependencies..."
	$(GOCMD) get github.com/stretchr/testify@v1.8.4
	$(GOMOD) tidy

# Run specific test
test-pass-state:
	$(GOTEST) $(TEST_FLAGS) ./$(INTERNAL_DIR)/states -run TestPassState

test-validator:
	$(GOTEST) $(TEST_FLAGS) ./$(INTERNAL_DIR)/validator

test-executor:
	$(GOTEST) $(TEST_FLAGS) ./$(PKG_DIR)/executor

# Format code
fmt:
	$(GOCMD) fmt ./$(INTERNAL_DIR)/... ./$(PKG_DIR)/...

# Run all checks (pre-commit)
check: fmt vet test

help:
	@echo "Available targets:"
	@echo "  all        - Run tests and build"
	@echo "  test       - Run all tests"
	@echo "  test-unit  - Run unit tests"
	@echo "  bench      - Run benchmarks"
	@echo "  coverage   - Generate coverage report"
	@echo "  vet        - Run go vet"
	@echo "  tidy       - Tidy go.mod"
	@echo "  build      - Build the project"
	@echo "  clean      - Clean build artifacts"
	@echo "  deps       - Install dependencies"
	@echo "  fmt        - Format code"
	@echo "  check      - Run all checks (fmt, vet, test)"
	@echo "  test-pass-state - Run only PassState tests"
	@echo "  help       - Show this help message"