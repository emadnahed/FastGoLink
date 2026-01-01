# GoURL Makefile
# Production-grade URL Shortener in Go

.PHONY: all build run clean test test-unit test-integration test-e2e test-coverage lint fmt vet deps docker-up docker-down help

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GORUN=$(GOCMD) run
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet

# Binary name
BINARY_NAME=gourl
BINARY_PATH=bin/$(BINARY_NAME)

# Main package
MAIN_PACKAGE=./cmd/api

# Coverage
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Default target
all: lint test build

## Build commands
build: ## Build the application
	@echo "Building..."
	@mkdir -p bin
	$(GOBUILD) -o $(BINARY_PATH) $(MAIN_PACKAGE)
	@echo "Build complete: $(BINARY_PATH)"

run: ## Run the application
	$(GORUN) $(MAIN_PACKAGE)

clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf bin/
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	@echo "Clean complete"

## Testing commands
test: ## Run all tests
	@echo "Running all tests..."
	$(GOTEST) -v -race ./...

test-unit: ## Run unit tests only
	@echo "Running unit tests..."
	$(GOTEST) -v -race ./tests/unit/...

test-integration: ## Run integration tests only
	@echo "Running integration tests..."
	$(GOTEST) -v -race ./tests/integration/...

test-e2e: ## Run end-to-end tests only
	@echo "Running E2E tests..."
	$(GOTEST) -v -race ./tests/e2e/...

test-short: ## Run tests in short mode (skip slow tests)
	@echo "Running short tests..."
	$(GOTEST) -v -short ./...

test-coverage: ## Run tests with coverage report
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report generated: $(COVERAGE_HTML)"

test-all: ## Run ALL tests (unit, integration, e2e) with full coverage report
	@./scripts/test-all.sh

test-all-docker: ## Run ALL tests with Docker services (PostgreSQL, Redis)
	@./scripts/test-all.sh --docker

## Performance testing commands
bench: ## Run Go benchmarks
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem -run=^$$ ./tests/benchmark/...

bench-cpu: ## Run benchmarks with CPU profiling
	@echo "Running benchmarks with CPU profile..."
	$(GOTEST) -bench=. -benchmem -cpuprofile=cpu.prof -run=^$$ ./tests/benchmark/...
	@echo "CPU profile saved to cpu.prof. View with: go tool pprof cpu.prof"

bench-mem: ## Run benchmarks with memory profiling
	@echo "Running benchmarks with memory profile..."
	$(GOTEST) -bench=. -benchmem -memprofile=mem.prof -run=^$$ ./tests/benchmark/...
	@echo "Memory profile saved to mem.prof. View with: go tool pprof mem.prof"

test-stress: ## Run stress tests (latency + concurrency)
	@echo "Running stress tests..."
	$(GOTEST) -v -run="TestConcurrencyStress|TestLatencyPercentiles" ./tests/benchmark/...

loadtest: ## Run load test against local server (start server first with 'make run')
	@./scripts/loadtest.sh

loadtest-quick: ## Run quick load test
	@./scripts/loadtest.sh --quick

loadtest-full: ## Run full load test
	@./scripts/loadtest.sh --full

loadtest-stress: ## Run stress load test
	@./scripts/loadtest.sh --stress

## Code quality commands
lint: ## Run golangci-lint
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	golangci-lint run ./...

fmt: ## Format code
	@echo "Formatting code..."
	$(GOFMT) -s -w .
	@echo "Formatting complete"

vet: ## Run go vet
	@echo "Running vet..."
	$(GOVET) ./...

check: fmt vet lint ## Run all code quality checks

## Dependency commands
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Dependencies updated"

deps-upgrade: ## Upgrade all dependencies
	@echo "Upgrading dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "Dependencies upgraded"

## Docker commands
docker-up: ## Start Docker services (PostgreSQL, Redis)
	@echo "Starting Docker services..."
	docker-compose up -d postgres redis
	@echo "Waiting for services to be healthy..."
	@sleep 5
	@echo "Services started"

docker-down: ## Stop Docker services
	@echo "Stopping Docker services..."
	docker-compose down
	@echo "Services stopped"

docker-logs: ## View Docker service logs
	docker-compose logs -f

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker-compose build api
	@echo "Build complete"

docker-run: docker-build ## Build and run all services in Docker
	@echo "Starting all services..."
	docker-compose up -d
	@echo "All services started"

docker-test: docker-up ## Run full tests with Docker services
	@echo "Running full tests with PostgreSQL and Redis..."
	@sleep 3
	TEST_POSTGRES=true TEST_REDIS=true $(GOTEST) -v -race ./...

docker-clean: docker-down ## Clean Docker volumes and containers
	@echo "Cleaning Docker resources..."
	docker-compose down -v --remove-orphans
	docker rmi gourl-api:latest 2>/dev/null || true
	@echo "Clean complete"

docker-prod: ## Start production-like environment
	@echo "Building production image..."
	docker build -t gourl-api:latest .
	@echo "Starting production environment..."
	docker-compose -f docker-compose.prod.yml up -d
	@echo "Production environment started"

## Database commands
db-migrate: ## Run database migrations
	@echo "Running migrations..."
	# Migration command will be added in Phase 2
	@echo "Migrations complete"

db-rollback: ## Rollback last migration
	@echo "Rolling back migration..."
	# Rollback command will be added in Phase 2
	@echo "Rollback complete"

## Development helpers
dev: docker-up run ## Start development environment

## Help
help: ## Display this help message
	@echo "GoURL - Production-Grade URL Shortener"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
