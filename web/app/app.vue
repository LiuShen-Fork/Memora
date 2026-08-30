<script setup lang="ts">
import dayjsLocale_zhCN from 'dayjs/locale/zh-cn'
import dayjsLocale_zhTW from 'dayjs/locale/zh-tw'
import dayjsLocale_zhHK from 'dayjs/locale/zh-hk'

const router = useRouter()
const dayjs = useDayjs()
const colorMode = useColorMode()
const localeRef = ref('en')
try {
  const { locale } = useI18n()
  watch(
    locale,
    (value) => {
      localeRef.value = value
    },
    { immediate: true },
  )
} catch {
  // i18n context may be unavailable during early server-side error rendering
}

// 初始化设置系统 - 一次性加载所有设置
const settingsStore = useSettingsStore()
const session = useUserSession()

// Settings are needed by navigation and controls, but should not hold the
// gallery request hostage. Start both lightweight bootstrap requests before
// fetching photos; the reactive settings/session state updates consumers when
// each request completes.
const settingsRequest = settingsStore.initSettings().catch((error) => {
  console.error('[Settings] Failed to initialize settings:', error)
})
void session.fetch()
const { loggedIn } = session

const appTitle = useSettingRef('app:title')

useHead({
  titleTemplate: (title) =>
    `${title ? title + ' | ' : ''}${appTitle.value || 'Memora'}`,
})

// 根据用户登录状态和当前路由决定使用哪个 API
// 登录用户或后台管理页面显示所有照片，未登录用户在前端页面只显示可见照片
const route = useRoute()
const shouldLoadPhotos = computed(() => {
  const path = route.path
  // Photo IDs are not limited to the legacy 32-character hash format
  // (uploads may use names such as MVIMG_20260827_194303). Keep the global
  // collection mounted for every single-segment photo detail route so
  // returning home does not trigger a second full collection fetch.
  const isPhotoDetailRoute =
    /^\/[^/]+$/.test(path) &&
    !['/signin', '/onboarding', '/dashboard'].includes(path)
  return (
    path === '/' ||
    path === '/albums' ||
    path === '/globe' ||
    path === '/dashboard/photos' ||
    isPhotoDetailRoute
  )
})
const apiEndpoint = computed(() => {
  // 后台管理页面始终显示所有照片
  if (route.path.startsWith('/dashboard')) {
    return '/api/photos'
  }
  // 前端页面：登录用户显示所有照片，未登录用户只显示可见照片
  return loggedIn.value ? '/api/photos' : '/api/photos/visible'
})
const photoQuery = computed(() => {
  if (route.path !== '/dashboard/photos') return undefined
  const rawPage = Number(route.query.page)
  const page = Number.isInteger(rawPage) && rawPage > 0 ? rawPage : 1
  return { page, pageSize: 20 }
})
// Keep the shell and route visible while the photo collection is loading.
// The collection can be large, and album pages fetch their own focused data.
const { data, refresh, status } = useFetch(() => apiEndpoint.value, {
  immediate: false,
  watch: false,
  lazy: true,
  default: () => [],
  query: photoQuery,
})
watch(
  [apiEndpoint, shouldLoadPhotos, photoQuery],
  ([endpoint, needed, query], previous) => {
    if (!needed || !endpoint) return
    const previousQuery = previous?.[2]
    const queryChanged =
      query?.page !== previousQuery?.page ||
      query?.pageSize !== previousQuery?.pageSize
    if (
      !previous ||
      endpoint !== previous[0] ||
      needed !== previous[1] ||
      queryChanged
    ) {
      void refresh()
    }
  },
  { immediate: true },
)

// Keep theme application deterministic even when settings arrive after the
// first gallery response.
const themeSetting = useSettingRef('app:appearance.theme')
// A browser-level choice must win over the server default on subsequent
// visits. The color-mode module writes this key whenever the user toggles it.
const hasStoredThemePreference = () =>
  import.meta.client && Boolean(window.localStorage.getItem('cframe-color-mode'))
watch(
  themeSetting,
  (theme) => {
    if (
      !hasStoredThemePreference() &&
      typeof theme === 'string' &&
      theme.length > 0
    ) {
      colorMode.preference = theme
    }
  },
  { immediate: true },
)
void settingsRequest

const photos = computed(() => {
  const value = data.value as any
  return (Array.isArray(value) ? value : value?.data) || []
})
const photoTotal = computed(() => {
  const value = data.value as any
  return typeof value?.total === 'number' ? value.total : photos.value.length
})

const { switchToIndex, closeViewer, clearReturnRoute } = useViewerState()
const {
  currentPhotoIndex,
  isViewerOpen,
  returnRoute,
  isDirectAccess,
  scopedPhotos,
} = storeToRefs(useViewerState())

// The photo collection the viewer actually navigates: the scoped list (e.g. an
// album) when present, otherwise the global list.
const viewerPhotos = computed(() => scopedPhotos.value ?? photos.value)

const handleIndexChange = (newIndex: number) => {
  switchToIndex(newIndex)
  router.replace(`/${viewerPhotos.value[newIndex]?.id}`)
}

const handleClose = () => {
  closeViewer()

  // 如果是直接访问详情页面，关闭时返回首页
  if (isDirectAccess.value) {
    isDirectAccess.value = false
    router.replace('/')
  } else if (returnRoute.value) {
    // 如果有指定的返回路由，返回到该路由
    const destination = returnRoute.value
    clearReturnRoute()
    router.replace(destination)
  } else {
    // 否则使用历史记录或默认返回首页
    if (window.history.length > 1) {
      router.back()
    } else {
      router.replace('/')
    }
  }
}

watchEffect(() => {
  dayjs.locale('zh-Hans', dayjsLocale_zhCN)
  dayjs.locale('zh-Hant-TW', dayjsLocale_zhTW)
  dayjs.locale('zh-Hant-HK', dayjsLocale_zhHK)
  dayjs.locale(localeRef.value)
})

// 在全局级别提供筛选功能的状态管理
provide(
  'photosFiltering',
  reactive({
    activeFilters: {
      tags: [],
      cameras: [],
      lenses: [],
      cities: [],
      ratings: [],
    },
  }),
)
</script>

<template>
  <UApp>
    <NuxtLoadingIndicator />
    <PhotosProvider
      :photos="photos"
      :refresh="refresh"
      :status="status"
      :total-count="photoTotal"
    >
      <NuxtLayout>
        <NuxtPage />
      </NuxtLayout>
      <ClientOnly>
        <PhotoViewer
          v-if="route.path !== '/globe'"
          :photos="viewerPhotos"
          :current-index="currentPhotoIndex"
          :is-open="isViewerOpen"
          @close="handleClose"
          @index-change="handleIndexChange"
        />
      </ClientOnly>
    </PhotosProvider>
  </UApp>
</template>

<style></style>
