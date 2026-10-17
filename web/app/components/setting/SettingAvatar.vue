<script lang="ts" setup>
/**
 * 站点头像：上传会立刻写入存储并更新设置，不需要再点保存；
 * 也可以直接填写外部图片地址，填写后按保存生效。
 */
interface Props {
  modelValue?: string | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const { t } = useI18n()
const toast = useToast()

// 与后端一致：既要是浏览器能直接显示的格式，也要是会被重新编码后存储的格式
const ACCEPTED_TYPES = [
  'image/jpeg',
  'image/png',
  'image/webp',
  'image/gif',
  'image/bmp',
  'image/avif',
]
const MAX_BYTES = 8 * 1024 * 1024

const fileInput = ref<HTMLInputElement | null>(null)
const urlDraft = ref('')
const uploading = ref(false)
const removing = ref(false)
const savingUrl = ref(false)
const previewFailed = ref(false)

const current = computed(() =>
  typeof props.modelValue === 'string' ? props.modelValue.trim() : '',
)
const showPreview = computed(() => !!current.value && !previewFailed.value)
const isUrlDirty = computed(() => urlDraft.value.trim() !== current.value)

watch(
  () => props.modelValue,
  () => {
    previewFailed.value = false
    urlDraft.value = current.value
  },
  { immediate: true },
)

const notify = (title: string, color: 'success' | 'error', error?: unknown) => {
  toast.add({
    title,
    description: error ? (error as Error).message : undefined,
    color,
  })
}

const pickFile = () => fileInput.value?.click()

const onFilePicked = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  // 清空后同一个文件再次选择才会重新触发 change
  input.value = ''
  if (!file) return

  if (file.type && !ACCEPTED_TYPES.includes(file.type)) {
    notify(t('settings.app.avatarUrl.invalidType'), 'error')
    return
  }
  if (file.size > MAX_BYTES) {
    notify(
      t('settings.app.avatarUrl.tooLarge', { size: MAX_BYTES / 1024 / 1024 }),
      'error',
    )
    return
  }

  const form = new FormData()
  form.append('file', file)
  uploading.value = true
  try {
    const { avatarUrl } = await $fetch<{ avatarUrl: string }>(
      '/api/system/settings/avatar',
      { method: 'POST', body: form },
    )
    emit('update:modelValue', avatarUrl)
    await refreshSettings()
    notify(t('settings.app.avatarUrl.uploadSuccess'), 'success')
  } catch (error) {
    notify(t('settings.app.avatarUrl.uploadFailed'), 'error', error)
  } finally {
    uploading.value = false
  }
}

const removeAvatar = async () => {
  removing.value = true
  try {
    await $fetch('/api/system/settings/avatar', { method: 'DELETE' })
    emit('update:modelValue', '')
    await refreshSettings()
    notify(t('settings.app.avatarUrl.removeSuccess'), 'success')
  } catch (error) {
    notify(t('settings.app.avatarUrl.removeFailed'), 'error', error)
  } finally {
    removing.value = false
  }
}

const saveUrl = async () => {
  const value = urlDraft.value.trim()
  if (!isUrlDirty.value) return
  savingUrl.value = true
  try {
    await $fetch('/api/system/settings/app/avatarUrl', {
      method: 'PUT',
      body: { value },
    })
    emit('update:modelValue', value)
    await refreshSettings()
    notify(t('settings.app.avatarUrl.urlSaved'), 'success')
  } catch (error) {
    notify(t('settings.app.avatarUrl.urlFailed'), 'error', error)
  } finally {
    savingUrl.value = false
  }
}
</script>

<template>
  <SettingRow
    data-tour-setting="app:avatarUrl"
    :label="$t('settings.app.avatarUrl.label')"
    :description="$t('settings.app.avatarUrl.description')"
  >
    <!-- 窄屏上头像另起一行：横着排的话，输入框只剩不到两百像素，
         连占位的那串地址都显示不全。 -->
    <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:gap-4">
      <div
        class="relative size-16 shrink-0 overflow-hidden rounded-full border border-default bg-elevated"
      >
        <img
          v-if="showPreview"
          :src="current"
          class="size-full object-cover"
          :alt="$t('ui.photo.avatarAlt')"
          @error="previewFailed = true"
        />
        <div
          v-else
          class="flex size-full items-center justify-center text-dimmed"
        >
          <UIcon
            name="tabler:user"
            class="size-7"
          />
        </div>
      </div>

      <div class="min-w-0 flex-1 space-y-2.5">
        <div class="flex flex-wrap items-center gap-2">
          <UButton
            type="button"
            color="neutral"
            variant="outline"
            size="sm"
            icon="tabler:upload"
            :loading="uploading"
            @click="pickFile"
          >
            {{
              current
                ? $t('settings.app.avatarUrl.replace')
                : $t('settings.app.avatarUrl.upload')
            }}
          </UButton>
          <UButton
            v-if="current"
            type="button"
            color="error"
            variant="soft"
            size="sm"
            icon="tabler:trash"
            :loading="removing"
            @click="removeAvatar"
          >
            {{ $t('settings.app.avatarUrl.remove') }}
          </UButton>
        </div>

        <!-- 图片地址也可以直接填：左边让出空间，右边那颗按钮才不会被压成一列字。 -->
        <div class="flex items-center gap-2">
          <UInput
            v-model="urlDraft"
            type="url"
            icon="tabler:link"
            class="min-w-0 max-w-sm flex-1"
            :placeholder="$t('settings.app.avatarUrl.placeholder')"
            @keyup.enter.prevent="saveUrl"
          />
          <UButton
            type="button"
            color="primary"
            variant="soft"
            size="sm"
            class="shrink-0"
            icon="tabler:check"
            :disabled="!isUrlDirty"
            :loading="savingUrl"
            @click="saveUrl"
          >
            {{ $t('common.actions.save') }}
          </UButton>
        </div>

        <p class="text-xs leading-relaxed text-dimmed">
          {{ $t('settings.app.avatarUrl.help') }}
        </p>
      </div>
    </div>

    <input
      ref="fileInput"
      type="file"
      class="hidden"
      :accept="ACCEPTED_TYPES.join(',')"
      @change="onFilePicked"
    />
  </SettingRow>
</template>

<style scoped></style>
