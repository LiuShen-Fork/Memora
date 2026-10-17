<script setup lang="ts">
import { resolveComponent } from 'vue'
import type { FieldDescriptor, FieldUIType } from '~~/shared/types/settings'

interface Props {
  field: FieldDescriptor
  modelValue: any
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:modelValue': [value: any]
}>()

const { t, te } = useI18n()

const getComponentName = (uiType: FieldUIType): string => {
  const componentMap: Record<FieldUIType, string> = {
    input: 'UInput',
    password: 'UInput',
    url: 'UInput',
    textarea: 'UTextarea',
    select: 'USelectMenu',
    radio: 'URadioGroup',
    tabs: 'UTabs',
    toggle: 'USwitch',
    number: 'UInput',
    custom: 'UInput', // 默认降级到 input
  }

  return componentMap[uiType] || 'UInput'
}

const UInput = resolveComponent('UInput')
const UTextarea = resolveComponent('UTextarea')
const USelectMenu = resolveComponent('USelectMenu')
const URadioGroup = resolveComponent('URadioGroup')
const UTabs = resolveComponent('UTabs')
const USwitch = resolveComponent('USwitch')

const componentName = computed(() => {
  const name = getComponentName(props.field.ui.type)
  switch (name) {
    case 'UInput':
      return UInput
    case 'UTextarea':
      return UTextarea
    case 'USelectMenu':
      return USelectMenu
    case 'URadioGroup':
      return URadioGroup
    case 'UTabs':
      return UTabs
    case 'USwitch':
      return USwitch
    default:
      return UInput
  }
})

/**
 * The words on one option of a select, a radio group or a tab row.
 *
 * What the server sends as an option's label is sometimes a value the interface
 * has a name for — the theme is stored as `light`, `dark` or `system` — and
 * sometimes the words themselves, which is what a language picker wants: a
 * person looking for Russian should see Русский, not the name of the locale
 * they are currently reading in. So the value is looked up, and what the server
 * sent is used when there is nothing to find it by.
 */
const optionLabel = (option: { label?: string; value: string }) => {
  const prefix = `settings.${props.field.namespace}.${props.field.key}`
  for (const path of [`${prefix}.options.${option.value}`, `${prefix}.${option.value}`]) {
    if (te(path)) return t(path)
  }
  return option.label ?? String(option.value)
}

const optionList = () =>
  props.field.ui.options
    ? Array.from(props.field.ui.options).map((option: any) => ({
        label: optionLabel(option),
        value: option.value,
        icon: option.icon,
      }))
    : []

/**
 * Get extra props for the component
 */
const getComponentProps = (): Record<string, any> => {
  const type = props.field.ui.type
  const propsMap: Record<string, any> = {}

  if (props.field.ui.placeholder) {
    propsMap.placeholder = props.field.ui.placeholder
  }

  switch (type) {
    case 'password':
    case 'url':
    case 'number':
      propsMap.type = type
      break
    case 'select':
      propsMap.items = optionList()
      propsMap['label-key'] = 'label'
      propsMap['value-key'] = 'value'
      break
    case 'radio':
      propsMap.options = optionList()
      break
    case 'tabs':
      // 一排小分段按钮，宽度跟着字数走：三个字的选择不需要铺满整张卡片，
      // 也不必居中 —— 它要和上面下面的控件从同一条竖线开始。
      propsMap.items = optionList()
      propsMap.variant = 'pill'
      propsMap.color = 'neutral'
      propsMap.size = 'sm'
      propsMap.ui = { list: 'w-fit', root: 'items-start' }
      break
    case 'textarea':
      propsMap.rows = props.field.ui.rows ?? 3
      break
  }

  return propsMap
}

const componentProps = computed(() => getComponentProps())

const handleChange = (value: any) => {
  emit('update:modelValue', value)
}

const labelKey = computed(() => {
  // 尝试从 label 字段获取翻译键
  if (props.field.label) {
    return props.field.label
  }
  // 否则构造一个默认的翻译键
  return `settings.${props.field.namespace}.${props.field.key}.label`
})

const descriptionKey = computed(() => {
  if (props.field.description) {
    return props.field.description
  }
  return `settings.${props.field.namespace}.${props.field.key}.description`
})

const isToggleField = computed(() => props.field.ui.type === 'toggle')

const inputId = computed(
  () => `setting-${props.field.namespace}-${props.field.key}`,
)
const descriptionId = computed(() => `${inputId.value}-description`)

/**
 * How wide a control is allowed to be.
 *
 * A settings control is read one line at a time, and a line of text stops
 * being easy to follow well before a card is full: an input stretched across
 * a 1000px card asks the eye to travel the whole way back and forth for a
 * value like "Memora". Each one is capped, and a textarea — the only control
 * that is genuinely read as a block — is capped less tightly.
 */
const controlWidth = computed(() =>
  props.field.ui.type === 'textarea' ? 'max-w-2xl' : 'max-w-lg',
)
</script>

<template>
  <!-- 一项设置一行：左边说这是什么，右边是能改的那个控件。 -->
  <SettingRow
    :data-tour-setting="`${field.namespace}:${field.key}`"
    :label="$t(labelKey)"
    :description="$t(descriptionKey)"
    :description-id="descriptionId"
    :html-for="isToggleField ? undefined : inputId"
    :required="field.ui.required"
  >
    <component
      :is="componentName"
      :id="isToggleField ? undefined : inputId"
      :model-value="modelValue"
      v-bind="componentProps"
      class="w-full"
      :class="isToggleField ? '' : controlWidth"
      :aria-describedby="descriptionId"
      @update:model-value="handleChange"
    />
    <p
      v-if="field.ui.help"
      class="mt-1.5 text-xs leading-relaxed text-dimmed"
    >
      {{ $t(field.ui.help) }}
    </p>
  </SettingRow>
</template>
