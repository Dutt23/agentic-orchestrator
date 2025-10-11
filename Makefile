no.PHONY: help start stop restart status logs build clean test dev

# Default target
help:
	@echo "Orchestrator Platform - Available Commands"
	@echo ""
	@echo "  make start      - Start all services"
	@echo "  make stop       - Stop all services"
	@echo "  make restart    - Restart all services"
	@echo "  make status     - Show service status"
	@echo "  make logs       - Tail all logs"
	@echo "  make build      - Build all services"
	@echo "  make test       - Run tests"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make dev        - Start in development mode"
	@echo ""

# Start all services
start:
	./start.sh start

# Stop all services
stop:
	./start.sh stop

# Restart all services
restart:
	./start.sh restart

# Show service status
status:
	./start.sh status

# Tail logs
logs:
	./start.sh logs

# Build all services
build:
	@echo "Building Go services..."
	@mkdir -p bin
	@if [ -d "cmd/orchestrator" ]; then \
		echo "Building orchestrator..."; \
		go build -o bin/orchestrator ./cmd/orchestrator; \
	fi
	@if [ -d "cmd/api" ]; then \
		echo "Building api..."; \
		go build -o bin/api ./cmd/api; \
	fi
	@if [ -d "cmd/runner" ]; then \
		echo "Building runner..."; \
		go build -o bin/runner ./cmd/runner; \
	fi
	@if [ -d "cmd/fanout" ]; then \
		echo "Building fanout..."; \
		go build -o bin/fanout ./cmd/fanout; \
	fi
	@echo "Build complete!"

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	@echo "Coverage report:"
	go tool cover -func=coverage.out | grep total

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf logs/
	rm -rf .pids/
	rm -rf data/
	rm -f coverage.out
	@if [ -d "ui/build" ]; then rm -rf ui/build; fi
	@if [ -d "ui/node_modules" ]; then rm -rf ui/node_modules; fi
	@echo "Clean complete!"

# Development mode (with auto-reload)
dev:
	@echo "Starting in development mode..."
	@echo "This requires 'air' for Go hot-reload: go install github.com/cosmtrek/air@latest"
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "Installing air..."; \
		go install github.com/cosmtrek/air@latest; \
		air; \
	fi

# Initialize project (first time setup)
init:
	@echo "Initializing project..."
	@mkdir -p cmd/orchestrator cmd/api cmd/runner cmd/fanout
	@mkdir -p pkg/events pkg/state pkg/runner pkg/overlays pkg/policy
	@mkdir -p migrations
	@mkdir -p ui
	@echo "Project structure created!"
	@echo "Run 'make scaffold' to generate boilerplate code"

# Quick demo workflow
demo:
	@echo "Running demo workflow..."
	curl -X POST http://localhost:8081/api/runs \
		-H "Content-Type: application/json" \
		-d @examples/demo-workflow.json
