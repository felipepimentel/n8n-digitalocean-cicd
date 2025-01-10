---
layout: default
title: Monitoring & Maintenance
description: Comprehensive guide for monitoring and maintaining your n8n deployment
---

# Monitoring & Maintenance Guide

This guide covers all aspects of monitoring and maintaining your n8n deployment on DigitalOcean, including health checks, backups, and routine maintenance tasks.

## Table of Contents

- [Monitoring Overview](#monitoring-overview)
- [Health Checks](#health-checks)
- [Resource Monitoring](#resource-monitoring)
- [Backup System](#backup-system)
- [Alerting System](#alerting-system)
- [Maintenance Tasks](#maintenance-tasks)
- [Troubleshooting](#troubleshooting)

## Monitoring Overview

Our monitoring system consists of several components:

1. **Container Health Checks**: Docker-level monitoring
2. **Application Monitoring**: n8n internal metrics
3. **System Monitoring**: Host-level metrics
4. **External Monitoring**: Endpoint availability
5. **Backup Monitoring**: Backup job status

## Health Checks

### 1. Container Health Check

The n8n container includes built-in health checks:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:5678/healthz"]
  interval: 1m
  timeout: 10s
  retries: 3
  start_period: 30s
```

### 2. Application Health Check

The monitoring script (`/usr/local/bin/monitor.sh`):

```bash
#!/bin/bash
set -e

# Configuration
N8N_URL="https://${N8N_DOMAIN}"
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"
ALERT_EMAIL="${ALERT_EMAIL:-}"

# Health check function
check_health() {
    local endpoint="$1"
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" "$endpoint/healthz")
    
    if [ "$response" != "200" ]; then
        send_alert "Health check failed for $endpoint (Status: $response)"
        return 1
    fi
    return 0
}

# Resource check function
check_resources() {
    # CPU Usage
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d. -f1)
    if [ "$cpu_usage" -gt 80 ]; then
        send_alert "High CPU usage: ${cpu_usage}%"
    fi

    # Memory Usage
    memory_usage=$(free | grep Mem | awk '{print $3/$2 * 100.0}' | cut -d. -f1)
    if [ "$memory_usage" -gt 80 ]; then
        send_alert "High memory usage: ${memory_usage}%"
    fi

    # Disk Usage
    disk_usage=$(df -h / | awk 'NR==2 {print $5}' | cut -d% -f1)
    if [ "$disk_usage" -gt 80 ]; then
        send_alert "High disk usage: ${disk_usage}%"
    fi
}

# Send alert function
send_alert() {
    local message="$1"
    
    # Slack notification
    if [ -n "$SLACK_WEBHOOK_URL" ]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"$message\"}" \
            "$SLACK_WEBHOOK_URL"
    fi
    
    # Email notification
    if [ -n "$ALERT_EMAIL" ]; then
        echo "$message" | mail -s "n8n Alert" "$ALERT_EMAIL"
    fi
    
    # Log the alert
    logger -t n8n-monitor "$message"
}

# Main monitoring loop
main() {
    check_health "$N8N_URL"
    check_resources
}

main
```

## Resource Monitoring

### 1. System Resources

Monitor system resources using DigitalOcean monitoring:

```bash
# CPU Monitoring
curl -s "http://169.254.169.254/metadata/v1/monitoring/cpu"

# Memory Monitoring
curl -s "http://169.254.169.254/metadata/v1/monitoring/memory"

# Disk Monitoring
curl -s "http://169.254.169.254/metadata/v1/monitoring/disk"
```

### 2. Container Resources

Monitor Docker container resources:

```bash
# Container Stats
docker stats n8n-container --no-stream --format \
    "CPU: {{.CPUPerc}}, Memory: {{.MemPerc}}, Network: {{.NetIO}}"

# Container Logs
docker logs n8n-container --since 1h
```

## Backup System

### 1. Automated Backups

The backup script (`/usr/local/bin/backup.sh`):

```bash
#!/bin/bash
set -e

# Configuration
BACKUP_DIR="/opt/n8n/backups"
RETENTION_DAYS=7
MAX_BACKUPS=10

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Generate backup filename with timestamp
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/n8n-backup-$TIMESTAMP.tar.gz"

# Create backup
echo "Creating backup: $BACKUP_FILE"
docker run --rm \
    --volumes-from n8n-container \
    -v "$BACKUP_DIR:/backup" \
    alpine tar czf "/backup/$(basename "$BACKUP_FILE")" /home/node/.n8n

# Encrypt backup
echo "Encrypting backup"
openssl enc -aes-256-cbc -salt -pbkdf2 \
    -in "$BACKUP_FILE" \
    -out "$BACKUP_FILE.enc" \
    -pass file:/opt/n8n/backup.key

# Remove unencrypted backup
rm "$BACKUP_FILE"

# Cleanup old backups
find "$BACKUP_DIR" -name "n8n-backup-*.tar.gz.enc" -mtime +$RETENTION_DAYS -delete

# Keep only MAX_BACKUPS most recent backups
ls -t "$BACKUP_DIR"/n8n-backup-*.tar.gz.enc | \
    tail -n +$((MAX_BACKUPS + 1)) | \
    xargs -r rm

# Verify backup
if [ -f "$BACKUP_FILE.enc" ]; then
    echo "Backup completed successfully"
    logger -t n8n-backup "Backup created: $BACKUP_FILE.enc"
else
    echo "Backup failed"
    logger -t n8n-backup "Backup failed"
    exit 1
fi
```

### 2. Backup Verification

Verify backup integrity:

```bash
# Test backup decryption
openssl enc -aes-256-cbc -d -salt -pbkdf2 \
    -in latest-backup.tar.gz.enc \
    -pass file:/opt/n8n/backup.key \
    -out /dev/null
```

## Alerting System

### 1. Slack Alerts

Configure Slack notifications:

```yaml
notifications:
  slack:
    webhook_url: ${SLACK_WEBHOOK_URL}
    channel: "#n8n-alerts"
    username: "N8N Monitor"
    icon_emoji: ":robot_face:"
    templates:
      alert: |
        *Alert:* {{ .title }}
        *Status:* {{ .status }}
        *Details:* {{ .message }}
        *Time:* {{ .timestamp }}
```

### 2. Email Alerts

Configure email notifications:

```bash
# Install mailutils
apt-get install -y mailutils

# Configure email settings
cat > /etc/ssmtp/ssmtp.conf << EOF
root=your-email@example.com
mailhub=smtp.example.com:587
AuthUser=your-username
AuthPass=your-password
UseSTARTTLS=YES
EOF
```

## Maintenance Tasks

### 1. Daily Tasks

```bash
# Check logs for errors
grep -i error /var/log/n8n/error.log

# Monitor disk space
df -h

# Check running processes
docker ps -a
```

### 2. Weekly Tasks

```bash
# Update system packages
apt-get update && apt-get upgrade -y

# Rotate logs
logrotate -f /etc/logrotate.d/n8n

# Check SSL certificate expiry
openssl x509 -enddate -noout -in /etc/ssl/certs/n8n.crt
```

### 3. Monthly Tasks

```bash
# Full system update
apt-get dist-upgrade -y

# Clean up old Docker images
docker system prune -af

# Test backup restoration
./scripts/test-backup-restore.sh
```

## Troubleshooting

### 1. Common Issues

#### Container Won't Start

```bash
# Check container logs
docker logs n8n-container

# Check container status
docker inspect n8n-container

# Verify resource availability
docker stats --no-stream
```

#### High Resource Usage

```bash
# Identify resource-intensive processes
top -b -n 1

# Check Docker stats
docker stats --no-stream

# Monitor system load
uptime
```

#### Backup Failures

```bash
# Check backup logs
tail -f /var/log/n8n/backup.log

# Verify backup directory permissions
ls -la /opt/n8n/backups

# Test backup script manually
/usr/local/bin/backup.sh -v
```

### 2. Recovery Procedures

#### Restore from Backup

```bash
# Stop n8n container
docker stop n8n-container

# Restore data
docker run --rm \
    -v n8n_data:/recovery \
    -v /opt/n8n/backups:/backups \
    alpine sh -c "cd /recovery && \
        tar xzf /backups/n8n-backup-latest.tar.gz"

# Start n8n container
docker start n8n-container
```

## Additional Resources

- [n8n Monitoring Guide](https://docs.n8n.io/hosting/monitoring/)
- [Docker Monitoring](https://docs.docker.com/config/containers/runmetrics/)
- [DigitalOcean Monitoring](https://www.digitalocean.com/docs/monitoring/)
- [Linux System Monitoring](https://www.cyberciti.biz/tips/top-linux-monitoring-tools.html) 