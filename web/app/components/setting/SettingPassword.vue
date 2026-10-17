<script lang="ts" setup>
import { z } from 'zod'

/**
 * 修改登录密码：与站点头像一样，自己保存、自己生效，不参与页面上方的保存按钮。
 * 当前密码要重新输入一次，避免别人拿到一个已登录的会话就能改掉密码。
 */
const MIN_LENGTH = 6

const { t } = useI18n()
const toast = useToast()

const state = reactive({
  currentPassword: '',
  newPassword: '',
  confirmPassword: '',
})
const saving = ref(false)

const tooShortMessage = computed(() =>
  t('settings.password.errors.tooShort', { count: MIN_LENGTH }),
)
const newPlaceholder = computed(() =>
  t('settings.password.newPlaceholder', { count: MIN_LENGTH }),
)

const schema = computed(() =>
  z
    .object({
      currentPassword: z
        .string()
        .min(1, t('settings.password.errors.currentRequired')),
      newPassword: z.string().min(MIN_LENGTH, tooShortMessage.value),
      confirmPassword: z.string(),
    })
    .refine((data) => data.newPassword === data.confirmPassword, {
      message: t('settings.password.errors.mismatch'),
      path: ['confirmPassword'],
    }),
)

const filled = computed(
  () =>
    state.currentPassword.length > 0 &&
    state.newPassword.length > 0 &&
    state.confirmPassword.length > 0,
)

const changePassword = async () => {
  saving.value = true
  try {
    await $fetch('/api/profile/password', {
      method: 'PUT',
      body: {
        currentPassword: state.currentPassword,
        newPassword: state.newPassword,
      },
    })
    state.currentPassword = ''
    state.newPassword = ''
    state.confirmPassword = ''
    toast.add({
      title: t('settings.password.changed'),
      description: t('settings.password.changedDescription'),
      color: 'success',
    })
  } catch (error: any) {
    toast.add({
      title: t('settings.password.failed'),
      description: error?.data?.message,
      color: 'error',
    })
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <UForm
    :schema="schema"
    :state="state"
    @submit="changePassword"
  >
    <div class="grid gap-5 px-5 py-5 md:grid-cols-3">
      <UFormField
        :label="$t('settings.password.current')"
        name="currentPassword"
      >
        <UInput
          v-model="state.currentPassword"
          type="password"
          autocomplete="current-password"
          class="w-full"
          icon="tabler:lock"
          :placeholder="$t('settings.password.currentPlaceholder')"
        />
      </UFormField>

      <UFormField
        :label="$t('settings.password.new')"
        name="newPassword"
      >
        <UInput
          v-model="state.newPassword"
          type="password"
          autocomplete="new-password"
          class="w-full"
          icon="tabler:key"
          :placeholder="newPlaceholder"
        />
      </UFormField>

      <UFormField
        :label="$t('settings.password.confirm')"
        name="confirmPassword"
      >
        <UInput
          v-model="state.confirmPassword"
          type="password"
          autocomplete="new-password"
          class="w-full"
          icon="tabler:key"
        />
      </UFormField>
    </div>

    <div
      class="flex flex-wrap items-center justify-between gap-3 border-t border-neutral-200 px-5 py-4 dark:border-neutral-800"
    >
      <p class="text-xs text-neutral-500 dark:text-neutral-500">
        {{ $t('settings.password.help') }}
      </p>
      <UButton
        type="submit"
        :loading="saving"
        :disabled="!filled"
        icon="tabler:key"
      >
        {{ $t('settings.password.save') }}
      </UButton>
    </div>
  </UForm>
</template>

<style scoped></style>
