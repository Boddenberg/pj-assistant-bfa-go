.PHONY: help build test run lint clean docker-up docker-down

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ============================================================
# BFA (Go)
# ============================================================

build: ## Build the BFA binary
	go build -o bin/bfa ./cmd/bfa

run: build ## Run the BFA locally
	./bin/bfa

test: ## Run all Go tests
	go test ./... -v -race -cover

test-cover: ## Run tests with coverage report
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

lint: ## Lint Go code
	golangci-lint run ./...

# ============================================================
# Agent (Python)
# ============================================================

sync-kb: ## Sync knowledge_base/ into agent/knowledge_base/ (single source of truth)
	rm -rf agent/knowledge_base/
	cp -r knowledge_base/ agent/knowledge_base/

agent-install: ## Install Python agent dependencies
	cd agent && pip install -e ".[dev]"

agent-run: sync-kb ## Run the Python agent locally
	cd agent && uvicorn app.server:app --reload --port 8090

agent-test: ## Run Python agent tests
	cd agent && pytest tests/ -v --cov=app --cov-report=term-missing

agent-lint: ## Lint Python code
	cd agent && ruff check .

# ============================================================
# Docker
# ============================================================

docker-up: sync-kb ## Start all services with Docker Compose
	docker compose up --build -d

docker-down: ## Stop all services
	docker compose down

docker-logs: ## Follow logs of all services
	docker compose logs -f

# ============================================================
# Full
# ============================================================

test-all: test agent-test ## Run all tests (Go + Python)

clean: ## Clean build artifacts
	rm -rf bin/ coverage.out coverage.html
	rm -rf agent/data/chroma/
	rm -rf agent/__pycache__/ agent/.pytest_cache/
