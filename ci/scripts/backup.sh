#!/bin/bash
set -e

# Configuration
CONTAINER_NAME="n8n-db-1"
BACKUP_DIR="/opt/n8n/backups"
LOG_FILE="/var/log/n8n-backup.log"
WEBHOOK_URL="${WEBHOOK_URL}"
ALERT_EMAIL="${ALERT_EMAIL}"
RETENTION_DAYS=7

# Logging function
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "${LOG_FILE}"
}

# Send notification
send_notification() {
    local status="$1"
    local message="$2"
    
    log "${message}"
    
    # Send to Slack if webhook is configured
    if [ -n "${WEBHOOK_URL}" ]; then
        curl -s -X POST -H 'Content-type: application/json' \
            --data "{\"attachments\":[{\"color\":\"${status}\",\"text\":\"${message}\"}]}" \
            "${WEBHOOK_URL}"
    fi
    
    # Send email if configured
    if [ -n "${ALERT_EMAIL}" ]; then
        echo "${message}" | mail -s "N8N Backup ${status}" "${ALERT_EMAIL}"
    fi
}

# Check disk space
check_disk_space() {
    local disk_usage
    disk_usage=$(df -h "${BACKUP_DIR}" | awk 'NR==2 {print $5}' | cut -d'%' -f1)
    
    if [ "${disk_usage}" -gt 90 ]; then
        send_notification "danger" "ERROR: Insufficient disk space for backup (${disk_usage}% used)"
        exit 1
    fi
}

# Cleanup old backups
cleanup_old_backups() {
    log "Cleaning up backups older than ${RETENTION_DAYS} days"
    find "${BACKUP_DIR}" -name "n8n-*.sql.gz" -type f -mtime +${RETENTION_DAYS} -delete
}

# Create backup
create_backup() {
    local timestamp
    timestamp=$(date +%Y%m%d_%H%M%S)
    local backup_file="${BACKUP_DIR}/n8n-${timestamp}.sql"
    
    log "Creating backup: ${backup_file}"
    
    # Create database backup
    if docker exec "${CONTAINER_NAME}" pg_dump -U n8n n8n > "${backup_file}"; then
        # Compress backup
        gzip "${backup_file}"
        send_notification "good" "Backup completed successfully: ${backup_file}.gz"
    else
        send_notification "danger" "ERROR: Backup failed"
        exit 1
    fi
}

# Main backup process
main() {
    log "Starting backup process"
    
    # Create backup directory if it doesn't exist
    mkdir -p "${BACKUP_DIR}"
    
    # Check disk space
    check_disk_space
    
    # Create backup
    create_backup
    
    # Cleanup old backups
    cleanup_old_backups
    
    log "Backup process completed"
}

# Run main function
main 