<script setup lang="ts">
import { z } from 'zod'

definePageMeta({
  layout: 'onboarding',
})

const router = useRouter()
const { t } = useI18n()

// Use wizard form for 'app' namespace (site settings)
const {
  fields,
  state,
  loading: fetchingSchema,
  isFieldVisible,
} = useWizardForm('app')

const schema = computed(() =>
  z.object({
    title: z.string().min(
      1,
      t('onboarding.validation.required', {
        field: t('settings.app.title.label'),
      }),
    ),
    slogan: z.string().optional(),
    author: z.string().optional(),
    avatarUrl: z
      .string()
      .url(t('onboarding.validation.invalidUrl'))
      .optional()
      .or(z.literal('')),
  }),
)

function onSubmit() {
  // Validation passed, data is already in the store via useWizardForm binding
  router.push('/onboarding/storage')
}
</script>

<template>
  <WizardStep
    :title="$t('onboarding.site.title')"
    :description="$t('onboarding.site.description')"
  >
    <div
      v-if="fetchingSchema"
      class="flex justify-center py-8"
    >
      <UIcon
        name="tabler:loader"
        class="animate-spin w-8 h-8 text-gray-400"
      />
    </div>

    <UForm
      v-else
      id="site-form"
      :schema="schema"
      :state="state"
      class="space-y-4"
      @submit="onSubmit"
    >
      <template
        v-for="field in fields"
        :key="field.key"
      >
        <WizardFormField
          v-if="isFieldVisible(field)"
          :label="$t(field.label || '')"
          :name="field.key"
          :required="field.ui.required"
          :help="$t(field.ui.help || '')"
        >
          <WizardInput
            v-model="state[field.key]"
            :type="field.ui.type === 'url' ? 'url' : 'text'"
            :placeholder="field.ui.placeholder"
          />
        </WizardFormField>
      </template>
    </UForm>

    <template #actions>
      <WizardButton
        to="/onboarding/admin"
        variant="outline"
        color="primary"
        size="lg"
        leading-icon="tabler:arrow-left"
      >
        {{ $t('onboarding.actions.previous') }}
      </WizardButton>
      <WizardButton
        type="submit"
        form="site-form"
        color="primary"
        size="lg"
        :disabled="fetchingSchema"
        trailing-icon="tabler:arrow-right"
      >
        {{ $t('onboarding.actions.next') }}
      </WizardButton>
    </template>
  </WizardStep>
</template>
