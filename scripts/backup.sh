#!/bin/bash
set -e

# Configuration
CONTAINER_NAME="n8n-container"
BACKUP_DIR="/opt/n8n/backups"
LOG_FILE="/opt/n8n/logs/backup.log"
RETENTION_DAYS=7
SLACK_WEBHOOK_URL="${SLACK_WEBHOOK_URL:-}"
ALERT_EMAIL="${ALERT_EMAIL:-}"

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# Send notification
send_notification() {
    local message="$1"
    local subject="N8N Backup: $message"
    
    log "Notification: $message"

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

# Create backup
create_backup() {
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="${BACKUP_DIR}/n8n_backup_${timestamp}.tar.gz"
    
    log "Starting backup..."
    
    # Create temporary directory for backup
    local temp_dir
    temp_dir=$(mktemp -d)
    
    # Copy n8n data to temporary directory
    docker cp "${CONTAINER_NAME}:/home/node/.n8n" "${temp_dir}/"
    
    # Create tar archive
    tar -czf "$backup_file" -C "${temp_dir}" .n8n
    
    # Cleanup temporary directory
    rm -rf "${temp_dir}"
    
    # Verify backup file exists and has size > 0
    if [ -s "$backup_file" ]; then
        log "Backup created successfully: $backup_file"
        send_notification "Backup created successfully"
        return 0
    else
        log "Error: Backup file is empty or does not exist"
        send_notification "Backup failed: Empty or missing backup file"
        return 1
    fi
}

# Cleanup old backups
cleanup_old_backups() {
    log "Cleaning up old backups..."
    
    find "${BACKUP_DIR}" -name "n8n_backup_*.tar.gz" -type f -mtime "+${RETENTION_DAYS}" -delete
    
    # Count remaining backups
    local backup_count
    backup_count=$(find "${BACKUP_DIR}" -name "n8n_backup_*.tar.gz" -type f | wc -l)
    log "Remaining backups: $backup_count"
}

# Check available disk space
check_disk_space() {
    local available_space
    available_space=$(df -P "${BACKUP_DIR}" | awk 'NR==2 {print $4}')
    
    # Require at least 1GB free space
    if [ "$available_space" -lt 1048576 ]; then
        log "Error: Insufficient disk space (less than 1GB available)"
        send_notification "Backup failed: Insufficient disk space"
        return 1
    fi
    return 0
}

# Main backup process
main() {
    # Create backup directory if it doesn't exist
    mkdir -p "${BACKUP_DIR}"
    
    # Check disk space
    if ! check_disk_space; then
        return 1
    fi
    
    # Create backup
    if ! create_backup; then
        return 1
    fi
    
    # Cleanup old backups
    cleanup_old_backups
    
    log "Backup process completed successfully"
    return 0
}

main 