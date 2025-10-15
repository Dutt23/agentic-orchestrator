#!/bin/bash

# Quick test script for Docker services
set -e

echo "🧪 Testing Docker services..."

# Test orchestrator API
echo "Testing Orchestrator API..."
if curl -f -s http://localhost:8081/health > /dev/null; then
    echo "✓ Orchestrator API responding"
else
    echo "✗ Orchestrator API not responding"
    exit 1
fi

# Test fanout
echo "Testing Fanout service..."
if curl -f -s http://localhost:8085/health > /dev/null; then
    echo "✓ Fanout service responding"
else
    echo "✗ Fanout service not responding"
    exit 1
fi

# Test postgres
echo "Testing Postgres..."
if docker exec orchestrator-postgres pg_isready -U orchestrator > /dev/null 2>&1; then
    echo "✓ Postgres accepting connections"
else
    echo "✗ Postgres not ready"
    exit 1
fi

# Test redis
echo "Testing Redis..."
if docker exec orchestrator-redis redis-cli ping > /dev/null 2>&1; then
    echo "✓ Redis responding"
else
    echo "✗ Redis not responding"
    exit 1
fi

# Check all containers are running
echo ""
echo "Container status:"
docker-compose ps

echo ""
echo "✅ All core services are operational!"
