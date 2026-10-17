<script lang="ts" setup>
/**
 * One row of a settings card: what the setting is on the left, the thing that
 * changes it on the right.
 *
 * The row is a grid rather than a stack so the controls all start at the same
 * x — a card of settings is read by scanning down the controls, and that scan
 * only works if they line up. Below `sm` the two columns become one, because
 * an 18rem label column on a phone leaves nothing for the control.
 */
defineProps<{
  /** Left out while the setting is still being fetched and has no name yet. */
  label?: string
  description?: string
  /** The id of the control, so clicking the label focuses it. */
  htmlFor?: string
  /** Where the description sits, so a control can point its help text at it. */
  descriptionId?: string
  required?: boolean
}>()
</script>

<template>
  <div
    class="grid gap-x-10 gap-y-2 py-4 sm:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]"
  >
    <div class="min-w-0">
      <label
        v-if="label"
        :for="htmlFor"
        class="text-sm font-medium text-highlighted"
      >
        {{ label }}
        <span
          v-if="required"
          class="text-error"
          aria-hidden="true"
        >*</span>
      </label>
      <p
        v-if="description"
        :id="descriptionId"
        class="mt-0.5 text-xs leading-relaxed text-muted"
      >
        {{ description }}
      </p>
    </div>

    <div class="min-w-0">
      <slot />
    </div>
  </div>
</template>
