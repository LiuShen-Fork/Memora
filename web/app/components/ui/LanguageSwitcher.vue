<script setup lang="ts">
withDefaults(
  defineProps<{
    compact?: boolean
    block?: boolean
  }>(),
  { compact: false, block: false },
)

const { locale } = useI18n()

const languageOptions = [
  { label: '简体中文', value: 'zh-Hans' },
  { label: '繁體中文（台灣）', value: 'zh-Hant-TW' },
  { label: '繁體中文（香港）', value: 'zh-Hant-HK' },
  { label: 'English', value: 'en' },
  { label: '日本語', value: 'ja' },
  { label: 'Русский', value: 'ru' },
]

const menuItems = computed(() =>
  languageOptions.map((option) => ({
    ...option,
    onSelect: () => {
      locale.value = option.value
    },
  })),
)
</script>

<template>
  <UDropdownMenu
    :items="menuItems"
    :content="{
      align: 'end',
      sideOffset: 6,
      collisionPadding: 8,
    }"
    :ui="{
      content: 'w-40 max-w-[calc(100vw-1rem)]',
    }"
  >
    <UTooltip :text="$t('common.languageSwitcher.label')">
      <UButton
        variant="ghost"
        color="neutral"
        icon="tabler:language"
        :square="compact"
        :size="compact ? 'xs' : 'sm'"
        :aria-label="$t('common.languageSwitcher.label')"
        :class="[
          'cursor-pointer',
          compact ? 'size-7 rounded-full p-0 inline-flex items-center justify-center' : 'rounded-full',
          'bg-transparent hover:bg-neutral-100 dark:hover:bg-neutral-700',
          block ? 'w-full justify-center' : '',
        ]"
      />
    </UTooltip>
  </UDropdownMenu>
</template>
