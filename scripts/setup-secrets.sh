#!/bin/bash

# Função para verificar se o GitHub CLI está instalado
check_gh_cli() {
    if ! command -v gh &> /dev/null; then
        echo "❌ GitHub CLI não está instalado"
        echo "Por favor, instale o GitHub CLI: https://cli.github.com/"
        exit 1
    fi
    echo "✅ GitHub CLI está instalado"
}

# Função para verificar se o usuário está autenticado
check_gh_auth() {
    if ! gh auth status &> /dev/null; then
        echo "❌ Você não está autenticado no GitHub CLI"
        echo "Por favor, execute: gh auth login"
        exit 1
    fi
    echo "✅ Autenticado no GitHub CLI"
}

# Função para gerar uma nova chave de criptografia
generate_encryption_key() {
    openssl rand -hex 16
}

# Função para verificar se o segredo já existe
check_secret_exists() {
    gh secret list | grep -q "^N8N_ENCRYPTION_KEY"
}

# Configuração inicial
echo "🔧 Configurando segredos para n8n..."
check_gh_cli
check_gh_auth

# Verifica se o segredo já existe
if check_secret_exists; then
    echo "⚠️ O segredo N8N_ENCRYPTION_KEY já existe"
    read -p "Deseja gerar uma nova chave? (s/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Ss]$ ]]; then
        echo "🛑 Operação cancelada"
        exit 0
    fi
fi

# Gera uma nova chave
new_key=$(generate_encryption_key)
if [ -z "$new_key" ]; then
    echo "❌ Erro ao gerar a chave"
    exit 1
fi

# Salva a chave como segredo
echo "🔑 Salvando chave como segredo..."
echo "$new_key" | gh secret set N8N_ENCRYPTION_KEY

if [ $? -eq 0 ]; then
    echo "✅ Segredo configurado com sucesso"
    echo "🔐 Nova chave: $new_key"
    echo "⚠️ Guarde esta chave em um local seguro!"
else
    echo "❌ Erro ao configurar o segredo"
    exit 1
fi 