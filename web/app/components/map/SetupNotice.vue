<script setup lang="ts">
const { isOpen, isAdmin, dismiss } = useMapSetupNotice()

async function openSettings() {
  dismiss()
  await navigateTo('/dashboard/settings/map')
}
</script>

<template>
  <UModal
    v-model:open="isOpen"
    :title="$t('map.setupNotice.title')"
    :ui="{
      overlay: 'z-[80]',
      content: 'z-[80]',
      footer: 'justify-end',
    }"
  >
    <template #body>
      <div class="flex items-start gap-4">
        <div
          class="flex size-10 shrink-0 items-center justify-center rounded-full bg-warning-100 text-warning-600 dark:bg-warning-900/40 dark:text-warning-400"
        >
          <Icon
            name="tabler:map-off"
            class="size-6"
          />
        </div>
        <p
          class="text-sm leading-relaxed text-neutral-500 dark:text-neutral-400"
        >
          {{ $t('map.setupNotice.description') }}
        </p>
      </div>
    </template>
    <template #footer>
      <UButton
        color="neutral"
        variant="outline"
        @click="dismiss"
      >
        {{ $t('map.setupNotice.dismiss') }}
      </UButton>
      <UButton
        v-if="isAdmin"
        icon="tabler:settings"
        @click="openSettings"
      >
        {{ $t('map.setupNotice.openSettings') }}
      </UButton>
    </template>
  </UModal>
</template>
