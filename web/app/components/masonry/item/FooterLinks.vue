<script lang="ts" setup>
/**
 * 照片墙左下角那一条：站点胶囊，以及站点自己挂的社交链接。
 *
 * 社交链接做成和胶囊一样高的圆形按钮，排在同一行里——它们属于同一件事
 * （「这是谁做的、去哪里找他」），分成两处反而不整齐。设置里没有链接时
 * 只留下胶囊，和以前完全一样。
 */
const { links } = useSocialLinks();

const defaultFooterLinkUrl = "https://github.com/LiuShen-Fork/Memora";
const customFooterLinkText = computed(() =>
  String(getSetting("app:footerLinkText") || "").trim(),
);
const customFooterLinkUrl = computed(() =>
  String(getSetting("app:footerLinkUrl") || "").trim(),
);
const hasCustomFooterLink = computed(
  () => Boolean(customFooterLinkText.value || customFooterLinkUrl.value),
);
const footerLinkText = computed(() => {
  return customFooterLinkText.value || "Memora";
});
const footerLinkUrl = computed(() => {
  const value = customFooterLinkUrl.value;
  try {
    const parsed = new URL(value);
    if (parsed.protocol === "http:" || parsed.protocol === "https:") {
      return parsed.toString();
    }
  } catch {
    // Invalid custom URLs use the project repository.
  }
  return defaultFooterLinkUrl;
});
</script>

<template>
  <div class="masonry-page-footer-group">
    <a
      :href="footerLinkUrl"
      target="_blank"
      rel="noopener noreferrer"
      class="masonry-page-footer"
    >
      <Icon v-if="!hasCustomFooterLink" name="tabler:brand-github" class="size-3.5" />
      <span class="max-w-36 truncate">{{ footerLinkText }}</span>
    </a>

    <a
      v-for="link in links"
      :key="link.href"
      :href="link.href"
      target="_blank"
      rel="noopener noreferrer"
      class="masonry-social-link"
      :title="link.label"
      :aria-label="link.label"
    >
      <Icon :name="link.icon" class="size-3.5" />
    </a>
  </div>
</template>

<style scoped>
.masonry-page-footer-group {
  position: fixed;
  z-index: 35;
  left: 0.75rem;
  bottom: 0.75rem;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  max-width: calc(100vw - 1.5rem);
  pointer-events: auto;
}

/* 站点胶囊和社交按钮共用一套外观：同一行里的东西，高度和材质都要一样。 */
.masonry-page-footer,
.masonry-social-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  height: 1.75rem;
  border: 1px solid rgb(255 255 255 / 30%);
  border-radius: 999px;
  background: rgb(255 255 255 / 48%);
  color: rgb(63 63 70 / 92%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 6%);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
  transition:
    background-color 0.15s ease,
    color 0.15s ease;
}

.masonry-page-footer {
  gap: 0.3rem;
  max-width: 10rem;
  padding: 0 0.6rem;
  font-size: 0.68rem;
  font-weight: 500;
}

.masonry-social-link {
  width: 1.75rem;
  flex: 0 0 auto;
}

.dark .masonry-page-footer,
.dark .masonry-social-link {
  border-color: rgb(255 255 255 / 10%);
  background: rgb(24 24 27 / 58%);
  color: rgb(228 228 231 / 82%);
}

.masonry-social-link:hover,
.masonry-page-footer:hover {
  background: rgb(255 255 255 / 78%);
  color: rgb(24 24 27);
}

.dark .masonry-social-link:hover,
.dark .masonry-page-footer:hover {
  background: rgb(39 39 42 / 78%);
  color: rgb(250 250 250);
}

@media (max-width: 768px) {
  .masonry-page-footer-group {
    left: 0.5rem;
    bottom: 0.5rem;
    gap: 0.3rem;
  }

  .masonry-page-footer {
    max-width: 7rem;
  }
}
</style>
