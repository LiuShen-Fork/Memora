FROM node:22.22.3-alpine AS web-build
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable
WORKDIR /src

COPY package.json pnpm-lock.yaml pnpm-workspace.yaml ./
COPY packages/webgl-image/package.json ./packages/webgl-image/
RUN --mount=type=cache,id=pnpm,target=/pnpm/store pnpm install --frozen-lockfile

COPY . .
RUN NODE_OPTIONS="--max-old-space-size=4096" pnpm run build:deps \
    && NODE_OPTIONS="--max-old-space-size=8192" pnpm generate \
    && find .output/public -type f -name '*.map' -delete

FROM golang:1.25-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/chronoframe ./cmd/chronoframe

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates ffmpeg perl exiftool \
    && addgroup -S chronoframe \
    && adduser -S -G chronoframe -h /app chronoframe \
    && mkdir -p /app/data /app/web \
    && chown -R chronoframe:chronoframe /app
WORKDIR /app

COPY --from=go-build /out/chronoframe /usr/local/bin/chronoframe
COPY --from=web-build /src/.output/public /app/web

USER chronoframe
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

ENTRYPOINT ["/usr/local/bin/chronoframe"]
