# ChronoFrame Coding Guidelines

ChronoFrame is a static Nuxt 4 frontend served by a Go HTTP backend. SQLite schema and the `./data:/app/data` Docker mount are compatibility contracts.

## Architecture

- `app/`: Vue/Nuxt static client, Pinia stores, composables, maps, and WebGL viewer.
- `cmd/chronoframe/`: Go server, SQLite access, authentication, storage adapters, HTTP routes, and durable media queue.
- `packages/webgl-image/`: local WebGL viewer package.
- `shared/`: client/server TypeScript contracts.

The production process is one Go binary. It serves `.output/public` (or `CFRAME_WEB_DIR`) and owns all `/api`, `/storage`, `/image`, and `/thumb` routes. Do not add Nitro API routes or a second Node server.

## Development

```bash
pnpm install
pnpm build:deps
pnpm generate
go run ./cmd/chronoframe
```

On PowerShell, use an absolute database path when the working directory may vary:

```powershell
$env:DATABASE_URL = (Resolve-Path "data/app.sqlite3").Path
$env:CFRAME_WEB_DIR = (Resolve-Path ".output/public").Path
go run ./cmd/chronoframe
```

Useful checks: `go test ./...`, `go test -race ./...`, `go vet ./...`, `pnpm lint`, and `pnpm generate`.

## Runtime contracts

- Preserve the existing SQLite tables, JSON columns, timestamps, WAL mode, and data paths.
- Keep storage providers compatible with local, S3-compatible, and OpenList configurations.
- Uploads use `POST /api/photos`, a direct `PUT` to the returned URL, and a durable queue task. Do not collapse these steps.
- Long media, EXIF, thumbnail, reverse-geocoding, and metadata rewrites belong in the queue, not in request handlers.
- Use existing Go helpers and shared types. Keep handlers split by domain.
- Map providers support MapLibre, Mapbox, and AMap. Preserve WGS84/GCJ-02 conversion behavior.

## Configuration

FFmpeg, FFprobe, and ExifTool are runtime dependencies. Configure local binaries with `CFRAME_FFMPEG_PATH`, `CFRAME_FFPROBE_PATH`, and `EXIFTOOL_PATH`; the Docker image installs them at `/usr/bin/ffmpeg`, `/usr/bin/ffprobe`, and `/usr/bin/exiftool`.

Do not commit `data/`, database journals, generated JSON, executables, or build output. The Docker data mount must remain `./data:/app/data`.

## Frontend conventions

Use the existing Nuxt UI components, Tailwind tokens, and shared composables. Keep tables horizontally scrollable inside their own container, use truncation for dense descriptions, retain full text in detail views, and avoid making large gallery requests block route shells.
