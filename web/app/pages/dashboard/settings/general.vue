<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.generalSettings'),
})

const colorMode = useColorMode()

const { fields, state, submit, loading } = useSettingsForm('app')

// 头像自己上传、自己保存，不参与下面的表单，避免出现「已经生效却提示未保存」，
// 也避免重置按钮把已上传的图片退回旧地址。
const AVATAR_KEY = 'avatarUrl'

const appFields = computed(() =>
  fields.value.filter(
    (f) => !f.key.startsWith('appearance.') && f.key !== AVATAR_KEY,
  ),
)

const appearanceFields = computed(() =>
  fields.value.filter((f) => f.key.startsWith('appearance.')),
)

const sameValue = (left: any, right: any) =>
  JSON.stringify(left ?? null) === JSON.stringify(right ?? null)

const getDefaultFieldValue = (field: (typeof fields.value)[number]) =>
  field.value ?? field.defaultValue ?? null

const isAppDirty = computed(() =>
  appFields.value.some((field) =>
    !sameValue(state[field.key], getDefaultFieldValue(field)),
  ),
)

const isAppearanceDirty = computed(() =>
  appearanceFields.value.some((field) =>
    !sameValue(state[field.key], getDefaultFieldValue(field)),
  ),
)

const resetAppSettings = () => {
  appFields.value.forEach((field) => {
    state[field.key] = getDefaultFieldValue(field)
  })
}

const resetAppearanceSettings = () => {
  appearanceFields.value.forEach((field) => {
    state[field.key] = getDefaultFieldValue(field)
  })
}

const handleAppSettingsSubmit = async () => {
  const appData = Object.fromEntries(
    appFields.value.map((f) => [f.key, state[f.key]]),
  )
  try {
    await submit(appData)
  } catch {
    /* empty */
  }
}

const handleAppearanceSettingsSubmit = async () => {
  const appearanceData = Object.fromEntries(
    appearanceFields.value.map((f) => [f.key, state[f.key]]),
  )
  try {
    await submit(appearanceData)
    if (state['appearance.theme']) {
      colorMode.preference = state['appearance.theme']
    }
  } catch {
    /* empty */
  }
}
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <DashboardPageHeader
        :title="$t('title.generalSettings')"
        :description="$t('settings.general.description')"
      />
    </template>

    <template #body>
      <div class="mx-auto w-full max-w-4xl space-y-6">
        <SettingSkeleton
          v-if="loading && appFields.length === 0"
          :rows="3"
        />

        <SettingCard
          v-else
          :title="$t('settings.general.sections.site')"
          icon="tabler:info-circle"
          form-id="appSettingsForm"
          :dirty="isAppDirty"
          :loading="loading"
          @reset="resetAppSettings"
        >
          <div class="divide-y divide-default">
            <!-- 头像自己上传、自己保存，不跟着下面的保存按钮走 -->
            <SettingAvatar v-model="state[AVATAR_KEY]" />

            <UForm
              id="appSettingsForm"
              class="divide-y divide-default"
              @submit="handleAppSettingsSubmit"
            >
              <SettingField
                v-for="field in appFields"
                :key="field.key"
                :field="field"
                :model-value="state[field.key]"
                @update:model-value="(val) => (state[field.key] = val)"
              />
            </UForm>
          </div>
        </SettingCard>

        <SettingSkeleton
          v-if="loading && appearanceFields.length === 0"
          :rows="1"
        />

        <SettingCard
          v-else
          :title="$t('title.appearanceSettings')"
          icon="tabler:palette"
          form-id="appearanceSettingsForm"
          :dirty="isAppearanceDirty"
          :loading="loading"
          @reset="resetAppearanceSettings"
        >
          <UForm
            id="appearanceSettingsForm"
            class="divide-y divide-default"
            @submit="handleAppearanceSettingsSubmit"
          >
            <SettingField
              v-for="field in appearanceFields"
              :key="field.key"
              :field="field"
              :model-value="state[field.key]"
              @update:model-value="(val) => (state[field.key] = val)"
            />
          </UForm>
        </SettingCard>

        <SettingPassword />
      </div>
    </template>
  </UDashboardPanel>
</template>
