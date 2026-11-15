# Memora

[English](README.md) · **简体中文**

自托管的照片相册：瀑布流浏览、相册整理、在地图上探索照片。支持 EXIF、反向地理编码、Live/Motion Photo、三种存储后端，桌面与手机自适应。

Memora 是用 Go 重写的 [HoshinoSuzumi/ChronoFrame](https://github.com/HoshinoSuzumi/ChronoFrame)。SQLite 结构和 `data/` 目录布局与原版一致，已有的 ChronoFrame 部署把新镜像挂到同一个数据卷即可迁移。

## 功能

- 瀑布流照片墙、相册、搜索、筛选、访客表态、逐张编辑元数据
- MapLibre / Mapbox / 高德地图浏览，自动反向地理编码
- JPEG、PNG、WebP、GIF、BMP、TIFF、HEIC/HEIF 与 Live/Motion Photo（HEIC 上传时转成 JPEG）
- 本地磁盘、S3 兼容对象存储、OpenList 三种存储
- 后台队列处理缩略图、元数据与地点，失败可重试
- 六种界面语言、明暗主题、可安装为 PWA

## 快速开始

```bash
docker run -d --name memora -p 3000:3000 \
  -v "$(pwd)/data:/app/data" \
  ghcr.io/liushen-fork/memora:latest
```

或者用 Compose：

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

打开 `http://localhost:3000`，首次访问会进入安装向导：管理员账号、站点信息、存储、地图服务商。向导里填的内容之后都能在后台修改。

**不需要任何环境变量**，镜像里已经设好监听地址、数据目录、数据库路径和媒体工具路径。

**请备份整个 `data/` 目录**，而不只是数据库——自动生成的会话密钥和本地存储的原图都在里面。

## 配置

需要手动设置的只有下面这些：

| 变量 | 默认值 | 用途 |
| --- | --- | --- |
| `MEMORA_SESSION_SECRET` | 自动生成到 `data/.session-secret` | 会话 Cookie 的签名密钥。多副本部署必须显式设置同一个值。 |
| `NUXT_SESSION_PASSWORD` | — | 上游的同名变量，仍然可用。 |
| `CFRAME_SESSION_SECRET` | — | 旧的兼容名字，使用时会输出弃用警告。 |
| `MEMORA_ADMIN_NAME` / `MEMORA_ADMIN_EMAIL` / `MEMORA_ADMIN_PASSWORD` | `admin` / 空 / 空 | 预填安装向导；本身不会创建或修改账号。 |
| `DATABASE_URL` | `./data/app.sqlite3` | SQLite 文件。 |
| `MEMORA_DATA_DIR` | `./data` | 数据目录。 |
| `MEMORA_WEB_DIR` | `./web/.output/public` | 生成的前端目录。 |
| `MEMORA_ADDR` | `:3000` | 监听地址。 |
| `MEMORA_WORKERS` | 队列自身的设置 | 启动时的媒体工作进程数。 |
| `MEMORA_MEDIA_MAX_MB` | `32` | 媒体处理的兜底上限。 |
| `MEMORA_FFMPEG_PATH` / `MEMORA_FFPROBE_PATH` / `EXIFTOOL_PATH` | `ffmpeg` / `ffprobe` / `exiftool` | 媒体工具路径。 |
| `NUXT_OAUTH_GITHUB_CLIENT_ID` / `_SECRET` | 空 | 启用 GitHub 登录。 |

所有 `MEMORA_*` 变量都兼容旧的 `CFRAME_*` 写法；仍在使用旧名字时，启动日志会输出一次弃用警告并指出新名字。

会话密钥的取值顺序：环境变量 → `data/.session-secret` → 都没有就以 `0600` 权限生成并写入该文件。已有的密钥会被复用，重启不会让已登录的客户端掉线。

存储、地图、上传上限、语言、主题、首页视图、页头布局、社交链接都在后台设置里——见**控制台 → 站点设置**。

## 开发

需要 Go 1.25+、Node.js 22+、pnpm、FFmpeg、ExifTool。

```bash
pnpm --dir web install          # 只需一次
pnpm --dir web dev:debug        # Go API 在 :3000，Nuxt 开发服务器在 :3001
```

打开 `http://127.0.0.1:3001`。开发服务器把 `/api`、`/storage`、`/image`、`/thumb` 代理到 Go 进程。后端会读取根目录的 `.env`（如果有）。

想直接跑生产形态的静态前端：

```bash
pnpm --dir web generate         # 构建静态前端
go run ./cmd/memora             # 在 :3000 同时提供前端和 API
```

提交前请跑：

```bash
gofmt -l cmd/memora
go vet ./cmd/...
go test ./...
cd web && npx oxlint && npx nuxi generate
```

Windows 上 `npm run build` 是空操作，请直接在 `web/` 下运行 `npx nuxi generate`。

## 目录结构

```text
cmd/memora/    Go 服务端：API、队列、存储适配、数据库结构
web/           Nuxt 前端（静态产物在 web/.output/public）
data/          SQLite 数据库、生成的密钥、本地媒体
Dockerfile     生产镜像（Go 二进制 + 静态前端）
```

## 致谢与许可证

基于 [HoshinoSuzumi/ChronoFrame](https://github.com/HoshinoSuzumi/ChronoFrame)。以 MIT 许可证发布，见 [LICENSE](LICENSE)。
