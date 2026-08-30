<script setup lang="ts">
const {
  currentSortLabel,
  currentSortIcon,
  currentSortOption,
  availableSorts,
  setSortOption,
} = usePhotoSort()
const {
  selectedColumns,
  minColumns,
  maxColumns,
  isAdjustable,
} = useMasonryLayout()
</script>

<template>
  <UPopover>
    <UTooltip
      :text="$t('ui.action.displaySettings.tooltip')"
      ignore-non-keyboard-focus
    >
      <UButton
        variant="ghost"
        :color="currentSortOption?.key === 'dateTaken-desc' ? 'neutral' : 'info'"
        class="masonry-topbar__button"
        icon="tabler:adjustments-horizontal"
        size="sm"
        :aria-label="$t('ui.action.displaySettings.tooltip')"
      />
    </UTooltip>
    <template #content>
      <UCard variant="glassmorphism" class="w-3xs">
        <template #header>
          <h3 class="p-1 text-sm font-bold">
            {{ $t('ui.action.displaySettings.title') }}
          </h3>
        </template>
        <div class="space-y-4">
          <div class="space-y-1">
            <p class="px-1 text-xs font-medium text-neutral-500 dark:text-neutral-400">
              {{ $t('ui.action.displaySettings.sort') }}
            </p>
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
          <div v-if="isAdjustable" class="space-y-2 border-t border-neutral-200/70 pt-3 dark:border-neutral-700/70">
            <div class="flex items-center justify-between px-1 text-xs font-medium text-neutral-600 dark:text-neutral-300">
              <span>{{ $t('ui.action.displaySettings.columns') }}</span>
              <span>{{ selectedColumns }}</span>
            </div>
            <USlider
              v-model="selectedColumns"
              :min="minColumns"
              :max="maxColumns"
              :step="1"
              color="neutral"
              size="sm"
              :aria-label="$t('ui.action.displaySettings.columns')"
            />
            <div class="flex justify-between px-1 text-[11px] text-neutral-400">
              <span>{{ minColumns }}</span>
              <span>{{ maxColumns }}</span>
            </div>
          </div>
        </div>
      </UCard>
    </template>
  </UPopover>
</template>
