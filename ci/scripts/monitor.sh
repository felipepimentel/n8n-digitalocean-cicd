#!/bin/bash
set -e

# Configuration
CONTAINER_NAME="n8n-1"
SLACK_WEBHOOK_URL="${WEBHOOK_URL}"
ALERT_EMAIL="${ALERT_EMAIL}"
LOG_FILE="/var/log/n8n-monitor.log"
MEMORY_THRESHOLD=90
CPU_THRESHOLD=80

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "${LOG_FILE}"
}

# Send notification
send_notification() {
    local status="$1"
    local message="$2"
    
    # Send to Slack if webhook is configured
    if [ -n "${SLACK_WEBHOOK_URL}" ]; then
        send_slack_notification "${status}" "${message}"
    fi
    
    # Send email if configured
    if [ -n "${ALERT_EMAIL}" ]; then
        send_email_notification "${status}" "${message}"
    fi
}

# Send Slack notification
send_slack_notification() {
    local status="$1"
    local message="$2"
    local color
    
    if [ "${status}" = "error" ]; then
        color="danger"
    else
        color="good"
    fi
    
    curl -s -X POST -H 'Content-type: application/json' \
        --data "{\"attachments\":[{\"color\":\"${color}\",\"text\":\"${message}\"}]}" \
        "${SLACK_WEBHOOK_URL}"
}

# Send email notification
send_email_notification() {
    local status="$1"
    local message="$2"
    local subject="N8N Monitor Alert: ${status}"
    
    echo "${message}" | mail -s "${subject}" "${ALERT_EMAIL}"
}

# Check container status
check_container_status() {
    if ! docker ps -f "name=${CONTAINER_NAME}" --format '{{.Status}}' | grep -q "Up"; then
        log "ERROR: Container ${CONTAINER_NAME} is not running"
        send_notification "error" "Container ${CONTAINER_NAME} is not running"
        return 1
    fi
    return 0
}

# Check container health
check_container_health() {
    if ! docker inspect --format='{{.State.Health.Status}}' "${CONTAINER_NAME}" | grep -q "healthy"; then
        log "ERROR: Container ${CONTAINER_NAME} is not healthy"
        send_notification "error" "Container ${CONTAINER_NAME} is not healthy"
        return 1
    fi
    return 0
}

# Check memory usage
check_memory_usage() {
    local memory_usage
    memory_usage=$(docker stats "${CONTAINER_NAME}" --no-stream --format "{{.MemPerc}}" | cut -d'%' -f1)
    
    if [ "${memory_usage%.*}" -gt "${MEMORY_THRESHOLD}" ]; then
        log "ERROR: High memory usage: ${memory_usage}%"
        send_notification "error" "High memory usage: ${memory_usage}%"
        return 1
    fi
    return 0
}

# Check CPU usage
check_cpu_usage() {
    local cpu_usage
    cpu_usage=$(docker stats "${CONTAINER_NAME}" --no-stream --format "{{.CPUPerc}}" | cut -d'%' -f1)
    
    if [ "${cpu_usage%.*}" -gt "${CPU_THRESHOLD}" ]; then
        log "ERROR: High CPU usage: ${cpu_usage}%"
        send_notification "error" "High CPU usage: ${cpu_usage}%"
        return 1
    fi
    return 0
}

# Check disk space
check_disk_space() {
    local disk_usage
    disk_usage=$(df -h / | awk 'NR==2 {print $5}' | cut -d'%' -f1)
    
    if [ "${disk_usage}" -gt 90 ]; then
        log "ERROR: High disk usage: ${disk_usage}%"
        send_notification "error" "High disk usage: ${disk_usage}%"
        return 1
    fi
    return 0
}

# Main monitoring loop
log "Starting n8n monitoring"

# Run checks
check_container_status
check_container_health
check_memory_usage
check_cpu_usage
check_disk_space

log "Monitoring checks completed" 