<script lang="ts" setup>
import type { AttributionControlOptions, StyleSpecification } from 'maplibre-gl'
import { twMerge } from 'tailwind-merge'
import type { MapboxMap, MapInstance, MaplibreMap } from '~~/shared/types/map'
import { isMapServiceConfigured } from '~~/shared/utils/map-service'

import MemoraLightStyle from '~/assets/mapStyles/memora_light.json'
import MemoraDarkStyle from '~/assets/mapStyles/memora_dark.json'

const props = withDefaults(
  defineProps<{
    class?: string
    mapId?: string
    center?: [number, number]
    zoom?: number
    interactive?: boolean
    attributionControl?: false | AttributionControlOptions
    language?: string
    setupNotice?: boolean
  }>(),
  {
    class: undefined,
    mapId: undefined,
    center: undefined,
    zoom: 2,
    interactive: true,
    attributionControl: false,
    language: undefined,
    setupNotice: false,
  },
)

const emit = defineEmits<{
  load: [map: MapInstance]
  zoom: []
}>()

const colorMode = useColorMode()

const mapConfig = computed(() => {
  const config = getSetting('map')
  return typeof config === 'object' && config ? config : {}
})

const provider = computed(() => mapConfig.value.provider || 'maplibre')
const isConfigured = computed(() =>
  isMapServiceConfigured(mapConfig.value as Record<string, unknown>),
)
// AMap 自己会说明缺了 Key，不去替它说。
const hasBlankCanvas = computed(
  () => !isConfigured.value && provider.value !== 'amap',
)

// 地图画不出来时，说清楚为什么，并指向能把它配好的地方。只有地球页需要这段话，
// 其余用地图的地方（照片小地图、位置选择）由各自的空状态交代。
const { show: showMapSetupNotice, hide: hideMapSetupNotice } =
  useMapSetupNotice()

const syncMapSetupNotice = () => {
  if (!props.setupNotice) return
  if (isConfigured.value) {
    hideMapSetupNotice()
    return
  }
  showMapSetupNotice()
}

onMounted(syncMapSetupNotice)
watch([isConfigured, () => props.setupNotice], () => {
  if (import.meta.client) syncMapSetupNotice()
})
onBeforeUnmount(() => {
  if (!props.setupNotice) return
  hideMapSetupNotice()
})

const mapStyle = computed(() => {
  if (provider.value === 'mapbox') {
    return mapConfig.value['mapbox.style'] || `mapbox://styles/mapbox/standard`
  } else {
    const styleConfig =
      colorMode.value === 'dark' ? MemoraDarkStyle : MemoraLightStyle
    return (
      mapConfig.value['maplibre.style'] ||
      ({
        ...styleConfig,
        sources: {
          openmaptiles: {
            ...styleConfig.sources?.openmaptiles,
            url: `https://api.maptiler.com/tiles/v3-openmaptiles/tiles.json?key=${mapConfig.value['maplibre.token']}`,
          },
        },
        glyphs: `https://api.maptiler.com/fonts/{fontstack}/{range}.pbf?key=${mapConfig.value['maplibre.token']}`,
      } as StyleSpecification)
    )
  }
})
</script>

<template>
  <div :class="twMerge('w-full h-full', $props.class)">
    <!-- 没有配置地图服务时不去加载地图：画布是空的，等它报错不如直接说明 -->
    <div
      v-if="hasBlankCanvas"
      class="flex h-full w-full items-center justify-center bg-neutral-100 dark:bg-neutral-900"
    >
      <Icon
        name="tabler:map-off"
        class="size-10 text-neutral-400"
      />
    </div>
    <ClientOnly v-else>
      <MglMap
        v-if="provider === 'maplibre'"
        class="w-full h-full"
        :map-key="mapId"
        :map-style="mapStyle as StyleSpecification"
        :center
        :zoom
        :interactive
        :attribution-control
        @map:load="emit('load', $event.map as MaplibreMap)"
        @map:zoom="emit('zoom')"
      >
        <slot />
      </MglMap>
      <MapboxMap
        v-else-if="provider === 'mapbox'"
        class="w-full h-full"
        :map-id="mapId || 'cframe-mapbox-map'"
        :options="{
          accessToken: mapConfig['mapbox.token'],
          style: mapStyle,
          center: center,
          zoom: zoom,
          interactive: interactive,
          attributionControl: attributionControl,
          language: language,
          config: {
            basemap: {
              lightPreset: $colorMode.value === 'dark' ? 'night' : 'day',
              colorThemes: 'faded',
            },
          },
        }"
        @load="emit('load', $event as MapboxMap)"
        @zoom="emit('zoom')"
      >
        <slot />
      </MapboxMap>
      <MapAMapProvider
        v-else-if="provider === 'amap'"
        class="h-full w-full"
        :map-id="mapId"
        :center
        :zoom
        :interactive
        @load="emit('load', $event)"
        @zoom="emit('zoom')"
      >
        <template #default="{ map }">
          <slot :map="map" />
        </template>
      </MapAMapProvider>
    </ClientOnly>
  </div>
</template>

<style scoped></style>
