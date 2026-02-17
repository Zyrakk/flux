.PHONY: build test lint docker-build helm-install compose-up compose-down migrate clean

# Binaries
BINARIES := api worker-rss worker-hn worker-reddit worker-github processor briefing-gen
BUILD_DIR := ./bin
DOCKER_IMAGES := $(BINARIES) embeddings-svc frontend

# Go
GOFLAGS := -ldflags="-w -s"

# ============================================================================
# Build
# ============================================================================

build: ## Build all binaries
	@echo "Building all binaries..."
	@mkdir -p $(BUILD_DIR)
	@for svc in $(BINARIES); do \
		echo "  Building $$svc..."; \
		CGO_ENABLED=0 go build $(GOFLAGS) -o $(BUILD_DIR)/flux-$$svc ./cmd/$$svc/; \
	done
	@echo "Done."

build-%: ## Build a specific binary (e.g., make build-api)
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(GOFLAGS) -o $(BUILD_DIR)/flux-$* ./cmd/$*/

# ============================================================================
# Test & Lint
# ============================================================================

test: ## Run all tests
	go test -race -count=1 ./...

test-cover: ## Run tests with coverage
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: ## Run golangci-lint
	golangci-lint run ./...

# ============================================================================
# Docker
# ============================================================================

docker-build: ## Build all Docker images
	@for svc in $(DOCKER_IMAGES); do \
		echo "Building flux-$$svc image..."; \
		docker build -f deploy/docker/Dockerfile.$$svc -t flux-$$svc:latest .; \
	done

docker-build-%: ## Build a specific Docker image (e.g., make docker-build-api)
	docker build -f deploy/docker/Dockerfile.$* -t flux-$*:latest .

# ============================================================================
# Docker Compose
# ============================================================================

compose-up: ## Start all services with Docker Compose
	docker compose up -d

compose-down: ## Stop all services
	docker compose down

compose-logs: ## Tail logs from all services
	docker compose logs -f

compose-ps: ## Show running services
	docker compose ps

# ============================================================================
# Helm
# ============================================================================

helm-install: ## Install Flux via Helm into k3s
	helm install flux ./deploy/helm/flux --namespace flux --create-namespace

helm-upgrade: ## Upgrade Flux via Helm
	helm upgrade flux ./deploy/helm/flux --namespace flux

helm-uninstall: ## Uninstall Flux from k3s
	helm uninstall flux --namespace flux

helm-template: ## Render Helm templates locally (dry-run)
	helm template flux ./deploy/helm/flux --namespace flux

# ============================================================================
# Database
# ============================================================================

migrate: ## Run database migrations
	@echo "Running migrations against DATABASE_URL..."
	@if command -v migrate > /dev/null 2>&1; then \
		migrate -path migrations -database "$${DATABASE_URL}" up; \
	else \
		echo "Install golang-migrate: go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest"; \
	fi

migrate-down: ## Rollback all migrations
	@migrate -path migrations -database "$${DATABASE_URL}" down

migrate-create: ## Create a new migration (usage: make migrate-create name=add_stories)
	@migrate create -ext sql -dir migrations -seq $(name)

# ============================================================================
# Development
# ============================================================================

dev-api: ## Run API server locally
	go run ./cmd/api/

dev-deps: ## Start only infrastructure (postgres, nats, redis) via compose
	docker compose up -d postgres nats redis

clean: ## Clean build artifacts
	rm -rf $(BUILD_DIR) coverage.out coverage.html

# ============================================================================
# Help
# ============================================================================

help: ## Show this help
	@grep -E '^[a-zA-Z_%-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

.DEFAULT_GOAL := help
