#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Helper functions
log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')] $1${NC}"
}

warn() {
    echo -e "${YELLOW}[$(date '+%Y-%m-%d %H:%M:%S')] WARNING: $1${NC}"
}

error() {
    echo -e "${RED}[$(date '+%Y-%m-%d %H:%M:%S')] ERROR: $1${NC}"
    exit 1
}

# Check required commands
check_requirements() {
    log "Checking requirements..."
    
    commands=("docker" "docker-compose" "curl" "openssl" "jq")
    for cmd in "${commands[@]}"; do
        if ! command -v "$cmd" &> /dev/null; then
            error "$cmd is required but not installed."
        fi
    done
}

# Generate encryption key
generate_encryption_key() {
    log "Generating N8N encryption key..."
    ENCRYPTION_KEY=$(openssl rand -hex 16)
    sed -i "s/32-char-encryption-key/$ENCRYPTION_KEY/" .env
}

# Create required directories
create_directories() {
    log "Creating required directories..."
    
    dirs=("caddy" "scripts" "backups" "logs")
    for dir in "${dirs[@]}"; do
        mkdir -p "/opt/n8n/$dir"
    done
}

# Set correct permissions
set_permissions() {
    log "Setting correct permissions..."
    
    chmod +x scripts/*.sh
    chown -R 1000:1000 /opt/n8n
}

# Verify environment variables
verify_env() {
    log "Verifying environment variables..."
    
    required_vars=(
        "DIGITALOCEAN_ACCESS_TOKEN"
        "DOCKER_REGISTRY"
        "DO_SSH_KEY_ID"
        "N8N_DOMAIN"
        "N8N_BASIC_AUTH_USER"
        "N8N_BASIC_AUTH_PASSWORD"
    )
    
    for var in "${required_vars[@]}"; do
        if [ -z "${!var}" ]; then
            error "Required environment variable $var is not set"
        fi
    done
    
    # Validate password strength
    if [ ${#N8N_BASIC_AUTH_PASSWORD} -lt 12 ]; then
        error "N8N_BASIC_AUTH_PASSWORD must be at least 12 characters long"
    fi
    
    # Validate username length
    if [ ${#N8N_BASIC_AUTH_USER} -lt 8 ]; then
        error "N8N_BASIC_AUTH_USER must be at least 8 characters long"
    fi
}

# Initialize Docker
init_docker() {
    log "Initializing Docker..."
    
    # Create Docker network
    docker network create n8n-network 2>/dev/null || true
    
    # Create Docker volumes
    docker volume create n8n_data
    docker volume create caddy_data
    docker volume create caddy_config
}

# Test Docker registry access
test_registry() {
    log "Testing Docker registry access..."
    
    if ! docker login registry.digitalocean.com -u "$DIGITALOCEAN_ACCESS_TOKEN" -p "$DIGITALOCEAN_ACCESS_TOKEN"; then
        error "Failed to authenticate with DigitalOcean registry"
    fi
}

# Main setup process
main() {
    log "Starting N8N setup..."
    
    check_requirements
    
    # Source environment variables if .env exists
    if [ -f .env ]; then
        source .env
    else
        error ".env file not found"
    fi
    
    verify_env
    create_directories
    generate_encryption_key
    set_permissions
    init_docker
    test_registry
    
    log "Setup completed successfully!"
    log "You can now run 'go run ci/main.go' to deploy N8N"
    log "Access your instance at: https://${N8N_DOMAIN}"
}

main 