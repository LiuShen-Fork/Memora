# Update Guide

This document will guide you through safely updating and upgrading Memora to the latest version.

## Version Check

### View Current Version

#### Through Web Interface

1. Login to Memora admin dashboard
2. Go to "Dashboard" page
3. Check version number in "Runtime Information" panel

## Update Process

### Preparation

#### 1. Data Backup

```bash
# Stop service
docker-compose down

# Create complete backup
ts=$(date +%Y%m%d-%H%M%S) && mkdir -p backups/$ts && cp -r data/ .env docker-compose.yml backups/$ts/
```

#### 2. Check Compatibility

Review [Release Notes](https://github.com/LiuShen-Fork/Memora/releases) to understand:

- Breaking changes
- New environment variables
- Feature deprecation notices

### Docker Compose Update (Recommended)

#### Standard Update Process

```bash
# 1. Enter project directory
cd /path/to/memora

# 2. Backup current configuration
cp docker-compose.yml docker-compose.yml.backup

# 3. Stop current service
docker-compose down

# 4. Pull latest image
docker-compose pull

# 5. Start new version
docker-compose up -d

# 6. View startup logs
docker-compose logs -f memora
```

#### Specific Version Update

If you need to update to a specific version:

```yaml
# docker-compose.yml
services:
  memora:
    image: ghcr.io/liu-shen-fork/memora:v1.2.3 # Specify version
    # ... other configurations
```

```bash
docker-compose up -d
```

### Single Container Update

```bash
# Stop existing container
docker stop memora
docker rm memora

# Pull latest image
docker pull ghcr.io/liu-shen-fork/memora:latest

# Start new container with same configuration
docker run -d \
  --name memora \
  -p 3000:3000 \
  -v $(pwd)/data:/app/data \
  --env-file .env \
  ghcr.io/liu-shen-fork/memora:latest
```

## Database Compatibility

The Go service verifies and creates missing compatibility tables and indexes on
startup. It does not require a Node, Nitro, Drizzle, or manual migration
command. Back up `data/` before upgrades and keep the Docker mount unchanged:
`./data:/app/data`.
