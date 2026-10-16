FROM node:22.22.3-alpine AS web-build
ARG MEMORA_VERSION=dev
ENV MEMORA_VERSION=$MEMORA_VERSION
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable
WORKDIR /src

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/packages/memora-webgl-image/package.json ./packages/memora-webgl-image/
RUN --mount=type=cache,id=pnpm,target=/pnpm/store pnpm install --frozen-lockfile

COPY web ./
RUN NODE_OPTIONS="--max-old-space-size=4096" pnpm run build:deps \
    && NODE_OPTIONS="--max-old-space-size=8192" pnpm generate \
    && find .output/public -type f -name '*.map' -delete

FROM golang:1.25-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/memora ./cmd/memora

# FFmpeg gained HEIC/HEIF reading in 7.0; the 6.1 build shipped with Alpine 3.22
# cannot open HEIC uploads at all.
FROM alpine:3.24 AS runtime
# The label below is the only place the published image records which release it
# is; without it GHCR shows a digest and nothing else.
ARG MEMORA_VERSION=dev
LABEL org.opencontainers.image.title="Memora" \
    org.opencontainers.image.version="$MEMORA_VERSION" \
    org.opencontainers.image.description="Self-hosted photo gallery with EXIF, maps and Live Photos" \
    org.opencontainers.image.source="https://github.com/LiuShen-Fork/Memora" \
    org.opencontainers.image.licenses="MIT"
RUN apk add --no-cache ca-certificates ffmpeg perl exiftool \
    && addgroup -S memora \
    && adduser -S -G memora -h /app memora \
    && mkdir -p /app/data /app/web \
    && chown -R memora:memora /app
WORKDIR /app

COPY --from=go-build /out/memora /usr/local/bin/memora
COPY --from=web-build /src/.output/public /app/web

USER memora
EXPOSE 3000
VOLUME ["/app/data"]

ENV CFRAME_ADDR=:3000 \
    CFRAME_DATA_DIR=/app/data \
    CFRAME_WEB_DIR=/app/web \
    DATABASE_URL=/app/data/app.sqlite3 \
    CFRAME_FFMPEG_PATH=/usr/bin/ffmpeg \
    CFRAME_FFPROBE_PATH=/usr/bin/ffprobe \
    EXIFTOOL_PATH=/usr/bin/exiftool \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:3000/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/memora"]
