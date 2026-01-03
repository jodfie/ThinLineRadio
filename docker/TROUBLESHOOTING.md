# Docker Troubleshooting Guide for ThinLine Radio

This guide helps you diagnose and fix common Docker deployment issues.

## Table of Contents

1. [Container Won't Start](#container-wont-start)
2. [Database Connection Issues](#database-connection-issues)
3. [Port Conflicts](#port-conflicts)
4. [Permission Errors](#permission-errors)
5. [Performance Issues](#performance-issues)
6. [Audio Processing Issues](#audio-processing-issues)
7. [Build Failures](#build-failures)
8. [Network Issues](#network-issues)
9. [Data Loss or Corruption](#data-loss-or-corruption)
10. [Debugging Tools](#debugging-tools)

---

## Container Won't Start

### Symptom
Container exits immediately or restarts repeatedly.

### Diagnosis
```bash
# Check container status
docker compose ps

# View recent logs
docker compose logs --tail=100 thinline-radio

# Check for errors
docker compose logs thinline-radio | grep -i "error\|fatal\|panic"
```

### Common Causes & Solutions

**1. Missing or invalid .env file**
```bash
# Check if .env exists
ls -la .env

# Verify required variables
grep "DB_PASS" .env

# Solution: Create from template
cp env.docker.example .env
nano .env  # Edit DB_PASS
```

**2. Database not ready**
```bash
# Check postgres health
docker compose logs postgres

# Solution: Wait longer or fix postgres
docker compose restart postgres
sleep 10
docker compose restart thinline-radio
```

**3. Configuration error**
```bash
# Check for config errors in logs
docker compose logs thinline-radio | grep -i "config"

# Solution: Validate .env file
docker compose config
```

---

## Database Connection Issues

### Symptom
"Failed to connect to database" or "connection refused" errors.

### Diagnosis
```bash
# Check if postgres is running
docker compose ps postgres

# Check postgres logs
docker compose logs postgres

# Test connection from thinline-radio container
docker compose exec thinline-radio ping postgres

# Try connecting directly
docker compose exec postgres psql -U thinline_user -d thinline_radio
```

### Solutions

**1. Wrong credentials**
```bash
# Verify credentials in .env
cat .env | grep DB_

# Ensure they match in docker-compose.yml
docker compose config | grep -A 5 "POSTGRES_"
```

**2. Postgres not healthy**
```bash
# Check health
docker compose exec postgres pg_isready -U thinline_user

# Restart postgres
docker compose restart postgres

# If still failing, check data directory permissions
sudo chown -R 999:999 docker/data/postgres
```

**3. Network issue**
```bash
# Check if containers are on same network
docker network ls
docker network inspect thinlineradio_thinline_network

# Recreate network
docker compose down
docker compose up -d
```

**4. Port conflict (if exposing postgres)**
```bash
# Check if port 5432 is in use
sudo lsof -i :5432

# Solution: Change port in docker-compose.yml or stop conflicting service
```

---

## Port Conflicts

### Symptom
"Port is already allocated" or "address already in use" errors.

### Diagnosis
```bash
# Check what's using port 3000
sudo lsof -i :3000

# Or on Linux
sudo netstat -tulpn | grep 3000
```

### Solutions

**1. Change port in .env**
```bash
# Edit .env
nano .env

# Change HTTP_PORT
HTTP_PORT=3001

# Restart
docker compose down
docker compose up -d
```

**2. Stop conflicting service**
```bash
# Kill process using port (replace PID)
sudo kill -9 <PID>

# Or stop Docker container
docker stop <container-name>
```

**3. Use different ports for different environments**
```bash
# Development on 3001, production on 3000
HTTP_PORT=3001 docker compose up -d
```

---

## Permission Errors

### Symptom
"Permission denied" when accessing volumes or files.

### Diagnosis
```bash
# Check ownership of docker data directory
ls -la docker/data/

# Check container user
docker compose exec thinline-radio id

# Check volume mounts
docker compose exec thinline-radio ls -la /app/data
```

### Solutions

**1. Fix directory ownership**
```bash
# ThinLine Radio runs as UID 1000
sudo chown -R 1000:1000 docker/data/
sudo chmod -R 755 docker/data/
```

**2. PostgreSQL data directory**
```bash
# PostgreSQL runs as UID 999
sudo chown -R 999:999 docker/data/postgres
```

**3. Config files**
```bash
# Ensure readable by container
chmod 644 docker/config/*.ini
chmod 600 docker/config/ssl/*.pem
```

**4. SELinux issues (Fedora/RHEL/CentOS)**
```bash
# Add SELinux label
sudo chcon -Rt svirt_sandbox_file_t docker/data/

# Or disable SELinux (not recommended for production)
sudo setenforce 0
```

---

## Performance Issues

### Symptom
Slow response times, high CPU/memory usage, or container restarts.

### Diagnosis
```bash
# Check resource usage
docker stats

# Check container logs for performance warnings
docker compose logs thinline-radio | grep -i "slow\|timeout\|memory"

# Check database performance
docker compose exec postgres psql -U thinline_user thinline_radio -c "
  SELECT query, calls, mean_exec_time 
  FROM pg_stat_statements 
  ORDER BY mean_exec_time DESC 
  LIMIT 10;"
```

### Solutions

**1. Increase resource limits**

Edit `docker-compose.yml`:
```yaml
services:
  thinline-radio:
    deploy:
      resources:
        limits:
          cpus: '8'
          memory: 8G
```

**2. Optimize database**
```bash
# Vacuum database
docker compose exec postgres psql -U thinline_user thinline_radio -c "VACUUM FULL;"

# Analyze tables
docker compose exec postgres psql -U thinline_user thinline_radio -c "ANALYZE;"

# Add indexes (see docker/init-db/01-custom-indexes.sql.example)
```

**3. Check disk I/O**
```bash
# Check disk usage
df -h docker/data/

# Check inode usage
df -i docker/data/

# Move to faster storage (SSD) if on HDD
```

**4. Increase database connections**

Edit `docker-compose.yml`:
```yaml
postgres:
  environment:
    POSTGRES_MAX_CONNECTIONS: 400
```

---

## Audio Processing Issues

### Symptom
Audio files not processing, transcription failures, or "ffmpeg not found" errors.

### Diagnosis
```bash
# Check if FFmpeg is installed in container
docker compose exec thinline-radio ffmpeg -version
docker compose exec thinline-radio ffprobe -version

# Check audio processing logs
docker compose logs thinline-radio | grep -i "ffmpeg\|audio\|transcode"

# Test audio processing manually
docker compose exec thinline-radio ffmpeg -i /app/data/test.m4a -f wav pipe:1 | head -c 100
```

### Solutions

**1. Rebuild container if FFmpeg missing**
```bash
docker compose build --no-cache
docker compose up -d
```

**2. Check audio file permissions**
```bash
# Ensure container can read audio files
docker compose exec thinline-radio ls -la /app/data/

# Fix permissions
sudo chown -R 1000:1000 docker/data/thinline/
```

**3. Increase timeout for large files**

Edit application config to increase ffmpeg timeouts (if supported).

---

## Build Failures

### Symptom
"docker compose build" fails or takes extremely long.

### Diagnosis
```bash
# Build with verbose output
docker compose build --no-cache --progress=plain

# Check disk space
df -h

# Check Docker daemon
docker info
```

### Solutions

**1. Clear build cache**
```bash
# Remove build cache
docker builder prune -a

# Remove unused images
docker image prune -a
```

**2. Insufficient disk space**
```bash
# Check space
df -h

# Clean up Docker
docker system prune -a --volumes
```

**3. Network issues during build**
```bash
# Retry with different network
docker compose build --network=host

# Check if npm/go modules can be downloaded
docker run --rm node:16-alpine npm config get registry
```

**4. Memory issues during build**
```bash
# Increase Docker memory limit (Docker Desktop)
# Settings → Resources → Memory → Increase to 4GB+

# Or build on machine with more RAM
```

---

## Network Issues

### Symptom
Containers can't communicate, or external services unreachable.

### Diagnosis
```bash
# List networks
docker network ls

# Inspect network
docker network inspect thinlineradio_thinline_network

# Test connectivity between containers
docker compose exec thinline-radio ping postgres
docker compose exec thinline-radio nslookup postgres

# Test external connectivity
docker compose exec thinline-radio ping 8.8.8.8
docker compose exec thinline-radio curl -I https://google.com
```

### Solutions

**1. Recreate network**
```bash
docker compose down
docker network prune
docker compose up -d
```

**2. Check firewall**
```bash
# Temporarily disable firewall for testing
sudo ufw disable  # Ubuntu
sudo systemctl stop firewalld  # Fedora/CentOS

# If that fixes it, add Docker rules
sudo ufw allow from 172.28.0.0/16
```

**3. DNS issues**
```bash
# Add custom DNS to docker-compose.yml
services:
  thinline-radio:
    dns:
      - 8.8.8.8
      - 8.8.4.4
```

**4. IP conflicts**
```bash
# Change subnet in docker-compose.yml
networks:
  thinline_network:
    ipam:
      config:
        - subnet: 172.29.0.0/16
```

---

## Data Loss or Corruption

### Symptom
Missing data, corrupted database, or empty volumes after restart.

### Diagnosis
```bash
# Check if volumes are mounted
docker compose exec thinline-radio ls -la /app/data/
docker compose exec postgres ls -la /var/lib/postgresql/data/

# Check volume configuration
docker volume ls
docker volume inspect thinlineradio_postgres_data

# Check host directory
ls -la docker/data/postgres/
ls -la docker/data/thinline/
```

### Solutions

**1. Backup immediately**
```bash
# Backup database
docker compose exec postgres pg_dump -U thinline_user thinline_radio > backup-$(date +%Y%m%d).sql

# Backup audio files
tar -czf thinline-backup-$(date +%Y%m%d).tar.gz docker/data/thinline/
```

**2. Check volume type**

In `docker-compose.yml`, ensure volumes use bind mounts:
```yaml
volumes:
  postgres_data:
    driver: local
    driver_opts:
      type: none
      o: bind
      device: ./docker/data/postgres
```

**3. Restore from backup**
```bash
# Stop containers
docker compose down

# Restore database
cat backup-20240102.sql | docker compose exec -T postgres psql -U thinline_user thinline_radio

# Restore audio
tar -xzf thinline-backup-20240102.tar.gz

# Restart
docker compose up -d
```

---

## Debugging Tools

### Interactive Shell Access
```bash
# Access ThinLine Radio container
docker compose exec thinline-radio sh

# Access PostgreSQL container
docker compose exec postgres bash

# Run as root (if needed)
docker compose exec -u root thinline-radio sh
```

### View Live Logs
```bash
# All services
docker compose logs -f

# Specific service
docker compose logs -f thinline-radio

# Last 100 lines
docker compose logs --tail=100

# Since timestamp
docker compose logs --since 2024-01-02T10:00:00
```

### Inspect Container
```bash
# Detailed container info
docker inspect thinline-radio

# Container processes
docker compose top thinline-radio

# Resource usage
docker stats thinline-radio
```

### Database Debugging
```bash
# Connect to database
docker compose exec postgres psql -U thinline_user thinline_radio

# Run SQL file
docker compose exec postgres psql -U thinline_user thinline_radio < query.sql

# Check database size
docker compose exec postgres psql -U thinline_user thinline_radio -c "
  SELECT pg_size_pretty(pg_database_size('thinline_radio'));"

# List tables
docker compose exec postgres psql -U thinline_user thinline_radio -c "\dt"
```

### Network Debugging
```bash
# Test DNS resolution
docker compose exec thinline-radio nslookup postgres

# Test port connectivity
docker compose exec thinline-radio nc -zv postgres 5432

# Trace route
docker compose exec thinline-radio traceroute postgres
```

### File System Debugging
```bash
# Check disk usage inside container
docker compose exec thinline-radio df -h

# Find large files
docker compose exec thinline-radio find /app/data -type f -size +100M

# Check inode usage
docker compose exec thinline-radio df -i
```

---

## Getting Help

If you've tried these troubleshooting steps and still have issues:

1. **Collect diagnostic information:**
   ```bash
   # Create a diagnostic report
   ./docker-test.sh > diagnostic-report.txt 2>&1
   docker compose logs > docker-logs.txt
   docker compose ps > docker-status.txt
   ```

2. **Search existing issues:**
   - Check GitHub Issues
   - Search Docker forums
   - Check ThinLine Radio documentation

3. **Report the issue:**
   - Include diagnostic information
   - Describe steps to reproduce
   - Share relevant logs (remove sensitive data)

4. **Contact support:**
   - Email: support@thinlinedynamic.com
   - GitHub Issues: https://github.com/Thinline-Dynamic-Solutions/ThinLineRadio/issues

---

## Preventive Maintenance

### Regular Tasks

**Daily:**
- Monitor logs for errors
- Check disk space
- Verify backups completed

**Weekly:**
- Review resource usage trends
- Update Docker images if available
- Test backup restoration

**Monthly:**
- Vacuum database
- Rotate logs
- Review security updates

**Commands:**
```bash
# Check health
docker compose ps
docker stats --no-stream

# Backup
docker compose exec postgres pg_dump -U thinline_user thinline_radio > backup.sql

# Update
docker compose pull
docker compose up -d
```

---

**Remember:** Always test changes in a development environment before applying to production!

