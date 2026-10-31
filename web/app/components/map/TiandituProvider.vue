<script lang="ts" setup>
import { twMerge } from 'tailwind-merge'
import type { TiandituMap } from '~~/shared/types/map'

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
    mapId: 'tianditu-container',
    center: () => [116.397428, 39.90923],
    zoom: 10,
    interactive: true,
  },
)

const emit = defineEmits<{
  load: [map: TiandituMap]
  zoom: []
}>()

const colorMode = useColorMode()
const mapInstance = shallowRef<TiandituMap>()
const isLoaded = ref(false)

const mapConfig = computed(() => {
  const config = getSetting('map')
  return typeof config === 'object' && config ? config : {}
})
const tiandituKey = computed(() => String(mapConfig.value['tianditu.key'] || ''))
const securityKey = computed(() =>
  String(mapConfig.value['tianditu.securityKey'] || ''),
)

let scriptRequest: Promise<void> | null = null
const loadScript = () => {
  if (window.T) return Promise.resolve()
  if (scriptRequest) return scriptRequest
  scriptRequest = new Promise((resolve, reject) => {
    if (!tiandituKey.value) {
      reject(new Error('Tianditu key is not configured'))
      return
    }
    const script = document.createElement('script')
    script.src = `https://api.tianditu.gov.cn/api?v=4.0&tk=${encodeURIComponent(tiandituKey.value)}`
    script.onload = () => resolve()
    script.onerror = () => reject(new Error('Failed to load Tianditu script'))
    document.head.appendChild(script)
  })
  return scriptRequest
}

/**
 * Tianditu carries the key as a global (`T.tk`) that the API concatenates into
 * every tile address it builds, and the security key is not part of it.
 *
 * A key configured with a security key answers 301020 until the request carries
 * one, so the two are folded into that string before the first map is created —
 * setting it afterwards leaves the addresses already handed to the browser
 * alone. The trailing separator is what the API expects to find before it
 * appends its own parameters.
 */
const applySecurityKey = () => {
  const key = tiandituKey.value
  if (!key || !securityKey.value) return
  window.T.tk = `tk=${key}&sk=${securityKey.value}&`
}

/**
 * The same compatibility methods the Tencent provider adds, for the same
 * reason: the callers were written against AMap and MapLibre. Tianditu speaks
 * `centerAndZoom` and `panTo`, and its events are add/removeEventListener.
 */
const withCompatibility = (map: TiandituMap) => {
  const asLatLng = (center: [number, number]) =>
    new window.T.LngLat(center[0], center[1])

  // `zoomIn` and `zoomOut` are already Tianditu's own, and both take no
  // arguments, which is how the callers use them.
  map.setZoomAndCenter = (zoom: number, center: [number, number]) => {
    map.centerAndZoom(asLatLng(center), zoom)
  }
  map.fitBounds = (bounds: [[number, number], [number, number]]) => {
    const [[swLng, swLat], [neLng, neLat]] = bounds
    map.setViewport([
      new window.T.LngLat(swLng, swLat),
      new window.T.LngLat(neLng, neLat),
    ])
  }
  if (typeof map.on !== 'function') {
    map.on = (event: string, handler: (...args: any[]) => void) =>
      map.addEventListener(event, handler)
    map.off = (event: string, handler: (...args: any[]) => void) =>
      map.removeEventListener(event, handler)
  }
  return map
}

const applyInteractive = (map: TiandituMap) => {
  const enabled = props.interactive
  if (enabled) {
    map.enableDrag?.()
    map.enableScrollWheelZoom?.()
    map.enableDoubleClickZoom?.()
    map.enableKeyboard?.()
  } else {
    map.disableDrag?.()
    map.disableScrollWheelZoom?.()
    map.disableDoubleClickZoom?.()
    map.disableKeyboard?.()
  }
}

const initMap = async () => {
  if (!tiandituKey.value) return
  try {
    await loadScript()
    applySecurityKey()
    const map = new window.T.Map(props.mapId)
    withCompatibility(map)
    map.centerAndZoom(
      new window.T.LngLat(props.center[0], props.center[1]),
      props.zoom,
    )
    applyInteractive(map)
    map.addEventListener('zoomend', () => emit('zoom'))
    map.addEventListener('load', () => {
      isLoaded.value = true
      emit('load', map)
    })
    mapInstance.value = map
    // `load` is not guaranteed once the tiles are cached, and a map with no
    // listener firing never tells its markers to appear.
    setTimeout(() => {
      if (isLoaded.value) return
      isLoaded.value = true
      emit('load', map)
    }, 1200)
  } catch (error) {
    console.error('[Tianditu] failed to initialize:', error)
  }
}

watch(
  () => [props.center, props.zoom] as const,
  ([center, zoom]) => {
    if (!mapInstance.value || !center) return
    mapInstance.value.centerAndZoom(
      new window.T.LngLat(center[0], center[1]),
      zoom,
    )
  },
)

provide('tianditu', mapInstance)
onMounted(initMap)
onBeforeUnmount(() => mapInstance.value?.clearOverLays?.())
defineExpose({ map: mapInstance })
</script>

<template>
  <div
    :class="
      twMerge(
        'relative h-full w-full',
        colorMode.value === 'dark' ? 'tianditu-dark' : '',
        $props.class,
      )
    "
  >
    <div
      :id="mapId"
      class="h-full w-full"
    />
    <div
      v-if="!isLoaded && !tiandituKey"
      class="absolute inset-0 flex items-center justify-center bg-neutral-100/80 text-xs text-neutral-500 dark:bg-neutral-900/80 dark:text-neutral-400"
    >
      {{ $t('map.tianditu.missingKey') }}
    </div>
    <slot
      v-if="isLoaded"
      :map="mapInstance"
    />
  </div>
</template>

<style scoped>
/* Tianditu's raster basemaps have no dark variant. Inverting the drawn tiles
 * keeps the map from being a sheet of white in the dark theme; the hue rotation
 * puts the greens and blues back where they belong, and the brightness comes
 * down far enough that markers and labels still read against it. */
.tianditu-dark :deep(.tdt-tile-pane) {
  filter: invert(1) hue-rotate(180deg) brightness(0.82) contrast(0.9);
}
</style>
