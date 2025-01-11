#!/bin/bash

# Health check URL
HEALTH_URL="http://localhost:5678/healthz"
WEBHOOK_URL="${WEBHOOK_URL}"
ALERT_EMAIL="${ALERT_EMAIL}"

# Function to send Slack notification
send_slack_notification() {
    if [ -n "$WEBHOOK_URL" ]; then
        curl -X POST -H 'Content-type: application/json' \
            --data "{\"text\":\"$1\"}" \
            "$WEBHOOK_URL"
    fi
}

# Function to send email notification
send_email_notification() {
    if [ -n "$ALERT_EMAIL" ]; then
        echo "$1" | mail -s "N8N Alert" "$ALERT_EMAIL"
    fi
}

# Check n8n health
response=$(curl -s -o /dev/null -w "%{http_code}" "$HEALTH_URL")

if [ "$response" != "200" ]; then
    message="⚠️ N8N health check failed! HTTP Status: $response"
    send_slack_notification "$message"
    send_email_notification "$message"
    
    # Try to restart n8n
    docker-compose restart n8n
else
    # Check resource usage
    cpu_usage=$(docker stats --no-stream --format "{{.CPUPerc}}" n8n)
    memory_usage=$(docker stats --no-stream --format "{{.MemUsage}}" n8n)
    
    if [[ $cpu_usage > "80%" ]] || [[ $memory_usage =~ ^[0-9]+(\.[0-9]+)?GB$ && ${BASH_REMATCH[1]} > 1.8 ]]; then
        message="⚠️ High resource usage detected!\nCPU: $cpu_usage\nMemory: $memory_usage"
        send_slack_notification "$message"
        send_email_notification "$message"
    fi
fi 