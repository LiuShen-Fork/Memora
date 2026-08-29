<script setup lang="ts">
withDefaults(
  defineProps<{
    compact?: boolean
    block?: boolean
    buttonClass?: string
    tooltip?: boolean
  }>(),
  { compact: false, block: false, buttonClass: '', tooltip: true },
)

const { locale } = useI18n()
const isOpen = ref(false)

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
      isOpen.value = false
    },
  })),
)
</script>

<template>
  <UPopover
    v-model:open="isOpen"
    :content="{
      align: 'center',
      sideOffset: 6,
      collisionPadding: 8,
    }"
  >
    <UTooltip
      :text="$t('common.languageSwitcher.label')"
      :disabled="!tooltip"
    >
      <UButton
        variant="ghost"
        color="neutral"
        icon="tabler:language"
        :square="compact && !buttonClass"
        :size="buttonClass ? 'sm' : compact ? 'xs' : 'sm'"
        :aria-label="$t('common.languageSwitcher.label')"
        :class="[
          compact && !buttonClass ? 'size-7 p-0 inline-flex items-center justify-center' : '',
          block ? 'w-full justify-center' : '',
          buttonClass,
        ]"
      />
    </UTooltip>
    <template #content>
      <UCard
        variant="glassmorphism"
        class="w-40 max-w-[calc(100vw-1rem)]"
      >
        <div class="space-y-1">
          <UButton
            v-for="option in menuItems"
            :key="option.value"
            :variant="locale === option.value ? 'soft' : 'ghost'"
            :color="locale === option.value ? 'info' : 'neutral'"
            icon="tabler:language"
            size="sm"
            block
            class="justify-start"
            @click="option.onSelect"
          >
            {{ option.label }}
          </UButton>
        </div>
      </UCard>
    </template>
  </UPopover>
</template>
