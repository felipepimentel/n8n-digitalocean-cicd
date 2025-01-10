---
layout: default
title: N8N DigitalOcean CI/CD
description: Automated deployment of n8n to DigitalOcean using GitHub Actions and Dagger
---

# N8N DigitalOcean CI/CD Documentation

Welcome to the documentation for the n8n DigitalOcean CI/CD project. This project provides an automated solution for deploying [n8n](https://n8n.io/) workflow automation platform to DigitalOcean using GitHub Actions and Dagger.

## Quick Links

- [Getting Started](./getting-started.md)
- [Architecture Overview](./architecture.md)
- [Configuration Guide](./configuration.md)
- [Security Best Practices](./security.md)
- [Monitoring & Maintenance](./monitoring.md)
- [Troubleshooting](./troubleshooting.md)
- [Contributing](./contributing.md)

## Overview

This project automates the deployment of n8n to DigitalOcean, handling everything from infrastructure provisioning to application deployment and monitoring. Key features include:

- 🚀 **Fully Automated Deployment**: One-click deployment using GitHub Actions
- 🔒 **Security-First Approach**: Built-in security features including fail2ban and UFW
- 📊 **Monitoring & Alerts**: Integrated health checks and monitoring
- 💾 **Automated Backups**: Regular backups of your n8n data
- 🔄 **CI/CD Pipeline**: Using Dagger for consistent and reliable deployments
- 🌐 **Custom Domain Support**: Automatic DNS configuration
- 🐳 **Docker Integration**: Containerized deployment for consistency

## Prerequisites

Before getting started, ensure you have:

1. A [DigitalOcean](https://www.digitalocean.com/) account
2. A [GitHub](https://github.com/) account
3. A domain name for your n8n instance
4. Basic understanding of:
   - Docker and containers
   - GitHub Actions
   - YAML configuration
   - SSH and basic networking

## Quick Start

1. Fork this repository
2. Configure the required [secrets](./configuration.md#github-secrets)
3. Push to the main branch or manually trigger the workflow
4. Access your n8n instance at your configured domain

For detailed setup instructions, visit our [Getting Started](./getting-started.md) guide.

## Project Structure

```
.
├── .github/workflows/   # GitHub Actions workflow definitions
├── ci/                 # Dagger pipeline and deployment code
│   ├── main.go        # Main deployment logic
│   └── ssh/           # SSH client implementation
├── scripts/           # Monitoring and backup scripts
└── docs/             # Documentation (you are here)
```

## Support and Community

- 📚 [Documentation Home](./index.md)
- 🐛 [Issue Tracker](https://github.com/yourusername/n8n-digitalocean-cicd/issues)
- 💬 [Discussions](https://github.com/yourusername/n8n-digitalocean-cicd/discussions)

## License

This project is licensed under the MIT License - see the [LICENSE](../LICENSE) file for details. 