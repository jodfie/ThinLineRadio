# ThinLine Radio - Configuration Directory

This directory contains configuration files that are mounted into the Docker container.

## Directory Structure

```
docker/config/
├── README.md              # This file
├── ssl/                   # SSL/TLS certificates (optional)
│   ├── cert.pem
│   └── key.pem
└── credentials/           # Service credentials (optional)
    ├── google-credentials.json
    └── other-credentials.json
```

## Configuration Methods

ThinLine Radio supports multiple configuration methods (in order of precedence):

1. **Command-line arguments** (highest priority)
2. **Environment variables** (from `.env` file)
3. **Configuration file** (`thinline-radio.ini`)
4. **Default values** (lowest priority)

## SSL/TLS Configuration

### Option 1: Let's Encrypt (Recommended)

For automatic SSL certificates, set in your `.env` file:

```bash
SSL_AUTO_CERT=yourdomain.com
```

**Requirements:**
- Domain must point to your server
- Port 80 must be accessible for ACME challenge
- Port 443 should forward to container port 3443

### Option 2: Custom Certificates

Place your SSL certificates in `docker/config/ssl/`:

```bash
docker/config/ssl/
├── cert.pem    # Your SSL certificate (full chain)
└── key.pem     # Your private key
```

**Generate self-signed certificate (testing only):**
```bash
cd docker/config/ssl/

# Generate private key
openssl genrsa -out key.pem 2048

# Generate self-signed certificate
openssl req -new -x509 -key key.pem -out cert.pem -days 365 \
  -subj "/C=US/ST=State/L=City/O=Organization/CN=yourdomain.com"

# Set permissions
chmod 600 key.pem
chmod 644 cert.pem
```

**Get certificates from Let's Encrypt manually:**
```bash
# Install certbot
sudo apt install certbot

# Generate certificate
sudo certbot certonly --standalone -d yourdomain.com

# Copy to docker config
sudo cp /etc/letsencrypt/live/yourdomain.com/fullchain.pem docker/config/ssl/cert.pem
sudo cp /etc/letsencrypt/live/yourdomain.com/privkey.pem docker/config/ssl/key.pem
```

**Using certificates in docker-compose.yml:**

Uncomment these lines in `docker-compose.yml`:
```yaml
environment:
  SSL_CERT_FILE: /app/config/ssl/cert.pem
  SSL_KEY_FILE: /app/config/ssl/key.pem

volumes:
  - ./docker/config/ssl:/app/config/ssl:ro
```

## Transcription Service Credentials

### Google Cloud Speech-to-Text

1. **Create service account:**
   - Go to: https://console.cloud.google.com/
   - Create project or select existing
   - Enable Speech-to-Text API
   - Create service account
   - Download JSON key

2. **Place credentials:**
   ```bash
   cp ~/Downloads/google-credentials.json docker/config/credentials/
   chmod 600 docker/config/credentials/google-credentials.json
   ```

3. **Configure in docker-compose.yml:**
   ```yaml
   environment:
     GOOGLE_APPLICATION_CREDENTIALS: /app/config/credentials/google-credentials.json
   
   volumes:
     - ./docker/config/credentials:/app/config/credentials:ro
   ```

### Azure Cognitive Services

**No files needed** - configure via environment variables in `.env`:

```bash
AZURE_SPEECH_KEY=your-azure-key-here
AZURE_SPEECH_REGION=eastus
```

### OpenAI Whisper API

**No files needed** - configure via environment variables in `.env`:

```bash
WHISPER_API_KEY=sk-your-openai-key-here
```

### AssemblyAI

**No files needed** - configure via environment variables in `.env`:

```bash
ASSEMBLYAI_API_KEY=your-assemblyai-key-here
```

## Configuration File (Advanced)

If you prefer using a configuration file instead of environment variables, create `docker/config/thinline-radio.ini`:

```ini
# ThinLine Radio Configuration File

[database]
db_type = postgresql
db_host = postgres
db_port = 5432
db_name = thinline_radio
db_user = thinline_user
db_pass = your_password_here

[server]
listen = 0.0.0.0:3000
ssl_listen = 0.0.0.0:3443

[ssl]
# ssl_cert_file = /app/config/ssl/cert.pem
# ssl_key_file = /app/config/ssl/key.pem
# ssl_auto_cert = yourdomain.com

[transcription]
# Google Cloud
# google_credentials = /app/config/credentials/google-credentials.json

# Azure
# azure_speech_key = your-key
# azure_speech_region = eastus

# OpenAI Whisper
# whisper_api_key = sk-...

# AssemblyAI
# assemblyai_api_key = your-key
```

**Mount in docker-compose.yml:**
```yaml
volumes:
  - ./docker/config/thinline-radio.ini:/app/config/thinline-radio.ini:ro

command:
  - "-config"
  - "/app/config/thinline-radio.ini"
```

## Security Best Practices

1. **File Permissions:**
   ```bash
   # Restrict access to sensitive files
   chmod 600 docker/config/ssl/*.pem
   chmod 600 docker/config/credentials/*.json
   chmod 600 docker/config/*.ini
   
   # Make sure you own the files
   chown $USER:$USER docker/config/ssl/*
   chown $USER:$USER docker/config/credentials/*
   ```

2. **Never Commit Secrets:**
   - Add to `.gitignore`:
     ```
     docker/config/**/*.pem
     docker/config/**/*.key
     docker/config/**/*.json
     docker/config/**/*.ini
     !docker/config/README.md
     ```

3. **Rotate Credentials Regularly:**
   - Change passwords every 90 days
   - Rotate API keys periodically
   - Update SSL certificates before expiration

4. **Use Secrets Management (Production):**
   - Consider Docker Secrets (Swarm mode)
   - Or external secrets managers (Vault, AWS Secrets Manager, etc.)

## Example: Complete Configuration

**File: `docker/config/thinline-radio.ini`**
```ini
# Production Configuration Example

[database]
db_type = postgresql
db_host = postgres
db_port = 5432
db_name = thinline_radio
db_user = thinline_user
db_pass = ${DB_PASS}  # From environment variable

[server]
listen = 0.0.0.0:3000
ssl_listen = 0.0.0.0:3443
base_dir = /app/data

[ssl]
ssl_auto_cert = scanner.yourdomain.com

[email]
smtp_host = smtp.gmail.com
smtp_port = 587
smtp_user = notifications@yourdomain.com
smtp_pass = ${SMTP_PASS}
smtp_from = ThinLine Radio <noreply@yourdomain.com>

[transcription]
google_credentials = /app/config/credentials/google-credentials.json
```

## Troubleshooting

**Permission denied errors:**
```bash
# Fix ownership
sudo chown -R 1000:1000 docker/config/

# Fix permissions
chmod -R 755 docker/config/
chmod 600 docker/config/ssl/*.pem
chmod 600 docker/config/credentials/*.json
```

**SSL certificate errors:**
```bash
# Verify certificate validity
openssl x509 -in docker/config/ssl/cert.pem -text -noout

# Check certificate and key match
openssl x509 -noout -modulus -in docker/config/ssl/cert.pem | openssl md5
openssl rsa -noout -modulus -in docker/config/ssl/key.pem | openssl md5
# (both should output the same hash)
```

**Configuration not loading:**
```bash
# Check if file is mounted correctly
docker-compose exec thinline-radio ls -la /app/config/

# Check container logs for errors
docker-compose logs thinline-radio | grep -i config
```

## Additional Resources

- [Main Documentation](../../docs/setup-and-administration.md)
- [Docker Deployment Guide](./README.md)
- [SSL/TLS Guide](https://letsencrypt.org/docs/)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)

