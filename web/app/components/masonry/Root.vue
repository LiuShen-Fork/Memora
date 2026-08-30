<script setup lang="ts">
import { motion } from 'motion-v'
import { livePhotoVideoSource } from '~/utils/live-photo'
interface Props {
  photos: Photo[]
  columns?: number | 'auto'
}

const props = withDefaults(defineProps<Props>(), {
  columns: 'auto',
})

const dayjs = useDayjs()
const { locale } = useI18n()
const router = useRouter()

const dayjsLocale = computed(() => {
  const localeMap: Record<string, string> = {
    'zh-Hans': 'zh-cn',
    'zh-Hant-TW': 'zh-tw',
    'zh-Hant-HK': 'zh-hk',
  }
  return localeMap[locale.value] || locale.value
})

const formatDate = (value: string | Date | ReturnType<typeof dayjs>, format = 'll') =>
  dayjs(value).locale(dayjsLocale.value).format(format)

const { filteredPhotos, hasActiveFilters } = usePhotoFilters()
const { sortedPhotos } = usePhotoSort()

const displayPhotos = computed(() => {
  return hasActiveFilters.value ? filteredPhotos.value : sortedPhotos.value
})

const { currentPhotoIndex, isViewerOpen } = storeToRefs(useViewerState())

const GALLERY_PAGE_SIZE = 100
const MASONRY_GAP = 4

const masonryWrapper = ref<HTMLElement>()
const showFloatingActions = ref(false)
const dateRange = ref<string>()
const visibleCities = ref<string>()
const visiblePhotos = ref(new Set<number>())
const animatedPhotoIds = ref(new Set<string>())

const isMobile = useMediaQuery('(max-width: 768px)')
const { batchProcessLivePhotos } = useLivePhotoProcessor()
const { columns: layoutColumns } = useMasonryLayout()

const processedBatch = ref(new Set<string>())
const columnWidth = computed(() => {
  if (props.columns === 'auto') {
    return 180
  }
  return 180
})

const maxColumns = computed(() => {
  if (props.columns !== 'auto') {
    return props.columns
  }
  return layoutColumns.value
})

const minColumns = computed(() => {
  if (props.columns !== 'auto') {
    return props.columns
  }
  return 2
})

const visiblePhotoCount = ref(GALLERY_PAGE_SIZE)
const animationBatchStart = ref(0)

// Prepare items for masonry-wall
const masonryItems = computed(() => {
  return (
    displayPhotos.value?.slice(0, visiblePhotoCount.value).map((photo, index) => ({
      id: photo.id,
      photo,
      originalIndex: index,
    })) ?? []
  )
})

const hasMorePhotos = computed(
  () => visiblePhotoCount.value < displayPhotos.value.length,
)

const loadMorePhotos = () => {
  const previousCount = visiblePhotoCount.value
  visiblePhotoCount.value = Math.min(
    visiblePhotoCount.value + GALLERY_PAGE_SIZE,
    displayPhotos.value.length,
  )

  // Start a short stagger for the newly appended batch. The item component
  // keeps the completed IDs so scrolling back does not replay the animation.
  animationBatchStart.value = previousCount
}

watch(displayPhotos, () => {
  visiblePhotoCount.value = GALLERY_PAGE_SIZE
  animationBatchStart.value = 0
  animatedPhotoIds.value = new Set()
  visiblePhotos.value = new Set()
  dateRange.value = undefined
  visibleCities.value = undefined
})

const shouldAnimatePhoto = (photo: Photo) => !animatedPhotoIds.value.has(photo.id)

const markPhotoAnimated = (photoId: string) => {
  if (animatedPhotoIds.value.has(photoId)) return
  animatedPhotoIds.value = new Set(animatedPhotoIds.value).add(photoId)
}

const animationDelayForIndex = (index: number) => {
  const offset = Math.max(0, index - animationBatchStart.value)
  return Math.min(offset, 40) * 0.012
}

