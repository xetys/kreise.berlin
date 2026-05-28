SHELL := /bin/bash

GOLANGCI_LINT_VERSION := v2.3.0
SQLC_VERSION := v1.30.0
GOVULNCHECK_VERSION := v1.1.4

# Pinned tool runners (no install required; cached by Go module proxy after first run).
GOLANGCI_LINT := go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)
GOVULNCHECK := go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

# Targets that need env vars from .env should explicitly source it via the
# bash recipe pattern below (set -a; . ./.env; set +a). Make's `include` does
# not understand bash inline comments, so we don't use it.

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make <target>\n\nTargets:\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# --- compose ---

.PHONY: up down logs reset-db
up: ## Start dev infra (Postgres, MinIO, mailcatcher, Adminer)
	docker compose up -d

down: ## Stop dev infra
	docker compose down

logs: ## Tail compose logs
	docker compose logs -f

reset-db: ## Drop and recreate the dev database
	docker compose exec -T postgres psql -U tickets -d postgres -c "DROP DATABASE IF EXISTS tickets;"
	docker compose exec -T postgres psql -U tickets -d postgres -c "CREATE DATABASE tickets OWNER tickets;"

# --- dev ---

.PHONY: dev dev-backend dev-frontend
dev: up ## Start everything (compose + backend + frontend)
	@trap 'kill 0' EXIT INT TERM; \
	$(MAKE) -j2 dev-backend dev-frontend

dev-backend: ## Run the Go backend in foreground
	go run ./backend/cmd/server

dev-frontend: ## Run the Next.js frontend in foreground
	cd frontend && npm run dev

# --- build ---

.PHONY: build build-backend build-frontend
build: build-backend build-frontend ## Build backend binaries and frontend bundle

build-backend: ## Build server + migrate binaries into ./bin
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -o bin/server ./backend/cmd/server
	CGO_ENABLED=0 go build -trimpath -o bin/migrate ./backend/cmd/migrate

build-frontend: ## Build the Next.js production bundle
	cd frontend && npm run build

# --- test / lint ---

.PHONY: test test-backend test-frontend test-db-init lint lint-backend lint-frontend vet vuln
test: test-backend test-frontend ## Run all tests

test-db-init: ## (Re)create the integration test database and apply migrations
	@docker compose exec -T postgres psql -U tickets -d postgres -c "DROP DATABASE IF EXISTS tickets_test;"
	@docker compose exec -T postgres psql -U tickets -d postgres -c "CREATE DATABASE tickets_test OWNER tickets;"
	@DATABASE_URL=postgres://tickets:tickets@localhost:5432/tickets_test?sslmode=disable LOG_LEVEL=info go run ./backend/cmd/migrate up

test-backend: ## Run Go tests against tickets_test (-p 1 because integration tests share the DB)
	@TEST_DATABASE_URL=postgres://tickets:tickets@localhost:5432/tickets_test?sslmode=disable go test -p 1 -race ./...

test-frontend: ## Run frontend tests
	cd frontend && npm test --if-present

lint: lint-backend lint-frontend ## Run all linters

lint-backend: ## Run golangci-lint
	$(GOLANGCI_LINT) run

lint-frontend: ## Lint frontend
	cd frontend && npm run lint

vet: ## go vet
	go vet ./...

vuln: ## Scan for known vulnerabilities in Go deps
	$(GOVULNCHECK) ./...

# --- migrations ---

.PHONY: migrate-up migrate-down migrate-status migrate-create
migrate-up: ## Apply all pending migrations
	go run ./backend/cmd/migrate up

migrate-down: ## Roll back the most recent migration
	go run ./backend/cmd/migrate down

migrate-status: ## Show migration status
	go run ./backend/cmd/migrate status

migrate-create: ## Create a new migration: make migrate-create NAME=add_foo
	@test -n "$(NAME)" || (echo "NAME is required, e.g. make migrate-create NAME=add_foo" && exit 2)
	@cd backend/internal/migrations && go run github.com/pressly/goose/v3/cmd/goose@latest -dir . create $(NAME) sql

# --- sqlc ---

.PHONY: sqlc-gen sqlc-vet
sqlc-gen: ## Regenerate sqlc Go code from queries
	$(SQLC) generate

sqlc-vet: ## Static-check sqlc queries
	$(SQLC) vet

# --- seed ---

.PHONY: seed
seed: ## Seed the local DB with the Dao Dance reference event (idempotent)
	@set -a && . ./.env && set +a && go run ./backend/cmd/seed-dao-dance

# --- docker / deploy ---

# Image registry + tag. Override on the command line:
#   make docker-push IMAGE_TAG=v1.2.3
IMAGE_REPO ?= ghcr.io/xetys/kreise-berlin
IMAGE_TAG  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
PLATFORM   ?= linux/amd64

KUBECONFIG_PATH ?= $(HOME)/Downloads/kubeconfig-admin-mqwtngwgph-stytex-cloud
NAMESPACE       ?= kreise-berlin
RELEASE         ?= kreise

.PHONY: docker-build docker-build-backend docker-build-frontend docker-push helm-deploy helm-template

docker-build-backend: ## buildx-build the backend image (loads to local docker)
	docker buildx build --platform $(PLATFORM) \
	  -f backend/Dockerfile \
	  -t $(IMAGE_REPO)-backend:$(IMAGE_TAG) \
	  --load .

docker-build-frontend: ## buildx-build the frontend image (loads to local docker)
	docker buildx build --platform $(PLATFORM) \
	  -f frontend/Dockerfile \
	  -t $(IMAGE_REPO)-frontend:$(IMAGE_TAG) \
	  --load ./frontend

docker-build: docker-build-backend docker-build-frontend ## Build both images locally

docker-push: ## buildx-build AND push both images to the registry (also re-tags :latest)
	docker buildx build --platform $(PLATFORM) \
	  -f backend/Dockerfile \
	  -t $(IMAGE_REPO)-backend:$(IMAGE_TAG) \
	  -t $(IMAGE_REPO)-backend:latest \
	  --push .
	docker buildx build --platform $(PLATFORM) \
	  -f frontend/Dockerfile \
	  -t $(IMAGE_REPO)-frontend:$(IMAGE_TAG) \
	  -t $(IMAGE_REPO)-frontend:latest \
	  --push ./frontend

helm-template: ## Render the chart locally (sanity-check before deploy)
	helm template $(RELEASE) ./chart \
	  --set image.tag=$(IMAGE_TAG) \
	  --set app.tokenSigningKey=dummy \
	  --set app.postgresPassword=dummy \
	  --set app.minioAccessKey=dummy \
	  --set app.minioSecretKey=dummy

helm-deploy: ## helm upgrade --install (requires DEPLOY_VALUES=path/to/secrets.yaml)
	@test -n "$(DEPLOY_VALUES)" || (echo "DEPLOY_VALUES=path/to/secrets.yaml is required" && exit 2)
	KUBECONFIG=$(KUBECONFIG_PATH) helm upgrade --install $(RELEASE) ./chart \
	  --namespace $(NAMESPACE) --create-namespace \
	  --set image.tag=$(IMAGE_TAG) \
	  -f $(DEPLOY_VALUES)

# --- clean ---

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/ frontend/.next frontend/out
