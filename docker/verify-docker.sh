#!/bin/bash

# Docker Compose Verification Script
set -e

echo "🐳 Verifying Docker Compose Setup..."
echo ""

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check prerequisites
echo "1️⃣  Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    echo -e "${RED}❌ Docker not found${NC}"
    echo "Install from: https://docs.docker.com/get-docker/"
    exit 1
fi
echo -e "${GREEN}✓ Docker installed${NC}"

if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    echo -e "${RED}❌ docker-compose not found${NC}"
    echo "Install from: https://docs.docker.com/compose/install/"
    exit 1
fi
echo -e "${GREEN}✓ docker-compose installed${NC}"

# Check if Docker daemon is running
if ! docker info &> /dev/null; then
    echo -e "${RED}❌ Docker daemon not running${NC}"
    echo "Start Docker Desktop or run: sudo systemctl start docker"
    exit 1
fi
echo -e "${GREEN}✓ Docker daemon running${NC}"
echo ""

# Check environment file
echo "2️⃣  Checking environment configuration..."
if [ ! -f .env.docker ]; then
    echo -e "${YELLOW}⚠️  .env.docker not found, creating from template...${NC}"
    cat > .env.docker << 'ENVFILE'
DB_USER=orchestrator
DB_PASSWORD=orchestrator_dev
DB_NAME=orchestrator
OPENAI_API_KEY=your_key_here
LOG_LEVEL=info
HTTP_WORKER_REPLICAS=1
AGENT_WORKER_REPLICAS=1
ENVFILE
    echo -e "${GREEN}✓ Created .env.docker${NC}"
else
    echo -e "${GREEN}✓ .env.docker exists${NC}"
fi

# Copy to .env if not exists
if [ ! -f .env ]; then
    cp .env.docker .env
    echo -e "${GREEN}✓ Created .env from .env.docker${NC}"
fi
echo ""

# Clean up any previous runs
echo "3️⃣  Cleaning up previous containers..."
docker-compose down -v 2>/dev/null || true
echo -e "${GREEN}✓ Cleaned up${NC}"
echo ""

# Build images
echo "4️⃣  Building Docker images..."
echo "This may take 5-10 minutes on first run..."
if docker-compose build --no-cache 2>&1 | tee /tmp/docker-build.log; then
    echo -e "${GREEN}✓ Build successful${NC}"
else
    echo -e "${RED}❌ Build failed${NC}"
    echo "Check /tmp/docker-build.log for details"
    exit 1
fi
echo ""

# Start services
echo "5️⃣  Starting services..."
docker-compose up -d

# Wait for services to initialize
echo ""
echo "6️⃣  Waiting for services to become healthy..."
sleep 5

# Check service status
echo ""
echo "7️⃣  Checking service health..."

SERVICES=(
    "orchestrator-postgres:5432:postgres"
    "orchestrator-redis:6379:redis"
    "orchestrator-api:8081:orchestrator"
    "orchestrator-workflow-runner::workflow-runner"
    "orchestrator-http-worker::http-worker"
    "orchestrator-hitl-worker::hitl-worker"
    "orchestrator-agent-runner::agent-runner"
    "orchestrator-fanout:8085:fanout"
)

ALL_HEALTHY=true

for service_info in "${SERVICES[@]}"; do
    IFS=':' read -r container port name <<< "$service_info"

    # Check if container is running
    if docker ps --filter "name=$container" --format "{{.Names}}" | grep -q "$container"; then
        echo -e "${GREEN}✓ $name running${NC}"

        # If port specified, check if it's listening
        if [ -n "$port" ]; then
            sleep 2
            if curl -f -s -o /dev/null http://localhost:$port/health 2>/dev/null || \
               docker exec $container pgrep -f $name > /dev/null 2>&1; then
                echo -e "  ${GREEN}✓ Health check passed${NC}"
            else
                echo -e "  ${YELLOW}⚠️  Health check pending (service may still be starting)${NC}"
            fi
        fi
    else
        echo -e "${RED}✗ $name NOT running${NC}"
        ALL_HEALTHY=false
    fi
done

echo ""

# Show container status
echo "8️⃣  Container status:"
docker-compose ps

echo ""

# Check logs for errors
echo "9️⃣  Checking for errors in logs..."
ERROR_COUNT=$(docker-compose logs --tail=50 2>&1 | grep -i "error\|fatal\|panic" | wc -l)
if [ "$ERROR_COUNT" -gt 0 ]; then
    echo -e "${YELLOW}⚠️  Found $ERROR_COUNT error messages in logs${NC}"
    echo "Run 'docker-compose logs' to investigate"
else
    echo -e "${GREEN}✓ No errors found${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ "$ALL_HEALTHY" = true ]; then
    echo -e "${GREEN}✅ All services are running!${NC}"
    echo ""
    echo "Exposed ports:"
    echo "  • Orchestrator API:  http://localhost:8081"
    echo "  • Fanout WebSocket:  http://localhost:8085"
    echo "  • Postgres:          localhost:5432"
    echo "  • Redis:             localhost:6379"
    echo ""
    echo "Test the API:"
    echo "  curl http://localhost:8081/health"
    echo ""
    echo "View logs:"
    echo "  docker-compose logs -f"
    echo ""
    echo "Stop all:"
    echo "  docker-compose down"
else
    echo -e "${RED}❌ Some services failed to start${NC}"
    echo ""
    echo "Check logs:"
    echo "  docker-compose logs"
    echo ""
    echo "Check specific service:"
    echo "  docker-compose logs orchestrator"
    echo "  docker-compose logs workflow-runner"
    exit 1
fi
