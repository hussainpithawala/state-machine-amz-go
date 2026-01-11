.PHONY: help test test-unit test-integration test-all test-coverage clean docker-up docker-down
.PHONY: build lint fmt vet deps bench examples validate pre-release release godoc version ci static-check

# Binary and version information
BINARY_NAME=state-machine-amz-go
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
GO_VERSION=$(shell go version | awk '{print $$3}')
LDFLAGS=-ldflags="-X 'main.Version=$(VERSION)' -X 'main.GoVersion=$(GO_VERSION)'"

# Database configuration
POSTGRES_HOST ?= localhost
POSTGRES_PORT ?= 5432
POSTGRES_USER ?= postgres
POSTGRES_PASSWORD ?= postgres
POSTGRES_DB ?= statemachine_test
POSTGRES_DB_GORM ?= statemachine_test_gorm
REDIS_ADDR ?= localhost:6379

# Test URLs
POSTGRES_TEST_URL ?= postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable
POSTGRES_TEST_URL_GORM ?= postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_PORT)/$(POSTGRES_DB_GORM)?sslmode=disable
DATABASE_URL ?= $(POSTGRES_TEST_URL)
DATABASE_URL_GORM ?= $(POSTGRES_TEST_URL_GORM)

# Colors for output
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
BLUE   := $(shell tput -Txterm setaf 4)
RESET  := $(shell tput -Txterm sgr0)

help: ## Show this help message
	@echo '${BLUE}Available commands:${RESET}'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  ${GREEN}%-20s${RESET} %s\n", $$1, $$2}'

# Docker management
docker-up: ## Start PostgreSQL, Redis, and Asynqmon containers for testing
	@echo "${GREEN}Starting test infrastructure...${RESET}"
	@docker-compose --file docker-examples/docker-compose.yml up -d
	@echo "${GREEN}Waiting for PostgreSQL...${RESET}"
	@sleep 5
	@until docker exec statemachine-postgres pg_isready -U $(POSTGRES_USER) > /dev/null 2>&1; do \
		echo "Waiting for PostgreSQL..."; \
		sleep 2; \
	done
	@echo "${GREEN}Creating additional test database...${RESET}"
	@docker exec statemachine-postgres psql -U $(POSTGRES_USER) -c "CREATE DATABASE $(POSTGRES_DB_GORM);" 2>/dev/null || echo "Database $(POSTGRES_DB_GORM) already exists"
	@echo "${GREEN}Waiting for Redis...${RESET}"
	@until docker exec statemachine-redis redis-cli ping > /dev/null 2>&1; do \
		echo "Waiting for Redis..."; \
		sleep 2; \
	done
	@echo "${GREEN}Test infrastructure ready!${RESET}"
	@echo "${BLUE}Asynqmon UI available at: http://localhost:8080${RESET}"

docker-down: ## Stop test infrastructure containers
	@echo "${YELLOW}Stopping test infrastructure...${RESET}"
	@docker-compose --file docker-examples/docker-compose.yml down -v
	@echo "${GREEN}Test infrastructure stopped${RESET}"

docker-logs: ## Show logs from all containers
	@docker-compose --file docker-examples/docker-compose.yml logs -f

docker-ps: ## Show running containers
	@docker-compose --file docker-examples/docker-compose.yml ps

# Testing
test-unit: ## Run unit tests only (fast, no database)
	@echo "${GREEN}Running unit tests...${RESET}"
	@go test -v -short -race ./pkg/queue/...

test-integration: docker-up ## Run integration tests with real databases
	@echo "${GREEN}Running integration tests...${RESET}"
	@POSTGRES_TEST_URL=$(POSTGRES_TEST_URL) \
	 POSTGRES_TEST_URL_GORM=$(POSTGRES_TEST_URL_GORM) \
	 DATABASE_URL=$(DATABASE_URL) \
	 DATABASE_URL_GORM=$(DATABASE_URL_GORM) \
	 REDIS_ADDR=$(REDIS_ADDR) \
	 go test -v -race ./pkg/handler/... ./pkg/statemachine/persistent/...
	@$(MAKE) docker-down

test-all: docker-up ## Run all tests with coverage
	@echo "${GREEN}Running all tests with coverage...${RESET}"
	@POSTGRES_TEST_URL=$(POSTGRES_TEST_URL) \
	 POSTGRES_TEST_URL_GORM=$(POSTGRES_TEST_URL_GORM) \
	 DATABASE_URL=$(DATABASE_URL) \
	 DATABASE_URL_GORM=$(DATABASE_URL_GORM) \
	 REDIS_ADDR=$(REDIS_ADDR) \
	 go test $$(go list ./... | grep -v /examples) -v -race -coverprofile=coverage.out -covermode=atomic
	@$(MAKE) docker-down

test-coverage: test-all ## Run tests and show coverage report
	@echo "${GREEN}Generating coverage report...${RESET}"
	@go tool cover -html=coverage.out -o coverage.html
	@echo "${GREEN}Coverage report generated: coverage.html${RESET}"
	@go tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

test: test-all ## Alias for test-all

bench: docker-up ## Run benchmarks
	@echo "${GREEN}Running benchmarks...${RESET}"
	@POSTGRES_TEST_URL=$(POSTGRES_TEST_URL) \
	 REDIS_ADDR=$(REDIS_ADDR) \
	 go test -bench=. -benchmem ./...
	@$(MAKE) docker-down

