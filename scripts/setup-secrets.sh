#!/bin/bash

# Função para verificar se o GitHub CLI está instalado
check_gh_cli() {
    if ! command -v gh &> /dev/null; then
        echo "❌ GitHub CLI não está instalado"
        echo "Por favor, instale seguindo as instruções em: https://cli.github.com/"
        exit 1
    fi
}

# Função para verificar se está logado no GitHub CLI
check_gh_auth() {
    if ! gh auth status &> /dev/null; then
        echo "❌ Não está autenticado no GitHub CLI"
        echo "Execute 'gh auth login' primeiro"
        exit 1
    fi
}

# Função para gerar uma nova chave de criptografia
generate_encryption_key() {
    openssl rand -hex 16
}

# Função para verificar se o secret já existe
check_secret_exists() {
    local secret_name="N8N_ENCRYPTION_KEY"
    gh secret list | grep -q "^$secret_name"
    return $?
}

# Função principal
main() {
    echo "🔐 Configurando secrets do n8n..."
    
    # Verifica pré-requisitos
    check_gh_cli
    check_gh_auth
    
    # Verifica se o secret já existe
    if check_secret_exists; then
        echo "✅ Secret N8N_ENCRYPTION_KEY já existe"
        read -p "Deseja gerar uma nova chave? (s/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Ss]$ ]]; then
            echo "🛑 Mantendo a chave existente"
            exit 0
        fi
    fi
    
    # Gera nova chave
    echo "🔑 Gerando nova chave de criptografia..."
    new_key=$(generate_encryption_key)
    
    if [ -z "$new_key" ] || [ ${#new_key} -ne 32 ]; then
        echo "❌ Erro ao gerar chave de criptografia"
        echo "Comprimento da chave: ${#new_key} (esperado: 32)"
        exit 1
    fi
    
    # Salva a chave como secret
    echo "💾 Salvando chave como secret..."
    echo "$new_key" | gh secret set N8N_ENCRYPTION_KEY
    
    if [ $? -eq 0 ]; then
        echo "✅ Secret N8N_ENCRYPTION_KEY configurado com sucesso"
        echo "⚠️ IMPORTANTE: Guarde esta chave em um local seguro!"
        echo "Chave: $new_key"
    else
        echo "❌ Erro ao salvar o secret"
        exit 1
    fi
}

# Executa o script
main 