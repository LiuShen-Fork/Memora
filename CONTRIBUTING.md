# Contributing

ChronoFrame has a static Nuxt frontend and a Go backend. The Go process serves
the generated frontend and owns the API, storage, media routes, and queue.

## Requirements

- Go 1.25 or newer
- Node.js 22 and pnpm 10 for the frontend build
- FFmpeg, FFprobe, and ExifTool on the local `PATH` (or configure their paths)

## Local Setup

```bash
pnpm install
pnpm build:deps
pnpm generate
go test ./...
go run ./cmd/chronoframe
```

The service listens on `http://127.0.0.1:3000`, serves `.output/public`, and
uses `./data/app.sqlite3` by default. On PowerShell, absolute paths avoid
working-directory surprises:

```powershell
$env:DATABASE_URL = (Resolve-Path "data/app.sqlite3").Path
$env:CFRAME_WEB_DIR = (Resolve-Path ".output/public").Path
go run ./cmd/chronoframe
```

Keep the existing SQLite schema and the Docker mount `./data:/app/data`.
Do not add Nitro API routes, a Node server, or a second database layer.

## Checks

```bash
go test ./...
go test -race ./...
go vet ./...
pnpm lint
pnpm generate
git diff --check
```

## Code Layout

- `cmd/chronoframe/`: Go HTTP handlers, SQLite access, storage, and queue.
- `app/`: Nuxt pages, components, stores, and composables.
- `packages/webgl-image/`: WebGL viewer package.
- `shared/`: frontend contracts and shared types.

Long media, EXIF, thumbnail, and reverse-geocoding work belongs in the durable
queue. Keep handlers small and reuse the existing storage and settings helpers.
