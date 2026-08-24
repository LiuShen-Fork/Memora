# 开发指南

ChronoFrame 使用静态 Nuxt 前端和一个 Go HTTP 服务。Go 服务负责静态文件、
`/api`、`/storage`、`/image`、`/thumb` 路由以及媒体队列。

## 构建与启动

```bash
pnpm install
pnpm build:deps
pnpm generate
go test ./...
go run ./cmd/chronoframe
```

默认数据库为 `./data/app.sqlite3`。Docker 必须保留 `./data:/app/data` 挂载。
媒体处理需要 FFmpeg、FFprobe 和 ExifTool；如果不在 `PATH` 中，请配置
`CFRAME_FFMPEG_PATH`、`CFRAME_FFPROBE_PATH` 和 `EXIFTOOL_PATH`。

## 目录

- `cmd/chronoframe/`：Go 路由、SQLite、存储适配器和队列。
- `app/`：Nuxt 页面、组件、状态和 composables。
- `packages/webgl-image/`：WebGL 图片查看器。
- `shared/`：前端共享类型和接口契约。

## 约束

- 保持现有 SQLite 表结构、JSON 字段和数据路径兼容。
- 不要新增 Nitro 路由、Drizzle 迁移或 Node 后端。
- 媒体、EXIF、缩略图和地理编码等耗时工作必须进入持久化队列。
- 提交前运行 `go test ./...`、`go vet ./...`、`pnpm lint` 和 `pnpm generate`。
