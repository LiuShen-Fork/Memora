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
  <DashboardPanel
    :title="$t('settings.password.title')"
    :description="$t('settings.password.description')"
    icon="tabler:lock"
    :ui="{ body: 'px-4 py-0 sm:px-4' }"
  >
    <UForm
      id="passwordSettingsForm"
      class="divide-y divide-default"
      :schema="schema"
      :state="state"
      @submit="changePassword"
    >
      <!-- UFormField 只用来挂校验信息，标签在左边那一列，所以这里不给它 label。 -->
      <SettingRow
        :label="$t('settings.password.current')"
        html-for="password-current"
      >
        <UFormField name="currentPassword">
          <UInput
            id="password-current"
            v-model="state.currentPassword"
            type="password"
            autocomplete="current-password"
            class="w-full max-w-lg"
            icon="tabler:lock"
            :placeholder="$t('settings.password.currentPlaceholder')"
          />
        </UFormField>
      </SettingRow>

      <SettingRow
        :label="$t('settings.password.new')"
        html-for="password-new"
      >
        <UFormField name="newPassword">
          <UInput
            id="password-new"
            v-model="state.newPassword"
            type="password"
            autocomplete="new-password"
            class="w-full max-w-lg"
            icon="tabler:key"
            :placeholder="newPlaceholder"
          />
        </UFormField>
      </SettingRow>

      <SettingRow
        :label="$t('settings.password.confirm')"
        html-for="password-confirm"
      >
        <UFormField name="confirmPassword">
          <UInput
            id="password-confirm"
            v-model="state.confirmPassword"
            type="password"
            autocomplete="new-password"
            class="w-full max-w-lg"
            icon="tabler:key"
          />
        </UFormField>
      </SettingRow>
    </UForm>

    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
        <p class="text-xs text-muted">
          {{ $t('settings.password.help') }}
        </p>
        <UButton
          type="submit"
          size="sm"
          form="passwordSettingsForm"
          :loading="saving"
          :disabled="!filled"
          icon="tabler:key"
        >
          {{ $t('settings.password.save') }}
        </UButton>
      </div>
    </template>
  </DashboardPanel>
</template>
