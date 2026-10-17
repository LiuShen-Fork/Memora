<script lang="ts" setup>
/**
 * The bar at the top of a page: what this page is, and what it is for.
 *
 * The line under the name is the page's own description, and this is the only
 * place it belongs. It used to sit in the body under a second copy of the page
 * name, while the first card of the page carried a third copy — on a settings
 * page that is the same three words stacked down the screen. The name is here,
 * the reason is here, and the cards below are free to be named after what each
 * one actually holds.
 */
defineProps<{
  title: string
  description?: string
}>()
</script>

<template>
  <UDashboardNavbar :title="title">
    <!-- 名字和说明挤在同一行会看不完，所以标题槽里竖着放两行。 -->
    <template #title>
      <span class="min-w-0">
        <span class="block truncate font-semibold text-highlighted">
          {{ title }}
        </span>
        <span
          v-if="description"
          class="block truncate text-xs font-normal text-muted"
        >
          {{ description }}
        </span>
      </span>
    </template>

    <template
      v-if="$slots.right"
      #right
    >
      <slot name="right" />
    </template>
  </UDashboardNavbar>
</template>
