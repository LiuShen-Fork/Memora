<script lang="ts" setup>
import type { FieldDescriptor } from '~~/shared/types/settings'

definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.systemSettings'),
})

const {
  fields: rawSystemFields,
  state: systemState,
  submit: submitSystem,
  loading: systemLoading,
} = useSettingsForm('system')

const systemFields = computed(() =>
  rawSystemFields.value.filter((field) => !field.isReadonly),
)

type SystemSection = {
  id: string
  titleKey: string
  keys: string[]
}

type SystemSectionWithFields = SystemSection & {
  fields: FieldDescriptor[]
}

const SYSTEM_SECTION_ORDER: SystemSection[] = [
  {
    id: 'thirdPartyLogin',
    titleKey: 'settings.system.sections.thirdPartyLogin',
    keys: [
      'auth.oauth.enabled',
      'auth.oauth.name',
      'auth.oauth.clientId',
      'auth.oauth.clientSecret',
      'auth.oauth.authorizationUrl',
      'auth.oauth.tokenUrl',
      'auth.oauth.userInfoUrl',
      'auth.oauth.scope',
      'auth.github.enabled',
      'auth.github.clientId',
      'auth.github.clientSecret',
    ],
  },
  {
    id: 'fileProcessing',
    titleKey: 'settings.system.sections.fileProcessing',
    keys: [
      'upload.maxFileSize',
      'upload.duplicateCheck.enabled',
      'upload.duplicateCheck.mode',
    ],
  },
  {
    id: 'debug',
    titleKey: 'settings.system.sections.debugSettings',
    keys: ['webglImageViewerDebug'],
  },
] as const

/** 分组在卡片头上各有一个图标，扫一眼就知道这是哪一类设置。 */
const SECTION_ICONS: Record<string, string> = {
  thirdPartyLogin: 'tabler:login',
  fileProcessing: 'tabler:files',
  debug: 'tabler:bug',
}

const isFieldDescriptor = (
  field: FieldDescriptor | undefined,
): field is FieldDescriptor => Boolean(field)

const systemFieldSections = computed<SystemSectionWithFields[]>(() =>
  SYSTEM_SECTION_ORDER.map((section) => ({
    ...section,
    fields: section.keys
      .map((key) => systemFields.value.find((field) => field.key === key))
      .filter(isFieldDescriptor),
  })).filter((section) => section.fields.length > 0),
)

const sameValue = (left: any, right: any) =>
  JSON.stringify(left ?? null) === JSON.stringify(right ?? null)

const getDefaultFieldValue = (field: (typeof rawSystemFields.value)[number]) =>
  field.value ?? field.defaultValue ?? null

const getSectionFormId = (sectionId: string) =>
  `systemSettingsForm-${sectionId}`

const isSectionDirty = (section: SystemSectionWithFields) =>
  section.fields.some(
    (field) => !sameValue(systemState[field.key], getDefaultFieldValue(field)),
  )

const resetSectionSettings = (section: SystemSectionWithFields) => {
  section.fields.forEach((field) => {
    systemState[field.key] = getDefaultFieldValue(field)
  })
}

const handleSectionSettingsSubmit = async (
  section: SystemSectionWithFields,
) => {
  const systemData = Object.fromEntries(
    section.fields.map((f) => [f.key, systemState[f.key]]),
  )

  try {
    await submitSystem(systemData)
  } catch {
    /* empty */
  }
}
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <DashboardPageHeader
        :title="$t('title.systemSettings')"
        :description="$t('settings.system.sectionDescription')"
      />
    </template>

    <template #body>
      <div class="mx-auto w-full max-w-4xl space-y-6">
        <template v-if="systemLoading && systemFieldSections.length === 0">
          <SettingSkeleton
            v-for="index in 3"
            :key="index"
          />
        </template>

        <SettingCard
          v-for="section in systemFieldSections"
          :key="section.id"
          :title="$t(section.titleKey)"
          :icon="SECTION_ICONS[section.id]"
          :form-id="getSectionFormId(section.id)"
          :dirty="isSectionDirty(section)"
          :loading="systemLoading"
          @reset="resetSectionSettings(section)"
        >
          <UForm
            :id="getSectionFormId(section.id)"
            class="divide-y divide-default"
            @submit="handleSectionSettingsSubmit(section)"
          >
            <SettingField
              v-for="field in section.fields"
              :key="field.key"
              :field="field"
              :model-value="systemState[field.key]"
              @update:model-value="(val) => (systemState[field.key] = val)"
            />
          </UForm>
        </SettingCard>

        <TourControls />
      </div>
    </template>
  </UDashboardPanel>
</template>
