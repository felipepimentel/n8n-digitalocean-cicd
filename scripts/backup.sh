#!/bin/bash
set -e

# Configuration
BACKUP_DIR="/opt/n8n/backups"
DATE=$(date +%Y%m%d_%H%M%S)
RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-7}

# Create backup directory if it doesn't exist
mkdir -p "$BACKUP_DIR"

# Backup function with error handling
backup_with_retry() {
    local max_attempts=3
    local attempt=1
    local command=$1
    local description=$2

    while [ $attempt -le $max_attempts ]; do
        echo "🔄 Attempting $description (attempt $attempt/$max_attempts)..."
        if eval "$command"; then
            echo "✅ $description successful!"
            return 0
        else
            echo "⚠️ $description failed on attempt $attempt"
            attempt=$((attempt + 1))
            [ $attempt -le $max_attempts ] && sleep 5
        fi
    done

    echo "❌ $description failed after $max_attempts attempts"
    return 1
}

# Backup PostgreSQL database
echo "📦 Starting PostgreSQL backup..."
PGPASSWORD=${DB_PASSWORD} backup_with_retry \
    "docker exec n8n-db pg_dump -U n8n -d n8n > $BACKUP_DIR/n8n_db_$DATE.sql" \
    "PostgreSQL backup"

# Backup n8n data
echo "📦 Starting n8n data backup..."
backup_with_retry \
    "docker run --rm -v n8n_data:/data -v $BACKUP_DIR:/backup alpine tar czf /backup/n8n_data_$DATE.tar.gz -C /data ." \
    "n8n data backup"

# Create combined backup
echo "📦 Creating combined backup archive..."
backup_with_retry \
    "tar czf $BACKUP_DIR/n8n_full_backup_$DATE.tar.gz -C $BACKUP_DIR n8n_db_$DATE.sql n8n_data_$DATE.tar.gz" \
    "Combined backup archive creation"

# Cleanup individual backup files
rm "$BACKUP_DIR/n8n_db_$DATE.sql" "$BACKUP_DIR/n8n_data_$DATE.tar.gz"

# Cleanup old backups
echo "🧹 Cleaning up old backups..."
find "$BACKUP_DIR" -name "n8n_full_backup_*.tar.gz" -mtime +$RETENTION_DAYS -delete

# Verify backup
echo "🔍 Verifying backup..."
if [ -f "$BACKUP_DIR/n8n_full_backup_$DATE.tar.gz" ]; then
    backup_size=$(du -h "$BACKUP_DIR/n8n_full_backup_$DATE.tar.gz" | cut -f1)
    echo "✅ Backup completed successfully! Size: $backup_size"
    echo "📁 Backup location: $BACKUP_DIR/n8n_full_backup_$DATE.tar.gz"
else
    echo "❌ Backup verification failed!"
    exit 1
fi

# Optional: Upload to remote storage (if configured)
if [ -n "$BACKUP_S3_BUCKET" ]; then
    echo "☁️ Uploading backup to S3..."
    backup_with_retry \
        "aws s3 cp $BACKUP_DIR/n8n_full_backup_$DATE.tar.gz s3://$BACKUP_S3_BUCKET/n8n_full_backup_$DATE.tar.gz" \
        "S3 upload"
fi 