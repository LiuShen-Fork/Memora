<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

// 文件名 map.vue 会让 Vue 以 "map" 当组件名，而 <map> 是 HTML 自带元素，
// 于是每次进这一页都会报一句组件名保留的警告。路由不受影响，名字单独给一个。
defineOptions({ name: 'SettingsMapPage' })

useHead({
  title: () => $t('title.mapAndLocation'),
})

const {
  fields: mapFields,
  state: mapState,
  submit: submitMap,
  loading: mapLoading,
} = useSettingsForm('map')
const {
  fields: locationFields,
  state: locationState,
  submit: submitLocation,
  loading: locationLoading,
} = useSettingsForm('location')

const mapProvider = computed({
  get: () => String(mapState.provider || 'maplibre'),
  set: (value: string) => {
    mapState.provider = value
  },
})

const mapProviderTabs = [
  { label: 'Mapbox', value: 'mapbox' },
  { label: 'MapLibre', value: 'maplibre' },
  { label: 'AMap', value: 'amap' },
]

const locationProvider = computed({
  get: () => String(locationState.provider || 'nominatim'),
  set: (value: string) => {
    locationState.provider = value
  },
})

const locationProviderTabs = [
  { label: 'Nominatim', value: 'nominatim' },
  { label: 'Mapbox', value: 'mapbox' },
  { label: 'AMap', value: 'amap' },
]

const visibleMapFields = computed(() =>
  mapFields.value.filter((field) => {
    if (field.key === 'provider') return false
    return field.key.startsWith(mapProvider.value + '.')
  }),
)

const visibleLocationFields = computed(() =>
  locationFields.value.filter((field) => {
    if (field.key === 'provider') return false
    if (field.key === 'language') return true
    return field.key.startsWith(locationProvider.value + '.')
  }),
)

const sameValue = (left: any, right: any) =>
  JSON.stringify(left ?? null) === JSON.stringify(right ?? null)

const getDefaultFieldValue = (field: (typeof mapFields.value)[number]) =>
  field.value ?? field.defaultValue ?? null

const isMapDirty = computed(() =>
  mapFields.value.some(
    (field) => !sameValue(mapState[field.key], getDefaultFieldValue(field)),
  ),
)

const isLocationDirty = computed(() =>
  locationFields.value.some(
    (field) =>
      !sameValue(
        locationState[field.key],
        field.value ?? field.defaultValue ?? null,
      ),
  ),
)

const resetMapSettings = () => {
  mapFields.value.forEach((field) => {
    mapState[field.key] = getDefaultFieldValue(field)
  })
}

const resetLocationSettings = () => {
  locationFields.value.forEach((field) => {
    locationState[field.key] = field.value ?? field.defaultValue ?? null
  })
}

const handleMapSettingsSubmit = async () => {
  const mapData = Object.fromEntries(
    mapFields.value.map((f) => [f.key, mapState[f.key]]),
  )
  try {
    await submitMap(mapData)
  } catch {
    /* empty */
  }
}

const handleLocationSettingsSubmit = async () => {
  const locationData = Object.fromEntries(
    locationFields.value.map((f) => [f.key, locationState[f.key]]),
  )
  try {
    await submitLocation(locationData)
  } catch {
    /* empty */
  }
}
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <DashboardPageHeader
        :title="$t('title.mapAndLocation')"
        :description="$t('settings.map.sectionDescription')"
      />
    </template>

    <template #body>
      <div class="mx-auto w-full max-w-4xl space-y-6">
        <SettingSkeleton v-if="mapLoading && visibleMapFields.length === 0" />

        <SettingCard
          v-else
          :title="$t('settings.map.sections.display')"
          icon="tabler:map-2"
          form-id="mapSettingsForm"
          :dirty="isMapDirty"
          :loading="mapLoading"
          @reset="resetMapSettings"
        >
          <UForm
            id="mapSettingsForm"
            class="divide-y divide-default"
            @submit="handleMapSettingsSubmit"
          >
            <!-- 换一家地图服务，下面要填的东西也跟着换 -->
            <SettingRow
              :label="$t('settings.map.provider.label')"
              :description="$t('settings.map.provider.description')"
            >
              <UTabs
                v-model="mapProvider"
                data-tour-setting="map:provider"
                :items="mapProviderTabs"
                color="neutral"
                variant="pill"
                size="sm"
                :ui="{ list: 'w-fit', root: 'items-start' }"
              />
            </SettingRow>

            <SettingField
              v-for="field in visibleMapFields"
              :key="field.key"
              :field="field"
              :model-value="mapState[field.key]"
              @update:model-value="(val) => (mapState[field.key] = val)"
            />
          </UForm>
        </SettingCard>

        <SettingSkeleton v-if="locationLoading && visibleLocationFields.length === 0" />

        <SettingCard
          v-else
          :title="$t('title.location')"
          icon="tabler:map-pin"
          form-id="locationSettingsForm"
          :dirty="isLocationDirty"
          :loading="locationLoading"
          @reset="resetLocationSettings"
        >
          <UForm
            id="locationSettingsForm"
            class="divide-y divide-default"
            @submit="handleLocationSettingsSubmit"
          >
            <SettingRow
              :label="$t('settings.location.provider.label')"
              :description="$t('settings.location.provider.description')"
            >
              <UTabs
                v-model="locationProvider"
                :items="locationProviderTabs"
                color="neutral"
                variant="pill"
                size="sm"
                :ui="{ list: 'w-fit', root: 'items-start' }"
              />
            </SettingRow>

            <SettingField
              v-for="field in visibleLocationFields"
              :key="field.key"
              :field="field"
              :model-value="locationState[field.key]"
              @update:model-value="(val) => (locationState[field.key] = val)"
            />
          </UForm>
        </SettingCard>
      </div>
    </template>
  </UDashboardPanel>
</template>

