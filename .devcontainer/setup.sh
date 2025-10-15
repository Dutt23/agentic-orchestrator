#!/bin/bash
# Post-create setup script for GitHub Codespaces

set -e

echo "=========================================="
echo "Setting up Orchestrator Dev Environment"
echo "=========================================="

# Install additional tools
echo "Installing additional tools..."
sudo apt-get update
sudo apt-get install -y postgresql-client redis-tools jq curl

# Install Go tools
echo "Installing Go tools..."
go install golang.org/x/tools/gopls@latest
go install github.com/go-delve/delve/cmd/dlv@latest

# Install Python tools
echo "Installing Python tools..."
pip install --user black pylint pytest

# Install Node tools (for frontend)
echo "Installing Node tools..."
npm install -g npm@latest

# Create .env.example if it doesn't exist
if [ ! -f ".env.example" ]; then
    echo "Creating .env.example..."
    cat > .env.example << 'EOF'
# Required
OPENAI_API_KEY=sk-your-api-key-here

# Database
DB_USER=orchestrator
DB_PASSWORD=orchestrator
DB_NAME=orchestrator
DB_PORT=5432

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Logging
LOG_LEVEL=info

# Workers
HTTP_WORKER_REPLICAS=2
AGENT_WORKER_REPLICAS=2
EOF
fi

# Check if .env exists, if not copy from example
if [ ! -f ".env" ]; then
    echo "Creating .env from example..."
    cp .env.example .env
    echo ""
    echo "⚠️  IMPORTANT: Edit .env and add your OPENAI_API_KEY"
    echo ""
fi

# Create symlink for docker directory
if [ ! -L "docker/.env" ]; then
    echo "Creating docker/.env symlink..."
    ln -sf ../.env docker/.env
fi

# Install frontend dependencies (if frontend exists)
if [ -d "frontend/flow-builder" ]; then
    echo "Installing frontend dependencies..."
    cd frontend/flow-builder
    npm install
    cd ../..
fi

echo ""
echo "=========================================="
echo "✅ Setup Complete!"
echo "=========================================="
echo ""
echo "Next steps:"
echo "  1. Edit .env and add your OPENAI_API_KEY"
echo "  2. Run: cd docker && ./setup.sh"
echo "  3. Open: http://localhost:3000"
echo ""
echo "Useful commands:"
echo "  - Start services: cd docker && docker-compose up -d"
echo "  - View logs: docker-compose logs -f"
echo "  - Check status: docker-compose ps"
echo ""
