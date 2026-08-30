<script setup lang="ts" generic="T">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'

type KeyMapper<T> = (
  item: T,
  column: number,
  row: number,
  index: number,
) => string | number | symbol | undefined

const props = withDefaults(
  defineProps<{
    items: T[]
    columnWidth?: number | number[]
    gap?: number
    minColumns?: number
    maxColumns?: number
    rtl?: boolean
    ssrColumns?: number
    keyMapper?: KeyMapper<T>
  }>(),
  {
    columnWidth: 400,
    gap: 0,
    minColumns: 1,
    maxColumns: undefined,
    rtl: false,
    ssrColumns: 0,
    keyMapper: undefined,
  },
)

defineSlots<{
  default?: (props: {
    item: T
    column: number
    columnCount: number
    row: number
    index: number
  }) => unknown
}>()

const wall = ref<HTMLElement>()
const columns = ref<number[][]>([])
let redrawToken = 0
let resizeFrame: number | undefined
let resizeObserver: ResizeObserver | undefined
let observedWidth = 0

const widthForColumn = (index: number) => {
  const widths = Array.isArray(props.columnWidth)
    ? props.columnWidth
    : [props.columnWidth]
  return widths[index % widths.length] || widths[0] || 400
}

const createColumns = (count: number) =>
  Array.from({ length: Math.max(1, count) }, () => [] as number[])

const columnCount = () => {
  const availableWidth = wall.value?.getBoundingClientRect().width || 0
  if (!availableWidth) return props.ssrColumns || props.minColumns

  let count = 0
  let consumed = -props.gap
  while (consumed + props.gap + widthForColumn(count) <= availableWidth) {
    consumed += props.gap + widthForColumn(count)
    count += 1
  }
  return Math.max(props.minColumns, Math.min(props.maxColumns || count, count || 1))
}

const itemKey = (item: T, index: number) =>
  props.keyMapper?.(item, 0, 0, index) ?? index

const isAppend = (previous: T[], next: T[]) => {
  if (next.length <= previous.length || previous.length === 0) return false
  return previous.every((item, index) => Object.is(itemKey(item, index), itemKey(next[index]!, index)))
}

const currentHeights = () =>
  [...(wall.value?.children || [])].map((column) =>
    (column as HTMLElement).getBoundingClientRect().height,
  )

const itemHeights = () => {
  const heights = new Map<number, number>()
  wall.value?.querySelectorAll<HTMLElement>('[data-masonry-item-index]').forEach((element) => {
    const index = Number(element.dataset.masonryItemIndex)
    if (Number.isInteger(index)) heights.set(index, element.getBoundingClientRect().height)
  })
  return heights
}

// Build a new assignment without clearing the rendered columns. This keeps
// the old wall visible while sorting, resizing, or changing the column count.
const rebuild = async () => {
  const token = ++redrawToken
  await nextTick()
  if (token !== redrawToken) return

  const count = columnCount()
  const heights = itemHeights()
  const nextColumns = createColumns(count)
  const nextHeights = Array.from({ length: count }, () => 0)

  props.items.forEach((_item, index) => {
    let target = 0
    for (let column = 1; column < count; column += 1) {
      if (nextHeights[column]! < nextHeights[target]!) target = column
    }
    nextColumns[target]!.push(index)
    nextHeights[target]! += (heights.get(index) || 0) + props.gap
  })

  columns.value = nextColumns
}

// Append one item at a time so each new card is placed using the measured
// current column height; existing cards never leave the DOM during loading.
const append = async (start: number) => {
  const token = ++redrawToken
  for (let index = start; index < props.items.length; index += 1) {
    if (token !== redrawToken) return
    await nextTick()
    const heights = currentHeights()
    let target = 0
    for (let column = 1; column < heights.length; column += 1) {
      if (heights[column]! < heights[target]!) target = column
    }
    const nextColumns = columns.value.map((column) => [...column])
    ;(nextColumns[target] ||= []).push(index)
    columns.value = nextColumns
  }
}

const handleItemsChange = (next: T[], previous: T[]) => {
  if (isAppend(previous, next)) {
    void append(previous.length)
  } else {
    void rebuild()
  }
}

onMounted(async () => {
  const initialCount = props.ssrColumns || props.minColumns
  columns.value = createColumns(initialCount)
  for (let index = 0; index < props.items.length; index += 1) {
    columns.value[index % columns.value.length]!.push(index)
  }
  await rebuild()

  resizeObserver = new ResizeObserver(() => {
    const nextWidth = wall.value?.getBoundingClientRect().width || 0
    if (nextWidth === observedWidth) return
    observedWidth = nextWidth
    if (resizeFrame !== undefined) cancelAnimationFrame(resizeFrame)
    resizeFrame = requestAnimationFrame(() => void rebuild())
  })
  if (wall.value) resizeObserver.observe(wall.value)
})

watch(() => props.items, handleItemsChange)
watch(
  () => [props.columnWidth, props.gap, props.minColumns, props.maxColumns],
  () => void rebuild(),
)

onUnmounted(() => {
  redrawToken += 1
  resizeObserver?.disconnect()
  if (resizeFrame !== undefined) cancelAnimationFrame(resizeFrame)
})
</script>

<template>
  <div
    ref="wall"
    class="masonry-wall"
    :class="`masonry-columns-${columns.length}`"
    :style="{
      display: 'flex',
      gap: `${gap}px`,
      flexDirection: rtl ? 'row-reverse' : undefined,
    }"
  >
    <div
      v-for="(column, columnIndex) in columns"
      :key="columnIndex"
      class="masonry-column"
      :data-index="columnIndex"
      :style="{
        display: 'flex',
        flexBasis: `${widthForColumn(columnIndex)}px`,
        flexDirection: 'column',
        flexGrow: 1,
        gap: `${gap}px`,
        height: 'max-content',
        minWidth: 0,
      }"
    >
      <div
        v-for="(itemIndex, row) in column"
        :key="itemKey(items[itemIndex]!, itemIndex)"
        class="masonry-item"
        :data-masonry-item-index="itemIndex"
      >
        <slot
          :item="items[itemIndex]!"
          :column="columnIndex"
          :column-count="columns.length"
          :row="row"
          :index="itemIndex"
        />
      </div>
    </div>
  </div>
</template>
