---
layout: default
title: Configuration Guide
description: Detailed configuration guide for n8n deployment on DigitalOcean
---

# Configuration Guide

This guide provides detailed information about all configuration options available in the n8n DigitalOcean deployment.

## Table of Contents

- [Environment Variables](#environment-variables)
- [GitHub Secrets](#github-secrets)
- [DigitalOcean Settings](#digitalocean-settings)
- [Docker Configuration](#docker-configuration)
- [Security Configuration](#security-configuration)
- [Monitoring Configuration](#monitoring-configuration)
- [Backup Configuration](#backup-configuration)

## Environment Variables

### Required Variables

| Variable | Description | Example | Generation Method |
|----------|-------------|---------|------------------|
| `N8N_ENCRYPTION_KEY` | Key for encrypting credentials | `c8d6d2d5f6a9b7c8d6d2d5f6a9b7c8d6` | `openssl rand -hex 16` |
| `N8N_BASIC_AUTH_USER` | Admin username | `admin` | User defined |
| `N8N_BASIC_AUTH_PASSWORD` | Admin password | `strongpassword` | `openssl rand -base64 32` |
| `N8N_PROTOCOL` | Protocol for n8n | `https` | Fixed value |
| `N8N_PORT` | Internal port | `5678` | Fixed value |
| `N8N_HOST` | Domain name | `n8n.example.com` | User defined |

### Optional Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `N8N_METRICS` | `true` | Enable Prometheus metrics |
| `N8N_DIAGNOSTICS` | `true` | Enable diagnostics |
| `N8N_USER_FOLDER` | `/home/node/.n8n` | Data directory |
| `NODE_ENV` | `production` | Node.js environment |

### Example `.env` File

```env
# Required Settings
N8N_ENCRYPTION_KEY=c8d6d2d5f6a9b7c8d6d2d5f6a9b7c8d6
N8N_BASIC_AUTH_USER=admin
N8N_BASIC_AUTH_PASSWORD=strongpassword
N8N_PROTOCOL=https
N8N_PORT=5678
N8N_HOST=n8n.example.com

# Optional Settings
N8N_METRICS=true
N8N_DIAGNOSTICS=true
N8N_USER_FOLDER=/home/node/.n8n
NODE_ENV=production

# Monitoring Settings
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/xxx/yyy/zzz
ALERT_EMAIL=admin@example.com
```

## GitHub Secrets

### Required Secrets

| Secret Name | Description | Example |
|-------------|-------------|---------|
| `DIGITALOCEAN_ACCESS_TOKEN` | DO API token | `dop_v1_...` |
| `DO_SSH_KEY_ID` | SSH key ID | `12345678` |
| `DO_SSH_PRIVATE_KEY` | SSH private key | `-----BEGIN OPENSSH PRIVATE KEY-----...` |
| `N8N_DOMAIN` | Domain name | `n8n.example.com` |
| `N8N_ENCRYPTION_KEY` | Encryption key | `c8d6d2d5f6a9b7c8d6d2d5f6a9b7c8d6` |

### Optional Secrets

| Secret Name | Default | Description |
|-------------|---------|-------------|
| `DOCKER_REGISTRY` | `registry.digitalocean.com` | Docker registry URL |
| `DROPLET_NAME` | `n8n-server` | Droplet name |
| `N8N_VERSION` | `latest` | n8n version |
| `N8N_BASIC_AUTH_USER` | `admin` | Basic auth username |
| `N8N_BASIC_AUTH_PASSWORD` | Same as `N8N_ENCRYPTION_KEY` | Basic auth password |

### Setting Up Secrets

1. Go to your repository's Settings
2. Navigate to Secrets and variables > Actions
3. Click "New repository secret"
4. Add each secret with its value

## DigitalOcean Settings

### Droplet Configuration

```yaml
size: s-2vcpu-2gb
region: nyc1
image: docker-20-04
monitoring: true
vpc_uuid: configured-automatically
```

### Firewall Rules

```yaml
inbound_rules:
  - protocol: tcp
    ports: [22, 80, 443]
    sources:
      addresses: ["0.0.0.0/0"]

outbound_rules:
  - protocol: tcp
    ports: [1-65535]
    destinations:
      addresses: ["0.0.0.0/0"]
```

### Network Configuration

```yaml
vpc_network:
  name: n8n-vpc
  ip_range: 10.10.10.0/24
  region: nyc1
```

## Docker Configuration

### Container Settings

```yaml
version: '3'
services:
  n8n:
    image: registry.digitalocean.com/n8n-app:latest
    restart: unless-stopped
    ports:
      - "80:5678"
      - "443:5678"
    environment:
      - NODE_ENV=production
      - N8N_PROTOCOL=https
      - N8N_PORT=5678
      - N8N_HOST=${N8N_DOMAIN}
      - N8N_ENCRYPTION_KEY=${N8N_ENCRYPTION_KEY}
      - N8N_BASIC_AUTH_ACTIVE=true
      - N8N_BASIC_AUTH_USER=${N8N_BASIC_AUTH_USER}
      - N8N_BASIC_AUTH_PASSWORD=${N8N_BASIC_AUTH_PASSWORD}
    volumes:
      - n8n_data:/home/node/.n8n
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:5678/healthz"]
      interval: 1m
      timeout: 10s
      retries: 3
    networks:
      - n8n-network
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G

volumes:
  n8n_data:

networks:
  n8n-network:
    driver: bridge
```

## Security Configuration

### SSH Configuration

```bash
# /etc/ssh/sshd_config
PermitRootLogin no
PasswordAuthentication no
X11Forwarding no
MaxAuthTries 3
LoginGraceTime 20
```

### UFW Configuration

```bash
# Default policies
ufw default deny incoming
ufw default allow outgoing

# Allow specific ports
ufw allow ssh
ufw allow http
ufw allow https

# Rate limiting
ufw limit ssh
```

### fail2ban Configuration

```ini
# /etc/fail2ban/jail.local
[sshd]
enabled = true
bantime = 3600
findtime = 600
maxretry = 3

[n8n-auth]
enabled = true
filter = n8n-auth
logpath = /var/log/n8n/auth.log
maxretry = 5
findtime = 300
bantime = 3600
```

## Monitoring Configuration

### Health Check Script

```bash
#!/bin/bash
# /usr/local/bin/monitor.sh

# Configuration
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"
ALERT_EMAIL="${ALERT_EMAIL:-}"
THRESHOLD_CPU=80
THRESHOLD_MEMORY=80
THRESHOLD_DISK=80

# Check functions
check_health() {
    curl -s -f http://localhost:5678/healthz >/dev/null
}

check_resources() {
    # CPU Usage
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}')
    if (( $(echo "$cpu_usage > $THRESHOLD_CPU" | bc -l) )); then
        send_alert "High CPU usage: ${cpu_usage}%"
    fi

    # Memory Usage
    memory_usage=$(free | grep Mem | awk '{print $3/$2 * 100.0}')
    if (( $(echo "$memory_usage > $THRESHOLD_MEMORY" | bc -l) )); then
        send_alert "High memory usage: ${memory_usage}%"
    fi

    # Disk Usage
    disk_usage=$(df -h / | awk 'NR==2 {print $5}' | cut -d% -f1)
    if [ "$disk_usage" -gt "$THRESHOLD_DISK" ]; then
        send_alert "High disk usage: ${disk_usage}%"
    fi
}
```

## Backup Configuration

### Backup Script

```bash
#!/bin/bash
# /usr/local/bin/backup.sh

# Configuration
BACKUP_DIR="/opt/n8n/backups"
RETENTION_DAYS=7
MAX_BACKUPS=10

# Backup function
create_backup() {
    local timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="$BACKUP_DIR/n8n-backup-$timestamp.tar.gz"
    
    docker run --rm \
        --volumes-from n8n-container \
        -v "$BACKUP_DIR:/backup" \
        alpine tar czf "/backup/$(basename "$backup_file")" /home/node/.n8n
    
    # Encrypt backup
    openssl enc -aes-256-cbc -salt -pbkdf2 \
        -in "$backup_file" \
        -out "$backup_file.enc" \
        -pass file:/opt/n8n/backup.key
    
    rm "$backup_file"
}

# Cleanup function
cleanup_backups() {
    # Remove old backups
    find "$BACKUP_DIR" -name "n8n-backup-*.tar.gz.enc" -mtime +$RETENTION_DAYS -delete
    
    # Keep only MAX_BACKUPS most recent backups
    ls -t "$BACKUP_DIR"/n8n-backup-*.tar.gz.enc | \
        tail -n +$((MAX_BACKUPS + 1)) | \
        xargs -r rm
}
```

## Additional Resources

- [n8n Configuration Docs](https://docs.n8n.io/reference/configuration.html)
- [Docker Compose Reference](https://docs.docker.com/compose/compose-file/)
- [DigitalOcean API Reference](https://developers.digitalocean.com/documentation/v2/)
- [GitHub Actions Documentation](https://docs.github.com/en/actions) 