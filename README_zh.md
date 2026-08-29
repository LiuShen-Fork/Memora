# Memora

原项目地址：[HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe)

Memora 是一个可自托管的个人相册，用于浏览、整理和在地图上探索照片。它支持 EXIF 信息、逆地理编码、相册、Live/Motion Photo、多种存储后端，以及桌面和移动端布局。

## 与原项目的区别

Memora 是对原 Nuxt 应用的大幅重构：

- 生产环境使用轻量 Go 进程，同时提供 API 和预生成的静态前端。
- SQLite 数据库结构和 Docker 的 `/app/data` 挂载保持兼容，原有数据无需迁移。
- 使用 FFmpeg 生成缩略图并处理 Live/Motion Photo，使用 ExifTool 提取元数据。
- Node.js 仅在重新构建前端时需要，生产运行不再需要 Node.js/Nitro 服务。

当前维护仓库为 [LiuShen-Fork/Memora](https://github.com/LiuShen-Fork/Memora)，发布的 Docker 镜像为 `ghcr.io/liushen-fork/memora`（Docker 镜像名通常使用小写）。

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
  ghcr.io/liushen-fork/memora:latest
```

从 ChronoFrame 迁移时请保持原有的 `./data:/app/data` 挂载不变。大多数配置都可以在后台完成，无需填写大量环境变量。

## 配置

首次启动时设置管理员邮箱和密码，之后可以在后台配置数据库、媒体文件、地图服务、存储提供商、上传大小、语言、主题和页脚链接。

通常只需要在 `.env` 中填写：

```dotenv
CFRAME_ADMIN_EMAIL=admin@example.com
CFRAME_ADMIN_PASSWORD=change-this-password
NUXT_SESSION_PASSWORD=use-a-long-random-secret
```

使用本地存储时保持默认的 `./data` 目录即可。需要 S3 或 OpenList 时，在“仪表板 → 设置 → 存储”中选择提供商并填写凭据。地图服务和 API Key 位于“仪表板 → 设置 → 地图和位置”。

可选启动变量：

| 变量 | 用途 | 默认值 |
| --- | --- | --- |
| `DATABASE_URL` | SQLite 数据库路径 | `./data/app.sqlite3` |
| `CFRAME_DATA_DIR` | 应用数据目录 | `./data` |
| `CFRAME_WEB_DIR` | 静态前端目录 | `./web/.output/public` |
| `CFRAME_ADDR` | 监听地址 | `:3000` |
| `CFRAME_MEDIA_MAX_MB` | 媒体处理大小后备值 | `32` |
| `CFRAME_FFMPEG_PATH` | FFmpeg 可执行文件 | `ffmpeg` |
| `CFRAME_FFPROBE_PATH` | FFprobe 可执行文件 | `ffprobe` |
| `EXIFTOOL_PATH` | ExifTool 可执行文件 | `exiftool` |

最大上传和处理大小可在“系统设置 → 文件处理”中运行时修改，默认 32MB。环境变量仍保留用于兼容旧部署和自动化脚本。

### 本地开发

需要 Go 1.23+、Node.js 18+、pnpm 9+、FFmpeg 和 ExifTool。以下命令均在仓库根目录执行。

首次只需安装一次前端依赖：

```bash
pnpm --dir web install
```

日常调试时，使用以下命令同时启动 Go API 和支持热更新的前端：

```bash
pnpm --dir web dev:debug
```

访问 <http://127.0.0.1:3001>。Go 后端运行在 `3000` 端口，Nuxt 开发服务器会将 `/api`、`/storage`、`/image` 和 `/thumb` 代理到后端。后端会自动读取根目录的 `.env`；本地存储可不创建该文件，或复制 `.env.example` 后仅填写实际需要的配置。需要代理到其他后端时，在启动前设置 `MEMORA_BACKEND_ORIGIN`。

若要联调最终的静态前端，而不是使用热更新：

```bash
pnpm --dir web generate
go run ./cmd/memora
```

访问 <http://127.0.0.1:3000>。Go 进程会在同一端口提供 `web/.output/public` 静态前端和 API。当媒体工具不在 `PATH` 时，通过 `CFRAME_FFMPEG_PATH`、`CFRAME_FFPROBE_PATH`、`EXIFTOOL_PATH` 指定可执行文件路径。

提交前请运行：

```bash
go test ./...
pnpm --dir web generate
```

## 项目结构

```text
web/                  静态 Nuxt 前端源码
cmd/memora/           Go 服务、API、任务队列和存储适配器
data/                 SQLite 数据库和挂载的媒体文件（不要替换）
Dockerfile            生产镜像定义
```

## 参与贡献

欢迎提交 Pull Request。提交前请运行 `go test ./...` 和 `pnpm --dir web generate`。

## 致谢

Memora 的设计和功能大量参考并基于 [HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe)。感谢原项目作者及贡献者提供的设计、功能和开源基础。

## 许可证

Memora 使用 MIT 许可证，详见 [LICENSE](LICENSE)。
