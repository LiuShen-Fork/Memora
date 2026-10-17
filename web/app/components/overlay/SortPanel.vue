<script setup lang="ts">
// 按什么顺序看照片，画廊和后台照片表是同一件事，所以用的是同一份列表。
const { currentSortLabel, availableSorts, setSortOption } = usePhotoSort()
</script>

<template>
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
</template>
