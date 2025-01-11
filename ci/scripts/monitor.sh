#!/bin/bash
set -e

# Configuration
CONTAINER_NAME="n8n-n8n-1"
SLACK_WEBHOOK_URL="${WEBHOOK_URL}"
ALERT_EMAIL="${ALERT_EMAIL}"
LOG_FILE="/var/log/n8n-monitor.log"
MEMORY_THRESHOLD=90  # percentage
CPU_THRESHOLD=80     # percentage

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "${LOG_FILE}"
}

# Send notification
send_notification() {
    local message="$1"
    log "Alert: $message"
    
    # Send Slack notification if webhook is configured
    if [ -n "${SLACK_WEBHOOK_URL}" ]; then
        send_slack_notification "$message"
    fi
    
    # Send email if configured
    if [ -n "${ALERT_EMAIL}" ]; then
        send_email_notification "$message"
    fi
}

# Send Slack notification
send_slack_notification() {
    local message="$1"
    curl -s -X POST -H 'Content-type: application/json' \
        --data "{\"text\":\"$message\"}" \
        "${SLACK_WEBHOOK_URL}"
}

# Send email notification
send_email_notification() {
    local message="$1"
    echo "$message" | mail -s "N8N Alert" "${ALERT_EMAIL}"
}

# Check container status
check_container_status() {
    if ! docker ps -f "name=${CONTAINER_NAME}" --format '{{.Status}}' | grep -q "Up"; then
        send_notification "Container ${CONTAINER_NAME} is not running!"
        return 1
    fi
    return 0
}

# Check container health
check_container_health() {
    if ! docker ps -f "name=${CONTAINER_NAME}" --format '{{.Status}}' | grep -q "healthy"; then
        send_notification "Container ${CONTAINER_NAME} is not healthy!"
        return 1
    fi
    return 0
}

# Check memory usage
check_memory_usage() {
    local memory_usage
    memory_usage=$(docker stats ${CONTAINER_NAME} --no-stream --format "{{.MemPerc}}" | cut -d'%' -f1)
    
    if [ "${memory_usage%.*}" -gt "${MEMORY_THRESHOLD}" ]; then
        send_notification "High memory usage: ${memory_usage}%"
        return 1
    fi
    return 0
}

# Check CPU usage
check_cpu_usage() {
    local cpu_usage
    cpu_usage=$(docker stats ${CONTAINER_NAME} --no-stream --format "{{.CPUPerc}}" | cut -d'%' -f1)
    
    if [ "${cpu_usage%.*}" -gt "${CPU_THRESHOLD}" ]; then
        send_notification "High CPU usage: ${cpu_usage}%"
        return 1
    fi
    return 0
}

# Check disk space
check_disk_space() {
    local disk_usage
    disk_usage=$(df -h / | awk 'NR==2 {print $5}' | cut -d'%' -f1)
    
    if [ "${disk_usage}" -gt 90 ]; then
        send_notification "Low disk space: ${disk_usage}%"
        return 1
    fi
    return 0
}

# Main monitoring loop
main() {
    log "Starting N8N monitoring..."
    
    # Create log file if it doesn't exist
    touch "${LOG_FILE}"
    
    # Run checks
    check_container_status
    check_container_health
    check_memory_usage
    check_cpu_usage
    check_disk_space
    
    log "Monitoring checks completed"
}

# Run main function
main 