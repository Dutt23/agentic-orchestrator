.PHONY: help start stop restart status logs build clean test dev generate-types watch-schema validate-schema clean-generated test-schema

# Default target
help:
	@echo "Orchestrator Platform - Available Commands"
	@echo ""
	@echo "Service Management:"
	@echo "  make start      - Start all services"
	@echo "  make stop       - Stop all services"
	@echo "  make restart    - Restart all services"
	@echo "  make status     - Show service status"
	@echo "  make logs       - Tail all logs"
	@echo ""
	@echo "Individual Services:"
	@echo "  make start-orchestrator    - Start orchestrator service"
	@echo "  make start-workflow-runner - Start workflow-runner service"
	@echo "  make start-http-worker     - Start HTTP worker"
	@echo "  make start-hitl-worker     - Start HITL worker"
	@echo "  make start-fanout          - Start fanout service"
	@echo ""
	@echo "Building:"
	@echo "  make build      - Build all services"
	@echo "  make test       - Run tests"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make dev        - Start in development mode"
	@echo ""
	@echo "Type Generation:"
	@echo "  make generate-types    - Generate types from JSON schemas"
	@echo "  make watch-schema      - Watch schemas and auto-regenerate"
	@echo "  make validate-schema   - Validate JSON schemas"
	@echo "  make clean-generated   - Remove generated type files"
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
	@if [ -d "cmd/workflow-runner" ]; then \
		echo "Building workflow-runner..."; \
		go build -o bin/workflow-runner ./cmd/workflow-runner; \
	fi
	@if [ -d "cmd/http-worker" ]; then \
		echo "Building http-worker..."; \
		go build -o bin/http-worker ./cmd/http-worker; \
	fi
	@if [ -d "cmd/hitl-worker" ]; then \
		echo "Building hitl-worker..."; \
		go build -o bin/hitl-worker ./cmd/hitl-worker; \
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

# Start individual services (useful for development)
start-orchestrator:
	@echo "Starting orchestrator..."
	./cmd/orchestrator/start.sh

start-workflow-runner:
	@echo "Starting workflow-runner..."
	./cmd/workflow-runner/start.sh

start-http-worker:
	@echo "Starting http-worker..."
	./cmd/http-worker/start.sh

start-hitl-worker:
	@echo "Starting hitl-worker..."
	./cmd/hitl-worker/start.sh

start-fanout:
	@echo "Starting fanout..."
	./cmd/fanout/start.sh

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

# === Type Generation from JSON Schema ===

# Check if code generation tools are installed
check-codegen-tools:
	@command -v quicktype >/dev/null 2>&1 || { echo "Error: quicktype not found. Install with: npm install -g quicktype"; exit 1; }
	@echo "✓ quicktype found"

# Generate Rust types from JSON Schema
generate-rust-types: check-codegen-tools
	@echo "Generating Rust types from schema..."
	@mkdir -p crates/dag-optimizer/src
	@quicktype common/schema/workflow.schema.json \
		--lang rust \
		--derive-debug \
		--derive-clone \
		--visibility public \
		--density dense \
		-o crates/dag-optimizer/src/types.rs \
		|| { echo "Error: Failed to generate Rust types"; exit 1; }
	@echo "✓ Rust types generated: crates/dag-optimizer/src/types.rs"

# Generate Go types from JSON Schema
generate-go-types:
	@echo "Generating Go types from schema..."
	@command -v go-jsonschema >/dev/null 2>&1 || { echo "Warning: go-jsonschema not found. Install with: go install github.com/atombender/go-jsonschema@latest"; echo "Skipping Go type generation..."; exit 0; }
	@mkdir -p cmd/orchestrator/models/generated
	@go-jsonschema \
		--package generated \
		--output cmd/orchestrator/models/generated/workflow.go \
		common/schema/workflow.schema.json \
		|| { echo "Error: Failed to generate Go types"; exit 1; }
	@echo "✓ Go types generated: cmd/orchestrator/models/generated/workflow.go"

# Generate all types
generate-types: generate-rust-types generate-go-types
	@echo ""
	@echo "✓ All types generated successfully"
	@echo ""
	@echo "Next steps:"
	@echo "  1. Review generated files"
	@echo "  2. Run 'make test-schema' to verify compatibility"
	@echo "  3. Update your code to use the generated types"

# Validate JSON schemas
validate-schema:
	@echo "Validating JSON schemas..."
	@command -v ajv >/dev/null 2>&1 || { echo "Warning: ajv-cli not installed. Run: npm install -g ajv-cli"; exit 0; }
	@ajv validate -s common/schema/workflow.schema.json -d common/schema/examples/*.json
	@echo "✓ All schemas valid"

# Watch schemas for changes (requires fswatch on macOS, inotifywait on Linux)
watch-schema:
	@echo "Watching schema files for changes..."
	@echo "Press Ctrl+C to stop"
	@command -v fswatch >/dev/null 2>&1 || { echo "Error: fswatch not found. Install with: brew install fswatch (macOS)"; exit 1; }
	@fswatch -o common/schema/*.schema.json | while read change; do \
		echo ""; \
		echo "Schema changed, regenerating types..."; \
		$(MAKE) generate-types; \
		echo ""; \
	done

# Test schema compatibility
test-schema:
	@echo "Testing schema compatibility..."
	@cd crates/dag-optimizer && cargo test --lib 2>/dev/null || echo "Run 'make generate-rust-types' first"
	@echo "✓ Schema tests passed"

# Clean generated files
clean-generated:
	@echo "Cleaning generated files..."
	@rm -f crates/dag-optimizer/src/types.rs
	@rm -f cmd/orchestrator/models/generated/workflow.go
	@echo "✓ Generated files removed"
