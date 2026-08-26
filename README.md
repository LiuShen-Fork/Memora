# Memora

Original project: [HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe)

Memora is a self-hosted photo gallery for browsing, organizing, and exploring photos on a map. It supports EXIF metadata, reverse geocoding, albums, Live/Motion Photos, multiple storage backends, and responsive desktop/mobile views.

## What Is Different

Memora is a substantial rewrite of the original Nuxt application:

- The production server is a compact Go process serving the API and generated static frontend.
- SQLite schema and Docker `/app/data` mounts remain compatible with existing ChronoFrame deployments.
- FFmpeg handles thumbnails and Live/Motion Photo processing; ExifTool handles metadata extraction.
- Node.js is only required to rebuild the frontend, not to run production.

The maintained repository is [LiuShen-Fork/Memora](https://github.com/LiuShen-Fork/Memora). The published image is `ghcr.io/liushen-fork/memora` (Docker image names are conventionally lowercase).

## Features

- Gallery browsing with responsive masonry layout
- Albums, search, filters, reactions, and photo metadata editing
- Map exploration with MapLibre, Mapbox, or AMap
- JPEG, PNG, WebP, GIF, BMP, TIFF, HEIC/HEIF, and Live/Motion Photo support
- Local filesystem, S3-compatible, and OpenList storage
- Automatic thumbnails, EXIF parsing, reverse geocoding, and durable queues

## Quick Start

### Docker

```bash
docker run -d --name memora -p 3000:3000 \
  -v $(pwd)/data:/app/data --env-file .env \
  ghcr.io/liu-shen-fork/memora:latest
```

Keep the existing `./data:/app/data` mount when migrating from ChronoFrame. Storage and application settings can be changed from the dashboard, so most deployments need no extra environment variables.

## Configuration

On first start, set an administrator email and password. The database, media files, map settings, storage provider, upload limit, language, theme, and footer link can then be configured in the dashboard.

The usual `.env` only needs:

```dotenv
CFRAME_ADMIN_EMAIL=admin@example.com
CFRAME_ADMIN_PASSWORD=change-this-password
NUXT_SESSION_PASSWORD=use-a-long-random-secret
```

For local storage, keep the default `./data` directory. To use another storage provider, select it in **Dashboard → Settings → Storage** and enter its credentials. Map providers and API keys are configured in **Dashboard → Settings → Map and location**.

Optional startup variables:

| Variable | Purpose | Default |
| --- | --- | --- |
| `DATABASE_URL` | SQLite database path | `./data/app.sqlite3` |
| `CFRAME_DATA_DIR` | Application data directory | `./data` |
| `CFRAME_WEB_DIR` | Generated frontend directory | `./web/.output/public` |
| `CFRAME_ADDR` | Listen address | `:3000` |
| `CFRAME_MEDIA_MAX_MB` | Fallback media processing limit | `32` |
| `CFRAME_FFMPEG_PATH` | FFmpeg executable | `ffmpeg` |
| `CFRAME_FFPROBE_PATH` | FFprobe executable | `ffprobe` |
| `EXIFTOOL_PATH` | ExifTool executable | `exiftool` |

The maximum upload/processing size is configurable at runtime under **System Settings → File processing** and defaults to 32MB. Environment variables are retained for compatibility and automation.

### Local Development

Requirements: Go 1.23+, Node.js 18+, and pnpm 9+.

```bash
pnpm --dir web install
pnpm --dir web generate
go run ./cmd/memora
```

Open <http://127.0.0.1:3000>. The Go process serves `web/.output/public` and the API on the same port. Install FFmpeg and ExifTool, or set `CFRAME_FFMPEG_PATH`, `CFRAME_FFPROBE_PATH`, and `EXIFTOOL_PATH` to their executable paths.

## Project Layout

```text
web/                  Static Nuxt frontend source
cmd/memora/           Go server, API handlers, queue, and storage adapters
data/                 SQLite database and mounted media (do not replace)
Dockerfile            Production image definition
```

## Contributing

Pull requests are welcome. Run `go test ./...` and `pnpm --dir web generate` before submitting changes.

## Credits

Memora is inspired by and substantially based on [HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe). Please see the original project for its design history and upstream work.

## License

Memora is released under the MIT License. See [LICENSE](LICENSE).
