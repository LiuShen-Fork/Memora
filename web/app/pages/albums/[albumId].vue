<script lang="ts" setup>
import { motion, useAnimationControls } from 'motion-v'

const route = useRoute()
const router = useRouter()
const dayjs = useDayjs()

// The segment after /albums/ is either the album's number or its slug, and the
// server answers both, so an address handed out before slugs existed keeps
// working. The address bar is rewritten once the album says which slug it uses,
// which also replaces a retired one with the current one.
const albumSegment = computed(() => {
  const segment = route.params.albumId
  return (Array.isArray(segment) ? segment[0] : segment) || ''
})

const {
  data: album,
  error,
  pending,
  refresh,
} = await useFetch(
  () => `/api/albums/${encodeURIComponent(albumSegment.value)}`,
  {
    watch: [albumSegment],
  },
)

// A locked album refuses with 403 instead of pretending not to exist, so the
// visitor is asked for the password rather than sent to a 404.
const lockedAlbum = computed(() => {
  if (error.value?.statusCode !== 403) return null
  const data = (error.value as { data?: { title?: string; slug?: string } })
    .data
  return { title: data?.title || null, slug: data?.slug || '' }
})

watchEffect(() => {
  if (!import.meta.client) return
  const slug = album.value?.slug || lockedAlbum.value?.slug
  const segment = albumSegment.value
  if (!slug || !segment || segment === slug) return
  void router.replace({
    path: `/albums/${encodeURIComponent(slug)}`,
    query: route.query,
    hash: route.hash,
  })
})

if (error.value && !lockedAlbum.value) {
  throw createError({
    statusCode: 404,
    statusMessage: $t('album.notFound'),
  })
}

const password = ref('')
const isUnlocking = ref(false)
const unlockError = ref('')
const showPassword = ref(false)
const prefersReducedMotion = useMediaQuery('(prefers-reduced-motion: reduce)')
const unlockControls = useAnimationControls()

const unlockAlbum = async () => {
  if (isUnlocking.value || password.value === '') return
  isUnlocking.value = true
  unlockError.value = ''
  try {
    await $fetch(`/api/albums/${encodeURIComponent(albumSegment.value)}/unlock`, {
      method: 'POST',
      body: { password: password.value },
    })
    // The grant lives in a cookie, so the same request that was refused a
    // moment ago now answers. Nothing else has to be refetched.
    password.value = ''
    await refresh()
  } catch (failure: unknown) {
    const status = (failure as { statusCode?: number })?.statusCode
    unlockError.value =
      status === 429
        ? $t('album.unlock.tooManyAttempts')
        : status === 401
          ? $t('album.unlock.incorrectPassword')
          : $t('album.unlock.failedToUnlock')
    if (!prefersReducedMotion.value) {
      void unlockControls.start({
        x: [0, -9, 9, -6, 6, -3, 3, 0],
        transition: { duration: 0.45, ease: 'easeInOut' },
      })
    }
  } finally {
    isUnlocking.value = false
  }
}

const albumData = computed(() => album.value)

const albumStats = computed(() => {
  if (!albumData.value) return null

  const photos = albumData.value.photos || []
  const totalPhotos = photos.length
  const photosWithDates = photos.filter((p: any) => p.dateTaken).length
  const photosWithExif = photos.filter((p: any) => p.exif).length

  const allDates = photos
    .map((p: any) => p?.dateTaken)
    .filter((date: any): date is string => Boolean(date))
    .map((date: string) => dayjs(date))
    .sort((a, b) => (a.isBefore(b) ? -1 : 1))

  const dateRange =
    allDates.length > 0
      ? {
          start: allDates[0],
          end: allDates[allDates.length - 1],
        }
      : null

  return {
    total: totalPhotos,
    withDates: photosWithDates,
    withExif: photosWithExif,
    dateRange,
  }
})

// 计算日期范围文本
const dateRangeText = computed(() => {
  const range = albumStats.value?.dateRange
  if (!range || !range.start || !range.end) return null

  if (range.start.isSame(range.end, 'day')) {
    return range.start.format('ll')
  } else if (range.start.isSame(range.end, 'month')) {
    return range.start.format('MMM YYYY')
  } else if (range.start.isSame(range.end, 'year')) {
    return `${range.start.format('MMM')} - ${range.end.format('MMM YYYY')}`
  } else {
    return `${range.start.format('ll')} - ${range.end.format('ll')}`
  }
})

