<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.generalSettings'),
})

const colorMode = useColorMode()

const { fields, state, submit, loading, fetchFields } = useSettingsForm('app')
const { t } = useI18n()
const toast = useToast()

// 头像自己上传、自己保存，不参与下面的表单，避免出现「已经生效却提示未保存」，
// 也避免重置按钮把已上传的图片退回旧地址。
const AVATAR_KEY = 'avatarUrl'

const reactionFields = computed(() =>
  fields.value.filter((f) => f.key.startsWith('reactions.')),
)

// 社交链接是一行一个地址、可增删的列表，普通的「一个键一个输入框」放不下，
// 所以它有自己的卡片，不进下面这张表单。
const SOCIAL_LINKS_KEY = 'socialLinks'

const appFields = computed(() =>
  fields.value.filter(
    (f) =>
      !f.key.startsWith('appearance.') &&
      !f.key.startsWith('reactions.') &&
      f.key !== AVATAR_KEY &&
      f.key !== SOCIAL_LINKS_KEY,
  ),
)

const socialLinksField = computed(() =>
  fields.value.find((f) => f.key === SOCIAL_LINKS_KEY),
)

const socialLinks = computed<string[]>(() =>
  parseSocialLinks(state[SOCIAL_LINKS_KEY], true),
)
const setSocialLinks = (links: string[]) => {
  state[SOCIAL_LINKS_KEY] = links.length > 0 ? JSON.stringify(links) : ''
}
const addSocialLink = () => {
  setSocialLinks([...socialLinks.value, ''])
}
const removeSocialLink = (index: number) => {
  setSocialLinks(socialLinks.value.filter((_, i) => i !== index))
}
const updateSocialLink = (index: number, value: string) => {
  setSocialLinks(socialLinks.value.map((link, i) => (i === index ? value : link)))
}

const isSocialLinksDirty = computed(() =>
  socialLinksField.value
    ? !sameValue(state[SOCIAL_LINKS_KEY], getDefaultFieldValue(socialLinksField.value))
    : false,
)

const resetSocialLinksSettings = () => {
  if (!socialLinksField.value) return
  state[SOCIAL_LINKS_KEY] = getDefaultFieldValue(socialLinksField.value)
}

// 保存前先在本地把关：服务器同样会拒绝这些地址，但那里只会回一句英文错误，
// 而这里能指出是哪一行不对。
const socialLinkProblem = (link: string, index: number): string | null => {
  if (!link.trim()) return t('settings.general.socialLinks.empty')
  if (link !== link.trim()) return t('settings.general.socialLinks.spaces')
  if (link.length > 2048) return t('settings.general.socialLinks.tooLong')
  let url: URL
  try {
    url = new URL(link)
  } catch {
    return t('settings.general.socialLinks.invalid')
  }
  if (url.protocol !== 'https:') return t('settings.general.socialLinks.https')
  if (url.username || url.password) return t('settings.general.socialLinks.invalid')
  if (!url.hostname.includes('.')) return t('settings.general.socialLinks.invalid')
  const identity = `${url.origin}${url.pathname.replace(/\/+$/, '')}`
  const duplicate = socialLinks.value.some((other, i) => {
    if (i === index || !other.trim()) return false
    try {
      const parsed = new URL(other)
      return `${parsed.origin}${parsed.pathname.replace(/\/+$/, '')}` === identity
    } catch {
      return false
    }
  })
  if (duplicate) return t('settings.general.socialLinks.duplicate')
  return null
}

const socialLinkErrors = computed(() =>
  socialLinks.value.map((link, index) => socialLinkProblem(link, index)),
)

const socialLinksSaving = ref(false)

