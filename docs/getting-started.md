---
layout: default
title: Getting Started
description: Step-by-step guide to deploy n8n on DigitalOcean
---

# Getting Started

This guide will walk you through the process of setting up and deploying n8n on DigitalOcean using our automated CI/CD pipeline.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Initial Setup](#initial-setup)
- [Configuration](#configuration)
- [Deployment](#deployment)
- [Post-Deployment](#post-deployment)
- [Next Steps](#next-steps)

## Prerequisites

Before you begin, ensure you have:

1. **DigitalOcean Account**
   - [Sign up](https://cloud.digitalocean.com/registrations/new) if you haven't already
   - [Generate an API token](https://cloud.digitalocean.com/account/api/tokens) with write access

2. **GitHub Account**
   - Fork this repository
   - Enable GitHub Actions in your fork

3. **Domain Name**
   - A domain name you control
   - Access to your domain's DNS settings

4. **Development Environment**
   - Git installed locally
   - Basic understanding of YAML and Go (optional)

## Initial Setup

### 1. Fork the Repository

1. Visit the [repository page](https://github.com/yourusername/n8n-digitalocean-cicd)
2. Click the "Fork" button
3. Clone your fork locally:
   ```bash
   git clone https://github.com/yourusername/n8n-digitalocean-cicd.git
   cd n8n-digitalocean-cicd
   ```

### 2. DigitalOcean Setup

1. **Create API Token**:
   - Go to [API Settings](https://cloud.digitalocean.com/account/api/tokens)
   - Click "Generate New Token"
   - Give it a name (e.g., "n8n-deployment")
   - Ensure "Write" scope is selected
   - Copy the token immediately (it won't be shown again)

2. **Create SSH Key**:
   ```bash
   ssh-keygen -t rsa -b 4096 -C "your-email@example.com" -f ~/.ssh/do_n8n
   ```
   - Add the key to your DigitalOcean account:
     - Go to [Security Settings](https://cloud.digitalocean.com/account/security)
     - Click "Add SSH Key"
     - Paste the public key content (`cat ~/.ssh/do_n8n.pub`)
     - Note the SSH key fingerprint shown after adding the key

3. **Configure DNS**:
   - Add your domain to DigitalOcean:
     - Go to [Networking > Domains](https://cloud.digitalocean.com/networking/domains)
     - Add your domain
   - Update your domain's nameservers to DigitalOcean's:
     - ns1.digitalocean.com
     - ns2.digitalocean.com
     - ns3.digitalocean.com

## Configuration

### 1. Required GitHub Secrets

Go to your repository's Settings > Secrets and variables > Actions and add the following secrets:

| Secret Name | Description | Example |
|------------|-------------|---------|
| `DIGITALOCEAN_ACCESS_TOKEN` | Your DigitalOcean API token | `dop_v1_...` |
| `DO_SSH_KEY_ID` | SSH key ID from DigitalOcean | `12345678` |
| `DO_SSH_PRIVATE_KEY` | Content of your SSH private key | `-----BEGIN OPENSSH PRIVATE KEY-----...` |
| `N8N_DOMAIN` | Your domain for n8n | `n8n.example.com` |
| `N8N_ENCRYPTION_KEY` | Random 32-char string | Generate with: `openssl rand -hex 16` |

### 2. Optional Configuration

These variables have defaults but can be overridden:

| Variable Name | Default | Description |
|--------------|---------|-------------|
| `DOCKER_REGISTRY` | `registry.digitalocean.com` | Docker registry URL |
| `DROPLET_NAME` | `n8n-server` | Name of the DigitalOcean droplet |
| `N8N_VERSION` | `latest` | n8n version to deploy |
| `N8N_BASIC_AUTH_USER` | `admin` | Basic auth username |
| `N8N_BASIC_AUTH_PASSWORD` | Same as `N8N_ENCRYPTION_KEY` | Basic auth password |

## Deployment

### 1. Initial Deployment

The deployment will automatically trigger when you:

1. Push to the main branch:
   ```bash
   git push origin main
   ```

2. Or manually:
   - Go to Actions tab in your repository
   - Select "Deploy n8n"
   - Click "Run workflow"

### 2. Monitor Deployment

1. Watch the GitHub Actions workflow:
   - Click on the running workflow
   - Expand the deployment step to see logs

2. Deployment takes approximately 5-10 minutes and includes:
   - Creating DigitalOcean resources
   - Building and pushing Docker image
   - Configuring the droplet
   - Starting n8n

## Post-Deployment

After successful deployment:

1. **Access n8n**:
   - Visit `https://your-domain.com`
   - Log in with your configured credentials

2. **Verify Security**:
   - Confirm HTTPS is working
   - Test basic auth
   - Check firewall rules

3. **Monitor Health**:
   - Check DigitalOcean monitoring
   - Review n8n logs
   - Test backup system

## Next Steps

- [Configure Security Settings](./security.md)
- [Set Up Monitoring](./monitoring.md)
- [Configure Backups](./monitoring.md#backups)
- [Troubleshoot Common Issues](./troubleshooting.md)

## Need Help?

- Check our [Troubleshooting Guide](./troubleshooting.md)
- [Open an Issue](https://github.com/yourusername/n8n-digitalocean-cicd/issues)
- Review [Common Questions](./faq.md) 