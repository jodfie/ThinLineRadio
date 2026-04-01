# Docker Deployment Checklist for ThinLine Radio

Use this checklist to ensure proper Docker deployment.

## Pre-Deployment Checklist

### 1. Prerequisites ✓
- [ ] Docker Engine 20.10+ installed
- [ ] Docker Compose 2.0+ installed
- [ ] Minimum 4GB RAM available
- [ ] Minimum 10GB disk space available
- [ ] Ports 3000 and 3443 available (or configured differently)

**Verify:**
```bash
docker --version
docker compose version
df -h
free -h  # Linux only
```

### 2. Files Present ✓
- [ ] `Dockerfile` exists
- [ ] `docker-compose.yml` exists
- [ ] `.dockerignore` exists
- [ ] `env.docker.example` exists
- [ ] `docker/` directory structure created

**Verify:**
```bash
ls -la Dockerfile docker-compose.yml .dockerignore env.docker.example
ls -la docker/
```

### 3. Configuration ✓
- [ ] Copied `env.docker.example` to `.env`
- [ ] Changed `DB_PASS` to strong password
- [ ] Set correct `TZ` timezone
- [ ] Reviewed and set optional environment variables
- [ ] Created required directories

**Execute:**
```bash
cp env.docker.example .env
nano .env  # Edit configuration
mkdir -p docker/data/{postgres,thinline,logs}
mkdir -p docker/config/{ssl,credentials}
mkdir -p docker/init-db
```

### 4. Security Review ✓
- [ ] Strong database password set (min 16 chars, mixed case, numbers, symbols)
- [ ] `.env` file NOT committed to git
- [ ] SSL certificates ready (if using custom SSL)
- [ ] Firewall rules planned
- [ ] Admin password change planned (default: `admin`)

## Deployment Steps

### 5. Initial Build ✓
- [ ] Build Docker images
- [ ] Verify build completed successfully
- [ ] Check image sizes are reasonable

**Execute:**
```bash
docker-compose build --no-cache
docker images | grep thinline
```

### 6. Start Services ✓
- [ ] Start containers in detached mode
- [ ] Wait for services to initialize (30-60 seconds)
- [ ] Verify containers are running

**Execute:**
```bash
docker-compose up -d
sleep 30
docker-compose ps
```

### 7. Health Checks ✓
- [ ] PostgreSQL is healthy
- [ ] ThinLine Radio is responding
- [ ] No fatal errors in logs
- [ ] Can access web interface

**Execute:**
```bash
docker-compose exec postgres pg_isready -U thinline_user
docker-compose logs thinline-radio | grep -i "error\|fatal"
curl -I http://localhost:3000/
```

### 8. Initial Configuration ✓
- [ ] Access admin dashboard at http://localhost:3000/admin
- [ ] Login with default password (`admin`)
- [ ] **IMMEDIATELY change admin password**
- [ ] Configure systems and talkgroups
- [ ] Test audio upload

### 9. Optional Features ✓
- [ ] SSL/TLS configured (if needed)
- [ ] Transcription service configured (if needed)
- [ ] Email SMTP configured (if needed)
- [ ] Stripe billing configured (if needed)
- [ ] Push notifications configured (if needed)

### 10. Backup Setup ✓
- [ ] Test database backup
- [ ] Test audio files backup
- [ ] Set up automated backup script
- [ ] Test restore procedure
- [ ] Document backup location

**Execute:**
```bash
# Test backup
docker-compose exec postgres pg_dump -U thinline_user thinline_radio > test-backup.sql
tar -czf test-audio-backup.tar.gz docker/data/thinline/

# Test restore
cat test-backup.sql | docker-compose exec -T postgres psql -U thinline_user thinline_radio
```

## Post-Deployment Checklist

### 11. Testing ✓
- [ ] Run automated test suite
- [ ] Test web interface access
- [ ] Test admin dashboard access
- [ ] Upload test audio file
- [ ] Verify audio playback
- [ ] Test user registration (if enabled)
- [ ] Test mobile app connection (if applicable)

**Execute:**
```bash
./docker-test.sh
```

### 12. Monitoring Setup ✓
- [ ] Set up log monitoring
- [ ] Configure disk space alerts
- [ ] Set up uptime monitoring
- [ ] Document where to check logs
- [ ] Test log rotation

**Execute:**
```bash
docker-compose logs -f &  # Monitor in background
df -h docker/data/  # Check disk space
```

