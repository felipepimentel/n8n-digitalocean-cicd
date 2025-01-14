#!/bin/bash

# Configurações
WORKFLOW_FILE="deploy.yml"
WORKFLOW_NAME="Deploy n8n (GitHub Actions)"
REF="main"

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Função para exibir mensagens com timestamp
log() {
    echo -e "[$(date +'%Y-%m-%d %H:%M:%S')] $1"
}

# Função para verificar dependências
check_dependencies() {
    log "${YELLOW}Verificando dependências...${NC}"
    
    if ! command -v gh &> /dev/null; then
        log "${RED}❌ GitHub CLI (gh) não encontrado. Por favor, instale: https://cli.github.com/${NC}"
        exit 1
    fi
    
    if ! gh auth status &> /dev/null; then
        log "${RED}❌ GitHub CLI não está autenticado. Execute 'gh auth login' primeiro.${NC}"
        exit 1
    fi
    
    log "${GREEN}✅ Todas as dependências verificadas.${NC}"
}

# Função para disparar o workflow
trigger_workflow() {
    log "${YELLOW}Disparando workflow de deploy...${NC}"
    
    WORKFLOW_RUN_OUTPUT=$(gh workflow run "$WORKFLOW_NAME" --ref "$REF" 2>&1)
    if [ $? -ne 0 ]; then
        log "${RED}❌ Erro ao disparar o workflow:${NC}"
        log "${RED}$WORKFLOW_RUN_OUTPUT${NC}"
        exit 1
    fi
    
    log "${GREEN}✅ Workflow disparado com sucesso.${NC}"
}

# Função para monitorar o workflow
monitor_workflow() {
    log "${YELLOW}Monitorando execução do workflow...${NC}"
    
    while true; do
        # Obter o ID da última execução
        RUN_ID=$(gh run list --workflow="$WORKFLOW_FILE" --json databaseId --jq '.[0].databaseId')
        
        if [ -z "$RUN_ID" ]; then
            log "${RED}❌ Não foi possível obter o ID da execução.${NC}"
            exit 1
        fi
        
        # Obter status da execução
        STATUS=$(gh run list --workflow="$WORKFLOW_FILE" --json status,conclusion --jq '.[0] | "\(.status) \(.conclusion)"')
        STATUS_ARRAY=($STATUS)
        CURRENT_STATUS=${STATUS_ARRAY[0]}
        CONCLUSION=${STATUS_ARRAY[1]}
        
        case "$CURRENT_STATUS" in
            "completed")
                if [ "$CONCLUSION" = "success" ]; then
                    log "${GREEN}✅ Deploy concluído com sucesso!${NC}"
                    return 0
                else
                    log "${RED}❌ Deploy falhou. Obtendo logs...${NC}"
                    gh run view "$RUN_ID" --log > "deploy-${RUN_ID}.log"
                    log "${YELLOW}Logs salvos em: deploy-${RUN_ID}.log${NC}"
                    return 1
                fi
                ;;
            "in_progress")
                log "🔄 Deploy em andamento..."
                ;;
            *)
                log "⏳ Aguardando início do deploy..."
                ;;
        esac
        
        sleep 10
    done
}

# Função principal
main() {
    log "${YELLOW}Iniciando processo de deploy do n8n...${NC}"
    
    check_dependencies
    trigger_workflow
    monitor_workflow
    
    EXIT_CODE=$?
    
    if [ $EXIT_CODE -eq 0 ]; then
        log "${GREEN}🎉 Processo de deploy concluído com sucesso!${NC}"
    else
        log "${RED}❌ Processo de deploy falhou. Verifique os logs para mais detalhes.${NC}"
    fi
    
    exit $EXIT_CODE
}

# Executa a função principal
main 