const handleSocialLinksSubmit = async () => {
  const problem = socialLinkErrors.value.find(Boolean)
  if (problem) {
    toast.add({
      title: t('settings.form.saveFailed'),
      description: problem,
      color: 'error',
    })
    return
  }
  socialLinksSaving.value = true
  try {
    await $fetch(`/api/system/settings/app/${SOCIAL_LINKS_KEY}`, {
      method: 'PUT',
      body: { value: state[SOCIAL_LINKS_KEY] ?? '' },
    })
    await refreshSettings()
    await fetchFields()
    toast.add({ title: t('settings.form.saveSuccess'), color: 'success' })
  } catch (err) {
    toast.add({
      title: t('settings.form.saveFailed'),
      description: (err as { data?: { message?: string } })?.data?.message,
      color: 'error',
    })
  } finally {
    socialLinksSaving.value = false
  }
}

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

const isReactionsDirty = computed(() =>
  reactionFields.value.some((field) =>
    !sameValue(state[field.key], getDefaultFieldValue(field)),
  ),
)

const resetReactionSettings = () => {
  reactionFields.value.forEach((field) => {
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

const handleReactionSettingsSubmit = async () => {
  const reactionData = Object.fromEntries(
    reactionFields.value.map((f) => [f.key, state[f.key]]),
  )
  try {
    await submit(reactionData)
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
          v-if="loading && !socialLinksField"
          :rows="2"
        />

        <SettingCard
          v-else
          :title="$t('settings.general.sections.socialLinks')"
          icon="tabler:link"
          form-id="socialLinksForm"
          :dirty="isSocialLinksDirty"
          :loading="socialLinksSaving"
          @reset="resetSocialLinksSettings"
        >
          <UForm
            id="socialLinksForm"
            class="space-y-3 py-4"
            @submit="handleSocialLinksSubmit"
          >
            <p class="text-xs text-muted">
              {{ $t('settings.general.socialLinks.hint') }}
            </p>

            <p
              v-if="socialLinks.length === 0"
              class="rounded-lg border border-dashed border-default px-3 py-4 text-center text-xs text-muted"
            >
              {{ $t('settings.general.socialLinks.none') }}
            </p>

            <div
              v-for="(link, index) in socialLinks"
              :key="index"
              class="space-y-1"
            >
              <div class="flex items-center gap-2">
                <UInput
                  :model-value="link"
                  :placeholder="$t('settings.general.socialLinks.placeholder')"
                  :leading-icon="link ? resolveSocialPlatform(link).icon : 'tabler:link'"
                  class="flex-1"
                  @update:model-value="(value) => updateSocialLink(index, String(value ?? ''))"
                />
                <UButton
                  type="button"
                  color="neutral"
                  variant="ghost"
                  icon="tabler:trash"
                  :aria-label="$t('settings.general.socialLinks.remove')"
                  @click="removeSocialLink(index)"
                />
              </div>
              <p v-if="socialLinkErrors[index]" class="text-xs text-error">
                {{ socialLinkErrors[index] }}
              </p>
              <p v-else-if="link" class="text-xs text-muted">
                {{ resolveSocialPlatform(link).label }}
              </p>
            </div>

            <UButton
              type="button"
              color="neutral"
              variant="subtle"
              size="sm"
              icon="tabler:plus"
              :disabled="socialLinks.length >= 4"
              @click="addSocialLink"
            >
              {{ $t('settings.general.socialLinks.add') }}
            </UButton>
          </UForm>
        </SettingCard>

        <SettingSkeleton
          v-if="loading && reactionFields.length === 0"
          :rows="1"
        />

        <SettingCard
          v-else
          :title="$t('settings.general.sections.reactions')"
          icon="tabler:mood-smile"
          form-id="reactionSettingsForm"
          :dirty="isReactionsDirty"
          :loading="loading"
          @reset="resetReactionSettings"
        >
          <UForm
            id="reactionSettingsForm"
            class="divide-y divide-default"
            @submit="handleReactionSettingsSubmit"
          >
            <SettingField
              v-for="field in reactionFields"
              :key="field.key"
              :field="field"
              :model-value="state[field.key]"
              @update:model-value="(val) => (state[field.key] = val)"
            />
          </UForm>
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
