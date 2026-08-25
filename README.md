# ChronoFrame

<p align="center">
  <a href="https://github.com/LiuShen-Fork/ChronoFrame"><strong>ChronoFrame Go Edition</strong></a>
</p>

<p align="center">
  <a href="https://github.com/LiuShen-Fork/ChronoFrame/releases/latest">
    <img src="https://badgen.net/github/release/LiuShen-Fork/ChronoFrame/stable?icon=docker&label=stable" alt="Latest Release">
  </a>
  <a href="https://github.com/LiuShen-Fork/ChronoFrame/releases?q=beta&expanded=false">
    <img src="https://badgen.net/github/release/LiuShen-Fork/ChronoFrame?icon=docker&label=nightly" alt="Latest Nightly Release">
  </a>
  <img src="https://img.shields.io/badge/License-MIT-green.svg" alt="License">
</p>

**Languages:** English | [中文](README_zh.md)

A smooth photo display and management application, supporting multiple image formats and large-size image rendering.

[Live Demo: TimoYin's Mems](https://lens.bh8.ga)

## This Fork

This repository is a Go migration of the original Nuxt application. The public UI and database/storage layout remain compatible, while the production runtime is now a single Go process that serves the generated static frontend, API, SQLite database, media queue, FFmpeg processing, and ExifTool integration. Node.js is only needed when rebuilding the frontend; it is not required to run the application.

Compared with the original project, this fork keeps the gallery, albums, maps, EXIF processing, uploads, authentication, and settings workflows while reducing runtime memory usage and removing the Nuxt/Nitro server from production. The maintained source repository is [LiuShen-Fork/ChronoFrame](https://github.com/LiuShen-Fork/ChronoFrame).

## ✨ Features

### 🖼️ Powerful Photo Management

- **Manage photos online** - Easily manage and browse photos via the web interface
- **Explore map** - Browse photo locations on a map
- **Smart EXIF parsing** - Automatically extracts metadata such as capture time, geolocation, and camera parameters
- **Reverse geocoding** - Automatically identifies photo shooting locations
- **Multi-format support** - Supports mainstream formats including JPEG, PNG, HEIC/HEIF
- **Smart thumbnails** - Efficient thumbnail generation using ThumbHash

### 🔧 Modern Tech Stack

- **Nuxt 4 static frontend** - A pre-generated SPA served by the Go backend
- **TypeScript** - Full type safety
- **TailwindCSS** - Modern CSS framework
- **Go backend** - A compact HTTP server with SQLite and a durable media queue

### ☁️ Flexible Storage Solutions

- **Multiple storage backends** - Supports S3-compatible storage, local filesystem
- **CDN acceleration** - Configurable CDN URL for faster photo delivery

## 🐳 Deployment

We recommend deploying with the prebuilt Docker image. [View the image on ghcr](https://github.com/LiuShen-Fork/ChronoFrame/pkgs/container/chronoframe)

Create a `.env` file and configure environment variables.

Below is a **minimal configuration** example. For complete configuration options, see [Configuration Guide](https://chronoframe.bh8.ga/guide/configuration.html):

```bash
# Admin email (required)
CFRAME_ADMIN_EMAIL=
# Admin username (optional, default Chronoframe)
CFRAME_ADMIN_NAME=
# Admin password (optional, default CF1234@!)
CFRAME_ADMIN_PASSWORD=

# Site metadata (all optional)
NUXT_PUBLIC_APP_TITLE=
NUXT_PUBLIC_APP_SLOGAN=
NUXT_PUBLIC_APP_AUTHOR=
NUXT_PUBLIC_APP_AVATAR_URL=

# Map provider (maplibre/mapbox)
NUXT_PUBLIC_MAP_PROVIDER=maplibre
# MapTiler access token for MapLibre
NUXT_PUBLIC_MAP_MAPLIBRE_TOKEN=
# Mapbox access token for Mapbox
NUXT_PUBLIC_MAPBOX_ACCESS_TOKEN=

# Mapbox unrestricted token (optional, reverse geocoding)
NUXT_MAPBOX_ACCESS_TOKEN=

# Storage provider (local, s3 or openlist)
NUXT_STORAGE_PROVIDER=local
NUXT_PROVIDER_LOCAL_PATH=/app/data/storage

# Session password (32‑char random string, required)
NUXT_SESSION_PASSWORD=
# Secret key for stable signing og images
# Use: npx nuxt-og-image generate-secret
NUXT_OG_IMAGE_SECRET=
```

### Pull Image

Use the published image on GitHub Container Registry and Docker Hub. Choose the source that works best for your network:

#### [GitHub Container Registry (GHCR)](https://github.com/HoshinoSuzumi/chronoframe/pkgs/container/chronoframe)

```bash
docker pull ghcr.io/hoshinosuzumi/chronoframe:latest
```

#### [Docker Hub](https://hub.docker.com/r/hoshinosuzumi/chronoframe)

```bash
docker pull hoshinosuzumi/chronoframe:latest
```

### Docker

Run with customized environment variables:

```bash
docker run -d --name chronoframe -p 3000:3000 -v $(pwd)/data:/app/data --env-file .env ghcr.io/hoshinosuzumi/chronoframe:latest
```

### Docker Compose

Create docker-compose.yml:

```yaml
services:
  chronoframe:
    image: ghcr.io/hoshinosuzumi/chronoframe:latest
    container_name: chronoframe
    restart: unless-stopped
    ports:
      - '3000:3000'
    volumes:
      - ./data:/app/data
    env_file:
      - .env
```

Start:

```bash
docker compose up -d
```

## 📖 User Guide

> If `CFRAME_ADMIN_EMAIL` and `CFRAME_ADMIN_PASSWORD` are not set, the default admin account is:
>
> - Email: `admin@chronoframe.com`
> - Password: `CF1234@!`

### Logging into the Dashboard

1. Click avatar to sign in with GitHub OAuth or use email/password login

### Uploading Photos

1. Go to the dashboard at /dashboard
2. On the Photos page, select and upload images (supports batch & drag-and-drop)
3. System will automatically parse EXIF data, generate thumbnails, and perform reverse geocoding

## 📸 Screenshots

![Gallery](./docs/images/screenshot1.png)
![Photo Detail](./docs/images/screenshot2.png)
![Map Explore](./docs/images/screenshot3.png)
![Dashboard](./docs/images/screenshot4.png)

## 🛠️ Development

### Requirements

- Node.js 18+
- pnpm 9.0+

### Install dependencies

```bash
# With pnpm (recommended)
pnpm --dir web install

# Or with other package managers
npm install
yarn install
```

### Configure environment variables

```bash
cp .env.example .env
```

### Build and start locally

```bash
pnpm --dir web install
pnpm --dir web build:deps
pnpm --dir web generate
go run ./cmd/chronoframe
```

The Go server serves `web/.output/public` at http://localhost:3000. Set `CFRAME_WEB_DIR` and `DATABASE_URL` to absolute paths when starting outside the repository root.

### Project Structure

```
chronoframe/
├── web/                    # Static Nuxt frontend
│   ├── app/                # Pages, components, composables, stores
│   ├── packages/           # Local frontend packages
│   └── shared/             # Shared TypeScript contracts
├── cmd/chronoframe/        # Go backend and HTTP routes
├── data/                   # SQLite and mounted media data (ignored)
└── Dockerfile              # Production image
```

### Build commands

```bash
# Build frontend dependencies
pnpm --dir web build:deps

# Build only dependencies
pnpm --dir web build:deps

# Production build
pnpm --dir web build

# Generate static frontend
pnpm --dir web generate

# Run backend
go run ./cmd/chronoframe
```

## 🤝 Contributing

Contributions are welcome! Please:

1. Fork the repo
2. Create a feature branch (git checkout -b feature/amazing-feature)
3. Commit changes (git commit -m 'Add some amazing feature')
4. Push to branch (git push origin feature/amazing-feature)
5. Open a Pull Request

### Coding Guidelines

- Use TypeScript for type safety
- Follow ESLint and Prettier conventions
- Update documentation accordingly

## 📄 License

This project is licensed under the MIT License.

## 👤 Author

**Timothy Yin**

- Email: master@uniiem.com
- GitHub: @HoshinoSuzumi
- Website: bh8.ga
- Gallery: lens.bh8.ga

## ❓ FAQ

<details>
  <summary>How is the admin user created?</summary>
  <p>
    On first startup, an admin user is created based on <code>CFRAME_ADMIN_EMAIL</code>, <code>CFRAME_ADMIN_NAME</code>, and <code>CFRAME_ADMIN_PASSWORD</code>. The email must match your GitHub account email used for login.
  </p>
</details>
<details>
  <summary>Which image formats are supported?</summary>
  <p>
    Supported formats: JPEG, PNG, HEIC/HEIF, MOV (for Live Photos).
  </p>
</details>
<details>
  <summary>Why can’t I use GitHub/Local storage?</summary>
  <p>
    Currently only S3-compatible storage is supported. GitHub and local storage support is planned.
  </p>
</details>
<details>
  <summary>Why is a map service required and how to configure it?</summary>
  <p>
    The map is used to browse photo locations and render mini-maps in photo details. Currently Mapbox is used. After registering, <a href="https://console.mapbox.com/account/access-tokens/">get an access token</a> and set it to the <code>MAPBOX_TOKEN</code> variable.
  </p>
</details>
<details>
  <summary>Why wasn’t my MOV file recognized as a Live Photo?</summary>
  <p>
    Ensure the image (.heic) and video (.mov) share the same filename (e.g., <code>IMG_1234.heic</code> and <code>IMG_1234.mov</code>). Upload order does not matter. If not recognized, you can trigger pairing manually from the dashboard.
  </p>
</details>
<details>
  <summary>How do I import existing photos from storage?</summary>
  <p>
    Direct import of existing photos is not yet supported. A directory scanning import feature is planned.
  </p>
</details>

## 🙏 Acknowledgements

This project was inspired by Afilmory, another excellent personal gallery project.

Thanks to the following open-source projects and libraries:

- [Nuxt](https://nuxt.com/)
- [TailwindCSS](https://tailwindcss.com/)
- [Go](https://go.dev/)

## ⭐️ Star History

<a href="https://star-history.dera.page/#HoshinoSuzumi/chronoframe&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://star-history.dera.page/svg?repos=HoshinoSuzumi/chronoframe&type=date&theme=dark&legend=top-left" />
   <source media="(prefers-color-scheme: light)" srcset="https://star-history.dera.page/svg?repos=HoshinoSuzumi/chronoframe&type=date&legend=top-left" />
   <img alt="Star History Chart" src="https://star-history.dera.page/svg?repos=HoshinoSuzumi/chronoframe&type=date&legend=top-left" />
 </picture>
</a>
