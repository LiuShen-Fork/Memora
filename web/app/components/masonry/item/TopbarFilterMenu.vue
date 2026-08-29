<script setup lang="ts">
const { hasActiveFilters, selectedCounts } = usePhotoFilters()

const totalSelectedFilters = computed(() =>
  Object.values(selectedCounts.value).reduce((total, count) => total + count, 0),
)
</script>

<template>
  <UPopover>
    <UTooltip
      :text="$t('ui.action.filter.tooltip')"
      ignore-non-keyboard-focus
    >
      <UChip inset size="sm" color="info" :show="totalSelectedFilters > 0">
        <UButton
          variant="ghost"
          :color="hasActiveFilters ? 'info' : 'neutral'"
          class="masonry-topbar__button"
          icon="tabler:filter"
          size="sm"
          :aria-label="$t('ui.action.filter.tooltip')"
        />
      </UChip>
    </UTooltip>
    <template #content>
      <UCard variant="glassmorphism">
        <OverlayFilterPanel />
      </UCard>
    </template>
  </UPopover>
</template>
