# Memora

Memora 是一个可自托管的个人相册，用于浏览、整理和在地图上探索照片。它支持 EXIF 信息、逆地理编码、相册、Live/Motion Photo、多种存储后端，以及桌面和移动端布局。

## 与原项目的区别

Memora 是对原 Nuxt 应用的大幅重构：

- 生产环境使用轻量 Go 进程，同时提供 API 和预生成的静态前端。
- SQLite 数据库结构和 Docker 的 `/app/data` 挂载保持兼容，原有数据无需迁移。
- 使用 FFmpeg 生成缩略图并处理 Live/Motion Photo，使用 ExifTool 提取元数据。
- Node.js 仅在重新构建前端时需要，生产运行不再需要 Node.js/Nitro 服务。

当前维护仓库为 [LiuShen-Fork/Memora](https://github.com/LiuShen-Fork/Memora)。

## 功能

- 响应式瀑布流相册浏览
- 相册、搜索、筛选、反应和照片信息编辑
- MapLibre、Mapbox、AMap 地图浏览
- JPEG、PNG、WebP、GIF、BMP、TIFF、HEIC/HEIF 和 Live/Motion Photo
- 本地文件系统、S3 兼容存储和 OpenList
- 自动生成缩略图、解析 EXIF、逆地理编码和持久化任务队列

## 快速开始

### Docker

```bash
docker run -d --name memora -p 3000:3000 \
  -v $(pwd)/data:/app/data --env-file .env \
  ghcr.io/liu-shen-fork/memora:latest
```

从 ChronoFrame 迁移时请保持原有的 `./data:/app/data` 挂载不变。完整环境变量请参考[配置指南](https://chronoframe.bh8.ga/zh/guide/configuration.html)，存储和应用设置也可以在后台修改。

### 本地开发

需要 Go 1.23+、Node.js 18+ 和 pnpm 9+：

```bash
pnpm --dir web install
pnpm --dir web generate
go run ./cmd/chronoframe
```

访问 <http://127.0.0.1:3000>。Go 进程会在同一端口提供 `web/.output/public` 静态前端和 API。请安装 FFmpeg、ExifTool，或通过 `CFRAME_FFMPEG_PATH`、`CFRAME_FFPROBE_PATH`、`EXIFTOOL_PATH` 指定可执行文件路径。

## 项目结构

```text
web/                  静态 Nuxt 前端源码
cmd/chronoframe/      Go 服务、API、任务队列和存储适配器
data/                 SQLite 数据库和挂载的媒体文件（不要替换）
Dockerfile            生产镜像定义
```

## 参与贡献

欢迎提交 Pull Request。提交前请运行 `go test ./...` 和 `pnpm --dir web generate`。

## 致谢

Memora 的设计和功能大量参考并基于 [HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe)。感谢原项目作者及贡献者提供的设计、功能和开源基础。

## 许可证

Memora 使用 MIT 许可证，详见 [LICENSE](LICENSE)。
