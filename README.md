# Memora

**English** · [简体中文](README_zh.md)

A self-hosted photo gallery: browse a masonry wall, organize albums, and explore your photos on a map. EXIF, reverse geocoding, Live/Motion Photos, three storage backends, and a responsive desktop/mobile UI.

Memora is a Go rewrite of [HoshinoSuzumi/ChronoFrame](https://github.com/HoshinoSuzumi/ChronoFrame). It keeps the same SQLite schema and the same `data/` layout, so an existing ChronoFrame deployment moves over by pointing the new image at the same volume.

## Features

- Masonry wall, albums, search, filters, reactions, and per-photo metadata editing
- Map view with MapLibre, Mapbox, or AMap; automatic reverse geocoding
- JPEG, PNG, WebP, GIF, BMP, TIFF, HEIC/HEIF, and Live/Motion Photos (HEIC is converted to JPEG on upload)
- Local disk, S3-compatible object storage, or OpenList
- Background queue for thumbnails, metadata, and location with retry
- Six UI languages, light/dark themes, installable PWA

## Quick start

```bash
docker run -d --name memora -p 3000:3000 \
  -v "$(pwd)/data:/app/data" \
  ghcr.io/liushen-fork/memora:latest
```

Or with Compose:

```yaml
services:
  memora:
    image: ghcr.io/liushen-fork/memora:latest
    container_name: memora
    restart: unless-stopped
    ports:
      - '3000:3000'
    volumes:
      - ./data:/app/data
```

Open `http://localhost:3000`. The first visit runs a setup wizard: administrator account, site details, storage, and map provider. Everything it asks for can be changed later in the dashboard.

No environment variables are required. The image already sets the listen address, data directory, database path, and media tool paths.

**Back up the whole `data/` directory**, not just the database — the generated session key and, depending on the storage provider, your originals live there too.

## Configuration

Only these are worth setting by hand:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MEMORA_SESSION_SECRET` | generated into `data/.session-secret` | Signs session cookies. Set it when several replicas must share sessions. |
| `NUXT_SESSION_PASSWORD` | — | Upstream's name for the same secret, still accepted. |
| `CFRAME_SESSION_SECRET` | — | Deprecated old name; the log warns when it is used. |
| `MEMORA_ADMIN_NAME` / `MEMORA_ADMIN_EMAIL` / `MEMORA_ADMIN_PASSWORD` | `admin` / empty / empty | Pre-fills the setup wizard; it does not create or change an account by itself. |
| `DATABASE_URL` | `./data/app.sqlite3` | SQLite file. |
| `MEMORA_DATA_DIR` | `./data` | Data directory. |
| `MEMORA_WEB_DIR` | `./web/.output/public` | Generated frontend. |
| `MEMORA_ADDR` | `:3000` | Listen address. |
| `MEMORA_WORKERS` | the queue's own setting | Number of media workers to start with. |
| `MEMORA_MEDIA_MAX_MB` | `32` | Fallback media processing limit. |
| `MEMORA_FFMPEG_PATH` / `MEMORA_FFPROBE_PATH` / `EXIFTOOL_PATH` | `ffmpeg` / `ffprobe` / `exiftool` | Media tools. |
| `NUXT_OAUTH_GITHUB_CLIENT_ID` / `_SECRET` | empty | Enables GitHub sign-in. |

Every `MEMORA_*` name also answers to its old `CFRAME_*` spelling; the first start that still uses one logs a deprecation warning naming the new variable.

The session secret is resolved in this order: an environment variable, then `data/.session-secret`, then a newly generated key written to that file with mode `0600`. A key that is already stored is reused, so restarts keep clients signed in.

Storage, map, upload limit, language, theme, home view, header layout, and social links are all dashboard settings — see **Dashboard → Settings**.

## Development

Requirements: Go 1.25+, Node.js 22+, pnpm, FFmpeg, and ExifTool.

```bash
pnpm --dir web install          # once
pnpm --dir web dev:debug        # Go API on :3000, Nuxt on :3001
```

Open `http://127.0.0.1:3001`. The dev server proxies `/api`, `/storage`, `/image`, and `/thumb` to the Go process. The backend reads a root `.env` when present.

To run the production-shaped static frontend instead:

```bash
pnpm --dir web generate         # build the static client
go run ./cmd/memora             # serve it and the API on :3000
```

Before submitting a change:

```bash
gofmt -l cmd/memora
go vet ./cmd/...
go test ./...
cd web && npx oxlint && npx nuxi generate
```

`npm run build` is a no-op on Windows; run `npx nuxi generate` directly from `web/`.

## Project layout

```text
cmd/memora/    Go server: API handlers, queue, storage adapters, schema
web/           Nuxt frontend (static output in web/.output/public)
data/          SQLite database, generated secret, and local media
Dockerfile     Production image (Go binary + static frontend)
```

## Credits and license

Based on [HoshinoSuzumi/ChronoFrame](https://github.com/HoshinoSuzumi/ChronoFrame). Released under the MIT License; see [LICENSE](LICENSE).
