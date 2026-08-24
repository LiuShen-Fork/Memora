<script lang="ts" setup>
import type { AMapMap } from '~~/shared/types/map'
import { transformCoordinate } from '~/utils/coordinate-transform'

const props = withDefaults(
  defineProps<{
    markerId?: string
    lnglat?: [number, number]
    map?: AMapMap
  }>(),
  { markerId: undefined, lnglat: undefined, map: undefined },
)

const marker = shallowRef<any>()
const markerMap = shallowRef<AMapMap>()
const markerContainer = shallowRef<HTMLDivElement | null>(null)
const injectedMap = inject<Ref<AMapMap | undefined>>('amap', ref())
const mapToUse = computed(() => props.map || injectedMap.value)

const removeMarker = () => {
  if (marker.value && markerMap.value) markerMap.value.remove(marker.value)
  marker.value = undefined
  markerMap.value = undefined
  markerContainer.value = null
}

const createMarker = (map: AMapMap, coordinates: [number, number]) => {
  if (!window.AMap?.Marker || !window.AMap?.Pixel) return
  removeMarker()
  const container = document.createElement('div')
  if (props.markerId) container.dataset.markerId = props.markerId
  markerContainer.value = container
  markerMap.value = map
  marker.value = new window.AMap.Marker({
    position: transformCoordinate(coordinates[0], coordinates[1], 'amap'),
    content: container,
    offset: new window.AMap.Pixel(-20, -20),
  })
  map.add(marker.value)
}

watch(
  () => [mapToUse.value, props.lnglat] as const,
  ([map, coordinates], previous) => {
    if (!map || !coordinates) {
      removeMarker()
      return
    }
    if (!marker.value || map !== previous?.[0]) {
      nextTick(() => createMarker(map, coordinates))
    } else {
      marker.value.setPosition(
        transformCoordinate(coordinates[0], coordinates[1], 'amap'),
      )
    }
  },
  { immediate: true },
)

watch(() => props.markerId, (id) => {
  if (!markerContainer.value) return
  if (id) markerContainer.value.dataset.markerId = id
  else delete markerContainer.value.dataset.markerId
})
onBeforeUnmount(removeMarker)
</script>

<template>
  <Teleport
    v-if="markerContainer"
    :to="markerContainer"
  >
    <slot name="marker" />
  </Teleport>
</template>
