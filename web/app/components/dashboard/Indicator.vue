<script lang="ts" setup>
import { isNil } from 'es-toolkit'

/**
 * One number from the admin, with the word that names it.
 *
 * These used to be panels of colour — a blue gradient, a green one, a purple
 * one — which made four equally important figures shout at four different
 * volumes and left the one that actually needed attention standing next to
 * three that were all shouting just as loudly. Colour is the scarcest thing
 * on a dashboard, so it is spent here on the one icon that tells you which
 * figure this is, and reserved for the case where it means something.
 */
const props = withDefaults(
  defineProps<{
    title?: string
    value?: string | number
    icon?: string
    /** Colour is meaning: a hue says this figure is good, or late, or wrong. */
    tone?: 'neutral' | 'accent' | 'positive' | 'attention' | 'negative'
    /** A line under the figure, for a comparison or a unit. */
    hint?: string
    clickable?: boolean
  }>(),
  {
    title: undefined,
    value: undefined,
    icon: undefined,
    tone: 'neutral',
    hint: undefined,
    clickable: false,
  },
)

const emit = defineEmits<{
  click: []
}>()

const TONES = {
  neutral: 'text-dimmed',
  accent: 'text-primary',
  positive: 'text-success',
  attention: 'text-warning',
  negative: 'text-error',
} as const

const toneClass = computed(() => TONES[props.tone])

// A figure is a date, a count or a size, and all three are read by comparing
// them with the figure beside them, so they get the large setting. A word is
// not a figure: at 26px "处理中" would be the loudest thing on the page, and
// it is only ever telling you which of two states you are in.
const isFigure = computed(() => !isNil(props.value) && /^[\d.,\s]+%?$/.test(String(props.value)))

// Red on a failure count of zero would be an alarm that never stops ringing.
const isAlarming = computed(
  () => props.tone === 'negative' && String(props.value ?? '').trim() !== '0' && !isNil(props.value),
)
</script>

<template>
  <DashboardPanel
    class="transition-colors"
    :class="clickable ? 'cursor-pointer hover:ring-accented' : ''"
    :ui="{ body: 'px-4 py-3.5 sm:px-4' }"
    @click="clickable ? emit('click') : undefined"
  >
    <div class="flex items-start justify-between gap-3">
      <p class="min-w-0 truncate text-xs font-medium text-muted">
        {{ title }}
      </p>
      <UIcon
        v-if="icon"
        :name="icon"
        class="mt-0.5 size-4 shrink-0"
        :class="toneClass"
      />
    </div>

    <p
      v-if="!isNil(value)"
      class="text-highlighted"
      :class="[
        isFigure
          ? 'mt-2 text-[26px] leading-7 font-medium tracking-tight tabular'
          : 'mt-1.5 text-base leading-6 font-medium',
        isAlarming ? 'text-error' : '',
      ]"
    >
      {{ value }}
    </p>

    <p
      v-if="hint"
      class="mt-1 truncate text-xs text-dimmed"
    >
      {{ hint }}
    </p>
  </DashboardPanel>
</template>
