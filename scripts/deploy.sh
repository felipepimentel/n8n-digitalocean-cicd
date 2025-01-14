#!/bin/bash

# Configuration
WORKFLOW_FILE="deploy.yml"
WORKFLOW_NAME="Deploy n8n (GitHub Actions)"
REF="main"
MAX_RETRIES=3
RETRY_INTERVAL=10

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Function to display messages with timestamp
log() {
    local level=$1
    local message=$2
    local color=$NC
    
    case $level in
        "INFO") color=$BLUE;;
        "SUCCESS") color=$GREEN;;
        "WARNING") color=$YELLOW;;
        "ERROR") color=$RED;;
    esac
    
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] ${color}${level}${NC}: $message"
}

# Function to check dependencies
check_dependencies() {
    log "INFO" "Checking dependencies..."
    
    # Check for GitHub CLI
    if ! command -v gh &> /dev/null; then
        log "ERROR" "GitHub CLI (gh) not found. Please install: https://cli.github.com/"
        return 1
    fi
    
    # Check GitHub CLI authentication with retry
    local retry_count=0
    while [ $retry_count -lt $MAX_RETRIES ]; do
        if gh auth status &> /dev/null; then
            log "SUCCESS" "GitHub CLI is authenticated"
            return 0
        fi
        
        retry_count=$((retry_count + 1))
        if [ $retry_count -lt $MAX_RETRIES ]; then
            log "WARNING" "Authentication check failed. Retrying in $RETRY_INTERVAL seconds... (Attempt $retry_count/$MAX_RETRIES)"
            sleep $RETRY_INTERVAL
        fi
    done
    
    log "ERROR" "GitHub CLI is not authenticated. Please run 'gh auth login' first"
    return 1
}

# Function to trigger workflow
trigger_workflow() {
    log "INFO" "Triggering deployment workflow..."
    
    local workflow_output
    workflow_output=$(gh workflow run "$WORKFLOW_NAME" --ref "$REF" 2>&1)
    if [ $? -ne 0 ]; then
        log "ERROR" "Failed to trigger workflow:"
        log "ERROR" "$workflow_output"
        return 1
    fi
    
    log "SUCCESS" "Workflow triggered successfully"
    return 0
}

# Function to get run status
get_run_status() {
    local run_id=$1
    gh run view "$run_id" --json status,conclusion --jq '[.status, .conclusion] | @tsv'
}

# Function to monitor workflow
monitor_workflow() {
    log "INFO" "Monitoring workflow execution..."
    
    local start_time=$(date +%s)
    local timeout=1800  # 30 minutes timeout
    
    while true; do
        # Check timeout
        local current_time=$(date +%s)
        local elapsed_time=$((current_time - start_time))
        if [ $elapsed_time -gt $timeout ]; then
            log "ERROR" "Deployment timed out after 30 minutes"
            return 1
        fi
        
        # Get latest run ID
        local run_id
        run_id=$(gh run list --workflow="$WORKFLOW_FILE" --json databaseId --jq '.[0].databaseId')
        if [ -z "$run_id" ]; then
            log "ERROR" "Failed to get run ID"
            return 1
        fi
        
        # Get run status
        local status_output
        status_output=$(get_run_status "$run_id")
        local status=$(echo "$status_output" | cut -f1)
        local conclusion=$(echo "$status_output" | cut -f2)
        
        case "$status" in
            "completed")
                if [ "$conclusion" = "success" ]; then
                    log "SUCCESS" "Deployment completed successfully!"
                    return 0
                else
                    log "ERROR" "Deployment failed. Retrieving logs..."
                    local log_file="deploy-${run_id}.log"
                    gh run view "$run_id" --log > "$log_file"
                    log "INFO" "Logs saved to: $log_file"
                    return 1
                fi
                ;;
            "in_progress")
                log "INFO" "Deployment in progress..."
                ;;
            "queued")
                log "INFO" "Deployment queued..."
                ;;
            *)
                log "WARNING" "Unknown status: $status"
                ;;
        esac
        
        sleep $RETRY_INTERVAL
    done
}

# Main function
main() {
    log "INFO" "Starting n8n deployment process..."
    
    # Check dependencies
    if ! check_dependencies; then
        log "ERROR" "Dependency check failed"
        exit 1
    fi
    
    # Trigger workflow
    if ! trigger_workflow; then
        log "ERROR" "Failed to trigger workflow"
        exit 1
    fi
    
    # Monitor workflow
    if ! monitor_workflow; then
        log "ERROR" "Deployment failed"
        exit 1
    fi
    
    log "SUCCESS" "Deployment process completed successfully!"
    exit 0
}

# Execute main function
main 