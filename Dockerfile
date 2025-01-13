# Build stage
FROM n8nio/n8n:${N8N_VERSION} as builder

# Install build dependencies
USER root
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    jq \
    && rm -rf /var/lib/apt/lists/*

# Create necessary directories
RUN mkdir -p /opt/n8n/scripts /opt/n8n/backups /opt/n8n/logs \
    && chown -R node:node /opt/n8n

# Copy scripts
COPY scripts/monitor.sh /usr/local/bin/monitor.sh
COPY scripts/backup.sh /usr/local/bin/backup.sh
RUN chmod +x /usr/local/bin/monitor.sh /usr/local/bin/backup.sh

# Final stage
FROM n8nio/n8n:${N8N_VERSION}

# Copy files from builder
COPY --from=builder /usr/local/bin/monitor.sh /usr/local/bin/monitor.sh
COPY --from=builder /usr/local/bin/backup.sh /usr/local/bin/backup.sh
COPY --from=builder --chown=node:node /opt/n8n /opt/n8n

# Install runtime dependencies and security updates
USER root
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    curl \
    ca-certificates \
    jq \
    fail2ban \
    && apt-get upgrade -y \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /home/node/.n8n \
    && chown -R node:node /home/node/.n8n

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
    N8N_USER_FOLDER=/home/node/.n8n \
    N8N_ENFORCE_SETTINGS_FILE_PERMISSIONS=true

# Expose port
EXPOSE 5678

# Set working directory
WORKDIR /home/node

# Copy and set up entrypoint script
COPY --chown=node:node docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Add security labels
LABEL org.opencontainers.image.vendor="Your Organization" \
      org.opencontainers.image.title="N8N Workflow Automation" \
      org.opencontainers.image.description="Secure N8N instance with monitoring and backup capabilities" \
      org.opencontainers.image.source="https://github.com/yourusername/n8n-digitalocean-cicd" \
      org.opencontainers.image.licenses="MIT"

# Set default security options
SECURITY_OPT no-new-privileges:true

ENTRYPOINT ["/docker-entrypoint.sh"]
