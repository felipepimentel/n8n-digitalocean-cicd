#!/bin/bash
set -e

# Configuration
CONTAINER_NAME="n8n-container"
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"
ALERT_EMAIL="${ALERT_EMAIL:-}"
LOG_FILE="/opt/n8n/logs/monitor.log"
MEMORY_THRESHOLD=90  # percentage
CPU_THRESHOLD=80     # percentage

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# Send notification
send_notification() {
    local message="$1"
    local subject="N8N Alert: $message"
    
    log "Alert: $message"

    # Send Slack notification if webhook is configured
    if [ -n "$SLACK_WEBHOOK_URL" ]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"$message\"}" \
            "$SLACK_WEBHOOK_URL"
    fi

    # Send email if configured
    if [ -n "$ALERT_EMAIL" ]; then
        echo "$message" | mail -s "$subject" "$ALERT_EMAIL"
    fi
}

# Check if container is running
check_container() {
    if ! docker ps -f "name=$CONTAINER_NAME" --format '{{.Status}}' | grep -q "Up"; then
        send_notification "Container $CONTAINER_NAME is not running!"
        return 1
    fi
    return 0
}

# Check container health
check_health() {
    if ! docker ps -f "name=$CONTAINER_NAME" --format '{{.Status}}' | grep -q "healthy"; then
        send_notification "Container $CONTAINER_NAME is unhealthy!"
        return 1
    fi
    return 0
}

# Check memory usage
check_memory() {
    local memory_usage
    memory_usage=$(docker stats "$CONTAINER_NAME" --no-stream --format "{{.MemPerc}}" | cut -d'.' -f1)
    
    if [ "$memory_usage" -gt "$MEMORY_THRESHOLD" ]; then
        send_notification "High memory usage: ${memory_usage}%"
        return 1
    fi
    return 0
}

# Check CPU usage
check_cpu() {
    local cpu_usage
    cpu_usage=$(docker stats "$CONTAINER_NAME" --no-stream --format "{{.CPUPerc}}" | cut -d'.' -f1)
    
    if [ "$cpu_usage" -gt "$CPU_THRESHOLD" ]; then
        send_notification "High CPU usage: ${cpu_usage}%"
        return 1
    fi
    return 0
}

# Check disk space
check_disk() {
    local disk_usage
    disk_usage=$(df -h /opt/n8n | awk 'NR==2 {print $5}' | cut -d'%' -f1)
    
    if [ "$disk_usage" -gt 85 ]; then
        send_notification "Low disk space: ${disk_usage}%"
        return 1
    fi
    return 0
}

# Main monitoring loop
main() {
    log "Starting monitoring check..."
    
    check_container || true
    check_health || true
    check_memory || true
    check_cpu || true
    check_disk || true
    
    log "Monitoring check completed"
}

main 