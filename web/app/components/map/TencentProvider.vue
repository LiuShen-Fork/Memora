<script lang="ts" setup>
import { twMerge } from 'tailwind-merge'
import type { TencentMap } from '~~/shared/types/map'
import { transformCoordinate } from '~/utils/coordinate-transform'

const props = withDefaults(
  defineProps<{
    class?: string
    mapId?: string
    center?: [number, number]
    zoom?: number
    interactive?: boolean
  }>(),
  {
    class: undefined,
    mapId: 'tencent-container',
    center: () => [116.397428, 39.90923],
    zoom: 10,
    interactive: true,
  },
)

const emit = defineEmits<{
  load: [map: TencentMap]
  zoom: []
}>()

const mapContainer = ref<HTMLDivElement | null>(null)
const mapInstance = shallowRef<TencentMap>()
const isLoaded = ref(false)
const colorMode = useColorMode()

const mapConfig = computed(() => {
  const config = getSetting('map')
  return typeof config === 'object' && config ? config : {}
})
const tencentKey = computed(() => String(mapConfig.value['tencent.key'] || ''))

/**
 * The two styles the API ships with. `style2` is its dark one; the rest of the
 * set is there for the map to draw on, and the interface picks between these
 * two the way it does for every other provider.
 */
const mapStyleId = computed(() =>
  colorMode.value === 'dark' ? 'style2' : 'style1',
)

let scriptRequest: Promise<void> | null = null
const loadScript = () => {
  if (window.TMap) return Promise.resolve()
  if (scriptRequest) return scriptRequest
  scriptRequest = new Promise((resolve, reject) => {
    if (!tencentKey.value) {
      reject(new Error('Tencent map key is not configured'))
      return
    }
    const script = document.createElement('script')
    script.src = `https://map.qq.com/api/gljs?v=1.exp&key=${encodeURIComponent(tencentKey.value)}`
    script.onload = () => resolve()
    script.onerror = () => reject(new Error('Failed to load Tencent map script'))
    document.head.appendChild(script)
  })
  return scriptRequest
}

/**
 * The handful of calls the rest of the app makes on whichever map it was
 * handed.
 *
 * The globe, the mini map and the location picker were written against AMap
 * (which has `setZoomAndCenter` and `flyTo`) and MapLibre (which has `flyTo`
 * and `easeTo`). Rather than teach each of them about a third and a fourth
 * vocabulary, the Tencent map is given the two methods they look for. Both take
 * the same arguments AMap's do, and both take a `[lng, lat]` pair, which is
 * what the callers pass.
 */
const withCompatibility = (map: TencentMap) => {
  const asLatLng = (center: [number, number] | { lng: number; lat: number }) =>
    Array.isArray(center)
      ? new window.TMap.LatLng(center[1], center[0])
      : new window.TMap.LatLng(center.lat, center.lng)

  map.setZoomAndCenter = (
    zoom: number,
    center: [number, number],
    animated = true,
    duration = 300,
  ) => {
    const target = asLatLng(center)
    if (animated && typeof map.easeTo === 'function') {
      map.easeTo({ center: target, zoom, duration })
      return
    }
    map.setCenter(target)
    map.setZoom(zoom)
  }
  map.zoomIn = () => map.setZoom(map.getZoom() + 1)
  map.zoomOut = () => map.setZoom(map.getZoom() - 1)
  map.fitBounds = (bounds: [[number, number], [number, number]]) => {
    const [[swLng, swLat], [neLng, neLat]] = bounds
    const box = new window.TMap.LatLngBounds(
      new window.TMap.LatLng(swLat, swLng),
      new window.TMap.LatLng(neLat, neLng),
    )
    // The GL API answers what zoom a box needs rather than moving there, which
    // is the half of `fitBounds` that takes judgement.
    const zoom =
      typeof map.getZoomByBounds === 'function'
        ? map.getZoomByBounds(box)
        : map.getZoom()
    map.setZoomAndCenter(zoom, [(swLng + neLng) / 2, (swLat + neLat) / 2])
  }
  return map
}

const applyInteractive = (map: TencentMap) => {
  const enabled = props.interactive
  map.setDraggable?.(enabled)
  map.setScrollable?.(enabled)
  map.setDoubleClickZoom?.(enabled)
  map.setRotatable?.(false)
}

const initMap = async () => {
  if (!mapContainer.value || !tencentKey.value) return
  try {
    await loadScript()
    const center = transformCoordinate(props.center[0], props.center[1], 'tencent')
    const map = new window.TMap.Map(mapContainer.value, {
      center: new window.TMap.LatLng(center[1], center[0]),
      zoom: props.zoom,
      mapStyleId: mapStyleId.value,
    })
    withCompatibility(map)
    applyInteractive(map)
    map.on('zoom', () => emit('zoom'))
    map.on('idle', () => {
      if (isLoaded.value) return
      isLoaded.value = true
      emit('load', map)
    })
    mapInstance.value = map
    // `idle` is the point where the first frame has been drawn. It does not
    // fire if the map never moves, so the first one is also waited for.
    setTimeout(() => {
      if (isLoaded.value) return
      isLoaded.value = true
      emit('load', map)
    }, 1200)
  } catch (error) {
    console.error('[Tencent] failed to initialize:', error)
  }
}

watch(
  () => [props.center, props.zoom] as const,
  ([center, zoom]) => {
    if (!mapInstance.value || !center) return
    const [lng, lat] = transformCoordinate(center[0], center[1], 'tencent')
    mapInstance.value.setZoomAndCenter(zoom, [lng, lat], true, 0)
  },
)

watch(mapStyleId, (style) => mapInstance.value?.setMapStyleId?.(style))

provide('tencent', mapInstance)
onMounted(initMap)
onBeforeUnmount(() => mapInstance.value?.destroy())
defineExpose({ map: mapInstance })
</script>

<template>
  <div :class="twMerge('relative h-full w-full', $props.class)">
    <div
      :id="mapId"
      ref="mapContainer"
      class="h-full w-full"
    />
    <div
      v-if="!isLoaded && !tencentKey"
      class="absolute inset-0 flex items-center justify-center bg-neutral-100/80 text-xs text-neutral-500 dark:bg-neutral-900/80 dark:text-neutral-400"
    >
      {{ $t('map.tencent.missingKey') }}
    </div>
    <slot
      v-if="isLoaded"
      :map="mapInstance"
    />
  </div>
</template>