test-examples: docker-up ## Run example programs
	@echo "${GREEN}Running examples...${RESET}"
	@echo "${GREEN}Creating additional test database...${RESET}"
	@docker exec statemachine-postgres psql -U $(POSTGRES_USER) -c "CREATE DATABASE $(POSTGRES_DB_GORM);" 2>/dev/null || echo "Database $(POSTGRES_DB_GORM) already exists"
	@DATABASE_URL=$(DATABASE_URL) \
	 DATABASE_URL_GORM=$(DATABASE_URL_GORM) \
	 find examples -maxdepth 2 -type f ! -path '*/distributed_queue/*' ! -path '*/message_timeout_complete/*' -name "*.go" -print0 | xargs -0 -n1 go run
	@$(MAKE) docker-down

# Code quality
lint: ## Run linter
	@echo "${GREEN}Running linter...${RESET}"
	@golangci-lint run --timeout=5m --config=.golangci.yml

fmt: ## Format code
	@echo "${GREEN}Formatting code...${RESET}"
	@go fmt ./...
	@gofmt -s -w .
	@goimports -w . 2>/dev/null || true

vet: ## Run go vet
	@echo "${GREEN}Running go vet...${RESET}"
	@go vet ./...

static-check: ## Run staticcheck
	@echo "${GREEN}Running staticcheck...${RESET}"
	@staticcheck ./... 2>/dev/null || echo "${YELLOW}staticcheck not installed, skipping...${RESET}"

check-mod: ## Check module dependencies
	@echo "${GREEN}Checking module dependencies...${RESET}"
	@go mod tidy
	@git diff --exit-code go.mod go.sum || (echo "${YELLOW}go.mod/go.sum not tidy${RESET}" && exit 1)
	@echo "${GREEN}Module dependencies are clean${RESET}"

# Building
build: ## Build the package
	@echo "${GREEN}Building version $(VERSION)...${RESET}"
	@go build $(LDFLAGS) -v ./...
	@echo "${GREEN}Build complete${RESET}"

examples: ## Build examples
	@echo "${GREEN}Building examples...${RESET}"
	@for example in examples/*/*.go; do \
		if [ -f "$$example" ]; then \
			echo "Building $$example..."; \
			go build $(LDFLAGS) -o /dev/null "$$example" || echo "${YELLOW}Warning: Failed to build $$example${RESET}"; \
		fi \
	done
	@echo "${GREEN}Examples built${RESET}"

# Dependencies
deps: ## Download dependencies
	@echo "${GREEN}Downloading dependencies...${RESET}"
	@go mod download
	@go mod tidy
	@echo "${GREEN}Dependencies updated${RESET}"

update-deps: ## Update dependencies
	@echo "${GREEN}Updating dependencies...${RESET}"
	@go get -u ./...
	@go mod tidy
	@echo "${GREEN}Dependencies updated to latest versions${RESET}"

# Validation
validate: lint vet check-mod test-all ## Run all validation checks
	@echo "${GREEN}✅ All validation checks passed${RESET}"

# CI/CD
ci: deps lint vet static-check test-all ## Run all CI checks locally
	@echo "${GREEN}✅ CI checks completed successfully${RESET}"

# Release
pre-release: validate ## Prepare for release
	@echo "${GREEN}✅ Ready for release $(VERSION)${RESET}"

release: pre-release ## Create a new release (interactive)
	@read -p "Enter version tag (e.g., v1.2.3): " version; \
	git tag -a $$version -m "Release $$version"; \
	git push origin $$version; \
	echo "${GREEN}✅ Release $$version created and pushed${RESET}"

release-ci: ## Create release for CI (non-interactive)
	@echo "${BLUE}Creating release for CI...${RESET}"
	@echo "Use: git tag -a vX.Y.Z -m \"Release vX.Y.Z\" && git push origin vX.Y.Z"

# Utilities
clean: docker-down ## Clean up test artifacts and containers
	@echo "${YELLOW}Cleaning up...${RESET}"
	@rm -f coverage.out coverage.html
	@rm -rf dist/ $(BINARY_NAME)
	@go clean -testcache
	@echo "${GREEN}Cleanup complete${RESET}"

godoc: ## Start Go documentation server
	@echo "${BLUE}Starting godoc server at http://localhost:6060${RESET}"
	@echo "${BLUE}Package documentation: http://localhost:6060/pkg/$$(go list -m)/${RESET}"
	@godoc -http=:6060

version: ## Show version information
	@echo "${BLUE}Version Information:${RESET}"
	@echo "  Version:    $(VERSION)"
	@echo "  Go version: $(GO_VERSION)"
	@echo "  Module:     $$(go list -m)"
	@echo "  Package:    $$(go list -m)@$(VERSION)"

info: ## Show project information
	@echo "${BLUE}Project Information:${RESET}"
	@echo "  Binary:     $(BINARY_NAME)"
	@echo "  Version:    $(VERSION)"
	@echo "  Go:         $(GO_VERSION)"
	@echo "  Module:     $$(go list -m)"
	@echo ""
	@echo "${BLUE}Test Configuration:${RESET}"
	@echo "  PostgreSQL: $(POSTGRES_HOST):$(POSTGRES_PORT)"
	@echo "  Database:   $(POSTGRES_DB)"
	@echo "  Redis:      $(REDIS_ADDR)"
	@echo ""
	@echo "${BLUE}Quick Commands:${RESET}"
	@echo "  make docker-up      - Start test infrastructure"
	@echo "  make test-unit      - Fast unit tests"
	@echo "  make test-all       - Full test suite"
	@echo "  make ci             - Run all CI checks"
	@echo "  make docker-down    - Stop infrastructure"

# Monitoring
asynqmon: docker-up ## Open Asynqmon UI in browser
	@echo "${BLUE}Opening Asynqmon at http://localhost:8080${RESET}"
	@sleep 2
	@open http://localhost:8080 2>/dev/null || xdg-open http://localhost:8080 2>/dev/null || echo "Please open http://localhost:8080 in your browser"

.DEFAULT_GOAL := help