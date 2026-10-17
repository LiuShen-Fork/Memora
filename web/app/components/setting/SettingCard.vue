<script lang="ts" setup>
/**
 * A card of settings: what the group is, the rows themselves, and the two
 * buttons that finish the job.
 *
 * Every settings page ended with the same footer — a warning that there are
 * unsaved changes, a way to put them back, and a way to keep them — written
 * out by hand and drifting a little further apart each time. The warning is a
 * line beside the buttons now instead of a block above them: it is not an
 * error, it is a state, and it belongs next to the thing that resolves it.
 *
 * The submit button lives outside the <form> it submits. That is why it names
 * the form outright: a button at the bottom of a card is easier to find than
 * one at the bottom of a form, and the button's own form attribute is what
 * makes both true at once.
 */
defineProps<{
  title: string
  description?: string
  icon?: string
  /** The id of the <form> the save button submits. */
  formId: string
  /** Whether anything in the group differs from what is saved. */
  dirty?: boolean
  loading?: boolean
}>()

const emit = defineEmits<{
  reset: []
}>()
</script>

<template>
  <DashboardPanel
    :title="title"
    :description="description"
    :icon="icon"
    :ui="{ body: 'px-4 py-0 sm:px-4' }"
  >
    <slot />

    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
        <p
          v-if="dirty"
          class="inline-flex items-center gap-1.5 text-xs text-warning"
        >
          <Icon
            name="tabler:alert-circle"
            class="size-3.5 shrink-0"
          />
          {{ $t('common.unsavedChanges') }}
        </p>

        <div class="ms-auto flex items-center gap-2">
          <UButton
            color="neutral"
            variant="outline"
            size="sm"
            :disabled="!dirty || loading"
            @click="emit('reset')"
          >
            {{ $t('common.actions.reset') }}
          </UButton>
          <UButton
            type="submit"
            size="sm"
            :form="formId"
            :loading="loading"
            :disabled="!dirty"
            icon="tabler:device-floppy"
          >
            {{ $t('common.actions.saveSettings') }}
          </UButton>
        </div>
      </div>
    </template>
  </DashboardPanel>
</template>
