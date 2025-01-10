#!/bin/sh
set -e

# Create data directory if it doesn't exist
mkdir -p /home/node/.n8n

# Set correct permissions
chmod 750 /home/node/.n8n

# Start n8n
exec n8n start 