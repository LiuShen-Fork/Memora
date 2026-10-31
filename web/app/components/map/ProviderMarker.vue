<script lang="ts" setup>
import type { AMapMap } from '~~/shared/types/map'

withDefaults(
  defineProps<{
    markerId?: string
    lnglat?: [number, number]
    map?: AMapMap
    offset?: [number, number]
  }>(),
  {
    markerId: undefined,
    lnglat: undefined,
    map: undefined,
  },
)

const mapConfig = computed(() => {
  const config = getSetting('map')
  return typeof config === 'object' && config ? config : {}
})

const provider = computed(() => mapConfig.value.provider || 'maplibre')
</script>

<template>
  <MapboxDefaultMarker
    v-if="provider === 'mapbox'"
    :marker-id
    :lnglat
  >
    <template #marker>
      <slot name="marker" />
    </template>
  </MapboxDefaultMarker>
  <MapAMapMarker
    v-else-if="provider === 'amap'"
    :marker-id
    :lnglat
    :map
    :offset
  >
    <template #marker>
      <slot name="marker" />
    </template>
  </MapAMapMarker>
  <!-- 腾讯和天地图都没有「一段 HTML 作为一个标记」的原生接口：腾讯的 DOMOverlay
       要先继承，天地图的 DivIcon 只收一段字符串。两边都由 ProjectedMarker 把
       一个 div 贴在地图容器里，按投影逐帧摆位，插槽里的内容照旧是 Vue 组件。 -->
  <MapProjectedMarker
    v-else-if="provider === 'tencent' || provider === 'tianditu'"
    :marker-id
    :lnglat
    :map
    :offset
  >
    <template #marker>
      <slot name="marker" />
    </template>
  </MapProjectedMarker>
  <MglMarker
    v-else
    :coordinates="lnglat"
  >
    <template #marker>
      <slot name="marker" />
    </template>
  </MglMarker>
</template>

<style scoped></style>
