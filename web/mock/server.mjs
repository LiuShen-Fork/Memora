#!/usr/bin/env node
/**
 * A stand-in for the Go backend, for building the dashboard without one.
 *
 * The Nuxt dev server already forwards /api, /storage, /image and /thumb to
 * MEMORA_BACKEND_ORIGIN, so pointing that at this process is the whole setup:
 *
 *   node mock/server.mjs                       # in one terminal
 *   MEMORA_BACKEND_ORIGIN=http://127.0.0.1:8787 npx nuxt dev
 *
 * It speaks the same shapes as cmd/memora — the settings descriptors are the
 * real ones, captured from the Go code into mock/fixtures — but the photos are
 * generated pictures and the queue is invented, so nothing here should be used
 * to judge backend behaviour. The dashboard is the point: every page can be
 * opened, screenshotted and clicked through without building the Go binary or
 * seeding a database.
 */

import { createServer } from 'node:http'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { deflateSync } from 'node:zlib'

const here = dirname(fileURLToPath(import.meta.url))
const port = Number(process.env.MOCK_PORT || 8787)

/* ------------------------------------------------------------------ *
 * PNG — a few kilobytes of encoder so the mock can serve real pictures
 * ------------------------------------------------------------------ */

const CRC_TABLE = (() => {
  const table = new Int32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    table[n] = c
  }
  return table
})()

function crc32(buf) {
  let c = -1
  for (let i = 0; i < buf.length; i++) {
    c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  }
  return (c ^ -1) >>> 0
}

function chunk(type, data) {
  const length = Buffer.alloc(4)
  length.writeUInt32BE(data.length)
  const body = Buffer.concat([Buffer.from(type, 'latin1'), data])
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(body))
  return Buffer.concat([length, body, crc])
}

function encodePNG(width, height, paint) {
  const raw = Buffer.alloc((width * 3 + 1) * height)
  let offset = 0
  for (let y = 0; y < height; y++) {
    raw[offset++] = 0 // filter: none
    for (let x = 0; x < width; x++) {
      const [r, g, b] = paint(x, y)
      raw[offset++] = r
      raw[offset++] = g
      raw[offset++] = b
    }
  }
  const header = Buffer.alloc(13)
  header.writeUInt32BE(width, 0)
  header.writeUInt32BE(height, 4)
  header[8] = 8 // bit depth
  header[9] = 2 // truecolour
  return Buffer.concat([
    Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
    chunk('IHDR', header),
    chunk('IDAT', deflateSync(raw, { level: 1 })),
    chunk('IEND', Buffer.alloc(0)),
  ])
}

/**
 * A deterministic little RNG, so the same photo id always draws the same
 * picture and screenshots stay comparable between runs.
 */
