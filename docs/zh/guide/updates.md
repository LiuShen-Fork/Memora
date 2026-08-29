# 升级指南

本文说明 Memora 的常规升级流程，以及从原项目
[HoshinoSuzumi/chronoframe](https://github.com/HoshinoSuzumi/chronoframe)
迁移到 Memora 的实际兼容范围。Memora 是基于原项目的硬分叉，生产服务改为
Go，前端改为生成后的静态文件；数据库和 Docker 数据挂载路径保持不变，但运行时
的迁移机制已经不同。

## 每次升级前

1. 停止旧进程或容器，不要让两个进程同时打开同一个 SQLite 数据库。
2. 完整备份 `data/`、`.env` 以及 Docker Compose 文件或启动命令。如果照片存放在
   OpenList、S3 等外部服务，还要单独确认并备份这些对象；它们不在 SQLite 备份中。

```bash
docker compose down
stamp=$(date +%Y%m%d-%H%M%S)
mkdir -p "backups/$stamp"
cp -a data .env docker-compose.yml "backups/$stamp/"
```

Docker 挂载必须保持原样：

```text
./data:/app/data
```

升级过程中不要删除或重新创建 `data/app.sqlite3`。

### 容器目录权限

生产镜像会使用专用的非 root 用户和用户组 `memora` 运行。首次启动前，请将宿主机挂载
目录授权给这个身份。不要直接假定固定的 UID/GID，应从实际镜像中读取：

```bash
docker run --rm --entrypoint id ghcr.io/liushen-fork/memora:1.1.2 memora
# 示例输出：uid=100(memora) gid=101(memora)
sudo chown -R 100:101 data
sudo chmod -R u+rwX data
```

将命令中的 `100:101` 替换为镜像打印出的 UID/GID。从 ChronoFrame 迁移已有 `data/`
目录后也要执行同样的授权，否则 SQLite、日志、缩略图和生成媒体可能因权限不足而失败。

## 从 ChronoFrame 迁移到 Memora

### 推荐路径

推荐以 **ChronoFrame `1.0.0-RC4`** 作为迁移起点。RC4 包含原项目完整的 Drizzle
顺序迁移。如果当前版本更早，先让 ChronoFrame 升级到 RC4，并确认原程序迁移完成、
可以正常登录和浏览照片，再切换到 Memora。

1. 备份上一节列出的文件和外部存储。
2. 停止现有的 Memora（如果已经安装）。
3. 使用相同的 `./data:/app/data` 挂载启动
   `ghcr.io/hoshinosuzumi/chronoframe:v1.0.0-rc.4`，或者在 ChronoFrame 源码中执行
   `pnpm db:migrate`，目标指向备份数据库。RC4 容器迁移完成后仍会继续运行，确认旧相册
   正常后再停止它。
4. 等待 RC4 迁移完成，确认旧相册工作正常。
5. 停止 ChronoFrame，再使用相同的数据挂载启动 Memora。

RC4 启动时会读取 Drizzle 的迁移历史表，并按顺序执行所有尚未执行的迁移。因此，较早
的 ChronoFrame 数据库通常可以通过启动 RC4 自动补齐到 RC4 结构，但前提是数据库可写、
迁移历史完整且备份可用；这不能替代升级前的备份和检查。

Docker 示例：

```bash
docker run -d --name chronoframe-rc4 \
  -v "$(pwd)/data:/app/data" \
  --env-file .env \
  -e DATABASE_URL=/app/data/app.sqlite3 \
  ghcr.io/hoshinosuzumi/chronoframe:v1.0.0-rc.4

docker logs -f chronoframe-rc4
# 确认 RC4 相册正常后，停止旧服务
docker stop chronoframe-rc4
docker rm chronoframe-rc4

docker run -d --name memora -p 3000:3000 \
  -v "$(pwd)/data:/app/data" --env-file .env \
  ghcr.io/liushen-fork/memora:1.1.2
```

如果使用源码，请在 ChronoFrame 仓库中先执行 `pnpm install`，再设置
`DATABASE_URL` 并执行 `pnpm db:migrate`。迁移完成后再启动 Memora。

### 旧版能否直接跳到 Memora？

不能把“启动时会创建缺失表”理解成“任意旧版都能自动升级”。ChronoFrame 的历史迁移
按顺序完成以下变化：

| 迁移 | 主要内容 |
| --- | --- |
| `0000` | 创建 `photos`、`users` |
| `0001` | 照片坐标、国家和城市字段 |
| `0002` | Live Photo 标记和视频字段 |
| `0003` | 缩略图 key |
| `0004`-`0006` | 处理队列及队列负载兼容 |
| `0007` | 照片反应 |
| `0008` | 相册及照片关联表 |
| `0009` | 设置和存储提供商设置 |
| `0010` | 隐藏相册 |
| `0011` | 位置语言枚举更新 |

当前 Go 后端没有通用的 Drizzle 迁移执行器。启动时只会创建缺失的表和索引、补充缺失
的默认设置；它不会可靠地为已有表补列，也不会重放上述历史 `ALTER TABLE` 步骤或修复
中断的旧迁移。因此：

- 已经迁移到 RC4 的数据库是受支持的交接点。
- 更早的 ChronoFrame 数据库应先升级到 RC4。
- 只有在你自行确认数据库已经包含当前全部表和字段时，才可以尝试直接切换；这不代表
  Memora 对旧版直升提供保证。
- 如果 Memora 报告数据库结构错误，应立即停止并从备份恢复，不要删除数据库或让半修复的
  副本成为唯一数据源。

Memora 启动时还会补充默认设置；如果数据库已有用户，会自动关闭首次启动向导。已有用户
却再次看到向导时，应检查数据库路径和结构，而不是新建数据库。

不必先安装 Memora `1.0.0` 再升级到 `1.1.2`。这两个版本都使用 Go 后端，不会重放
ChronoFrame 的 Drizzle 历史迁移。数据库已经通过 RC4 后，可以根据 Release Notes 的兼容
说明直接切换到较新的 Memora 版本。先运行 Memora `1.0.0` 再逐步升级也是可行的分阶段
方案，但它主要用于验证切换过程，并不代表每个 Memora 版本都会执行一次数据库迁移。

## Memora 常规升级

生产环境建议使用明确的版本标签，并尽量逐版本升级，便于回滚。更新 Compose 中的镜像
标签后执行：

```bash
docker compose pull
docker compose up -d
docker compose logs -f memora
```

当前版本镜像为：

```text
ghcr.io/liushen-fork/memora:1.1.2
```

只修改镜像名称或标签，保留原有的 volumes、环境变量和 `./data:/app/data` 挂载。生产
运行时不需要 Node.js、Nitro、Drizzle 或手动迁移命令；从源码重新生成前端时才需要
Node.js 和 pnpm。

## 发布版本与后台列表

前端 package 版本固定为 `0.0.0-dev` 用于本地开发。发布时在 GitHub Actions
中输入如 `1.1.2` 即可，脚本会自动创建 `v1.1.2` 标签，并通过
`MEMORA_VERSION` 注入前端构建，无需手动修改 package 文件。同一 Action 会推送对应的 Docker
标签，并在 GitHub Release 附上带 commit 链接的更新说明和多平台二进制程序。

相册库和队列管理页面按页请求数据，而不是一次性加载整张表。接口支持 `page` 和 `pageSize`（最大 100），
返回 `data`、`page`、`pageSize`、`total` 和 `totalPages`；省略分页参数时仍保留兼容旧调用的返回格式。

## 数据和生成文件

SQLite 文件及 `data/` 挂载结构会保留。存放在远程提供商中的原始照片仍由该提供商管理。
Memora 会通过处理队列生成或重新生成缩略图和 Live Photo MP4；这些生成文件不是原始
照片的替代品。Live Photo 处理失败时会先按普通照片展示，之后可以从后台重试任务。

跨越涉及数据库字段、存储 key 或生成媒体变化的版本升级前，请先阅读对应的 Release
Notes。