// 用于 MasonryWall 的照片数据
const masonryItems = computed(() => {
  return (
    albumData.value?.photos?.map((photo: any, index: number) => ({
      id: photo.id,
      photo,
      originalIndex: index,
    })) ?? []
  )
})

const isMobile = useMediaQuery('(max-width: 768px)')
const columnWidth = computed(() => (isMobile.value ? 280 : 280))
const maxColumns = computed(() => (isMobile.value ? 2 : 8))
const minColumns = computed(() => (isMobile.value ? 2 : 2))

const MASONRY_GAP = 4

const handleOpenViewer = (index: number) => {
  const photos = albumData.value?.photos
  if (photos && photos[index]) {
    const { openViewer } = useViewerState()
    const albumRoute = `/albums/${encodeURIComponent(albumData.value?.slug || albumSegment.value)}`
    // Scope the viewer to the album's photos so prev/next stays within this album.
    // The fetched photos are serialized rows of the same shape as Photo.
    openViewer(index, albumRoute, photos as Photo[])
    router.push(`/${photos[index].id}`)
  }
}

const coverPhoto = computed(() => {
  const album = albumData.value
  if (!album?.photos) return null

  // coverPhotoId first
  if (album.coverPhotoId) {
    const cover = album.photos.find((p: any) => p.id === album.coverPhotoId)
    if (cover) return cover
  }

  // otherwise first photo
  return album.photos[0] || null
})

const scrollToTop = () => {
  window.scrollTo({
    top: 0,
    behavior: 'smooth',
  })
}

const showFloatingActions = ref(false)

const handleScroll = () => {
  const scrollTop = window.pageYOffset || document.documentElement.scrollTop
  showFloatingActions.value = scrollTop > 500
}

const goBackToAlbums = () => {
  router.push('/albums')
}

