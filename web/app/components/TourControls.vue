<script setup lang="ts">
const { ready, busy, load, update } = useTours()
const { user } = useUserSession()

// The guides are read as soon as the administrator signs in; reading them again
// here keeps the button usable if that first read happened to fail.
onMounted(() => {
  if (!ready.value) void load()
})
</script>

<template>
  <section
    v-if="user?.isAdmin"
    data-tour="reset"
    class="rounded-md border border-neutral-200 bg-white dark:border-neutral-800 dark:bg-neutral-950"
  >
    <header
      class="border-b border-neutral-200 px-5 py-4 dark:border-neutral-800"
    >
      <h3
        class="text-base font-semibold text-neutral-900 dark:text-neutral-100"
      >
        {{ $t('tours.controlsTitle') }}
      </h3>
    </header>

    <div
      class="flex flex-col gap-3 px-5 py-5 sm:flex-row sm:items-center sm:justify-between"
    >
      <p class="text-sm text-neutral-600 dark:text-neutral-400">
        {{ $t('tours.resetHelp') }}
      </p>
      <UButton
        :loading="busy"
        :disabled="!ready"
        icon="tabler:refresh"
        color="neutral"
        variant="outline"
        class="shrink-0 self-start sm:self-auto"
        @click="update(undefined, 'pending', { redirectTo: '/dashboard' })"
      >
        {{ $t('tours.reset') }}
      </UButton>
    </div>
  </section>
</template>
