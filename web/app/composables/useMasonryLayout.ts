const MASONRY_COLUMNS_KEY = 'memora-masonry-columns'
const MASONRY_COLUMN_WIDTH = 180
const MASONRY_GAP = 4

export function useMasonryLayout() {
  const preferredColumns = useState<number | null>(
    'masonry-columns-preference',
    () => {
      if (!import.meta.client) return null
      const stored = Number(window.localStorage.getItem(MASONRY_COLUMNS_KEY))
      return Number.isInteger(stored) && stored >= 2 ? stored : null
    },
  )
  const { width } = useWindowSize()

  const bounds = computed(() => {
    const viewportWidth = width.value || 0
    if (viewportWidth <= 768) {
      return { min: 2, max: 2, adjustable: false }
    }

    const physicalMax = Math.max(
      2,
      Math.floor((viewportWidth + MASONRY_GAP) / (MASONRY_COLUMN_WIDTH + MASONRY_GAP)),
    )
    const breakpointMax = viewportWidth < 900 ? 4 : viewportWidth < 1280 ? 6 : 8
    const max = Math.min(physicalMax, breakpointMax)
    return { min: 2, max: Math.max(2, max), adjustable: max > 2 }
  })

  const columns = computed(() => {
    const selected = preferredColumns.value ?? bounds.value.max
    return Math.min(Math.max(selected, bounds.value.min), bounds.value.max)
  })

  const selectedColumns = computed({
    get: () => columns.value,
    set: (value: number | number[] | undefined) => {
      const next = Array.isArray(value) ? value[0] : value
      if (typeof next === 'number' && Number.isFinite(next)) {
        preferredColumns.value = Math.round(next)
      }
    },
  })

  watch(
    [() => bounds.value.max, () => bounds.value.min],
    ([max, min]) => {
      if (!width.value) return
      const selected = preferredColumns.value
      if (selected == null) return
      const clamped = Math.min(Math.max(selected, min), max)
      if (clamped !== selected) preferredColumns.value = clamped
    },
    { immediate: true },
  )

  watch(preferredColumns, (value) => {
    if (!import.meta.client) return
    if (value == null) window.localStorage.removeItem(MASONRY_COLUMNS_KEY)
    else window.localStorage.setItem(MASONRY_COLUMNS_KEY, String(value))
  })

  return {
    columns,
    selectedColumns,
    preferredColumns,
    minColumns: computed(() => bounds.value.min),
    maxColumns: computed(() => bounds.value.max),
    isAdjustable: computed(() => bounds.value.adjustable),
  }
}