function mulberry32(seed) {
  let a = seed >>> 0
  return () => {
    a = (a + 0x6d2b79f5) >>> 0
    let t = a
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
}

function hashString(value) {
  let h = 2166136261
  for (let i = 0; i < value.length; i++) {
    h ^= value.charCodeAt(i)
    h = Math.imul(h, 16777619)
  }
  return h >>> 0
}

function hsl(h, s, l) {
  h = ((h % 360) + 360) % 360
  s = Math.max(0, Math.min(1, s))
  l = Math.max(0, Math.min(1, l))
  const c = (1 - Math.abs(2 * l - 1)) * s
  const x = c * (1 - Math.abs(((h / 60) % 2) - 1))
  const m = l - c / 2
  const rgb = [
    [c, x, 0],
    [x, c, 0],
    [0, c, x],
    [0, x, c],
    [x, 0, c],
    [c, 0, x],
  ][Math.floor(h / 60) % 6]
  return rgb.map((v) => Math.round((v + m) * 255))
}

const mix = (a, b, t) => [
  a[0] + (b[0] - a[0]) * t,
  a[1] + (b[1] - a[1]) * t,
  a[2] + (b[2] - a[2]) * t,
]

const ease = (t) => t * t * (3 - 2 * t)

/**
 * A picture that reads as a photograph at thumbnail size: a graded sky, a sun,
 * two ridgelines, a vignette and a little grain. It is a landscape every time,
 * which is all the dashboard needs from it — flat colour would make the photo
 * grids look like swatches.
 */
function painter(seed) {
  const rnd = mulberry32(seed)
  const night = rnd() < 0.15
  const skyHue = 165 + rnd() * 75
  const sky = night ? hsl(skyHue, 0.4, 0.13) : hsl(skyHue, 0.5, 0.38)
  const horizonHaze = night ? hsl(skyHue + 8, 0.35, 0.28) : hsl(skyHue + 20, 0.45, 0.8)
  const farRidge = night ? hsl(skyHue + 12, 0.3, 0.1) : hsl(skyHue + 30, 0.25, 0.42)
  const nearRidge = night ? hsl(skyHue + 180, 0.2, 0.05) : hsl(skyHue + 190, 0.22, 0.15)
  const sun = night ? hsl(44, 0.55, 0.7) : hsl(40, 0.8, 0.85)
  const sunX = 0.14 + rnd() * 0.72
  const sunY = 0.12 + rnd() * 0.3
  const glow = 0.4 + rnd() * 0.5
  const mirror = rnd() < 0.3 // still water under the ridge

  // Two ridgelines, each a couple of sine waves with their own phase, so the
  // silhouette differs from picture to picture but repeats for a given id.
  const wave = (count, amp, base) => {
    const terms = Array.from({ length: count }, () => ({
      frequency: 1 + Math.floor(rnd() * 4) + rnd(),
      amplitude: (rnd() - 0.5) * amp,
      phase: rnd() * Math.PI * 2,
    }))
    return (u) =>
      base +
      terms.reduce(
        (sum, term) => sum + Math.sin(u * term.frequency * Math.PI * 2 + term.phase) * term.amplitude,
        0,
      )
  }
  const farTop = wave(3, 0.16, 0.44 + rnd() * 0.1)
  const nearTop = wave(2, 0.14, 0.6 + rnd() * 0.12)
  const grainSeed = seed ^ 0x9e3779b9

  return (x, y, width, height) => {
    const u = x / width
    const v = y / height
    const aspect = width / height

    let colour = mix(sky, horizonHaze, ease(Math.min(1, v / 0.62)))

    const dx = (u - sunX) * aspect
    const dy = v - sunY
    const strength = Math.max(0, 1 - Math.sqrt(dx * dx + dy * dy) / glow)
    colour = mix(colour, sun, strength * strength * (night ? 1 : 0.8))

    const far = farTop(u)
    const near = nearTop(u)
    if (v > far) colour = mix(farRidge, nearRidge, Math.min(1, (v - far) / 0.18))
    if (v > near) {
      colour = mirror
        ? mix(horizonHaze, nearRidge, 0.45)
        : nearRidge
      // A hint of texture so the foreground is not a flat block.
      colour = mix(colour, nearRidge, Math.min(1, (v - near) * 1.6) * 0.4)
    }

    const cx = (u - 0.5) * 2
    const cy = (v - 0.5) * 2
    const vignette = 1 - 0.26 * Math.min(1, (cx * cx + cy * cy) / 2)
    const noise = (((mulberry32(grainSeed + x * 73856093 + y * 19349663)() * 255) | 0) % 5) - 2

    return colour.map((value) =>
      Math.max(0, Math.min(255, Math.round(value * vignette + noise))),
    )
  }
}

const imageCache = new Map()

function image(seed, width, height) {
  const key = `${seed}:${width}x${height}`
  const cached = imageCache.get(key)
  if (cached) return cached
  const paint = painter(seed)
  const png = encodePNG(width, height, (x, y) => paint(x, y, width, height))
  imageCache.set(key, png)
  return png
}

/* ------------------------------------------------------------------ *
 * Invented content
 * ------------------------------------------------------------------ */

const ALBUMS = [
  ['川西环线', '折多山到新都桥，一路的云和光', false],
  ['京都的秋天', '十一月的东福寺和岚山', false],
  ['家里的猫', null, false],
  ['老周婚礼', '跟拍一天，挑出来的都在这里', false],
  ['城市夜景', '天台、长曝光、和迟到的末班地铁', false],
  ['海边三天', '潮汐表说这几天是大潮', false],
  ['徒步 · 贡嘎', '海拔四千米以上的两天', false],
  ['街拍存档', '不值得单独成册，但舍不得删', false],
  ['厨房记录', null, false],
  ['2025 回顾', '一年挑一百张', false],
  ['素材库', '还没归类的', false],
  ['私人', '这个相册上了锁', true],
]

const TITLES = [
  '晨雾里的折多山',
  '山口的路牌',
  '新都桥的下午',
  '草原上的牦牛',
  '湖面',
  '屋檐下的光',
  '窗台上的猫',
  '午后小睡',
  '院子里的柿子',
  '巷口',
  '纸灯',
  '石阶与苔',
  '清晨的第一班车',
  '天台上的风',
  '车流',
  '招牌和雨',
  '潮水退去',
  '沙滩上的脚印',
  '礁石',
  '远处的灯塔',
  '雪线以上',
  '帐篷外的夜',
  '晨光打在山脊',
  '碎石坡',
  '路过的橱窗',
  '玻璃上的倒影',
  '斑马线',
  '早点摊',
  '刚出炉的面包',
  '灶台上的锅',
  '切了一半的柠檬',
  '一桌菜',
  '春天的第一枝',
  '雨后的叶子',
  '旧木门',
  '楼梯转角',
  '堆在墙角的书',
  '阳台上晾的床单',
  '黄昏的操场',
  '球场边',
]

const TAGS = [
  ['旅行', '风光'],
  ['人像'],
  ['街拍', '黑白'],
  ['食物'],
  ['动物'],
  ['建筑'],
  ['夜景', '长曝光'],
  ['家人'],
]

const CAMERAS = [
  ['SONY', 'ILCE-7M4', 'FE 24-70mm F2.8 GM II'],
  ['FUJIFILM', 'X-T5', 'XF35mmF1.4 R'],
  ['NIKON Z 6_2', 'NIKKOR Z 50mm f/1.8 S'],
  ['Canon', 'EOS R6', 'RF70-200mm F2.8 L'],
  ['Apple', 'iPhone 15 Pro', 'iPhone 15 Pro back triple camera 6.765mm f/1.78'],
]

const PLACES = [
  ['中国', '甘孜', '折多山'],
  ['中国', '甘孜', '新都桥'],
  ['日本', '京都', '东山区'],
  ['中国', '上海', '徐汇区'],
  ['中国', '厦门', '鼓浪屿'],
  ['中国', '成都', '锦江区'],
]

const SHAPES = [
  [6000, 4000],
  [4000, 6000],
  [5472, 3648],
  [1920, 1080],
  [3024, 3024],
  [4032, 3024],
]

function buildPhotos() {
  const rnd = mulberry32(20261018)
  const photos = []
  const total = 186
  const now = Date.UTC(2026, 9, 18, 9, 0, 0)

  for (let i = 0; i < total; i++) {
    const [width, height] = SHAPES[Math.floor(rnd() * SHAPES.length)]
    const takenAt = new Date(now - Math.floor(rnd() * 900) * 86400000 - Math.floor(rnd() * 86400000))
    const [make, model, lens] = CAMERAS[Math.floor(rnd() * CAMERAS.length)]
    const [country, city, locationName] = PLACES[Math.floor(rnd() * PLACES.length)]
    const hasGps = rnd() < 0.62
    const id = `mock${String(i + 1).padStart(4, '0')}`
    const isVideoStill = rnd() < 0.06
    const fileName = `${make.replace(/\s+/g, '')}_${String(1000 + i)}`

    photos.push({
      id,
      title: rnd() < 0.85 ? TITLES[Math.floor(rnd() * TITLES.length)] : null,
      description: rnd() < 0.18 ? '随手记一句，方便以后找回来。' : null,
      width,
      height,
      aspectRatio: width / height,
      dateTaken: takenAt.toISOString(),
      storageKey: `photos/${fileName}.jpg`,
      thumbnailKey: `photos/thumbs/${fileName}.jpg`,
      fileSize: Math.round((2.1 + rnd() * 18) * 1024 * 1024),
      lastModified: new Date(takenAt.getTime() + 3600000).toISOString(),
      originalUrl: `/storage/photos/${fileName}.jpg`,
      thumbnailUrl: `/storage/photos/thumbs/${fileName}.jpg`,
      thumbnailHash: null,
      tags: TAGS[Math.floor(rnd() * TAGS.length)],
      exif: {
        Make: make,
        Model: model,
        LensModel: lens,
        FNumber: [1.4, 1.8, 2.8, 4, 5.6, 8][Math.floor(rnd() * 6)],
        ExposureTime: ['1/60', '1/125', '1/250', '1/500', '1/1000', '2'][
          Math.floor(rnd() * 6)
        ],
        ISO: [100, 200, 400, 800, 1600, 3200, 6400][Math.floor(rnd() * 7)],
        FocalLength: `${[24, 35, 50, 70, 135, 200][Math.floor(rnd() * 6)]} mm`,
        ColorSpace: 'sRGB',
        Rating: rnd() < 0.35 ? Math.ceil(rnd() * 5) : 0,
      },
      latitude: hasGps ? 29.5 + rnd() * 2 : null,
      longitude: hasGps ? 100.2 + rnd() * 2 : null,
      country: hasGps ? country : null,
      city: hasGps ? city : null,
      locationName: hasGps ? locationName : null,
      isLivePhoto: isVideoStill ? 1 : 0,
      livePhotoVideoUrl: isVideoStill ? `/storage/photos/${fileName}.mp4` : null,
      livePhotoVideoKey: isVideoStill ? `photos/${fileName}.mp4` : null,
    })
  }

  return photos
}

const photos = buildPhotos()

// Hand the albums a slice of the collection, in the order the photos were
// taken, which is what the real dashboard orders its covers by.
const albums = ALBUMS.map(([title, description, locked], index) => {
  const slice = photos
    .filter((_, i) => i % ALBUMS.length === index)
    .map((photo) => photo.id)
  return {
    id: index + 1,
    title,
    description,
    coverPhotoId: index === 10 || index === 8 ? null : slice[0] || null,
    isHidden: index === 11,
    hasPassword: Boolean(locked),
    createdAt: new Date(Date.UTC(2026, 1, 3 + index)).toISOString(),
    updatedAt: new Date(Date.UTC(2026, 9, 12 + (index % 6))).toISOString(),
    photoIds: slice,
  }
})

// The vocabulary the queue is written in. A task's type says what kind of
// work it is; the stage is which step of it is running, and the interface has
// a separate set of names for each. Sending a stage as a type renders as a
// raw translation key.
const QUEUE_TYPES = [
  'photo',
  'photo-metadata-update',
  'photo-reverse-geocoding',
  'photo-erase-location',
  'live-photo-video',
]

const QUEUE_STAGES = [
  'preprocessing',
  'metadata',
  'thumbnail',
  'exif',
  'motion-photo',
  'reverse-geocoding',
  'live-photo',
  'location-erase',
]

function buildTasks() {
  const rnd = mulberry32(7717)
  const tasks = []
  const now = Math.floor(Date.now() / 1000)
  for (let i = 0; i < 140; i++) {
    const status =
      i < 4
        ? 'in-stages'
        : i < 9
          ? 'pending'
          : rnd() < 0.08
            ? 'failed'
            : 'completed'
    const photo = photos[Math.floor(rnd() * photos.length)]
    const createdAt = now - Math.floor(rnd() * 90000)
    tasks.push({
      id: i + 1,
      payload: {
        type: QUEUE_TYPES[Math.floor(rnd() * QUEUE_TYPES.length)],
        photoId: photo.id,
        storageKey: photo.storageKey,
      },
      priority: Math.floor(rnd() * 5),
      attempts: status === 'failed' ? 3 : 1,
      maxAttempts: 3,
      status,
      statusStage:
        status === 'in-stages'
          ? QUEUE_STAGES[Math.floor(rnd() * QUEUE_STAGES.length)]
          : null,
      errorMessage:
        status === 'failed'
          ? rnd() < 0.5
            ? 'metadata: exiftool exited with status 1'
            : 'thumbnail: unsupported colour profile'
          : null,
      createdAt: new Date(createdAt * 1000).toISOString(),
      completedAt:
        status === 'completed' ? new Date((createdAt + 3) * 1000).toISOString() : null,
      availableAt: null,
    })
  }
  return tasks.sort((a, b) => b.createdAt.localeCompare(a.createdAt))
}

const tasks = buildTasks()

// The same scopes the Go backend writes under, and the same line shape:
// `<RFC 3339 stamp> [<scope>] <message>`. See cmd/memora/logging.go.
const LOG_LINES = [
  ['server', 'Memora Go backend listening on 127.0.0.1:3000'],
  ['storage', 'provider=local basePath=./data/storage'],
  ['queue', 'task 63 processed photo 9f4a12'],
  ['queue', 'task 64 processed photo 5b8c07'],
  ['queue', 'task 65 moved to failed: exiftool exited with status 1'],
  ['queue', 'failed to enqueue reverse geocoding 3d21aa: queue is closed'],
  ['storage', 'removed stale photo reference 7c0e51'],
  ['queue', 'removed 2 invalid generated Live Photo videos'],
  ['storage', 'starting a chunked upload: incomplete body'],
  ['queue', 'workers still busy after 10s, leaving the rest of the queue for the next start'],
  ['og', 'link preview cards are drawn in the Go font, which has no Chinese, Japanese or Korean letters'],
  ['server', 'received terminated, shutting down'],
]

function stamp(at) {
  const pad = (n, width = 2) => String(n).padStart(width, '0')
  const offset = -at.getTimezoneOffset()
  const sign = offset < 0 ? '-' : '+'
  const value = Math.abs(offset)
  return (
    `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}` +
    `T${pad(at.getHours())}:${pad(at.getMinutes())}:${pad(at.getSeconds())}.${pad(at.getMilliseconds(), 3)}` +
    `${sign}${pad(Math.floor(value / 60))}:${pad(value % 60)}`
  )
}

function logLines(count) {
  const rnd = mulberry32(4242)
  const out = []
  const now = Date.now()
  for (let i = count - 1; i >= 0; i--) {
    const [scope, message] = LOG_LINES[Math.floor(rnd() * LOG_LINES.length)]
    const at = new Date(now - i * (900 + Math.floor(rnd() * 4000)))
    out.push(`${stamp(at)} [${scope}] ${message}`)
  }
  return out
}

/* ------------------------------------------------------------------ *
 * Settings — the descriptors are the real ones, captured from the Go code
 * ------------------------------------------------------------------ */

const fixtures = JSON.parse(
  readFileSync(join(here, 'fixtures', 'settings-fields.json'), 'utf8'),
)
const settingsSchema = JSON.parse(
  readFileSync(join(here, 'fixtures', 'settings-schema.json'), 'utf8'),
)

const settingValues = {}
for (const [namespace, payload] of Object.entries(fixtures)) {
  settingValues[namespace] = {}
  for (const field of payload.fields) {
    settingValues[namespace][field.key] = field.value ?? field.defaultValue ?? null
  }
}

// A few values worth seeing filled in rather than left at their defaults.
Object.assign(settingValues.app, {
  title: 'Memora',
  slogan: '把光留在相纸上',
  author: 'LiuShen',
})
Object.assign(settingValues.map, {
  provider: 'maplibre',
  'maplibre.style': 'https://demotiles.maplibre.org/style.json',
})
Object.assign(settingValues.location, { provider: 'nominatim' })
Object.assign(settingValues.system, { queueConcurrency: 4, firstLaunch: false })

/* ------------------------------------------------------------------ *
 * Storage configs — a list the storage page reads and edits
 * ------------------------------------------------------------------ */

// The page manages several named configs and picks one of them as active, so
// the mock keeps the same three shapes a person is likely to have.
let storageConfigs = [
  {
    id: 1,
    name: '本地存储',
    provider: 'local',
    config: {
      provider: 'local',
      basePath: './data/storage',
      baseUrl: '/storage',
      prefix: 'photos/',
    },
    createdAt: '2026-01-04T09:12:00+08:00',
    updatedAt: '2026-03-11T20:41:00+08:00',
  },
  {
    id: 2,
    name: 'Cloudflare R2',
    provider: 's3',
    config: {
      provider: 's3',
      bucket: 'memora-photos',
      region: 'auto',
      endpoint: 'https://example.r2.cloudflarestorage.com',
      prefix: 'photos',
      cdnUrl: '',
      accessKeyId: '',
      secretAccessKey: '',
      forcePathStyle: false,
      maxKeys: 1000,
    },
    createdAt: '2026-02-19T14:03:00+08:00',
    updatedAt: '2026-02-19T14:03:00+08:00',
  },
  {
    id: 3,
    name: 'OpenList 备份',
    provider: 'openlist',
    config: {
      provider: 'openlist',
      baseUrl: 'https://openlist.example.com',
      rootPath: '/memora',
      token: '',
      uploadEndpoint: '/api/fs/put',
      downloadEndpoint: '',
      listEndpoint: '',
      deleteEndpoint: '/api/fs/remove',
      metaEndpoint: '/api/fs/get',
      pathField: 'path',
      cdnUrl: '',
    },
    createdAt: '2026-05-02T08:20:00+08:00',
    updatedAt: '2026-05-02T08:20:00+08:00',
  },
]
let nextStorageId = 4

/* ------------------------------------------------------------------ *
 * HTTP
 * ------------------------------------------------------------------ */

const admin = {
  id: 1,
  username: 'LiuShen',
  email: 'liushen@example.com',
  avatar: '',
  isAdmin: 1,
}

// The real backend decides this from a session cookie. The mock lets the
// dashboard through either way, because signing in is not what is being built.
let loggedIn = true

function send(res, status, body, headers = {}) {
  const payload = Buffer.isBuffer(body) ? body : Buffer.from(JSON.stringify(body))
  res.writeHead(status, {
    'Content-Type': Buffer.isBuffer(body) ? 'image/png' : 'application/json',
    'Content-Length': payload.length,
    ...headers,
  })
  res.end(payload)
}

const fail = (res, status, message) => send(res, status, { message })

function pagination(url) {
  const page = Math.max(1, Number(url.searchParams.get('page')) || 1)
  const pageSize = Number(url.searchParams.get('pageSize')) || 0
  return { page, pageSize }
}

/** A request body, once the whole of it has arrived. */
function readJson(req) {
  return new Promise((resolve) => {
    let raw = ''
    req.on('data', (chunk) => {
      raw += chunk
    })
    req.on('end', () => {
      try {
        resolve(JSON.parse(raw || '{}'))
      } catch {
        resolve({})
      }
    })
  })
}

const photoById = new Map(photos.map((photo) => [photo.id, photo]))

function albumPayload(album, withPhotos) {
  const base = {
    id: album.id,
    title: album.title,
    description: album.description,
    coverPhotoId: album.coverPhotoId,
    isHidden: album.isHidden,
    hasPassword: album.hasPassword,
    createdAt: album.createdAt,
    updatedAt: album.updatedAt,
    photoIds: album.photoIds,
  }
  if (withPhotos) {
    base.photos = album.photoIds
      .map((id) => photoById.get(id))
      .filter(Boolean)
  }
  return base
}

// The one thing here that has to be read off the request rather than looked up.
// Unreadable or malformed JSON comes back as null, and the route that asked for
// it turns that into the same 400 the real decode gives.
function readJSON(req) {
  return new Promise((resolve) => {
    const chunks = []
    req.on('data', (chunk) => chunks.push(chunk))
    req.on('end', () => {
      try {
        resolve(
          JSON.parse(Buffer.concat(chunks).toString('utf8') || 'null'),
        )
      } catch {
        resolve(null)
      }
    })
    req.on('error', () => resolve(null))
  })
}

function route(req, res, url) {
  const path = url.pathname
  const method = req.method

  if (path.startsWith('/storage/')) {
    const key = path.slice('/storage/'.length)
    const size = key.includes('/thumbs/') ? 480 : 1600
    const seed = hashString(key)
    const ratio = photos.find((p) => key.includes(p.storageKey.slice(7))) || photos[0]
    const width = size
    const height = Math.round(size * ((ratio.height || 4000) / (ratio.width || 6000)))
    const bytes = image(seed, width, Math.max(1, height))
    return send(res, 200, bytes, { 'Cache-Control': 'public, max-age=3600' })
  }

  if (path === '/api/health') return send(res, 200, { status: 'ok' })

  if (path === '/api/profile' && method === 'GET') {
    return loggedIn ? send(res, 200, admin) : send(res, 200, null)
  }
  if (path === '/api/login' && method === 'POST') {
    loggedIn = true
    return send(res, 200, { ok: true })
  }
  if (path === '/api/logout') {
    loggedIn = false
    return send(res, 200, { ok: true })
  }

  if (path === '/api/system/settings/all' && method === 'GET') {
    return send(res, 200, { timestamp: Date.now(), data: settingValues })
  }
  if (path === '/api/system/settings/fields' && method === 'GET') {
    const namespace = url.searchParams.get('namespace') || ''
    const payload = fixtures[namespace]
    if (!payload) return fail(res, 404, 'Namespace not found')
    const fields = payload.fields.map((field) => ({
      ...field,
      value: settingValues[namespace]?.[field.key] ?? field.value,
    }))
    return send(res, 200, { namespace, fields })
  }
  if (path === '/api/system/settings/schema' && method === 'GET') {
    return send(res, 200, settingsSchema)
  }
  if (path === '/api/system/settings/batch' && method === 'PUT') {
    return send(res, 200, { ok: true })
  }
  if (path === '/api/system/settings/storage' && method === 'GET') {
    return send(res, 200, {
      provider: 'local',
      config: { localPath: './data/storage', baseUrl: '/storage', prefix: 'photos/' },
    })
  }
  if (path === '/api/system/settings/storage/provider' && method === 'GET') {
    // The page reads the active config's id out of the setting's value.
    return send(res, 200, {
      namespace: 'storage',
      key: 'provider',
      value: storageConfigs[0]?.id ?? null,
    })
  }
  if (path === '/api/system/settings/storage/provider' && method === 'PUT') {
    return readJson(req).then((body) => {
      const wanted = storageConfigs.find((item) => item.id === Number(body.value))
      if (!wanted) return fail(res, 404, 'no such storage config')
      storageConfigs = [wanted, ...storageConfigs.filter((item) => item !== wanted)]
      return send(res, 200, {
        namespace: 'storage',
        key: 'provider',
        value: wanted.id,
      })
    })
  }
  if (path === '/api/system/settings/storage-config' && method === 'GET') {
    return send(res, 200, storageConfigs)
  }
  if (path === '/api/system/settings/storage-config' && method === 'POST') {
    return readJson(req).then((body) => {
      const now = new Date().toISOString()
      const created = {
        id: nextStorageId++,
        name: body.name ?? '未命名存储',
        provider: body.provider ?? 'local',
        config: body.config ?? {},
        createdAt: now,
        updatedAt: now,
      }
      storageConfigs.push(created)
      return send(res, 200, created)
    })
  }
  if (path.startsWith('/api/system/settings/storage-config/')) {
    const id = Number(path.slice('/api/system/settings/storage-config/'.length))
    const found = storageConfigs.find((item) => item.id === id)
    if (!found) return fail(res, 404, 'no such storage config')
    if (method === 'PUT') {
      return readJson(req).then((body) => {
        Object.assign(found, {
          name: body.name ?? found.name,
          provider: body.provider ?? found.provider,
          config: body.config ?? found.config,
          updatedAt: new Date().toISOString(),
        })
        return send(res, 200, found)
      })
    }
    if (method === 'DELETE') {
      storageConfigs = storageConfigs.filter((item) => item !== found)
      return send(res, 200, { ok: true })
    }
  }

  if (path === '/api/system/stats' && method === 'GET') {
    const now = Date.now()
    const taken = photos.map((photo) => Date.parse(photo.dateTaken))
    const day = 86400000
    const trends = []
    for (let i = 6; i >= 0; i--) {
      const date = new Date(now - i * day).toISOString().slice(0, 10)
      trends.push({
        date,
        count: taken.filter(
          (t) => new Date(t).toISOString().slice(0, 10) === date,
        ).length,
      })
    }
    const totalSize = photos.reduce((sum, photo) => sum + photo.fileSize, 0)
    const completed = tasks.filter((task) => task.status === 'completed').length
    const failed = tasks.filter((task) => task.status === 'failed').length
    const workers = 4
    return send(res, 200, {
      uptime: 86400 * 2 + 5400,
      // Must be one of the names the interface has a translation for; the
      // dashboard renders the raw key when it is not.
      runningOn: 'docker',
      memory: { used: 412 * 1024 * 1024, total: 2048 * 1024 * 1024 },
      photos: {
        total: photos.length,
        today: taken.filter((t) => now - t < day).length,
        thisWeek: taken.filter((t) => now - t < 7 * day).length,
        thisMonth: taken.filter((t) => now - t < 30 * day).length,
      },
      workerPool: {
        totalWorkers: workers,
        activeWorkers: 2,
        totalProcessed: completed,
        totalErrors: failed,
        averageSuccessRate: (completed / (completed + failed)) * 100,
        workers: Array.from({ length: workers }, (_, i) => ({
          workerId: `worker-${i + 1}`,
          isProcessing: i < 2,
          processedCount: Math.round(completed / workers),
          errorCount: Math.round(failed / workers),
          successRate: 98.6,
        })),
      },
      queue: {
        pending: tasks.filter((task) => task.status === 'pending').length,
        failed,
        oldestWaitSeconds: 12,
      },
      storage: {
        totalSize,
        averageSize: totalSize / photos.length,
        maxSize: Math.max(...photos.map((photo) => photo.fileSize)),
        activeProvider: { provider: 'local', name: '' },
      },
      trends,
      timestamp: new Date().toISOString(),
    })
  }

  if (path === '/api/system/tours' && method === 'GET') {
    return send(res, 200, {
      'dashboard-overview': 'done',
      'dashboard-photos': 'done',
      'dashboard-albums': 'pending',
      'dashboard-queue': 'pending',
      'dashboard-logs': 'pending',
      'dashboard-settings-general': 'pending',
    })
  }
  if (path === '/api/system/tours' && method === 'PUT') {
    return send(res, 200, { ok: true })
  }

  if (path === '/api/system/logs' && method === 'GET') {
    res.writeHead(200, {
      'Content-Type': 'text/event-stream',
      'Cache-Control': 'no-cache',
      Connection: 'keep-alive',
    })
    // The stream carries the line itself, after `id:` says where it ends — the
    // page resumes a dropped connection from that number.
    const asked = url.searchParams.get('initial')
    const initial = asked === 'all' ? 200 : Number(asked) || 80
    let offset = 0
    for (const line of logLines(Math.min(200, initial))) {
      offset += line.length + 1
      res.write(`id: ${offset}\ndata: ${line}\n\n`)
    }
    const timer = setInterval(() => {
      const [scope, message] = LOG_LINES[Math.floor(Math.random() * LOG_LINES.length)]
      const line = `${stamp(new Date())} [${scope}] ${message}`
      offset += line.length + 1
      res.write(`id: ${offset}\ndata: ${line}\n\n`)
    }, 2500)
    req.on('close', () => clearInterval(timer))
    return
  }

  if (path === '/api/photos' || path === '/api/photos/visible') {
    if (method === 'GET') {
      const { page, pageSize } = pagination(url)
      if (!pageSize) return send(res, 200, photos)
      const start = (page - 1) * pageSize
      return send(res, 200, {
        data: photos.slice(start, start + pageSize),
        page,
        total: photos.length,
      })
    }
    return fail(res, 405, 'Method Not Allowed')
  }

  if (path === '/api/photos/timeline' && method === 'GET') {
    const from = url.searchParams.get('from') || ''
    const to = url.searchParams.get('to') || ''
    const counts = new Map()
    for (const photo of photos) {
      const day = String(photo.dateTaken || photo.lastModified || '').slice(0, 10)
      if (!day) continue
      if (from && day < from) continue
      if (to && day > to) continue
      counts.set(day, (counts.get(day) || 0) + 1)
    }
    const days = [...counts.entries()]
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([date, count]) => ({ date, count }))
    return send(res, 200, { days })
  }

  if (path === '/api/photos/reactions' && method === 'GET') {
    return send(res, 200, [])
  }

  if (path.startsWith('/api/photos/')) {
    const rest = path.slice('/api/photos/'.length)
    const [id, sub] = rest.split('/')
    const photo = photoById.get(decodeURIComponent(id))
    if (!photo) return fail(res, 404, 'Photo not found')
    if (!sub && method === 'GET') return send(res, 200, photo)
    if (sub === 'albums' && method === 'GET') {
      const held = albums
        .filter((album) => album.photoIds.includes(photo.id))
        .map((album) => ({ id: album.id, title: album.title }))
      return send(res, 200, held)
    }
    if (sub === 'reactions' && method === 'GET') return send(res, 200, [])
    return send(res, 200, { ok: true })
  }

  if (path === '/api/albums' && method === 'GET') {
    return send(res, 200, albums.map((album) => albumPayload(album, false)))
  }
  if (path === '/api/albums/protection' && method === 'GET') {
    return send(res, 200, { locked: false })
  }
  // Ahead of the /api/albums/<id> block below, which would read "reorder" as an
  // album id and 404. The real route stores a position per album and reads the
  // list back in that order; here the array IS the order, so writing it back in
  // the order asked for is the same thing. It validates what the real one
  // validates, so a bad request fails here the same way it would there.
  if (path === '/api/albums/reorder' && method === 'PUT') {
    return readJSON(req).then((body) => {
      const ids = Array.isArray(body?.albumIds) ? body.albumIds : []
      if (ids.length === 0 || ids.length > 1000) {
        return fail(res, 400, 'albumIds must contain between 1 and 1000 items')
      }
      const seen = new Set()
      for (const id of ids) {
        if (!Number.isInteger(id) || id <= 0 || seen.has(id)) {
          return fail(res, 400, 'albumIds must be unique positive album IDs')
        }
        seen.add(id)
      }
      // An id that names no album is a no-op rather than an error, which is
      // what the UPDATE behind the real route does with one.
      const byId = new Map(albums.map((album) => [album.id, album]))
      const ordered = ids.map((id) => byId.get(id)).filter(Boolean)
      const rest = albums.filter((album) => !seen.has(album.id))
      albums.splice(0, albums.length, ...ordered, ...rest)
      return send(res, 200, { success: true })
    })
  }
  if (path.startsWith('/api/albums/')) {
    const rest = decodeURIComponent(path.slice('/api/albums/'.length))
    const [rawId, sub] = rest.split('/')
    const album = albums.find((entry) => String(entry.id) === rawId)
    if (!album) return fail(res, 404, 'Album not found')
    if (sub === 'photos') {
      return send(res, 200, {
        data: album.photoIds.map((id) => photoById.get(id)).filter(Boolean),
        page: 1,
        total: album.photoIds.length,
      })
    }
    return send(res, 200, albumPayload(album, true))
  }

  if (path === '/api/queue/task/list' && method === 'GET') {
    const status = url.searchParams.get('status')
    const filtered = status ? tasks.filter((task) => task.status === status) : tasks
    const { page, pageSize } = pagination(url)
    const size = pageSize || 50
    const stats = { pending: 0, 'in-stages': 0, completed: 0, failed: 0 }
    for (const task of tasks) stats[task.status] = (stats[task.status] || 0) + 1
    return send(res, 200, {
      data: filtered.slice((page - 1) * size, page * size),
      page,
      pageSize: size,
      total: filtered.length,
      stats,
    })
  }
  if (path === '/api/queue/recent' && method === 'GET') {
    // {tasks: [...]}, not a bare array, and the photo behind a task travels
    // with it: the list names each entry after the photo it worked on.
    return send(res, 200, {
      tasks: tasks.slice(0, 5).map((task) => ({
        id: task.id,
        type: task.payload.type,
        status: task.status,
        statusStage: task.statusStage,
        errorMessage: task.errorMessage,
        createdAt: task.createdAt,
        completedAt: task.completedAt,
        fileName: task.payload.fileName ?? null,
        photo: task.payload.photoId
          ? (photoById.get(task.payload.photoId) ?? null)
          : null,
      })),
    })
  }
  if (path === '/api/queue/concurrency' && method === 'GET') {
    return send(res, 200, { concurrency: 4 })
  }
  if (path === '/api/queue/concurrency' && method === 'PUT') {
    return send(res, 200, { concurrency: 4 })
  }
  if (path.startsWith('/api/queue/stats/')) {
    const id = Number(path.slice('/api/queue/stats/'.length))
    const task = tasks.find((entry) => entry.id === id)
    if (!task) return fail(res, 404, 'Task not found')
    return send(res, 200, {
      task,
      totalDuration: 1840,
      stageDurations: { preprocessing: 320, metadata: 900, thumbnail: 480, exif: 140 },
    })
  }
  if (path.startsWith('/api/queue/')) {
    // add / retry / clear / delete — the dashboard only needs a yes
    return send(res, 200, { ok: true })
  }

  if (path === '/api/wizard/schema' && method === 'GET') {
    return send(res, 200, { steps: [] })
  }

  return fail(res, 404, `No mock for ${method} ${path}`)
}

export function createMockApi() {
  return createServer((req, res) => {
    const url = new URL(req.url, `http://127.0.0.1:${port}`)
    if (process.env.MOCK_QUIET !== '1') {
      console.log(`${req.method} ${url.pathname}${url.search}`)
    }
    // Most routes answer and return nothing; the reorder one has to read the
    // request body first, which only comes in through events, so it hands back
    // a promise instead. Waiting on both here keeps that difference from
    // leaking out. A failure that already sent its own answer is left alone.
    Promise.resolve()
      .then(() => route(req, res, url))
      .catch((error) => {
        console.error(error)
        if (!res.headersSent) fail(res, 500, String(error && error.message))
      })
  })
}

export function startMockApi(onReady) {
  const server = createMockApi()
  server.listen(port, '127.0.0.1', () => {
    console.log(`mock api on http://127.0.0.1:${port}`)
    console.log(
      `${photos.length} photos · ${albums.length} albums · ${tasks.length} queue tasks`,
    )
    onReady?.()
  })
  return server
}

// Running this file directly starts a server on its own; mock/dev.mjs imports
// it instead, so the dev server and the mock share one process and one command.
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  startMockApi()
}
