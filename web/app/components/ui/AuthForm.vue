<script lang="ts" setup>
import { z } from 'zod'
import type { ButtonProps, FormSubmitEvent } from '@nuxt/ui'
import { twMerge } from 'tailwind-merge'

/**
 * The form that opens the archive.
 *
 * It is the site's own front door, so it says whose door it is: the owner's
 * avatar if there is one, the site's name, and the line the owner wrote under
 * it. Everything below that is the way in and nothing else — the page around
 * it holds the way back out.
 */
const props = withDefaults(
  defineProps<{
    title?: string
    subtitle?: string
    logo?: string
    providers?: Array<ButtonProps | false | undefined>
    class?: string
    loading?: boolean
  }>(),
  { logo: '/web-app-manifest-192x192.png' },
)

const emit = defineEmits<{
  submit: [event: FormSubmitEvent<Schema>]
}>()

const { t } = useI18n()

const schema = z.object({
  email: z.email(t('auth.form.errors.invalidEmail')),
  password: z.string().min(6, t('auth.form.errors.invalidPassword')),
})

type Schema = z.output<typeof schema>

const state = reactive<Partial<Schema>>({
  email: undefined,
  password: '',
})

const showPassword = ref(false)

// 头像取不到时退回图标，门上不留一张破图。
const logoFailed = ref(false)

const providers = computed(() =>
  (props.providers ?? []).filter((item): item is ButtonProps => Boolean(item)),
)

const onSubmit = async (event: FormSubmitEvent<Schema>) => {
  emit('submit', event)
}
</script>

<template>
  <div :class="twMerge('w-full max-w-sm', $props.class)">
    <div class="rounded-2xl border border-default bg-default p-6 shadow-sm">
      <!-- 这是谁家的门 -->
      <div class="flex flex-col items-center text-center">
        <img
          v-if="logo && !logoFailed"
          :src="logo"
          :alt="$t('ui.authForm.logoAlt')"
          class="size-16 rounded-full object-cover ring-1 ring-default"
          @error="logoFailed = true"
        />
        <div
          v-else
          class="flex size-16 items-center justify-center rounded-full bg-elevated text-dimmed ring-1 ring-default"
        >
          <UIcon
            name="tabler:aperture"
            class="size-8"
          />
        </div>

        <h1
          v-if="title"
          class="mt-4 text-lg font-semibold text-highlighted text-pretty"
        >
          {{ title }}
        </h1>
        <p
          v-if="subtitle"
          class="mt-1 text-sm text-muted text-pretty"
        >
          {{ subtitle }}
        </p>
      </div>

      <div class="mt-6 flex flex-col gap-4">
        <div
          v-if="providers.length"
          :class="providers.length > 3 ? 'space-y-2' : 'flex items-center gap-2'"
        >
          <UButton
            v-for="provider in providers"
            :key="provider.icon"
            v-bind="provider"
            :loading="loading"
          />
        </div>

        <USeparator
          v-if="providers.length"
          :label="$t('auth.form.action.or')"
        />

        <UForm
          class="space-y-4"
          :schema="schema"
          :state="state"
          :disabled="loading"
          @submit="onSubmit"
        >
          <UFormField
            :label="$t('auth.form.labels.email')"
            name="email"
          >
            <UInput
              v-model="state.email"
              type="email"
              autocomplete="username"
              icon="tabler:mail"
              size="lg"
              class="w-full"
            />
          </UFormField>

          <UFormField
            :label="$t('auth.form.labels.password')"
            name="password"
          >
            <UInput
              v-model="state.password"
              :type="showPassword ? 'text' : 'password'"
              autocomplete="current-password"
              icon="tabler:lock"
              size="lg"
              class="w-full"
            >
              <template #trailing>
                <UButton
                  type="button"
                  color="neutral"
                  variant="link"
                  size="xs"
                  :icon="showPassword ? 'tabler:eye-off' : 'tabler:eye'"
                  :aria-label="
                    $t(
                      showPassword
                        ? 'auth.form.action.hidePassword'
                        : 'auth.form.action.showPassword',
                    )
                  "
                  :padded="false"
                  @click="showPassword = !showPassword"
                />
              </template>
            </UInput>
          </UFormField>

          <UButton
            type="submit"
            size="lg"
            block
            trailing-icon="tabler:arrow-right"
            :loading="loading"
          >
            {{ $t('auth.form.action.continue') }}
          </UButton>
        </UForm>
      </div>
    </div>
  </div>
</template>
