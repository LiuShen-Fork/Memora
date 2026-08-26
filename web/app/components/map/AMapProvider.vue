<script lang="ts" setup>
import { twMerge } from 'tailwind-merge'
import type { AMapMap } from '~~/shared/types/map'
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
    mapId: 'amap-container',
    center: () => [116.397428, 39.90923],
    zoom: 10,
    interactive: true,
  },
)

const emit = defineEmits<{
  load: [map: AMapMap]
  zoom: []
}>()

const mapContainer = ref<HTMLDivElement | null>(null)
const mapInstance = shallowRef<AMapMap>()
const isLoaded = ref(false)
const colorMode = useColorMode()
const mapConfig = computed(() => {
  const config = getSetting('map')
  return typeof config === 'object' && config ? config : {}
})
const amapKey = computed(() => String(mapConfig.value['amap.key'] || ''))
const securityCode = computed(() => String(mapConfig.value['amap.securityCode'] || ''))
const mapStyle = computed(() =>
  colorMode.value === 'dark' ? 'amap://styles/dark' : 'amap://styles/normal',
)

let scriptRequest: Promise<void> | null = null
const loadScript = () => {
  if (window.AMap) return Promise.resolve()
  if (scriptRequest) return scriptRequest
  scriptRequest = new Promise((resolve, reject) => {
    if (!amapKey.value) {
      reject(new Error('AMap key is not configured'))
      return
    }
    if (securityCode.value) {
      window._AMapSecurityConfig = { securityJsCode: securityCode.value }
    }
    const script = document.createElement('script')
    script.src = `https://webapi.amap.com/maps?v=2.0&key=${encodeURIComponent(amapKey.value)}`
    script.onload = () => resolve()
    script.onerror = () => reject(new Error('Failed to load AMap script'))
    document.head.appendChild(script)
  })
  return scriptRequest
}

const initMap = async () => {
  if (!mapContainer.value || !amapKey.value) return
  try {
    await loadScript()
    const center = transformCoordinate(props.center[0], props.center[1], 'amap')
    const map = new window.AMap.Map(mapContainer.value, {
      center,
      zoom: props.zoom,
      dragEnable: props.interactive,
      zoomEnable: props.interactive,
      doubleClickZoom: props.interactive,
      keyboardEnable: props.interactive,
      scrollWheel: props.interactive,
      touchZoom: props.interactive,
      viewMode: '2D',
      mapStyle: mapStyle.value,
    })
    mapInstance.value = map
    map.on('complete', () => {
      isLoaded.value = true
      emit('load', map)
    })
    map.on('zoomend', () => emit('zoom'))
  } catch (error) {
    console.error('[AMap] failed to initialize:', error)
  }
}

watch(
  () => [props.center, props.zoom] as const,
  ([center, zoom]) => {
    if (!mapInstance.value || !center) return
    mapInstance.value.setZoomAndCenter(
      zoom,
      transformCoordinate(center[0], center[1], 'amap'),
      true,
      0,
    )
  },
)

watch(mapStyle, (style) => {
  mapInstance.value?.setMapStyle?.(style)
})

provide('amap', mapInstance)
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
      v-if="!isLoaded && !amapKey"
      class="absolute inset-0 flex items-center justify-center bg-neutral-100/80 text-xs text-neutral-500 dark:bg-neutral-900/80 dark:text-neutral-400"
    >
      {{ $t('map.amap.missingKey') }}
    </div>
    <slot
      v-if="isLoaded"
      :map="mapInstance"
    />
  </div>
</template>
