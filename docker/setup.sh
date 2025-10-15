#!/bin/bash
set -e

echo "=========================================="
echo "Orchestrator Docker Setup"
echo "=========================================="
echo ""

# Check if .env exists in parent directory
if [ ! -f "../.env" ]; then
    echo "❌ .env file not found in project root!"
    echo ""
    echo "Please create .env file:"
    echo "  1. cd .."
    echo "  2. cp .env.example .env"
    echo "  3. Edit .env and add OPENAI_API_KEY"
    echo ""
    exit 1
fi

# Check if OPENAI_API_KEY is set
if ! grep -q "OPENAI_API_KEY=sk-" ../.env; then
    echo "⚠️  Warning: OPENAI_API_KEY not found or invalid in .env"
    echo "   Agent-runner service will fail without a valid API key"
    echo ""
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

echo "✅ Environment file found"
echo ""

# Check if symlink exists
if [ ! -L ".env" ]; then
    echo "Creating symlink to .env file..."
    ln -sf ../.env .env
    echo "✅ Symlink created"
else
    echo "✅ Symlink already exists"
fi

echo ""
echo "Building and starting services..."
echo "This may take a few minutes on first run..."
echo ""

docker-compose up --build -d

echo ""
echo "=========================================="
echo "✅ Services Started!"
echo "=========================================="
echo ""
echo "Checking status..."
sleep 3
docker-compose ps

echo ""
echo "Test endpoints:"
echo "  - Frontend:     http://localhost:3000"
echo "  - Orchestrator: http://localhost:8081/health"
echo "  - Fanout:       http://localhost:8085/health"
echo ""
echo "View logs:"
echo "  docker-compose logs -f"
echo ""
echo "Stop services:"
echo "  docker-compose down"
echo ""
