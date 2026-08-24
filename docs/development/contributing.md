# Development Guide

ChronoFrame uses a static Nuxt client and one Go HTTP process. The Go process
serves `.output/public` and owns all `/api`, `/storage`, `/image`, and `/thumb`
routes.

## Build and Run

```bash
pnpm install
pnpm build:deps
pnpm generate
go test ./...
go run ./cmd/chronoframe
```

The default database is `./data/app.sqlite3`. Docker deployments must keep the
existing `./data:/app/data` mount. FFmpeg, FFprobe, and ExifTool are required
for media processing; set `CFRAME_FFMPEG_PATH`, `CFRAME_FFPROBE_PATH`, and
`EXIFTOOL_PATH` when they are not on `PATH`.

## Structure

- `cmd/chronoframe/`: Go handlers, SQLite access, storage adapters, and queue.
- `app/`: Nuxt pages, components, stores, and composables.
- `packages/webgl-image/`: WebGL image viewer.
- `shared/`: shared TypeScript contracts.

## Rules

- Preserve the existing SQLite tables and JSON/data paths.
- Do not add Nitro routes, Drizzle migrations, or a second Node backend.
- Keep expensive media and metadata work in the durable queue.
- Run `go test ./...`, `go vet ./...`, `pnpm lint`, and `pnpm generate` before a PR.
