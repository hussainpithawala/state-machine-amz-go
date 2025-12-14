# Makefile
.PHONY: help test build lint clean release validate

BINARY_NAME=state-machine-amz-go
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "v0.0.0-dev")
GO_VERSION=$(shell go version | awk '{print $$3}')
LDFLAGS=-ldflags="-X 'main.Version=$(VERSION)' -X 'main.GoVersion=$(GO_VERSION)'"

help: ## Display this help message
	@echo "Available commands:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

build: ## Build the package
	@echo "Building version $(VERSION)..."
	go build $(LDFLAGS) ./...

test: ## Run tests
	go test ./... -v -race -coverprofile=coverage.out

static-check:
	staticcheck ./...

test-coverage: test ## Run tests and show coverage
	go tool cover -html=coverage.out

lint: ## Run linter
	golangci-lint run --timeout=5m --config=.golangci.yml

clean: ## Clean build artifacts
	rm -rf coverage.out dist/ $(BINARY_NAME)

examples: ## Build examples
	@for example in examples/*/*.go; do \
		echo "Building $$example..."; \
		go build -o /dev/null $$example; \
	done

check-mod: ## Check module dependencies
	go mod tidy
	git diff --exit-code go.mod go.sum || (echo "go.mod/go.sum not tidy" && exit 1)

validate: lint test check-mod ## Run all validation checks
	@echo "✅ All validation checks passed"

pre-release: validate ## Prepare for release
	@echo "✅ Ready for release $(VERSION)"

release: pre-release ## Create a new release (interactive)
	@read -p "Enter version tag (e.g., v1.2.3): " version; \
	git tag -a $$version -m "Release $$version"; \
	git push origin $$version

release-ci: ## Create release for CI (non-interactive)
	@echo "Creating release for CI..."
	@echo "Use 'git tag -a vX.Y.Z -m \"Release vX.Y.Z\" && git push origin vX.Y.Z'"

godoc: ## Start Go documentation server
	@echo "Starting godoc server at http://localhost:6060"
	@echo "Package documentation: http://localhost:6060/pkg/$(shell go list -m)/"
	godoc -http=:6060

bench: ## Run benchmarks
	go test -bench=. -benchmem ./...

update-deps: ## Update dependencies
	go get -u ./...
	go mod tidy

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Go version: $(GO_VERSION)"
	@echo "Module: $(shell go list -m)"
	@echo "Package: $(shell go list -m)@$(VERSION)"

.DEFAULT_GOAL := help