<script lang="ts" setup>
/**
 * 相簿每一行的操作按钮。表格里的那一格和窄屏卡片底部那一行用的是同一份，
 * 免得两个地方各写一遍、日后只改一处。
 *
 * 只管「按了哪个键」，动的是哪本相簿由父组件自己知道。
 */
interface Props {
  /** 在列表里的位置，决定上下移动能不能按。 */
  index: number
  /** 列表长度，用来判断是不是最后一个。 */
  total: number
  reordering?: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  move: [offset: number]
  edit: []
  delete: []
}>()

const canMoveUp = computed(() => props.index > 0 && !props.reordering)
const canMoveDown = computed(
  () => props.index < props.total - 1 && !props.reordering,
)
</script>

<template>
  <div
    data-tour="album-actions"
    class="flex items-center gap-1"
  >
    <UButton
      variant="ghost"
      color="neutral"
      size="xs"
      icon="tabler:chevron-up"
      :title="$t('dashboard.albums.actions.moveUp')"
      :disabled="!canMoveUp"
      @click="emit('move', -1)"
    />
    <UButton
      variant="ghost"
      color="neutral"
      size="xs"
      icon="tabler:chevron-down"
      :title="$t('dashboard.albums.actions.moveDown')"
      :disabled="!canMoveDown"
      @click="emit('move', 1)"
    />
    <span class="mx-0.5 h-4 w-px bg-border" />
    <UButton
      variant="ghost"
      color="neutral"
      size="xs"
      icon="tabler:edit"
      :title="$t('common.actions.edit')"
      @click="emit('edit')"
    />
    <UButton
      variant="ghost"
      color="error"
      size="xs"
      icon="tabler:trash"
      :title="$t('common.actions.delete')"
      @click="emit('delete')"
    />
  </div>
</template>