const photoStats = computed(() => {
  const totalPhotos = displayPhotos.value?.length || 0
  const photosWithDates =
    displayPhotos.value?.filter((p) => p.dateTaken).length || 0
  const photosWithTitles =
    displayPhotos.value?.filter((p) => p.title).length || 0
  const photosWithExif = displayPhotos.value?.filter((p) => p.exif).length || 0

  // Get date range of all photos
  const allDates = displayPhotos.value
    ?.map((p) => p?.dateTaken)
    .filter((date): date is string => Boolean(date))
    .map((date) => dayjs(date))
    .sort((a, b) => (a.isBefore(b) ? -1 : 1))

  const dateRange =
    allDates.length > 0
      ? {
          start: formatDate(allDates[0]),
          end: formatDate(allDates[allDates.length - 1]),
        }
      : null

  return {
    total: totalPhotos,
    withDates: photosWithDates,
    withTitles: photosWithTitles,
    withExif: photosWithExif,
    dateRange,
  }
})

const dateRangeText = computed(() => {
  const range = photoStats.value?.dateRange
  if (!range || !range.start || !range.end) return ''
  return `${range.start} - ${range.end}`
})

const headerDateRangeText = computed(() => dateRange.value || dateRangeText.value)

const handleVisibilityChange = ({
  index,
  isVisible,
}: {
  index: number
  isVisible: boolean
  date: string | Date
}) => {
  if (isVisible) {
    visiblePhotos.value.add(index)
  } else {
    visiblePhotos.value.delete(index)
  }
  updateDateRange()

  // Process LivePhotos for visible photos
  nextTick(() => {
    processVisibleLivePhotos()
  })
}

// Process LivePhotos for currently visible photos
const processVisibleLivePhotos = async () => {
  const visiblePhotosArray = Array.from(visiblePhotos.value)
  const livePhotosToProcess = visiblePhotosArray
    .map((index) => displayPhotos.value[index])
    .filter(
      (photo): photo is Photo =>
        photo != null &&
        Boolean(photo.isLivePhoto) &&
        Boolean(livePhotoVideoSource(photo)) &&
        !processedBatch.value.has(photo.id),
    )

  if (livePhotosToProcess.length === 0) return

  // Mark as processed to avoid reprocessing
  livePhotosToProcess.forEach((photo) => {
    processedBatch.value.add(photo.id)
  })

  // Start background processing
  batchProcessLivePhotos(
    livePhotosToProcess.map((photo) => ({
      id: photo.id,
      livePhotoVideoUrl: livePhotoVideoSource(photo)!,
    })),
  )
}

const updateDateRange = () => {
  if (visiblePhotos.value.size === 0) {
    dateRange.value = undefined
    visibleCities.value = undefined
    return
  }

  const visiblePhotosArray = Array.from(visiblePhotos.value)

  // Calculate visible dates
  const visibleDates = visiblePhotosArray
    .map((index) => displayPhotos.value[index]?.dateTaken)
    .filter((date): date is string => Boolean(date))
    .map((date) => dayjs(date))
    .sort((a, b) => (a.isBefore(b) ? -1 : 1))

  // Calculate visible cities
  const cities = visiblePhotosArray
    .map((index) => displayPhotos.value[index]?.city)
    .filter((city): city is string => Boolean(city))

  const uniqueCities = [...new Set(cities)]

  if (uniqueCities.length === 0) {
    visibleCities.value = undefined
  } else if (uniqueCities.length === 1) {
    visibleCities.value = uniqueCities[0]
  } else if (uniqueCities.length <= 3) {
    visibleCities.value = uniqueCities.join('、')
  } else {
    visibleCities.value =
      `${uniqueCities.slice(0, 2).join('、')} ` +
      $t('ui.indexPanelCountCity', { count: uniqueCities.length })
  }

  if (visibleDates.length === 0) {
    dateRange.value = undefined
    return
  }

  const startDate = visibleDates[0]
  const endDate = visibleDates[visibleDates.length - 1]

  if (!startDate || !endDate) {
    dateRange.value = undefined
    return
  }

  // Check if dates are the same day
  if (startDate.isSame(endDate, 'day')) {
    // Same day
    dateRange.value = formatDate(startDate)
  } else if (startDate.isSame(endDate, 'month')) {
    // Same month
    dateRange.value = formatDate(startDate, 'MMM YYYY')
  } else if (startDate.isSame(endDate, 'year')) {
    // Same year, different months
    dateRange.value = `${formatDate(startDate, 'MMM')} - ${formatDate(endDate, 'MMM YYYY')}`
  } else {
    // Different years
    dateRange.value = `${formatDate(startDate)} - ${formatDate(endDate)}`
  }
}

watch(locale, () => updateDateRange())

const handleScroll = () => {
  const scrollTop = window.pageYOffset || document.documentElement.scrollTop
  showFloatingActions.value = scrollTop > 500
}

