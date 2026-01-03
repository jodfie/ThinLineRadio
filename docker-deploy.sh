#!/bin/bash

# Docker Deployment Helper Script
# This script helps you quickly set up and deploy ThinLine Radio using Docker

set -e

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}ThinLine Radio - Docker Deployment Helper${NC}"
echo "==========================================="
echo ""

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    echo -e "${RED}ERROR: Docker is not installed${NC}"
    echo "Please install Docker: https://docs.docker.com/get-docker/"
    exit 1
fi

# Check if Docker Compose is installed
if ! docker compose version &> /dev/null; then
    echo -e "${RED}ERROR: Docker Compose is not installed${NC}"
    echo "Please install Docker Compose: https://docs.docker.com/compose/install/"
    exit 1
fi

echo -e "${GREEN}✓ Docker installed:${NC} $(docker --version)"
echo -e "${GREEN}✓ Docker Compose installed:${NC} $(docker compose version)"
echo ""

# Check if .env file exists
if [ ! -f .env ]; then
    echo -e "${YELLOW}Creating .env file from template...${NC}"
    
    if [ -f env.docker.example ]; then
        cp env.docker.example .env
        echo -e "${GREEN}✓ Created .env file${NC}"
        echo ""
        echo -e "${YELLOW}IMPORTANT: You must edit .env and set a secure database password!${NC}"
        echo "Edit now? (y/n)"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            ${EDITOR:-nano} .env
        else
            echo -e "${RED}Please edit .env before continuing${NC}"
            exit 1
        fi
    else
        echo -e "${RED}ERROR: env.docker.example not found${NC}"
        exit 1
    fi
else
    echo -e "${GREEN}✓ .env file exists${NC}"
    
    # Check if password was changed
    if grep -q "change_this_password_immediately" .env; then
        echo -e "${RED}WARNING: Default database password detected!${NC}"
        echo "You MUST change the database password in .env"
        echo "Edit now? (y/n)"
        read -r response
        if [[ "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
            ${EDITOR:-nano} .env
        else
            echo -e "${RED}Please edit .env before continuing${NC}"
            exit 1
        fi
    fi
fi

echo ""

# Create required directories
echo -e "${YELLOW}Creating required directories...${NC}"

mkdir -p docker/data/postgres
mkdir -p docker/data/thinline
mkdir -p docker/data/logs
mkdir -p docker/config/ssl
mkdir -p docker/config/credentials
mkdir -p docker/init-db

# Set permissions
chmod -R 755 docker/

echo -e "${GREEN}✓ Directories created${NC}"
echo ""

# Check if containers are already running
if docker compose ps | grep -q "Up"; then
    echo -e "${YELLOW}Containers are already running${NC}"
    echo "What would you like to do?"
    echo "  1) Restart containers"
    echo "  2) Rebuild and restart"
    echo "  3) Stop containers"
    echo "  4) View logs"
    echo "  5) Exit"
    read -r choice
    
    case $choice in
        1)
            echo -e "${YELLOW}Restarting containers...${NC}"
            docker compose restart
            ;;
        2)
            echo -e "${YELLOW}Rebuilding and restarting...${NC}"
            docker compose up -d --build
            ;;
        3)
            echo -e "${YELLOW}Stopping containers...${NC}"
            docker compose down
            exit 0
            ;;
        4)
            echo -e "${YELLOW}Viewing logs (Ctrl+C to exit)...${NC}"
            docker compose logs -f
            exit 0
            ;;
        5)
            exit 0
            ;;
        *)
            echo -e "${RED}Invalid choice${NC}"
            exit 1
            ;;
    esac
else
    # Start containers
    echo -e "${YELLOW}Building and starting Docker containers...${NC}"
    echo "This may take several minutes on first run..."
    echo ""
    
    docker compose up -d
fi

echo ""
echo -e "${GREEN}=========================================${NC}"
echo -e "${GREEN}✓ ThinLine Radio is starting!${NC}"
echo -e "${GREEN}=========================================${NC}"
echo ""
echo "Waiting for services to be ready..."

# Wait for health checks
sleep 5

# Check status
echo ""
echo "Container Status:"
docker compose ps

echo ""
echo -e "${GREEN}Access ThinLine Radio:${NC}"
echo "  Web Interface: http://localhost:3000/"
echo "  Admin Dashboard: http://localhost:3000/admin"
echo ""
echo -e "${YELLOW}Default admin password: admin${NC}"
echo -e "${RED}CHANGE THIS IMMEDIATELY after first login!${NC}"
echo ""
echo "Useful commands:"
echo "  View logs:        docker compose logs -f"
echo "  Stop containers:  docker compose down"
echo "  Restart:          docker compose restart"
echo "  Status:           docker compose ps"
echo ""
echo "For more information, see: docker/README.md"
echo ""

