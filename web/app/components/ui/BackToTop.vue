<script lang="ts" setup>
/**
 * 右下角那颗「回到顶部」。
 *
 * 外形交给全局的 `.gallery-capsule`——它和旁边的站点胶囊、社交按钮是同一
 * 件东西，所以高度、材质、悬停都要一起变。这里只负责把图标和四个字放进去：
 * 一个圆圈里塞个箭头，第一次来的人还得猜一下，写出来就不用猜了。
 *
 * 组件自己只管长相和点击，什么时候出现、摆在哪里由调用方决定。
 */
defineEmits<{ (e: 'click'): void }>()
</script>

<template>
  <button
    type="button"
    class="gallery-capsule cursor-pointer"
    :aria-label="$t('ui.action.backtotop.ariaLabel')"
    @click="$emit('click')"
  >
    <Icon
      name="tabler:arrow-up"
      class="size-3.5"
    />
    <span>{{ $t('ui.action.backtotop.tooltip') }}</span>
  </button>
</template>
