---
layout: default
title: Troubleshooting Guide
description: Comprehensive troubleshooting guide for n8n deployment on DigitalOcean
---

# Troubleshooting Guide

This guide provides solutions for common issues you might encounter while deploying and running n8n on DigitalOcean.

## Table of Contents

- [Deployment Issues](#deployment-issues)
- [Runtime Issues](#runtime-issues)
- [Network Issues](#network-issues)
- [Security Issues](#security-issues)
- [Backup & Recovery Issues](#backup--recovery-issues)
- [Performance Issues](#performance-issues)
- [Common Error Messages](#common-error-messages)

## Deployment Issues

### GitHub Actions Pipeline Failures

#### Issue: Pipeline Authentication Errors
```
Error: Unable to authenticate with DigitalOcean API
```

**Solution:**
1. Verify `DIGITALOCEAN_ACCESS_TOKEN`:
   ```bash
   # Test token validity
   curl -X GET -H "Authorization: Bearer $DIGITALOCEAN_ACCESS_TOKEN" \
     "https://api.digitalocean.com/v2/account"
   ```
2. Check token permissions in DigitalOcean
3. Regenerate token if necessary

#### Issue: Docker Build Failures
```
Error: failed to push image to registry.digitalocean.com
```

**Solution:**
1. Verify registry authentication:
   ```bash
   docker login registry.digitalocean.com
   ```
2. Check registry quota:
   ```bash
   curl -X GET -H "Authorization: Bearer $DIGITALOCEAN_ACCESS_TOKEN" \
     "https://api.digitalocean.com/v2/registry"
   ```

### Droplet Creation Issues

#### Issue: Resource Limits
```
Error: You have reached your droplet limit
```

**Solution:**
1. Check current limits:
   ```bash
   curl -X GET -H "Authorization: Bearer $DIGITALOCEAN_ACCESS_TOKEN" \
     "https://api.digitalocean.com/v2/account"
   ```
2. Delete unused droplets or request limit increase

## Runtime Issues

### Container Startup Problems

#### Issue: Container Fails to Start
```
Error: Container n8n-container exited with code 1
```

**Diagnostic Steps:**
```bash
# Check container logs
docker logs n8n-container

# Check container configuration
docker inspect n8n-container

# Verify volume mounts
docker volume inspect n8n_data
```

**Solutions:**
1. Memory Issues:
   ```bash
   # Adjust container memory limit
   docker update --memory 2G n8n-container
   ```

2. Permission Issues:
   ```bash
   # Fix volume permissions
   docker run --rm -v n8n_data:/data alpine chown -R 1000:1000 /data
   ```

### Database Connection Issues

#### Issue: Database Connectivity
```
Error: ECONNREFUSED connecting to database
```

**Solutions:**
1. Check network connectivity:
   ```bash
   # Test network
   docker network inspect n8n-network
   
   # Verify DNS resolution
   docker run --rm --network n8n-network alpine nslookup database
   ```

2. Reset network:
   ```bash
   docker network rm n8n-network
   docker network create n8n-network
   ```

## Network Issues

### SSL/TLS Problems

#### Issue: SSL Certificate Errors
```
Error: unable to verify the first certificate
```

**Solutions:**
1. Check certificate validity:
   ```bash
   openssl x509 -in /etc/ssl/certs/n8n.crt -text -noout
   ```

2. Renew certificate:
   ```bash
   # Using Let's Encrypt
   certbot renew --force-renewal
   ```

### DNS Issues

#### Issue: Domain Not Resolving
```
Error: could not resolve host
```

**Solutions:**
1. Verify DNS records:
   ```bash
   # Check A record
   dig +short your-domain.com
   
   # Check propagation
   dig +trace your-domain.com
   ```

2. Update DNS records:
   ```bash
   doctl compute domain records create \
     --record-type A \
     --record-name @ \
     --record-data $DROPLET_IP \
     your-domain.com
   ```

## Security Issues

### Authentication Problems

#### Issue: Basic Auth Failures
```
Error: 401 Unauthorized
```

**Solutions:**
1. Verify environment variables:
   ```bash
   docker exec n8n-container env | grep N8N_BASIC_AUTH
   ```

2. Reset authentication:
   ```bash
   # Generate new credentials
   N8N_BASIC_AUTH_USER=admin
   N8N_BASIC_AUTH_PASSWORD=$(openssl rand -hex 16)
   
   # Update container
   docker update \
     --env N8N_BASIC_AUTH_USER=$N8N_BASIC_AUTH_USER \
     --env N8N_BASIC_AUTH_PASSWORD=$N8N_BASIC_AUTH_PASSWORD \
     n8n-container
   ```

### Firewall Issues

#### Issue: Connection Blocked
```
Error: connection refused
```

**Solutions:**
1. Check UFW rules:
   ```bash
   ufw status verbose
   ```

2. Update firewall rules:
   ```bash
   # Allow necessary ports
   ufw allow 80/tcp
   ufw allow 443/tcp
   ```

## Backup & Recovery Issues

### Backup Failures

#### Issue: Backup Creation Fails
```
Error: cannot create backup
```

**Solutions:**
1. Check disk space:
   ```bash
   df -h /opt/n8n/backups
   ```

2. Verify permissions:
   ```bash
   ls -la /opt/n8n/backups
   ```

3. Manual backup:
   ```bash
   # Create manual backup
   docker run --rm \
     --volumes-from n8n-container \
     -v /opt/n8n/backups:/backup \
     alpine tar czf /backup/manual-backup.tar.gz /home/node/.n8n
   ```

### Recovery Problems

#### Issue: Restore Fails
```
Error: unable to extract backup
```

**Solutions:**
1. Verify backup integrity:
   ```bash
   # Test backup file
   tar tzf n8n-backup.tar.gz
   ```

2. Clean restore:
   ```bash
   # Remove existing data
   docker volume rm n8n_data
   docker volume create n8n_data
   
   # Restore from backup
   docker run --rm \
     -v n8n_data:/recovery \
     -v /opt/n8n/backups:/backups \
     alpine sh -c "cd /recovery && tar xzf /backups/n8n-backup.tar.gz"
   ```

## Performance Issues

### High CPU Usage

#### Issue: CPU Throttling
```
Warning: High CPU usage detected
```

**Solutions:**
1. Check process usage:
   ```bash
   top -b -n 1 -o %CPU
   ```

2. Analyze Docker stats:
   ```bash
   docker stats --no-stream n8n-container
   ```

3. Adjust resources:
   ```bash
   # Update container CPU limits
   docker update --cpus 2 n8n-container
   ```

### Memory Problems

#### Issue: Out of Memory
```
Error: JavaScript heap out of memory
```

**Solutions:**
1. Check memory usage:
   ```bash
   free -h
   docker stats --no-stream
   ```

2. Adjust Node.js memory:
   ```bash
   # Update NODE_OPTIONS
   docker update \
     --env NODE_OPTIONS="--max-old-space-size=2048" \
     n8n-container
   ```

## Common Error Messages

### Error: EACCES: permission denied

```
Error: EACCES: permission denied, access '/home/node/.n8n'
```

**Solution:**
```bash
# Fix permissions
docker run --rm \
  -v n8n_data:/data \
  alpine sh -c "chown -R 1000:1000 /data"
```

### Error: listen EADDRINUSE

```
Error: listen EADDRINUSE: address already in use :::5678
```

**Solution:**
```bash
# Check port usage
netstat -tulpn | grep 5678

# Kill process using port
fuser -k 5678/tcp

# Restart container
docker restart n8n-container
```

## Debugging Tools

### Container Debugging

```bash
# Interactive shell
docker exec -it n8n-container sh

# Process list
docker top n8n-container

# Resource usage
docker stats n8n-container
```

### Log Analysis

```bash
# View logs with timestamps
docker logs --timestamps n8n-container

# Follow logs
docker logs -f n8n-container

# Filter errors
docker logs n8n-container 2>&1 | grep -i error
```

### Network Debugging

```bash
# Test connectivity
docker run --rm \
  --network n8n-network \
  appropriate/curl -v https://your-domain.com

# DNS lookup
docker run --rm \
  --network n8n-network \
  appropriate/curl nslookup n8n-container
```

## Additional Resources

- [n8n Troubleshooting Guide](https://docs.n8n.io/reference/server-setup.html#troubleshooting)
- [Docker Troubleshooting](https://docs.docker.com/config/daemon/troubleshoot/)
- [DigitalOcean Status](https://status.digitalocean.com/)
- [Common n8n Issues](https://github.com/n8n-io/n8n/issues) 