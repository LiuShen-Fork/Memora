<script lang="ts" setup>
/**
 * A settings card that has not arrived yet.
 *
 * It is the same shape as the card that replaces it — header, rows, the space
 * where the buttons go — so the page settles rather than jumps when the
 * settings load. The rows are drawn, not named: a label is fetched along with
 * its value, so there is nothing to name them with yet.
 */
withDefaults(
  defineProps<{
    rows?: number
  }>(),
  { rows: 2 },
)
</script>

<template>
  <DashboardPanel :ui="{ body: 'px-4 py-0 sm:px-4' }">
    <template #header>
      <div class="flex w-full items-center gap-2.5">
        <USkeleton class="size-[18px] rounded" />
        <USkeleton class="h-4 w-28" />
      </div>
    </template>

    <div class="divide-y divide-default">
      <SettingRow
        v-for="row in rows"
        :key="row"
      >
        <USkeleton class="h-9 w-full max-w-lg" />
      </SettingRow>
    </div>

    <template #footer>
      <div class="flex items-center justify-end gap-2">
        <USkeleton class="h-7 w-16 rounded-md" />
        <USkeleton class="h-7 w-24 rounded-md" />
      </div>
    </template>
  </DashboardPanel>
</template>
