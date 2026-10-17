<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.analyticsSettings'),
})

const {
  fields: analyticsFields,
  state: analyticsState,
  submit: submitAnalytics,
  loading: analyticsLoading,
} = useSettingsForm('analytics')

const sameValue = (left: any, right: any) =>
  JSON.stringify(left ?? null) === JSON.stringify(right ?? null)

const isAnalyticsDirty = computed(() =>
  analyticsFields.value.some(
    (field) =>
      !sameValue(
        analyticsState[field.key],
        field.value ?? field.defaultValue ?? null,
      ),
  ),
)

const resetAnalyticsSettings = () => {
  analyticsFields.value.forEach((field) => {
    analyticsState[field.key] = field.value ?? field.defaultValue ?? null
  })
}

const handleAnalyticsSettingsSubmit = async () => {
  const analyticsData = Object.fromEntries(
    analyticsFields.value.map((f) => [f.key, analyticsState[f.key]]),
  )
  try {
    await submitAnalytics(analyticsData)
  } catch {
    /* empty */
  }
}
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <DashboardPageHeader
        :title="$t('title.analyticsSettings')"
        :description="$t('settings.analytics.sectionDescription')"
      />
    </template>

    <template #body>
      <div class="mx-auto w-full max-w-4xl">
        <SettingSkeleton
          v-if="analyticsLoading && analyticsFields.length === 0"
          :rows="2"
        />

        <SettingCard
          v-else
          :title="$t('settings.analytics.sections.scripts')"
          icon="tabler:code"
          form-id="analyticsSettingsForm"
          :dirty="isAnalyticsDirty"
          :loading="analyticsLoading"
          @reset="resetAnalyticsSettings"
        >
          <!-- 粘进来的是代码，所以这两块用等宽字排版 -->
          <UForm
            id="analyticsSettingsForm"
            class="divide-y divide-default [&_textarea]:font-mono [&_textarea]:text-xs"
            @submit="handleAnalyticsSettingsSubmit"
          >
            <SettingField
              v-for="field in analyticsFields"
              :key="field.key"
              :field="field"
              :model-value="analyticsState[field.key]"
              @update:model-value="(val) => (analyticsState[field.key] = val)"
            />
          </UForm>
        </SettingCard>
      </div>
    </template>
  </UDashboardPanel>
</template>

