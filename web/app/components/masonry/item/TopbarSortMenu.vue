<script setup lang="ts">
const {
  currentSortLabel,
  currentSortIcon,
  currentSortOption,
  availableSorts,
  setSortOption,
} = usePhotoSort()
</script>

<template>
  <UPopover>
    <UTooltip :text="$t('ui.action.sort.tooltip')">
      <UButton
        variant="ghost"
        :color="currentSortOption?.key === 'dateTaken-desc' ? 'neutral' : 'info'"
        class="masonry-topbar__button"
        :icon="currentSortIcon"
        size="sm"
        :aria-label="$t('ui.action.sort.tooltip')"
      />
    </UTooltip>
    <template #content>
      <UCard variant="glassmorphism" class="w-3xs">
        <template #header>
          <h3 class="p-1 text-sm font-bold">
            {{ $t('ui.action.sort.title') }}
          </h3>
        </template>
        <div class="space-y-1">
          <UButton
            v-for="sort in availableSorts"
            :key="sort.key"
            :variant="currentSortLabel === sort.labelI18n ? 'soft' : 'ghost'"
            :color="currentSortLabel === sort.labelI18n ? 'info' : 'neutral'"
            :icon="sort.icon"
            size="sm"
            block
            class="justify-start"
            @click="setSortOption(sort.key)"
          >
            {{ $t(sort.labelI18n) }}
          </UButton>
        </div>
      </UCard>
    </template>
  </UPopover>
</template>
