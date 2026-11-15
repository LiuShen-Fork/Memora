# Nothing in the frontend or the Go build depends on the machine the image runs
# on: the bundle is plain JavaScript, and with CGO off Go cross-compiles. Both
# stages therefore run on the build machine, once, and the arm64 image copies
# their result instead of rebuilding it under QEMU — an emulated pnpm install
# alone used to cost hours.
FROM --platform=$BUILDPLATFORM node:22.22.3-alpine AS web-build
ENV PNPM_HOME="/pnpm"
ENV PATH="$PNPM_HOME:$PATH"
RUN corepack enable
WORKDIR /src

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/packages/memora-webgl-image/package.json ./packages/memora-webgl-image/
RUN --mount=type=cache,id=pnpm,target=/pnpm/store pnpm install --frozen-lockfile

COPY web ./

# Set here and not at the top of the stage on purpose. An ENV defined before
# the install step changes the cache key of everything after it, so the
# dependency install was thrown away on every release — the one thing that is
# worth keeping.
ARG MEMORA_VERSION=dev
ENV MEMORA_VERSION=$MEMORA_VERSION

RUN NODE_OPTIONS="--max-old-space-size=4096" pnpm run build:deps \
    && NODE_OPTIONS="--max-old-space-size=8192" pnpm generate \
    && find .output/public -type f -name '*.map' -delete

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-build
# TARGETARCH is set by BuildKit for every target platform; the compiler runs
# natively here and emits the arm64 binary itself.
ARG TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd ./cmd
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build -trimpath -ldflags='-s -w' -o /out/memora ./cmd/memora

# FFmpeg gained HEIC/HEIF reading in 7.0; the 6.1 build shipped with Alpine 3.22
# cannot open HEIC uploads at all.
FROM alpine:3.24 AS runtime
# The packages come first because they are the slowest step here — on arm64 apk
# runs under emulation. A release-specific label above this line would change
# the cache key and reinstall them on every release.
RUN apk add --no-cache ca-certificates ffmpeg perl exiftool su-exec \
    && addgroup -S memora \
    && adduser -S -G memora -h /app memora \
    && mkdir -p /app/data /app/web \
    && chown -R memora:memora /app
# The label below is the only place the published image records which release it
# is; without it GHCR shows a digest and nothing else.
ARG MEMORA_VERSION=dev
LABEL org.opencontainers.image.title="Memora" \
    org.opencontainers.image.version="$MEMORA_VERSION" \
    org.opencontainers.image.description="Self-hosted photo gallery with EXIF, maps and Live Photos" \
    org.opencontainers.image.source="https://github.com/LiuShen-Fork/Memora" \
    org.opencontainers.image.licenses="MIT"
WORKDIR /app

COPY --from=go-build /out/memora /usr/local/bin/memora
COPY --from=web-build /src/.output/public /app/web
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh

# No USER here on purpose: the entrypoint needs root for the one step that
# takes ownership of the mounted data directory, and it execs the server as
# memora. A container started with --user (or a Kubernetes securityContext)
# runs the entrypoint as that user, which skips the ownership step.
EXPOSE 3000
VOLUME ["/app/data"]

ENV MEMORA_ADDR=:3000 \
    MEMORA_DATA_DIR=/app/data \
    MEMORA_WEB_DIR=/app/web \
    DATABASE_URL=/app/data/app.sqlite3 \
    MEMORA_FFMPEG_PATH=/usr/bin/ffmpeg \
    MEMORA_FFPROBE_PATH=/usr/bin/ffprobe \
    EXIFTOOL_PATH=/usr/bin/exiftool \
    SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:3000/api/health || exit 1

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/usr/local/bin/memora"]
