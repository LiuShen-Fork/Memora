<script setup lang="ts">
withDefaults(
  defineProps<{
    compact?: boolean
  }>(),
  { compact: false },
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
</script>

<template>
  <USelectMenu
    v-model="locale"
    :items="languageOptions"
    value-key="value"
    label-key="label"
    :search-input="false"
    :aria-label="$t('common.languageSwitcher.label')"
    icon="tabler:language"
    :size="compact ? 'xs' : 'sm'"
    variant="soft"
    :class="compact ? 'w-32' : 'w-full'"
  />
</template>