### 13. Documentation ✓
- [ ] Document server IP/hostname
- [ ] Document ports used
- [ ] Document admin credentials (securely)
- [ ] Document backup locations
- [ ] Document recovery procedures
- [ ] Share access info with team

### 14. Security Hardening (Production) ✓
- [ ] Enable firewall and allow only necessary ports
- [ ] Set up reverse proxy (nginx/traefik) for SSL
- [ ] Remove PostgreSQL port exposure (if exposed)
- [ ] Enable rate limiting
- [ ] Review security logs
- [ ] Enable automatic security updates

**Execute (Ubuntu/Debian):**
```bash
# Firewall
sudo ufw allow 22/tcp   # SSH
sudo ufw allow 80/tcp   # HTTP
sudo ufw allow 443/tcp  # HTTPS
sudo ufw enable

# Do NOT expose PostgreSQL port 5432 in production
# Comment out in docker-compose.yml:
# ports:
#   - "5432:5432"
```

### 15. Performance Tuning ✓
- [ ] Adjust resource limits if needed
- [ ] Add database indexes if needed
- [ ] Configure database connection pool
- [ ] Enable query caching if needed
- [ ] Monitor resource usage

**Execute:**
```bash
docker stats
docker-compose logs postgres | grep -i "slow query"
```

## Production Deployment Extra Steps

### 16. High Availability (Optional) ✓
- [ ] Set up database replication
- [ ] Configure load balancer
- [ ] Set up health check endpoints
- [ ] Configure auto-scaling (if cloud)
- [ ] Test failover procedures

### 17. Compliance (If Required) ✓
- [ ] Enable audit logging
- [ ] Configure data retention policies
- [ ] Set up encryption at rest
- [ ] Document compliance measures
- [ ] Review GDPR/privacy requirements

### 18. Disaster Recovery ✓
- [ ] Document complete recovery procedure
- [ ] Test recovery from backups
- [ ] Set up off-site backup storage
- [ ] Document RTO/RPO requirements
- [ ] Create runbook for emergencies

## Maintenance Schedule

### Daily ✓
- [ ] Check logs for errors
- [ ] Verify backups completed
- [ ] Check disk space
- [ ] Monitor resource usage

**Execute:**
```bash
docker-compose logs --since 24h | grep -i "error"
df -h docker/data/
docker stats --no-stream
```

### Weekly ✓
- [ ] Review security logs
- [ ] Update Docker images (if available)
- [ ] Test backup restoration
- [ ] Review performance metrics
- [ ] Check for updates

**Execute:**
```bash
docker-compose pull
docker-compose up -d
```

### Monthly ✓
- [ ] Vacuum database
- [ ] Rotate logs
- [ ] Review disk usage trends
- [ ] Update documentation
- [ ] Security audit

**Execute:**
```bash
docker-compose exec postgres psql -U thinline_user thinline_radio -c "VACUUM FULL;"
docker-compose exec postgres psql -U thinline_user thinline_radio -c "ANALYZE;"
```

## Troubleshooting Resources

If issues occur, refer to:

1. **Quick diagnostics:**
   ```bash
   ./docker-test.sh
   docker-compose logs -f
   docker-compose ps
   ```

2. **Documentation:**
   - `DOCKER.md` - Quick start guide
   - `docker/README.md` - Full deployment guide
   - `docker/TROUBLESHOOTING.md` - Comprehensive troubleshooting
   - `DOCKER-IMPLEMENTATION.md` - Technical details

3. **Common Issues:**
   - Container won't start → Check logs and .env file
   - Database connection failed → Verify credentials and postgres health
   - Port conflict → Change HTTP_PORT in .env
   - Permission errors → Fix ownership with `chown -R 1000:1000 docker/data/`

## Sign-Off

Deployment completed by: ________________________

Date: ________________________

Environment: [ ] Development  [ ] Staging  [ ] Production

Notes:
_____________________________________________________________
_____________________________________________________________
_____________________________________________________________

## Quick Reference Commands

```bash
# Start
docker-compose up -d

# Stop
docker-compose down

# Restart
docker-compose restart

# Logs
docker-compose logs -f

# Status
docker-compose ps

# Stats
docker stats

# Backup DB
docker-compose exec postgres pg_dump -U thinline_user thinline_radio > backup.sql

# Restore DB
cat backup.sql | docker-compose exec -T postgres psql -U thinline_user thinline_radio

# Update
docker-compose pull
docker-compose up -d --build

# Clean up
docker system prune -a
```

---

**For detailed instructions, see `DOCKER.md` and `docker/README.md`**

