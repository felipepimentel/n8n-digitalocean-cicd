---
layout: default
title: Security Best Practices
description: Comprehensive security guide for n8n deployment on DigitalOcean
---

# Security Best Practices

This document outlines the security measures implemented in our n8n deployment solution and provides guidance on maintaining a secure environment.

## Table of Contents

- [Security Overview](#security-overview)
- [Network Security](#network-security)
- [Access Control](#access-control)
- [Data Security](#data-security)
- [Infrastructure Security](#infrastructure-security)
- [Monitoring & Auditing](#monitoring--auditing)
- [Security Checklist](#security-checklist)
- [Security Updates](#security-updates)

## Security Overview

Our security architecture follows these key principles:

1. **Defense in Depth**: Multiple layers of security controls
2. **Least Privilege**: Minimal access rights for components
3. **Secure by Default**: Conservative security defaults
4. **Regular Updates**: Automated security patches
5. **Continuous Monitoring**: Real-time security monitoring

## Network Security

### 1. Firewall Configuration

The UFW (Uncomplicated Firewall) is configured with these rules:

```bash
# Default policies
ufw default deny incoming
ufw default allow outgoing

# Allow specific ports
ufw allow ssh
ufw allow http
ufw allow https

# Rate limiting for SSH
ufw limit ssh
```

### 2. fail2ban Configuration

fail2ban is configured to protect against brute force attacks:

```ini
[sshd]
enabled = true
bantime = 3600
findtime = 600
maxretry = 3

[n8n-auth]
enabled = true
filter = n8n-auth
logpath = /var/log/n8n/auth.log
maxretry = 5
findtime = 300
bantime = 3600
```

### 3. SSL/TLS Configuration

HTTPS is enforced with modern SSL/TLS settings:

```nginx
ssl_protocols TLSv1.2 TLSv1.3;
ssl_prefer_server_ciphers on;
ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
ssl_session_timeout 1d;
ssl_session_cache shared:SSL:50m;
ssl_stapling on;
ssl_stapling_verify on;
```

## Access Control

### 1. SSH Security

Best practices for SSH configuration:

```bash
# /etc/ssh/sshd_config
PermitRootLogin no
PasswordAuthentication no
X11Forwarding no
MaxAuthTries 3
LoginGraceTime 20
```

### 2. n8n Authentication

Basic authentication configuration:

```env
N8N_BASIC_AUTH_ACTIVE=true
N8N_BASIC_AUTH_USER=secure_username
N8N_BASIC_AUTH_PASSWORD=strong_password
```

### 3. API Security

Securing n8n API access:

```env
N8N_PROTOCOL=https
N8N_ENCRYPTION_KEY=your_secure_key
N8N_JWT_SECRET=your_jwt_secret
```

## Data Security

### 1. Encryption at Rest

Data encryption configuration:

```env
# n8n encryption settings
N8N_ENCRYPTION_KEY=32_char_secure_encryption_key

# Volume encryption
VOLUME_ENCRYPTION=aes-256-gcm
```

### 2. Backup Security

Secure backup configuration:

```bash
#!/bin/bash
# Encrypted backup script
BACKUP_PASSWORD=$(openssl rand -hex 32)
tar czf - /opt/n8n/data | \
  openssl enc -aes-256-cbc -salt -pbkdf2 \
  -pass pass:$BACKUP_PASSWORD \
  -out /backup/n8n-backup-$(date +%Y%m%d).tar.gz.enc
```

### 3. Secrets Management

GitHub Actions secrets configuration:

```yaml
env:
  # Required secrets with strict access control
  DIGITALOCEAN_ACCESS_TOKEN: ${{ secrets.DIGITALOCEAN_ACCESS_TOKEN }}
  N8N_ENCRYPTION_KEY: ${{ secrets.N8N_ENCRYPTION_KEY }}
  DO_SSH_PRIVATE_KEY: ${{ secrets.DO_SSH_PRIVATE_KEY }}
```

## Infrastructure Security

### 1. Container Security

Docker security configuration:

```yaml
security_opt:
  - no-new-privileges:true
  - seccomp:unconfined
  - apparmor:unconfined
cap_drop:
  - ALL
cap_add:
  - NET_BIND_SERVICE
```

### 2. Resource Limits

Container resource constraints:

```yaml
resources:
  limits:
    cpus: '2'
    memory: 2G
  reservations:
    cpus: '1'
    memory: 1G
```

### 3. Update Policy

Automated security updates:

```bash
#!/bin/bash
# Security update script
apt-get update
apt-get upgrade -y
apt-get dist-upgrade -y
apt-get autoremove -y
```

## Monitoring & Auditing

### 1. Security Monitoring

Monitoring configuration:

```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:5678/healthz"]
  interval: 1m
  timeout: 10s
  retries: 3
  start_period: 30s
```

### 2. Audit Logging

Log configuration:

```yaml
logging:
  driver: "json-file"
  options:
    max-size: "10m"
    max-file: "3"
    labels: "production_status"
    env: "os,customer"
```

### 3. Alert Configuration

Alert settings:

```yaml
alerts:
  - name: high_cpu_usage
    condition: cpu_usage > 80%
    duration: 5m
    notifications:
      - type: slack
        channel: "#alerts"
      - type: email
        address: "admin@example.com"
```

## Security Checklist

### Initial Setup

- [ ] Generate strong encryption key
- [ ] Configure basic authentication
- [ ] Set up SSL/TLS certificates
- [ ] Configure firewall rules
- [ ] Enable fail2ban
- [ ] Set up secure SSH access

### Regular Maintenance

- [ ] Review security logs daily
- [ ] Check for security updates weekly
- [ ] Rotate encryption keys quarterly
- [ ] Update SSL certificates before expiry
- [ ] Review access permissions monthly
- [ ] Test backup restoration quarterly

## Security Updates

### Automated Updates

The system is configured for automated security updates:

1. **Daily Updates**:
   ```bash
   0 4 * * * apt-get update && apt-get upgrade -y
   ```

2. **Weekly Full Updates**:
   ```bash
   0 3 * * 0 apt-get dist-upgrade -y
   ```

3. **Container Updates**:
   ```bash
   0 2 * * * docker pull n8nio/n8n:latest
   ```

### Manual Security Reviews

Perform these checks monthly:

1. Review security logs
2. Check for unauthorized access attempts
3. Verify backup integrity
4. Test security controls
5. Update security documentation

## Additional Resources

- [n8n Security Documentation](https://docs.n8n.io/security/)
- [DigitalOcean Security Best Practices](https://www.digitalocean.com/community/tutorials/recommended-security-measures-to-protect-your-servers)
- [Docker Security](https://docs.docker.com/engine/security/)
- [Linux Security Checklist](https://www.cyberciti.biz/tips/linux-security.html) 