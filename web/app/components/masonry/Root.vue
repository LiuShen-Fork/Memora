<script setup lang="ts">
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

const MASONRY_GAP = 4

const masonryWrapper = ref<HTMLElement>()
// 顶栏收窄和「回到顶部」共用同一个判断：过了首屏才算滚起来。
const showFloatingActions = useScrolledPast()
const dateRange = ref<string>()
const visibleCities = ref<string>()
const visiblePhotos = ref(new Set<number>())
const animatedPhotoIds = ref(new Set<string>())

const isMobile = useMediaQuery('(max-width: 768px)')
const { columns: layoutColumns } = useMasonryLayout()
const {
  loadMore,
  hasMore: hasMorePhotos,
  isLoadingMore: isLoadingMorePhotos,
  totalCount,
} = usePhotos()

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

const animationBatchStart = ref(0)
const isWallAppending = ref(false)

// Prepare items for masonry-wall
const masonryItems = computed(() => {
  return (
    displayPhotos.value?.map((photo, index) => ({
      id: photo.id,
      photo,
      originalIndex: index,
    })) ?? []
  )
})

const loadMorePhotos = async () => {
  const previousCount = displayPhotos.value.length
  animationBatchStart.value = previousCount
  isWallAppending.value = true
  try {
    await loadMore()
    await nextTick()
    if (displayPhotos.value.length === previousCount) isWallAppending.value = false
  } catch (error) {
    isWallAppending.value = false
    console.error('[Gallery] Failed to load more photos:', error)
  }
}

watch(displayPhotos, (next, previous) => {
  const appended =
    next.length > previous.length &&
    previous.every((photo, index) => photo.id === next[index]?.id)
  if (appended) return
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
  const totalPhotos = hasActiveFilters.value
    ? displayPhotos.value.length
    : totalCount.value
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

// 首页页头的形状由设置决定，未知值一律回到紧凑顶栏 —— 它是这个项目一直
// 以来的样子，也是升级后没人动过设置时应该看到的样子。
const headerLayoutSetting = useSettingRef('app:appearance.homeHeaderLayout')
const headerLayout = computed<'compact' | 'embedded' | 'fixed'>(() => {
  const value = headerLayoutSetting.value
  return value === 'embedded' || value === 'fixed' ? value : 'compact'
})

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

onMounted(() => {
  nextTick(() => {
    if (currentPhotoIndex.value) {
      scrollToPhoto(currentPhotoIndex.value)
    }
  })
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
    <div
      class="lg:px-0 lg:pb-0"
      :class="isMobile ? 'pb-1' : 'py-1'"
    >
      <div
        ref="masonryWrapper"
        class="relative"
      >
        <!-- 紧凑顶栏是浮在照片上的，所以在照片墙前面留出它的高度；
             大页头自己占位置，不需要这块留白。 -->
        <div
          v-if="headerLayout === 'compact'"
          class="masonry-header-wrapper"
        />
        <MasonryItemHeader
          :layout="headerLayout"
          :stats="photoStats"
          :date-range-text="headerDateRangeText"
          :locations="visibleCities"
          :is-scrolled="showFloatingActions"
        />

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
          @layout-complete="isWallAppending = false"
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
          v-if="hasMorePhotos || isLoadingMorePhotos || isWallAppending"
          class="flex justify-center py-8"
        >
          <UButton
            color="neutral"
            variant="soft"
            icon="tabler:chevron-down"
            :label="$t('ui.action.loadMore')"
            :loading="isLoadingMorePhotos || isWallAppending"
            :disabled="isLoadingMorePhotos || isWallAppending"
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
