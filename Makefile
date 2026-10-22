.PHONY: help run-api run-worker build wire dev clean test \
        docker-up docker-deps docker-down docker-stop \
        docker-restart docker-build docker-logs docker-logs-api \
        docker-logs-worker docker-ps docker-down-volumes \
        docker-api docker-worker docker-postgres docker-redis \
        docker-rabbitmq

# --------------------------------------------------
# Project configuration
# --------------------------------------------------

COMPOSE_FILE := deployments/docker-compose.yaml
COMPOSE := docker compose -f $(COMPOSE_FILE)

.DEFAULT_GOAL := help

# --------------------------------------------------
# Help
# --------------------------------------------------

help: ## Show available commands
	@echo ""
	@echo "Retrievion - Available Commands"
	@echo ""
	@echo "Local Development:"
	@echo "  make run-api             Run API locally"
	@echo "  make run-worker          Run worker locally"
	@echo "  make build               Build API binary"
	@echo "  make wire                Generate Wire dependencies"
	@echo "  make dev                 Generate Wire dependencies and run API"
	@echo "  make test                Run all Go tests"
	@echo "  make clean               Remove build artifacts"
	@echo ""
	@echo "Docker Compose:"
	@echo "  make docker-deps         Start PostgreSQL, Redis, RabbitMQ"
	@echo "  make docker-up           Start all services"
	@echo "  make docker-build        Rebuild images and start all services"
	@echo "  make docker-api          Build and start API and dependencies"
	@echo "  make docker-worker       Build and start worker and dependencies"
	@echo "  make docker-postgres     Start PostgreSQL"
	@echo "  make docker-redis        Start Redis"
	@echo "  make docker-rabbitmq     Start RabbitMQ"
	@echo "  make docker-ps           Show service status"
	@echo "  make docker-logs         Follow all service logs"
	@echo "  make docker-logs-api     Follow API logs"
	@echo "  make docker-logs-worker  Follow worker logs"
	@echo "  make docker-restart      Restart all services"
	@echo "  make docker-stop         Stop containers"
	@echo "  make docker-down         Stop and remove containers"
	@echo "  make docker-down-volumes Remove containers and volumes (DESTRUCTIVE)"
	@echo ""

# --------------------------------------------------
# Local development
# --------------------------------------------------

run-api: ## Run API locally
	go run ./cmd/api

run-worker: ## Run worker locally
	go run ./cmd/worker

build: ## Build API binary
	mkdir -p bin
	go build -o bin/api ./cmd/api

wire: ## Generate Wire dependencies
	cd cmd/api && wire
	cd cmd/worker && wire

dev: wire ## Generate Wire dependencies and run API
	go run ./cmd/api

test: ## Run all Go tests
	go test -v ./...

clean: ## Remove build artifacts
	rm -rf bin/

# --------------------------------------------------
# Docker Compose lifecycle
# --------------------------------------------------

docker-up: ## Start all services
	$(COMPOSE) up -d

docker-deps: ## Start infrastructure dependencies only
	$(COMPOSE) up -d postgres redis rabbitmq

docker-build: ## Rebuild images and start all services
	$(COMPOSE) up -d --build

docker-down: ## Stop and remove containers; preserve named volumes
	$(COMPOSE) down

docker-stop: ## Stop containers without removing them
	$(COMPOSE) stop

docker-restart: ## Restart all services
	$(COMPOSE) restart

# --------------------------------------------------
# Individual services
# --------------------------------------------------

docker-api: ## Build and start API and its dependencies
	$(COMPOSE) up -d --build api

docker-worker: ## Build and start worker and its dependencies
	$(COMPOSE) up -d --build worker

docker-postgres: ## Start PostgreSQL
	$(COMPOSE) up -d postgres

docker-redis: ## Start Redis
	$(COMPOSE) up -d redis

docker-rabbitmq: ## Start RabbitMQ
	$(COMPOSE) up -d rabbitmq

# --------------------------------------------------
# Docker status and logs
# --------------------------------------------------

docker-ps: ## Show container status and published ports
	$(COMPOSE) ps

docker-logs: ## Follow logs from all services
	$(COMPOSE) logs -f

docker-logs-api: ## Follow API logs
	$(COMPOSE) logs -f api

docker-logs-worker: ## Follow worker logs
	$(COMPOSE) logs -f worker

# --------------------------------------------------
# Destructive operations
# --------------------------------------------------

docker-down-volumes: ## Remove containers and volumes (DESTRUCTIVE)
	@echo "WARNING: This removes named volumes and persisted database data."
	@printf "Continue? Type 'yes' to confirm: "; \
	read answer; \
	if [ "$$answer" = "yes" ]; then \
		$(COMPOSE) down -v; \
	else \
		echo "Cancelled."; \
	fi
