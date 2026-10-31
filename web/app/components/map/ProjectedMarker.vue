<script lang="ts" setup>
import { onBeforeUnmount, shallowRef, watch } from 'vue'

const props = withDefaults(
  defineProps<{
    markerId?: string
    /** `[longitude, latitude]`, the order the rest of the app uses. */
    lnglat?: [number, number]
    map?: any
    offset?: [number, number]
  }>(),
  {
    markerId: undefined,
    lnglat: undefined,
    map: undefined,
    offset: () => [-20, -20],
  },
)

const mapConfig = computed(() => {
  const config = getSetting('map')
  return typeof config === 'object' && config ? config : {}
})
const provider = computed(() => mapConfig.value.provider || 'maplibre')

const injectedMap = inject<Ref<any>>(
  provider.value === 'tencent' ? 'tencent' : 'tianditu',
  ref(),
)
const mapToUse = computed(() => props.map || injectedMap.value)

const host = shallowRef<HTMLDivElement | null>(null)
let element: HTMLDivElement | null = null
let boundMap: any = null
let unsubscribe: (() => void) | null = null

/**
 * Where a coordinate sits inside the map's own box.
 *
 * Both APIs answer the same question under a different name, and both return a
 * point in container pixels. Asking the map each time rather than caching the
 * answer is what keeps the marker over its coordinate while the map is panned.
 */
const project = (map: any, lng: number, lat: number) => {
  if (provider.value === 'tencent') {
    if (!window.TMap) return null
    const point = map.projectToContainer?.(new window.TMap.LatLng(lat, lng))
    return point ? { x: point.x, y: point.y } : null
  }
  if (!window.T) return null
  const point = map.lngLatToContainerPoint?.(new window.T.LngLat(lng, lat))
  return point ? { x: point.x, y: point.y } : null
}

const place = () => {
  if (!element || !props.lnglat) return
  const point = project(mapToUse.value, props.lnglat[0], props.lnglat[1])
  if (!point) return
  const [offsetX, offsetY] = props.offset
  element.style.transform = `translate(${point.x + offsetX}px, ${point.y + offsetY}px)`
}

/**
 * Redraw whenever the map does.
 *
 * Tencent's map offers a callback that runs on every frame it renders, which is
 * exactly the moment a projection is worth recomputing. Tianditu has no such
 * callback, so it is heard through the events that move it. Both approaches
 * stay silent while the map is still, so a page full of pins costs nothing.
 */
const subscribe = (map: any) => {
  unsubscribe?.()
  unsubscribe = null
  boundMap = map
  if (provider.value === 'tencent') {
    map.addRenderCallback?.(place)
    unsubscribe = () => map.removeRenderCallback?.(place)
    return
  }
  for (const event of ['move', 'zoom', 'resize']) {
    map.addEventListener?.(event, place)
  }
  unsubscribe = () => {
    for (const event of ['move', 'zoom', 'resize']) {
      map.removeEventListener?.(event, place)
    }
  }
}

const detach = () => {
  unsubscribe?.()
  unsubscribe = null
  boundMap = null
  element?.remove()
  element = null
  host.value = null
}

const attach = (map: any, coordinates: [number, number]) => {
  detach()
  const container = map?.getContainer?.()
  if (!container) return

  const node = document.createElement('div')
  node.dataset.markerId = props.markerId || ''
  // The map itself is dragged by pointer events; a pin must take the ones that
  // land on it and let the rest through, which is what stopping the click at
  // the pin's own box does.
  node.addEventListener('click', (event) => event.stopPropagation())
  node.addEventListener('dblclick', (event) => event.stopPropagation())
  node.style.position = 'absolute'
  node.style.left = '0'
  node.style.top = '0'
  // Above the map's own layers. Tianditu stacks its panes at 400 and keeps its
  // tiles inside them, so a pin at the usual small z-index is drawn underneath
  // the map and simply never appears — it still hit-tests, which is what makes
  // that failure look like a rendering bug rather than a layering one.
  node.style.zIndex = '1000'
  node.style.pointerEvents = 'auto'
  node.style.willChange = 'transform'

  element = node
  host.value = node
  container.appendChild(node)

  subscribe(map)
  place()
}

watch(
  () => [mapToUse.value, props.lnglat, props.offset, props.markerId] as const,
  ([map, coordinates], previous) => {
    if (!map || !coordinates) {
      detach()
      return
    }
    const sameMap = previous?.[0] === map && element !== null
    if (!sameMap) {
      nextTick(() => attach(map, coordinates))
      return
    }
    if (element) element.dataset.markerId = props.markerId || ''
    place()
  },
  { immediate: true },
)

onBeforeUnmount(detach)
</script>

<template>
  <Teleport
    v-if="host"
    :to="host"
  >
    <slot name="marker" />
  </Teleport>
</template>
