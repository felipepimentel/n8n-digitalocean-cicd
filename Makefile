.PHONY: help init dev prod clean lint test backup restore logs

# Variables
SHELL := /bin/bash
DOCKER_COMPOSE := docker-compose
DOCKER_COMPOSE_DEV := $(DOCKER_COMPOSE) -f docker-compose.dev.yml
DOCKER_COMPOSE_PROD := $(DOCKER_COMPOSE) -f docker-compose.prod.yml

# Help
help: ## Show this help message
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-20s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# Initialize project
init: ## Initialize the project
	@echo "Initializing project..."
	@./scripts/init.sh
	@cd ci && go mod tidy
	@echo "Project initialized successfully!"

# Development
dev: ## Start development environment
	@echo "Starting development environment..."
	@$(DOCKER_COMPOSE_DEV) up -d
	@echo "Development environment started! Access at http://localhost:5678"

# Production
prod: ## Start production environment
	@echo "Starting production environment..."
	@$(DOCKER_COMPOSE_PROD) up -d
	@echo "Production environment started! Access at https://${N8N_DOMAIN}"

# Clean
clean: ## Clean up containers, volumes, and temporary files
	@echo "Cleaning up..."
	@$(DOCKER_COMPOSE_DEV) down -v
	@$(DOCKER_COMPOSE_PROD) down -v
	@rm -rf backups/* logs/*
	@echo "Cleanup complete!"

# Lint
lint: ## Run linters
	@echo "Running linters..."
	@cd ci && golangci-lint run
	@echo "Linting complete!"

# Test
test: ## Run tests
	@echo "Running tests..."
	@cd ci && go test -v ./...
	@echo "Tests complete!"

# Backup
backup: ## Create a backup
	@echo "Creating backup..."
	@./scripts/backup.sh
	@echo "Backup complete!"

# Restore
restore: ## Restore from backup
	@echo "Available backups:"
	@ls -1 backups/*.tar.gz 2>/dev/null || echo "No backups found!"
	@read -p "Enter backup file name to restore: " file; \
	if [ -f "backups/$$file" ]; then \
		echo "Restoring from $$file..."; \
		$(DOCKER_COMPOSE_PROD) stop n8n; \
		tar -xzf "backups/$$file" -C /; \
		$(DOCKER_COMPOSE_PROD) start n8n; \
		echo "Restore complete!"; \
	else \
		echo "Backup file not found!"; \
		exit 1; \
	fi

# Logs
logs: ## View logs
	@echo "Viewing logs..."
	@$(DOCKER_COMPOSE_PROD) logs -f

# Deploy
deploy: ## Deploy to production
	@echo "Deploying to production..."
	@cd ci && go run main.go
	@echo "Deployment complete!"

# Monitor
monitor: ## Monitor the application
	@echo "Starting monitoring..."
	@./scripts/monitor.sh
	@echo "Monitoring complete!"

# Update
update: ## Update N8N version
	@echo "Current version: $${N8N_VERSION:-latest}"
	@read -p "Enter new version (or press enter for latest): " version; \
	if [ -n "$$version" ]; then \
		sed -i "s/N8N_VERSION=.*/N8N_VERSION=$$version/" .env; \
	else \
		sed -i "s/N8N_VERSION=.*/N8N_VERSION=latest/" .env; \
	fi
	@echo "Version updated! Run 'make deploy' to apply changes." 