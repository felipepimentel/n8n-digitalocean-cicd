#!/bin/bash

# Configuration
BACKUP_DIR="/opt/n8n/backups"
WEBHOOK_URL="${WEBHOOK_URL}"
ALERT_EMAIL="${ALERT_EMAIL}"
RETENTION_DAYS=7

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Function to send notifications
send_notification() {
    local message="$1"
    
    # Send Slack notification
    if [ -n "$WEBHOOK_URL" ]; then
        curl -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"$message\"}" \
            "$WEBHOOK_URL"
    fi
    
    # Send email notification
    if [ -n "$ALERT_EMAIL" ]; then
        echo "$message" | mail -s "N8N Backup Alert" "$ALERT_EMAIL"
    fi
}

# Backup timestamp
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Backup database
echo "Starting database backup..."
if docker-compose exec -T db pg_dump -U n8n n8n > "$BACKUP_DIR/n8n_db_$TIMESTAMP.sql"; then
    echo "Database backup completed successfully"
else
    send_notification "⚠️ Database backup failed!"
    exit 1
fi

# Backup n8n files
echo "Starting files backup..."
if tar -czf "$BACKUP_DIR/n8n_files_$TIMESTAMP.tar.gz" -C /opt/n8n local_files; then
    echo "Files backup completed successfully"
else
    send_notification "⚠️ Files backup failed!"
    exit 1
fi

# Clean up old backups
find "$BACKUP_DIR" -type f -mtime +$RETENTION_DAYS -delete

# Send success notification
send_notification "✅ Backup completed successfully!\nDatabase: n8n_db_$TIMESTAMP.sql\nFiles: n8n_files_$TIMESTAMP.tar.gz" 