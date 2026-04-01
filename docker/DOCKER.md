# Docker Quick Start Guide

Get ThinLine Radio running with Docker in 5 minutes!

## Prerequisites

- Docker Engine 20.10+
- Docker Compose 2.0+
- 4GB RAM minimum
- 10GB free disk space

## Quick Start

```bash
# 1. Copy environment template
cp env.docker.example .env

# 2. Edit .env and set your database password
nano .env
# Change: DB_PASS=change_this_password_immediately

# 3. Create data directories
mkdir -p docker/data/postgres docker/data/thinline docker/data/logs
mkdir -p docker/config docker/init-db

# 4. Start Docker containers
docker-compose up -d

# 5. View logs
docker-compose logs -f thinline-radio

# 6. Access admin dashboard
# Open: http://localhost:3000/admin
# Default password: admin
# CHANGE THIS IMMEDIATELY!
```

## What's Included

- ✅ ThinLine Radio server (Go + Angular)
- ✅ PostgreSQL 16 database
- ✅ FFmpeg for audio processing
- ✅ Health checks
- ✅ Automatic database initialization
- ✅ Volume persistence
- ✅ Network isolation

## Common Commands

```bash
# View logs
docker-compose logs -f

# Stop containers
docker-compose down

# Restart containers
docker-compose restart

# Rebuild after updates
docker-compose up -d --build

# Check status
docker-compose ps

# Check resource usage
docker stats
```

## Next Steps

- 📖 Read full documentation: [docker/README.md](docker/README.md)
- 🔒 Change admin password immediately
- ⚙️ Configure systems and talkgroups
- 🎤 Set up transcription services (optional)
- 🔔 Configure alerts and notifications (optional)

## Troubleshooting

**Container won't start?**
```bash
# Check logs
docker-compose logs

# Verify .env file
cat .env | grep DB_PASS
```

**Port already in use?**
```bash
# Edit .env and change HTTP_PORT
HTTP_PORT=3001
```

**Permission errors?**
```bash
# Fix permissions
sudo chown -R 1000:1000 docker/data/
```

## Support

- Full Docker guide: `docker/README.md`
- Setup guide: `docs/setup-and-administration.md`
- Issues: GitHub Issues
- Contact: Thinline Dynamic Solutions

---

**Pro Tip:** Use `docker-compose logs -f thinline-radio` to watch the logs in real-time during first startup.

