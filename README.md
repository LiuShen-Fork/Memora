# Memora

Memora is a self-hosted photo gallery for browsing, organizing, and exploring photos on a map. It supports EXIF metadata, reverse geocoding, albums, Live/Motion Photos, multiple storage backends, and responsive desktop/mobile views.

## What Is Different

Memora is a substantial rewrite of the original Nuxt application:

- The production server is a compact Go process serving the API and generated static frontend.
- SQLite schema and Docker `/app/data` mounts remain compatible with existing ChronoFrame deployments.
- FFmpeg handles thumbnails and Live/Motion Photo processing; ExifTool handles metadata extraction.
- Node.js is only required to rebuild the frontend, not to run production.

The maintained repository is [LiuShen-Fork/Memora](https://github.com/LiuShen-Fork/Memora).

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

Keep the existing `./data:/app/data` mount when migrating from ChronoFrame. See [the configuration guide](https://chronoframe.bh8.ga/guide/configuration.html) for all environment variables. Storage and application settings can also be changed from the dashboard.

### Local Development

Requirements: Go 1.23+, Node.js 18+, and pnpm 9+.

```bash
pnpm --dir web install
pnpm --dir web generate
go run ./cmd/chronoframe
```

Open <http://127.0.0.1:3000>. The Go process serves `web/.output/public` and the API on the same port. Install FFmpeg and ExifTool, or set `CFRAME_FFMPEG_PATH`, `CFRAME_FFPROBE_PATH`, and `EXIFTOOL_PATH` to their executable paths.

## Project Layout

```text
web/                  Static Nuxt frontend source
cmd/chronoframe/      Go server, API handlers, queue, and storage adapters
data/                 SQLite database and mounted media (do not replace)
Dockerfile            Production image definition
```

## Contributing

Pull requests are welcome. Run `go test ./...` and `pnpm --dir web generate` before submitting changes.

## Credits

Memora is inspired by and substantially based on [HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe). Please see the original project for its design history and upstream work.

## License

Memora is released under the MIT License. See [LICENSE](LICENSE).
