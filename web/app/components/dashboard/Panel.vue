<script lang="ts" setup>
/**
 * The dashboard's card.
 *
 * Everything in the admin sits on one of these, so this is where the surface,
 * the corner and the rhythm between a card's edge and its content are decided
 * once. Nuxt UI's card is generous — 24px of padding on every side — which is
 * pleasant for a form and wasteful for a table, so the padding is tightened
 * here and `flush` takes it away entirely when the content is a table that
 * should run to the card's edge.
 *
 * The header holds a title, an optional line under it, and whatever the page
 * wants on the right.
 */
const props = withDefaults(
  defineProps<{
    title?: string
    description?: string
    icon?: string
    /** Let the content reach the card's edges — for tables and lists. */
    flush?: boolean
    /** Slot classes to replace any of the ones chosen here. */
    ui?: Record<string, string>
  }>(),
  {
    title: undefined,
    description: undefined,
    icon: undefined,
    flush: false,
    ui: undefined,
  },
)

const cardUi = computed(() => ({
  root: 'shadow-xs',
  header: 'px-4 py-3 sm:px-4',
  body: 'px-4 py-4 sm:px-4',
  footer: 'px-4 py-3 sm:px-4',
  ...(props.flush ? { body: 'p-0 sm:p-0' } : {}),
  ...props.ui,
}))
</script>

<template>
  <UCard :ui="cardUi">
    <template
      v-if="title || $slots.header"
      #header
    >
      <slot name="header">
        <div class="flex items-center justify-between gap-3">
          <div class="flex min-w-0 items-center gap-2.5">
            <UIcon
              v-if="icon"
              :name="icon"
              class="size-[18px] shrink-0 text-dimmed"
            />
            <div class="min-w-0">
              <h2 class="truncate text-sm font-semibold text-highlighted">
                {{ title }}
              </h2>
              <p
                v-if="description"
                class="truncate text-xs text-muted"
              >
                {{ description }}
              </p>
            </div>
          </div>
          <div class="flex shrink-0 items-center gap-1.5">
            <slot name="actions" />
          </div>
        </div>
      </slot>
    </template>

    <slot />

    <template
      v-if="$slots.footer"
      #footer
    >
      <slot name="footer" />
    </template>
  </UCard>
</template>
