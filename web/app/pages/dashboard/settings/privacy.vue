<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.privacySettings'),
})

const {
  fields: privacyFields,
  state: privacyState,
  submit: submitPrivacy,
  loading: privacyLoading,
} = useSettingsForm('privacy')

const sameValue = (left: any, right: any) =>
  JSON.stringify(left ?? null) === JSON.stringify(right ?? null)

const isPrivacyDirty = computed(() =>
  privacyFields.value.some(
    (field) =>
      !sameValue(
        privacyState[field.key],
        field.value ?? field.defaultValue ?? null,
      ),
  ),
)

const resetPrivacySettings = () => {
  privacyFields.value.forEach((field) => {
    privacyState[field.key] = field.value ?? field.defaultValue ?? null
  })
}

const handlePrivacySettingsSubmit = async () => {
  const privacyData = Object.fromEntries(
    privacyFields.value.map((f) => [f.key, privacyState[f.key]]),
  )

  try {
    await submitPrivacy(privacyData)
  } catch {
    /* empty */
  }
}
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <DashboardPageHeader
        :title="$t('title.privacySettings')"
        :description="$t('settings.privacy.sectionDescription')"
      />
    </template>

    <template #body>
      <div class="mx-auto w-full max-w-4xl">
        <SettingSkeleton v-if="privacyLoading && privacyFields.length === 0" />

        <SettingCard
          v-else
          :title="$t('settings.privacy.sections.upload')"
          icon="tabler:shield-lock"
          form-id="privacySettingsForm"
          :dirty="isPrivacyDirty"
          :loading="privacyLoading"
          @reset="resetPrivacySettings"
        >
          <UForm
            id="privacySettingsForm"
            class="divide-y divide-default"
            @submit="handlePrivacySettingsSubmit"
          >
            <SettingField
              v-for="field in privacyFields"
              :key="field.key"
              :field="field"
              :model-value="privacyState[field.key]"
              @update:model-value="(val) => (privacyState[field.key] = val)"
            />
          </UForm>
        </SettingCard>
      </div>
    </template>
  </UDashboardPanel>
</template>
