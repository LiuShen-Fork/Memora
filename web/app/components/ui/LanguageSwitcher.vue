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
    :content="{ align: compact ? 'start' : 'end' }"
  >
    <UTooltip :text="$t('common.languageSwitcher.label')">
      <UButton
        variant="soft"
        color="neutral"
        icon="tabler:language"
        :size="compact ? 'xs' : 'sm'"
        :aria-label="$t('common.languageSwitcher.label')"
        :class="[
          'cursor-pointer',
          compact ? 'size-9 rounded-lg' : 'rounded-full',
          block ? 'w-full justify-center' : '',
        ]"
      />
    </UTooltip>
  </UDropdownMenu>
</template>
