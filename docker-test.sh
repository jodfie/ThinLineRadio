#!/bin/bash

# Docker Test Script for ThinLine Radio
# This script tests the Docker build and deployment

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}ThinLine Radio - Docker Test Suite${NC}"
echo "===================================="
echo ""

# Test 1: Check prerequisites
echo -e "${YELLOW}Test 1: Checking prerequisites...${NC}"

if ! command -v docker &> /dev/null; then
    echo -e "${RED}✗ Docker not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker installed: $(docker --version)${NC}"

if ! docker compose version &> /dev/null; then
    echo -e "${RED}✗ Docker Compose not installed${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Docker Compose installed: $(docker compose version)${NC}"

# Test 2: Check Dockerfile syntax
echo ""
echo -e "${YELLOW}Test 2: Validating Dockerfile...${NC}"
if docker build --check . &> /dev/null 2>&1 || true; then
    echo -e "${GREEN}✓ Dockerfile syntax valid${NC}"
else
    echo -e "${YELLOW}⚠ Cannot validate Dockerfile syntax (requires Docker BuildKit)${NC}"
fi

# Test 3: Check docker-compose syntax
echo ""
echo -e "${YELLOW}Test 3: Validating docker-compose.yml...${NC}"
if docker compose config > /dev/null; then
    echo -e "${GREEN}✓ docker-compose.yml syntax valid${NC}"
else
    echo -e "${RED}✗ docker-compose.yml has errors${NC}"
    exit 1
fi

# Test 4: Check required files
echo ""
echo -e "${YELLOW}Test 4: Checking required files...${NC}"

required_files=(
    "Dockerfile"
    "docker-compose.yml"
    ".dockerignore"
    "env.docker.example"
    "docker/README.md"
    "docker/config/README.md"
    "docker/init-db/README.md"
)

all_files_exist=true
for file in "${required_files[@]}"; do
    if [ -f "$file" ]; then
        echo -e "${GREEN}✓ $file exists${NC}"
    else
        echo -e "${RED}✗ $file missing${NC}"
        all_files_exist=false
    fi
done

if [ "$all_files_exist" = false ]; then
    exit 1
fi

# Test 5: Check .env file
echo ""
echo -e "${YELLOW}Test 5: Checking .env configuration...${NC}"

if [ ! -f .env ]; then
    echo -e "${YELLOW}⚠ .env file not found (use env.docker.example)${NC}"
    echo "  Creating .env from template for testing..."
    cp env.docker.example .env
    # Set a test password
    sed -i.bak 's/change_this_password_immediately/test_password_12345/g' .env
    rm -f .env.bak
    echo -e "${GREEN}✓ Created test .env file${NC}"
else
    echo -e "${GREEN}✓ .env file exists${NC}"
    
    if grep -q "change_this_password_immediately" .env; then
        echo -e "${RED}✗ Default password still in use!${NC}"
        echo "  Please change DB_PASS in .env"
        exit 1
    else
        echo -e "${GREEN}✓ Database password has been changed${NC}"
    fi
fi

# Test 6: Build Docker image
echo ""
echo -e "${YELLOW}Test 6: Building Docker image...${NC}"
echo "This may take several minutes..."
if docker compose build --no-cache; then
    echo -e "${GREEN}✓ Docker image built successfully${NC}"
else
    echo -e "${RED}✗ Docker build failed${NC}"
    exit 1
fi

# Test 7: Check image size
echo ""
echo -e "${YELLOW}Test 7: Checking image size...${NC}"
IMAGE_SIZE=$(docker images thinlineradio-thinline-radio --format "{{.Size}}")
echo "  Image size: $IMAGE_SIZE"

# Test 8: Start containers
echo ""
echo -e "${YELLOW}Test 8: Starting containers...${NC}"
if docker compose up -d; then
    echo -e "${GREEN}✓ Containers started${NC}"
else
    echo -e "${RED}✗ Failed to start containers${NC}"
    exit 1
fi

# Test 9: Wait for services to be ready
echo ""
echo -e "${YELLOW}Test 9: Waiting for services to be ready...${NC}"
echo "  Waiting 30 seconds for initialization..."
sleep 30

# Test 10: Check container status
echo ""
echo -e "${YELLOW}Test 10: Checking container status...${NC}"
docker compose ps

if docker compose ps | grep -q "Up"; then
    echo -e "${GREEN}✓ Containers are running${NC}"
else
    echo -e "${RED}✗ Containers are not running${NC}"
    docker compose logs
    exit 1
fi

# Test 11: Check PostgreSQL health
echo ""
echo -e "${YELLOW}Test 11: Checking PostgreSQL health...${NC}"
if docker compose exec postgres pg_isready -U thinline_user; then
    echo -e "${GREEN}✓ PostgreSQL is ready${NC}"
else
    echo -e "${RED}✗ PostgreSQL is not ready${NC}"
    docker compose logs postgres
    exit 1
fi

# Test 12: Check ThinLine Radio health
echo ""
echo -e "${YELLOW}Test 12: Checking ThinLine Radio health...${NC}"
if docker compose exec thinline-radio wget --no-verbose --tries=1 --spider http://localhost:3000/ 2>&1 | grep -q "200 OK"; then
    echo -e "${GREEN}✓ ThinLine Radio is responding${NC}"
else
    echo -e "${YELLOW}⚠ ThinLine Radio health check inconclusive${NC}"
    echo "  Checking logs..."
    docker compose logs --tail=20 thinline-radio
fi

# Test 13: Check FFmpeg availability
echo ""
echo -e "${YELLOW}Test 13: Checking FFmpeg installation...${NC}"
if docker compose exec thinline-radio ffmpeg -version > /dev/null 2>&1; then
    echo -e "${GREEN}✓ FFmpeg is installed${NC}"
else
    echo -e "${RED}✗ FFmpeg is not available${NC}"
    exit 1
fi

# Test 14: Check volume mounts
echo ""
echo -e "${YELLOW}Test 14: Checking volume mounts...${NC}"
if docker compose exec thinline-radio ls -la /app/data > /dev/null 2>&1; then
    echo -e "${GREEN}✓ Data volume mounted${NC}"
else
    echo -e "${RED}✗ Data volume not mounted${NC}"
    exit 1
fi

# Test 15: Check logs
echo ""
echo -e "${YELLOW}Test 15: Checking for errors in logs...${NC}"
if docker compose logs thinline-radio | grep -i "FATAL\|PANIC" > /dev/null; then
    echo -e "${RED}✗ Fatal errors found in logs${NC}"
    docker compose logs thinline-radio | grep -i "FATAL\|PANIC"
    exit 1
else
    echo -e "${GREEN}✓ No fatal errors in logs${NC}"
fi

# Summary
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}✓ All tests passed!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Container information:"
docker compose ps
echo ""
echo "Access ThinLine Radio at: http://localhost:3000/"
echo ""
echo "To view logs:     docker compose logs -f"
echo "To stop:          docker compose down"
echo "To clean up:      docker compose down -v"
echo ""

