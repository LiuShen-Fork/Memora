# 更新指南

升级前请备份 `data/`、`.env` 和 `docker-compose.yml`。

## Docker Compose

```bash
docker compose pull
docker compose up -d
docker compose logs -f chronoframe
```

指定版本时，将镜像改为 `ghcr.io/hoshinosuzumi/chronoframe:v1.2.3`。

## 数据库兼容性

Go 服务启动时会检查并创建缺失的兼容表和索引，不需要 Node、Nitro、Drizzle
或手动迁移命令。升级时必须保持 Docker 挂载 `./data:/app/data` 不变。
