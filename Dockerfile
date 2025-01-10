# Use the official n8n image as base
ARG N8N_VERSION=latest
FROM n8nio/n8n:${N8N_VERSION}

# Install additional tools and security updates
USER root
RUN apt-get update && \
    apt-get install -y \
    curl \
    ca-certificates \
    jq \
    fail2ban \
    && apt-get upgrade -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create necessary directories
RUN mkdir -p /opt/n8n/scripts /opt/n8n/backups /opt/n8n/logs \
    && chown -R node:node /opt/n8n

# Copy scripts
COPY scripts/monitor.sh /usr/local/bin/monitor.sh
COPY scripts/backup.sh /usr/local/bin/backup.sh
RUN chmod +x /usr/local/bin/monitor.sh /usr/local/bin/backup.sh

# Set up health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:5678/healthz || exit 1

# Switch back to non-root user
USER node

# Set environment variables
ENV NODE_ENV=production \
    N8N_METRICS=true \
    N8N_DIAGNOSTICS_ENABLED=false \
    N8N_HIRING_BANNER_ENABLED=false \
    N8N_VERSION_NOTIFICATIONS_ENABLED=true \
    N8N_PORT=5678 \
    N8N_PROTOCOL=https \
    N8N_USER_FOLDER=/home/node/.n8n

# Expose port
EXPOSE 5678

# Set working directory
WORKDIR /home/node

# Use custom entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
