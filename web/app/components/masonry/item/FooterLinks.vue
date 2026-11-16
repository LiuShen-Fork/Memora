<script lang="ts" setup>
/**
 * 照片墙右下角那一摞：上面是「回到顶部」，下面是站点胶囊和站点自己挂的
 * 社交链接。
 *
 * 三样东西都是浮在照片上的半透明胶囊，摆在两个对角只会互相打架——以前
 * 回到顶部单独飘在右下，胶囊和社交链接挤在左下，中间空出一大片却谁也不
 * 挨着谁。现在收进同一个停靠点，右对齐叠成一列，看起来是一组控件。
 *
 * 社交链接做成和胶囊一样高的圆形按钮，排在站点胶囊左边：属于同一件事
 * （「这是谁做的、去哪里找他」）。设置里没有链接时只留下胶囊。
 */
interface Props {
  /** 页面滚过首屏之后才给「回到顶部」让出位置。 */
  isScrolled?: boolean
}

withDefaults(defineProps<Props>(), {
  isScrolled: false,
})

const { links } = useSocialLinks()

/**
 * 站点胶囊最右边，可以是项目仓库，也可以是备案号——设置里填了自定义
 * 文案或链接就按设置的来。
 */
const defaultFooterLinkUrl = 'https://github.com/LiuShen-Fork/Memora'
const customFooterLinkText = computed(() =>
  String(getSetting('app:footerLinkText') || '').trim(),
)
const customFooterLinkUrl = computed(() =>
  String(getSetting('app:footerLinkUrl') || '').trim(),
)
const hasCustomFooterLink = computed(() =>
  Boolean(customFooterLinkText.value || customFooterLinkUrl.value),
)
const footerLinkText = computed(() => {
  return customFooterLinkText.value || 'Memora'
})
const footerLinkUrl = computed(() => {
  const value = customFooterLinkUrl.value
  try {
    const parsed = new URL(value)
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:') {
      return parsed.toString()
    }
  } catch {
    // Invalid custom URLs use the project repository.
  }
  return defaultFooterLinkUrl
})

const scrollToTop = () => {
  window.scrollTo({ top: 0, behavior: 'smooth' })
}
</script>

<template>
  <div class="masonry-corner">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0 translate-y-1 scale-90"
      leave-active-class="transition duration-150 ease-in"
      leave-to-class="opacity-0 translate-y-1 scale-90"
    >
      <BackToTop
        v-if="isScrolled"
        class="origin-bottom-right"
        @click="scrollToTop"
      />
    </Transition>

    <div class="masonry-corner__row">
      <a
        v-for="link in links"
        :key="link.href"
        :href="link.href"
        target="_blank"
        rel="noopener noreferrer"
        class="gallery-capsule gallery-capsule--round"
        :title="link.label"
        :aria-label="link.label"
      >
        <Icon
          :name="link.icon"
          class="size-3.5"
        />
      </a>

      <a
        :href="footerLinkUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="gallery-capsule masonry-page-footer"
      >
        <Icon
          v-if="!hasCustomFooterLink"
          name="tabler:brand-github"
          class="size-3.5"
        />
        <span class="max-w-36 truncate">{{ footerLinkText }}</span>
      </a>
    </div>
  </div>
</template>

<style scoped>
/* 停靠点贴着右下角：底边固定，所以「回到顶部」出现时整摞往上长，下面的
   那一行不会跟着跳。 */
.masonry-corner {
  position: fixed;
  z-index: 35;
  right: 0.75rem;
  bottom: 0.75rem;
  display: flex;
  max-width: calc(100vw - 1.5rem);
  flex-direction: column;
  align-items: flex-end;
  gap: 0.4rem;
  pointer-events: auto;
}

.masonry-corner__row {
  display: flex;
  max-width: 100%;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 0.4rem;
}

.masonry-page-footer {
  max-width: 10rem;
}

@media (max-width: 768px) {
  .masonry-corner {
    right: 0.5rem;
    bottom: 0.5rem;
    gap: 0.3rem;
  }

  .masonry-corner__row {
    gap: 0.3rem;
  }

  .masonry-page-footer {
    max-width: 7rem;
  }
}
</style>
