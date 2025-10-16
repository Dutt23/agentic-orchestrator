#!/bin/bash
# Post-create setup script for GitHub Codespaces

set -e

echo "=========================================="
echo "Setting up Orchestrator Dev Environment"
echo "=========================================="

# Install additional tools
echo "Installing additional tools..."
sudo apt-get update -qq
sudo apt-get install -y -qq postgresql-client redis-tools jq curl

# Install Go tools
echo "Installing Go tools..."
go install golang.org/x/tools/gopls@latest 2>/dev/null
go install github.com/go-delve/delve/cmd/dlv@latest 2>/dev/null

# Install Python tools
echo "Installing Python tools..."
pip install --user black pylint pytest 2>/dev/null

# Create .env from example if it doesn't exist
if [ ! -f ".env" ]; then
    if [ -f ".env.example" ]; then
        echo "Creating .env from example..."
        cp .env.example .env
    fi
fi

# Create docker symlink
ln -sf ../.env docker/.env 2>/dev/null || true

echo ""
echo "=========================================="
echo "✅ Setup Complete!"
echo "=========================================="
echo ""
echo "IMPORTANT: This is GitHub Codespaces"
echo ""
echo "Before starting services:"
echo "  1. Edit .env and add OPENAI_API_KEY"
echo "  2. The docker-compose.yml defaults work for local development"
echo "  3. For Codespaces, you'll need to update frontend URLs after services start"
echo ""
echo "To start services:"
echo "  cd docker && ./setup.sh"
echo ""
echo "Note: Codespaces may timeout after 4 hours of inactivity."
echo "      Your work is saved, but you'll need to restart services."
echo ""