const scrollToTop = () => {
  window.scrollTo({
    top: 0,
    behavior: 'smooth',
  })
}

onMounted(() => {
  window.addEventListener('scroll', handleScroll, { passive: true })

  nextTick(() => {
    if (currentPhotoIndex.value) {
      scrollToPhoto(currentPhotoIndex.value)
    }
  })
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
})

const handleOpenViewer = (index: number) => {
  router.push(`/${displayPhotos.value[index]?.id}`)
}

const scrollToPhoto = (photoIndex: number) => {
  if (!displayPhotos.value[photoIndex]) return

  const photoId = displayPhotos.value[photoIndex].id
  const photoElement = document.querySelector(`[data-photo-id="${photoId}"]`)

  if (photoElement) {
    const elementRect = photoElement.getBoundingClientRect()
    const windowHeight = window.innerHeight
    const currentScrollY = window.pageYOffset

    // 让图片在视口中央
    const targetScrollY =
      currentScrollY +
      elementRect.top -
      windowHeight / 2 +
      elementRect.height / 2

    window.scrollTo({
      top: Math.max(0, targetScrollY),
      behavior: 'smooth',
    })
  }
}

watch(currentPhotoIndex, (newIndex) => {
  if (isViewerOpen.value && newIndex >= 0) {
    nextTick(() => {
      scrollToPhoto(newIndex)
    })
  }
})
</script>

<template>
  <div class="relative w-full">
    <!-- Back to Top Button -->
    <motion.div
      v-if="showFloatingActions"
      class="fixed bottom-6 right-6 z-50"
      :initial="{ opacity: 0, scale: 0.8 }"
      :animate="{ opacity: 1, scale: 1 }"
      :exit="{ opacity: 0, scale: 0.8 }"
      :transition="{ duration: 0.2 }"
    >
      <UTooltip :text="$t('ui.action.backtotop.tooltip')">
        <UButton
          variant="soft"
          color="neutral"
          class="cursor-pointer bg-white/80 dark:bg-neutral-900/80 backdrop-blur-sm flex justify-center items-center rounded-full shadow-lg hover:bg-white dark:hover:bg-neutral-800 transition-all duration-300 border border-neutral-200/50 dark:border-neutral-700/50"
          icon="tabler:arrow-up"
          size="lg"
          :aria-label="$t('ui.action.backtotop.ariaLabel')"
          @click="scrollToTop"
        />
      </UTooltip>
    </motion.div>

    <div
      class="lg:px-0 lg:pb-0"
      :class="isMobile ? 'pb-1' : 'py-1'"
    >
      <div
        ref="masonryWrapper"
        class="relative"
      >
        <div
          class="masonry-header-wrapper"
        >
          <MasonryItemHeader
            :stats="photoStats"
            :date-range-text="headerDateRangeText"
            :locations="visibleCities"
            :is-scrolled="showFloatingActions"
          />
        </div>

        <!-- Masonry Wall -->
        <MasonryStableWall
          class="masonry-wall-with-header"
          :items="masonryItems"
          :column-width="columnWidth"
          :gap="MASONRY_GAP"
          :min-columns="minColumns"
          :max-columns="maxColumns"
          :ssr-columns="2"
          :key-mapper="
            (item) => item.id
          "
        >
          <template #default="{ item }">
            <!-- Photo Items -->
            <MasonryLazyItem
              v-if="item.photo && typeof item.originalIndex === 'number'"
              :key="item.photo.id"
              :photo="item.photo"
              :index="item.originalIndex"
              :animate="shouldAnimatePhoto(item.photo)"
              :animation-delay="animationDelayForIndex(item.originalIndex)"
              @animation-complete="markPhotoAnimated(item.photo.id)"
              @visibility-change="handleVisibilityChange"
              @open-viewer="handleOpenViewer($event)"
            />
          </template>
        </MasonryStableWall>

        <div
          v-if="hasMorePhotos"
          class="flex justify-center py-8"
        >
          <UButton
            color="neutral"
            variant="soft"
            icon="tabler:chevron-down"
            :label="$t('ui.action.loadMore')"
            @click="loadMorePhotos"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.masonry-header-wrapper {
  height: 3.1rem;
}

@media (max-width: 768px) {
  .masonry-header-wrapper {
    height: 2.8rem;
  }
}
</style>
