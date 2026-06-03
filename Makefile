# e-agora developer tasks. Run `make help` for the list.
.DEFAULT_GOAL := help
.PHONY: help dev up down logs ps test fmt check-size build prod-build clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

dev: ## Build & run the full stack (db + backend + frontend) in the foreground
	docker compose up --build

up: ## Same as dev but detached
	docker compose up --build -d

down: ## Stop the stack (Postgres data persists in the named volume)
	docker compose down

logs: ## Tail logs from all services
	docker compose logs -f

ps: ## Show stack status
	docker compose ps

test: ## Run backend tests
	cd backend && go test ./...

fmt: ## Format the backend (gofmt)
	cd backend && gofmt -w .

check-size: ## Enforce file size limits (<=500 lines, <=80 cols; warn >250 lines)
	./scripts/check-file-size.sh

build: ## Build backend binary + frontend bundle (no Docker)
	cd backend && go build ./...
	cd frontend && npm ci && npm run build

prod-build: ## Build the production single image (SPA + backend, served same-origin)
	docker build -f Dockerfile.prod -t e-agora:latest .

clean: ## Stop the stack and remove the Postgres volume (wipes data)
	docker compose down -v
