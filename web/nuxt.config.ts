import { useNuxt } from '@nuxt/kit'
import pkg from './package.json'
import type { AnalyticsConfig } from './shared/types/config'
import i18n, { dayjsLocales } from './i18n/i18n.options'

const backendOrigin = process.env.MEMORA_BACKEND_ORIGIN || 'http://127.0.0.1:3000'

// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  compatibilityDate: '2025-07-15',
  ssr: false,
  // DevTools is useful while developing but adds work to every build and is
  // not needed by the generated static frontend.
  devtools: { enabled: false },

  modules: [
    'reka-ui/nuxt',
    '@nuxt/ui',
    '@nuxt/icon',
    '@pinia/nuxt',
    'motion-v/nuxt',
    '@vueuse/nuxt',
    'dayjs-nuxt',
    '@nuxtjs/i18n',
    'nuxt-mapbox',
    'nuxt-maplibre',
    'nuxt-gtag',
  ],

  // Nuxt UI enables @nuxt/fonts by default. The app uses system/fallback
  // fonts, so disable the module to avoid remote Google Fonts lookups during
  // every build.
  ui: {
    fonts: false,
  },

  // Only the icons the build finds while reading the app are bundled into it,
  // and the scan reads templates, not TypeScript. The icons Nuxt UI's own
  // components ask for are named in app.config.ts, so the scan is told to read
  // .ts as well; otherwise a select draws no chevron and a toast no close
  // button, because the static frontend has no icon service to fetch from.
  //
  // This has to stay the only `icon` key in this file. There was a second one
  // at the bottom — `icon: { clientBundle: { scan: true } }`, whose `scan` means
  // "the module's own globs" rather than "this object" — and a duplicate key in
  // an object literal means the later one wins, so this block never ran and the
  // `.ts` was never in the glob. Nothing was visibly missing because of it (the
  // generated bundle holds the same 166 tabler icons either way), but the file
  // said one thing and did another, and the next person to change this glob
  // would have changed nothing at all.
  icon: {
    clientBundle: {
      scan: {
        globInclude: ['**/*.{vue,jsx,tsx,ts,md,mdc,mdx,yml,yaml}'],
      },
    },
  },

  css: ['~/assets/css/tailwind.css'],

  // The icons live in public/ and are named here so the generated page asks
  // for the vector one first and the manifest makes the site installable.
  app: {
    head: {
      link: [
        { rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' },
        { rel: 'icon', type: 'image/png', sizes: '96x96', href: '/favicon-96x96.png' },
        { rel: 'shortcut icon', href: '/favicon.ico' },
        { rel: 'apple-touch-icon', sizes: '180x180', href: '/apple-touch-icon.png' },
        { rel: 'manifest', href: '/site.webmanifest' },
      ],
    },
  },

  components: [{ path: '~/components/ui', pathPrefix: false }, '~/components'],

  runtimeConfig: {
    public: {
      // Release builds inject MEMORA_VERSION from GitHub Actions. Keep the
      // package value only as a local-development fallback.
      VERSION: process.env.MEMORA_VERSION || pkg.version,
      mapbox: {
        accessToken: '',
      },
      app: {
        title: 'Memora',
        slogan: '',
        author: '',
        avatarUrl: '',
      },
      map: {
        provider: 'maplibre' as 'mapbox' | 'maplibre' | 'amap',
        mapbox: {
          style: '',
        },
        maplibre: {
          token: '',
          style: '',
        },
        amap: {
          key: '',
          securityCode: '',
        },
      },
      analytics: {
        matomo: {
          enabled: false,
          url: '',
          siteId: '',
        },
      } satisfies AnalyticsConfig,
      oauth: {
        github: {
          enabled: false,
        },
      },
    },
    mapbox: {
      accessToken: '',
    },
    nominatim: {
      baseUrl: 'https://nominatim.openstreetmap.org',
    },
    STORAGE_PROVIDER: 's3' satisfies 's3' | 'local' | 'openlist',
    provider: {
      s3: {
        endpoint: '',
        bucket: '',
        region: 'auto',
        accessKeyId: '',
        secretAccessKey: '',
        prefix: '',
        cdnUrl: '',
        forcePathStyle: false,
      },
      local: {
        localPath: './data/storage',
        baseUrl: '/storage',
        prefix: 'photos/',
      },
      openlist: {
        baseUrl: '',
        rootPath: '',
        token: '',
        endpoints: {
          upload: '/api/fs/put',
          download: '',
          list: '',
          delete: '/api/fs/remove',
          meta: '/api/fs/get',
        },
        pathField: 'path',
        cdnUrl: '',
      } as {
        baseUrl: string
        rootPath: string
        token: string
        endpoints: {
          upload: string
          download: string
          list: string
          delete: string
          meta: string
        }
        pathField: string
        cdnUrl: string
      },
    },
    upload: {
      mime: {
        whitelistEnabled: true,
        whitelist:
          'image/jpeg,image/png,image/webp,image/gif,image/bmp,image/tiff,image/heic,image/heif,video/mp4',
      },
      duplicateCheck: {
        enabled: true,
        mode: 'skip' as 'warn' | 'block' | 'skip',
      },
    },
    /** @deprecated Defaults to allow insecure cookies now */
    allowInsecureCookie: false,
  },

  nitro: {
    preset: 'static',
    experimental: {
      websocket: true,
      tasks: true,
    },
  },

  vite: {
    server: {
      proxy: {
        '/api': backendOrigin,
        '/storage': backendOrigin,
        '/image': backendOrigin,
        '/thumb': backendOrigin,
      },
    },
    optimizeDeps: {
      include: [
        'zod',
        'dayjs',
        'dayjs/plugin/updateLocale',
        'dayjs/locale/zh-cn',
        'dayjs/locale/zh-hk',
        'dayjs/locale/zh-tw',
        'dayjs/locale/en',
        'dayjs/plugin/relativeTime',
        'dayjs/plugin/utc',
        'dayjs/plugin/timezone',
        'dayjs/plugin/duration',
        'dayjs/plugin/localizedFormat',
        'dayjs/plugin/isBetween',
        '@yeger/vue-masonry-wall',
        'motion-v',
        'swiper/vue',
        'swiper/modules',
        'tailwind-merge',
        'thumbhash',
        'mapbox-gl',
        'maplibre-gl',
        '@indoorequal/vue-maplibre-gl',
        'file-type',
        'reka-ui',
        'es-toolkit',
        'tippy.js',
      ],
    },
    ssr: {
      noExternal: ['@indoorequal/vue-maplibre-gl'],
    },
    css: {
      devSourcemap: false,
    },
    build: {
      sourcemap: false,
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (!id.includes('node_modules')) {
              return
            }
            // Stylesheets are not part of the lazy story: the entry imports
            // them, and a stylesheet dragged into a chunk takes that whole
            // chunk with it.
            if (id.includes('.css')) {
              return
            }

            // Only the map renderers themselves. The Nuxt modules and the Vue
            // wrapper around them pull in Nuxt's own app runtime, and putting
            // those in the same chunk makes the map chunk a dependency of every
            // other chunk — the exact opposite of splitting it off.
            if (
              id.includes('/mapbox-gl/') ||
              id.includes('/maplibre-gl/')
            ) {
              return 'vendor-map'
            }
            if (
              id.includes('/@nuxt/ui/') ||
              id.includes('/reka-ui/') ||
              id.includes('/@floating-ui/') ||
              id.includes('/tippy.js/')
            ) {
              return 'vendor-ui'
            }
            if (
              id.includes('/@intlify/') ||
              id.includes('/vue-i18n/') ||
              id.includes('/dayjs/')
            ) {
              return 'vendor-i18n'
            }
          },
        },
      },
      commonjsOptions: {
        include: [/maplibre-gl/, /node_modules/],
        transformMixedEsModules: true,
      },
    },
    plugins: [
      {
        apply: 'build',
        name: 'vite-plugin-ignore-sourcemap-warnings',
        configResolved(config) {
          const originalOnWarn = config.build.rollupOptions.onwarn
          config.build.rollupOptions.onwarn = (warning, warn) => {
            if (
              warning.code === 'SOURCEMAP_BROKEN' &&
              warning.plugin === '@tailwindcss/vite:generate:build'
            ) {
              return
            }

            if (originalOnWarn) {
              originalOnWarn(warning, warn)
            } else {
              warn(warning)
            }
          }
        },
      },
    ],
  },

  gtag: {
    enabled: process.env.NODE_ENV === 'production',
  },

  colorMode: {
    // preference: process.env.NUXT_PUBLIC_COLOR_MODE_PREFERENCE || 'dark',
    storageKey: 'cframe-color-mode',
  },

  dayjs: {
    locales: dayjsLocales,
    plugins: [
      'relativeTime',
      'utc',
      'timezone',
      'duration',
      'localizedFormat',
      'isBetween',
    ],
    defaultTimezone: 'Asia/Shanghai',
  },

  hooks: {
    // nuxt-maplibre adds the map stylesheet to every page. The map itself is
    // loaded on demand now, so the component brings its own stylesheet and the
    // wall stops paying for it.
    'modules:done'() {
      const nuxt = useNuxt()
      nuxt.options.css = nuxt.options.css.filter(
        (entry) => !String(entry).includes('maplibre-gl'),
      )
    },
  },

  i18n,
})
