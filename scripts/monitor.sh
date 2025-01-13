#!/bin/bash
set -e

# Configuration
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"
ALERT_EMAIL="${ALERT_EMAIL:-}"
THRESHOLD_CPU=80
THRESHOLD_MEMORY=80
THRESHOLD_DISK=80
CHECK_INTERVAL=300  # 5 minutes

# Notification function
send_alert() {
    local message="$1"
    local severity="$2"  # error, warning, info
    local timestamp=$(date '+%Y-%m-%d %H:%M:%S')
    local formatted_message="[$severity] $timestamp - $message"

    echo "$formatted_message"

    # Slack notification
    if [ -n "$SLACK_WEBHOOK_URL" ]; then
        local color
        case "$severity" in
            "error") color="#ff0000" ;;
            "warning") color="#ffa500" ;;
            "info") color="#00ff00" ;;
        esac

        curl -s -X POST -H 'Content-type: application/json' \
            --data "{
                \"attachments\": [{
                    \"color\": \"$color\",
                    \"text\": \"$formatted_message\",
                    \"title\": \"N8N Monitor Alert\"
                }]
            }" "$SLACK_WEBHOOK_URL"
    fi

    # Email notification
    if [ -n "$ALERT_EMAIL" ]; then
        echo "$formatted_message" | mail -s "N8N Monitor Alert: [$severity]" "$ALERT_EMAIL"
    fi
}

# Health check function
check_health() {
    # Check n8n container status
    if ! docker ps | grep -q n8n-container; then
        send_alert "n8n container is not running" "error"
        return 1
    fi

    # Check n8n health endpoint
    if ! curl -sf "http://localhost:5678/healthz" > /dev/null; then
        send_alert "n8n health check failed" "error"
        return 1
    fi

    # Check PostgreSQL container status
    if ! docker ps | grep -q n8n-db; then
        send_alert "PostgreSQL container is not running" "error"
        return 1
    fi

    # Check PostgreSQL health
    if ! docker exec n8n-db pg_isready -U n8n > /dev/null 2>&1; then
        send_alert "PostgreSQL health check failed" "error"
        return 1
    fi

    # Check Caddy container status
    if ! docker ps | grep -q caddy; then
        send_alert "Caddy container is not running" "error"
        return 1
    fi

    return 0
}

# Resource check function
check_resources() {
    # CPU Usage
    local cpu_usage=$(docker stats --no-stream --format "{{.CPUPerc}}" n8n-container | sed 's/%//')
    if (( $(echo "$cpu_usage > $THRESHOLD_CPU" | bc -l) )); then
        send_alert "High CPU usage: ${cpu_usage}%" "warning"
    fi

    # Memory Usage
    local memory_usage=$(docker stats --no-stream --format "{{.MemPerc}}" n8n-container | sed 's/%//')
    if (( $(echo "$memory_usage > $THRESHOLD_MEMORY" | bc -l) )); then
        send_alert "High memory usage: ${memory_usage}%" "warning"
    fi

    # Disk Usage
    local disk_usage=$(df -h /opt/n8n | awk 'NR==2 {print $5}' | sed 's/%//')
    if [ "$disk_usage" -gt "$THRESHOLD_DISK" ]; then
        send_alert "High disk usage: ${disk_usage}%" "warning"
    fi
}

# Certificate check function
check_certificates() {
    local domain="${N8N_DOMAIN}"
    local cert_file="/data/certs/cert.pem"
    
    # Check if certificate exists
    if [ ! -f "$cert_file" ]; then
        send_alert "SSL certificate not found" "error"
        return 1
    fi

    # Check certificate expiration
    local expiry_date=$(openssl x509 -enddate -noout -in "$cert_file" | cut -d= -f2)
    local expiry_epoch=$(date -d "$expiry_date" +%s)
    local current_epoch=$(date +%s)
    local days_until_expiry=$(( ($expiry_epoch - $current_epoch) / 86400 ))

    if [ "$days_until_expiry" -lt 7 ]; then
        send_alert "SSL certificate will expire in $days_until_expiry days" "warning"
    fi
}

# Backup check function
check_backups() {
    local backup_dir="/opt/n8n/backups"
    local latest_backup=$(ls -t "$backup_dir"/n8n_full_backup_*.tar.gz 2>/dev/null | head -n1)
    
    if [ -z "$latest_backup" ]; then
        send_alert "No backup files found" "warning"
        return 1
    fi

    # Check backup age
    local backup_age=$(( ($(date +%s) - $(date -r "$latest_backup" +%s)) / 3600 ))
    if [ "$backup_age" -gt 24 ]; then
        send_alert "Latest backup is $backup_age hours old" "warning"
    fi

    # Check backup size
    local backup_size=$(du -m "$latest_backup" | cut -f1)
    if [ "$backup_size" -lt 1 ]; then
        send_alert "Backup file is suspiciously small ($backup_size MB)" "warning"
    fi
}

# Main monitoring loop
main() {
    send_alert "N8N monitoring started" "info"

    while true; do
        echo "Running health checks..."
        check_health
        check_resources
        check_certificates
        check_backups

        sleep "$CHECK_INTERVAL"
    done
}

# Start monitoring
main 