onMounted(() => {
  window.addEventListener('scroll', handleScroll, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
})

useHead({
  title: computed(() => {
    const data = albumData.value
    if (data) return data.title
    // The unlock screen still names the album, so the tab is not just an error.
    return lockedAlbum.value?.title || $t('album.notFound')
  }),
})
</script>

<template>
  <div class="relative w-full">
    <div
      v-if="pending"
      class="flex flex-col items-center justify-center min-h-[50vh] gap-4"
    >
      <UIcon
        name="tabler:loader"
        class="animate-spin size-8 text-primary-600"
      />
      <p class="text-sm text-neutral-600 dark:text-neutral-400">
        {{ $t('ui.loading') }}
      </p>
    </div>

    <template v-else-if="albumData">
      <!-- Backdrop layer -->
      <div
        v-if="coverPhoto"
        class="absolute inset-0 h-2/3 sm:h-[500px] overflow-hidden -z-10"
      >
        <ThumbImage
          :src="coverPhoto.thumbnailUrl || ''"
          :thumbhash="coverPhoto.thumbnailHash"
          :alt="albumData.title"
          class="w-full h-full object-cover opacity-40 dark:opacity-20 scale-110 saturate-150"
        />
        <div
          class="absolute -inset-1 bg-gradient-to-b from-transparent via-white/50 to-white dark:via-neutral-900/50 dark:to-neutral-900 backdrop-blur-xl sm:backdrop-blur-2xl"
        />
      </div>

      <!-- Back Button -->
      <div class="relative container mx-auto px-4 sm:px-6 lg:px-8 pt-4 z-10">
        <UButton
          variant="ghost"
          color="neutral"
          icon="tabler:arrow-left"
          size="sm"
          @click="goBackToAlbums"
        />
      </div>

      <!-- Album Information -->
      <div class="relative container mx-auto px-4 sm:px-6 lg:px-8 py-8 z-10">
        <AnimatePresence>
          <motion.div
            class="flex flex-col gap-6"
            :initial="{ opacity: 0, y: 10 }"
            :animate="{ opacity: 1, y: 0 }"
            :transition="{ duration: 0.4 }"
          >
            <!-- title -->
            <div>
              <h1
                class="text-3xl sm:text-4xl font-bold text-neutral-900 dark:text-white tracking-tight"
              >
                {{ albumData.title }}
              </h1>
            </div>

            <!-- metadata -->
            <div class="flex flex-col gap-4">
              <!-- description -->
              <p
                class="text-base text-neutral-600 dark:text-neutral-300 leading-relaxed max-w-2xl"
              >
                {{ albumData.description || $t('album.noDescription') }}
              </p>

              <!-- metadata row -->
              <div class="flex flex-wrap items-center gap-4 text-sm">
                <!-- Photos -->
                <div class="flex items-center gap-1">
                  <Icon
                    name="tabler:photo"
                    class="size-4 -mt-0.5 text-neutral-400 dark:text-neutral-500"
                  />
                  <span class="text-neutral-700 dark:text-neutral-200">
                    <span class="text-neutral-900 dark:text-white">
                      {{ albumStats?.total || 0 }}
                    </span>
                    <span class="text-neutral-500 dark:text-neutral-400 ml-1">
                      {{ $t('album.metadata.photos') }}
                    </span>
                  </span>
                </div>

                <!-- Date Range -->
                <div
                  v-if="dateRangeText"
                  class="flex items-center gap-1"
                >
                  <Icon
                    name="tabler:calendar"
                    class="size-4 -mt-0.5 text-neutral-400 dark:text-neutral-500"
                  />
                  <span class="text-neutral-700 dark:text-neutral-200">
                    {{ dateRangeText }}
                  </span>
                </div>

                <!-- Created -->
                <div
                  class="flex items-center gap-1"
                  :title="$t('album.createdTooltip', { date: $dayjs(albumData.createdAt).format('YYYY-MM-DD HH:mm:ss') })"
                >
                  <Icon
                    name="tabler:clock-plus"
                    class="size-4 -mt-0.5 text-neutral-400 dark:text-neutral-500"
                  />
                  <span class="text-neutral-700 dark:text-neutral-200">
                    {{ $t('album.metadata.created') }}
                    {{ $dayjs(albumData.createdAt).fromNow() }}
                  </span>
                </div>
              </div>
            </div>
          </motion.div>
        </AnimatePresence>
      </div>

      <!-- Photos Waterfall or Empty State -->
      <div class="container mx-auto px-4 sm:px-6 lg:px-8 py-12">
        <motion.div
          :initial="{ opacity: 0 }"
          :animate="{ opacity: 1 }"
          :transition="{ delay: 0.2, duration: 0.4 }"
        >
          <div
            v-if="albumStats?.total === 0"
            class="flex flex-col items-center justify-center gap-6 px-4"
          >
            <div class="flex flex-col items-center gap-4">
              <Icon
                name="tabler:library-photo"
                class="size-20 text-neutral-300 dark:text-neutral-600"
              />
              <div class="text-center">
                <p
                  class="text-xl font-normal text-neutral-800 dark:text-neutral-200 mb-2"
                >
                  {{ $t('album.emptyAlbumTitle') }}
                </p>
              </div>
            </div>
          </div>

          <MasonryWall
            v-else
            :items="masonryItems"
            :column-width="columnWidth"
            :gap="MASONRY_GAP"
            :min-columns="minColumns"
            :max-columns="maxColumns"
            :ssr-columns="2"
            :key-mapper="
              (_item, _column, _row, index) =>
                masonryItems[index]?.originalIndex ?? index
            "
          >
            <template #default="{ item }">
              <MasonryItem
                v-if="item.photo && typeof item.originalIndex === 'number'"
                :key="item.photo.id"
                :photo="item.photo"
                :index="item.originalIndex"
                :animate="true"
                @open-viewer="handleOpenViewer($event)"
              />
            </template>
          </MasonryWall>
        </motion.div>
      </div>
    </template>

    <!-- Locked album: the browser knows it exists but not what is inside it -->
    <template v-else-if="lockedAlbum">
      <div class="absolute inset-0 -z-10 overflow-hidden">
        <div
          class="absolute left-1/2 top-0 size-[36rem] -translate-x-1/2 -translate-y-1/3 rounded-full bg-primary-500/10 blur-3xl dark:bg-primary-500/15"
        />
        <div
          class="absolute -bottom-1/4 right-0 size-[28rem] rounded-full bg-info-400/10 blur-3xl dark:bg-info-500/10"
        />
      </div>

      <div class="relative container mx-auto px-4 sm:px-6 lg:px-8 pt-4">
        <UButton
          variant="ghost"
          color="neutral"
          icon="tabler:arrow-left"
          size="sm"
          :aria-label="$t('album.backToAlbums')"
          @click="goBackToAlbums"
        />
      </div>

      <div
        class="relative container mx-auto flex min-h-[70vh] items-center justify-center px-4 py-10 sm:px-6 lg:px-8"
      >
        <motion.div
          class="w-full max-w-sm"
          :initial="{ opacity: 0, y: 14 }"
          :animate="{ opacity: 1, y: 0 }"
          :transition="{ duration: 0.4 }"
        >
          <motion.div
            :animate="unlockControls"
            class="rounded-3xl border border-neutral-200/70 bg-white/80 p-8 shadow-xl shadow-neutral-900/5 backdrop-blur-xl dark:border-neutral-800 dark:bg-neutral-900/70 dark:shadow-black/20"
          >
            <div class="flex justify-center">
              <div
                class="flex size-16 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-info-500 text-white shadow-lg shadow-primary-500/25"
              >
                <Icon
                  name="tabler:lock"
                  class="size-8"
                />
              </div>
            </div>

            <div class="mt-6 text-center">
              <h1
                class="text-xl font-semibold tracking-tight text-balance text-neutral-900 dark:text-white"
              >
                {{ lockedAlbum.title || $t('album.unlock.title') }}
              </h1>
              <p
                class="mt-2 text-sm text-pretty text-neutral-600 dark:text-neutral-400"
              >
                {{ $t('album.unlock.description') }}
              </p>
            </div>

            <form
              class="mt-6 space-y-3"
              @submit.prevent="unlockAlbum"
            >
              <UInput
                v-model="password"
                :type="showPassword ? 'text' : 'password'"
                :placeholder="$t('album.unlock.placeholder')"
                :aria-label="$t('album.unlock.placeholder')"
                :ui="{ base: 'text-center tracking-[0.2em]' }"
                size="lg"
                class="w-full"
                autofocus
              >
                <template #trailing>
                  <UButton
                    color="neutral"
                    variant="link"
                    size="xs"
                    :icon="showPassword ? 'tabler:eye-off' : 'tabler:eye'"
                    :aria-label="
                      showPassword
                        ? $t('album.unlock.hidePassword')
                        : $t('album.unlock.showPassword')
                    "
                    @click="showPassword = !showPassword"
                  />
                </template>
              </UInput>

              <UButton
                type="submit"
                block
                size="lg"
                icon="tabler:lock-open"
                :loading="isUnlocking"
                :disabled="password === ''"
              >
                {{ $t('album.unlock.submit') }}
              </UButton>
            </form>

            <Transition
              enter-active-class="transition duration-200 ease-out"
              enter-from-class="opacity-0 -translate-y-1"
              leave-active-class="transition duration-150 ease-in"
              leave-to-class="opacity-0"
            >
              <p
                v-if="unlockError"
                class="mt-4 flex items-center justify-center gap-1.5 text-center text-sm text-red-600 dark:text-red-400"
                role="alert"
                aria-live="polite"
              >
                <Icon
                  name="tabler:alert-circle"
                  class="size-4 shrink-0"
                />
                {{ unlockError }}
              </p>
            </Transition>
          </motion.div>
        </motion.div>
      </div>
    </template>

    <template v-else>
      <div class="flex flex-col items-center justify-center min-h-[50vh] gap-6">
        <div class="flex flex-col items-center gap-4">
          <Icon
            name="tabler:alert-circle"
            class="size-20 text-red-400"
          />
          <div class="text-center">
            <p
              class="text-2xl font-semibold text-neutral-800 dark:text-neutral-200 mb-2"
            >
              {{ $t('album.failedToLoadTitle') }}
            </p>
            <p
              class="text-base text-neutral-600 dark:text-neutral-400 max-w-md"
            >
              {{ $t('album.failedToLoadDescription') }}
            </p>
          </div>
        </div>
        <UButton @click="goBackToAlbums">
          {{ $t('album.backToAlbums') }}
        </UButton>
      </div>
    </template>

    <!-- 回到顶部：和照片墙右下角那颗同一个组件、同一种胶囊，只是相册页
         下面没有站点胶囊，所以它自己贴着角；而且照片墙永远压在照片上，
         相册的照片却可能早早排完，下面是白纸，所以这里用实一点的那版。 -->
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0 translate-y-1 scale-90"
      leave-active-class="transition duration-150 ease-in"
      leave-to-class="opacity-0 translate-y-1 scale-90"
    >
      <div
        v-if="showFloatingActions"
        class="fixed right-3 bottom-3 z-40 origin-bottom-right"
      >
        <BackToTop
          class="gallery-capsule--solid"
          @click="scrollToTop"
        />
      </div>
    </Transition>
  </div>
</template>

<style scoped></style>
