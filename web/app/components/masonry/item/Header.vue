<script lang="ts" setup>
interface Props {
  stats?: {
    total: number;
  };
  dateRangeText: string;
  locations?: string;
  /** 页面滚过首屏，紧凑顶栏借此把标语换成眼前的日期与地点。 */
  isScrolled?: boolean;
  /**
   * 首页页头的形状，由站点的 `app:appearance.homeHeaderLayout` 决定。
   *
   * - `compact`：一直吸在顶部的一条细栏，滚动时中间换内容。这是默认，也是
   *   这个项目一直以来的样子。
   * - `embedded`：一块随页面滚走的大页头，排在照片墙最前面。
   * - `fixed`：同样的大页头，但吸在顶部，滚过之后收成一行。
   */
  layout?: "compact" | "embedded" | "fixed";
}

const props = withDefaults(defineProps<Props>(), {
  stats: () => ({ total: 0 }),
  locations: "",
  isScrolled: false,
  layout: "compact",
});

// fixed 的页头滚过首屏后收成一行：站名留在左边，按钮挪到右边。
const isCollapsedHero = computed(
  () => props.layout === "fixed" && Boolean(props.isScrolled),
);

const router = useRouter();
const route = useRoute();
const { locale } = useI18n();

const photoCountLabel = computed(() => {
  switch (locale.value) {
    case "zh-Hant-TW":
    case "zh-Hant-HK":
      return "張作品";
    case "en":
      return " works";
    case "ja":
      return "作品";
    case "ru":
      return " работ";
    default:
      return "张作品";
  }
});

// 照片墙可能开在 `/`，也可能开在 `/photos`（首页选了相册时）。
const isWallPath = computed(() => ["/", "/photos"].includes(route.path));
</script>

<template>
  <header
    v-if="layout === 'compact'"
    class="masonry-topbar"
  >
    <div class="masonry-topbar__inner">
      <div
        class="masonry-topbar__identity"
        data-tour="home-avatar"
      >
        <img
          :src="(getSetting('app:avatarUrl') as string) || '/web-app-manifest-192x192.png'"
          class="size-8 shrink-0 rounded-full object-cover ring-1 ring-black/10 dark:ring-white/15"
          :alt="$t('ui.photo.avatarAlt')"
        />
        <div class="min-w-0 leading-tight">
          <h1 class="truncate text-sm font-semibold text-neutral-900 dark:text-white">
            {{ getSetting("app:title") }}
          </h1>
          <p class="truncate text-[11px] text-neutral-500 dark:text-white/55">
            {{ stats.total }}{{ photoCountLabel }}
            <span v-if="getSetting('app:author')" class="masonry-topbar__author">
              · @{{ getSetting("app:author") }}
            </span>
          </p>
        </div>
      </div>

      <div class="masonry-topbar__status" :class="{ 'is-scrolled': isScrolled }">
        <Transition name="topbar-status" mode="out-in">
          <div v-if="isScrolled" key="context" class="masonry-topbar__context">
            <span class="masonry-topbar__date">{{ dateRangeText }}</span>
            <span v-if="locations" class="masonry-topbar__location">
              {{ locations }}
            </span>
          </div>
          <span v-else-if="getSetting('app:slogan')" key="slogan" class="masonry-topbar__slogan">
            {{ getSetting("app:slogan") }}
          </span>
        </Transition>
      </div>

      <MasonryItemTopbarActions />
    </div>
  </header>

  <!--
    大页头：头像、站名、标语和照片总量竖着居中排在照片墙最前面。
    `fixed` 的时候它 sticky 在顶部，滚过之后收成一行；`embedded` 的时候
    就留在页面里，跟着照片一起滚走。
  -->
  <header
    v-else
    class="masonry-hero"
    :class="{ 'is-fixed': layout === 'fixed', 'is-compact': isCollapsedHero }"
    data-tour="home-avatar"
  >
    <div class="masonry-hero__inner">
      <img
        :src="(getSetting('app:avatarUrl') as string) || '/web-app-manifest-192x192.png'"
        class="masonry-hero__avatar"
        :alt="$t('ui.photo.avatarAlt')"
      />
      <h1 class="masonry-hero__title">
        {{ getSetting("app:title") }}
      </h1>
      <p v-if="getSetting('app:slogan')" class="masonry-hero__slogan">
        {{ getSetting("app:slogan") }}
      </p>
      <p class="masonry-hero__meta">
        <span>{{ stats.total }}{{ photoCountLabel }}</span>
        <span v-if="getSetting('app:author')" class="masonry-hero__author">
          · @{{ getSetting("app:author") }}
        </span>
        <span v-if="dateRangeText" class="masonry-hero__dates">
          · {{ dateRangeText }}
        </span>
      </p>
      <MasonryItemTopbarActions
        :align="isCollapsedHero ? 'end' : 'center'"
        class="masonry-hero__actions"
      />
    </div>
  </header>

  <MasonryItemFooterLinks v-if="isWallPath" />
</template>

<style scoped>
/* ── 紧凑顶栏 ─────────────────────────────────────────────── */

.masonry-topbar {
  position: fixed;
  inset: 0 0 auto;
  z-index: 40;
  pointer-events: none;
  padding: 0.3rem 0.75rem;
}

.masonry-topbar::before {
  position: absolute;
  inset: 0 0 auto;
  z-index: 0;
  height: 3.1rem;
  background: linear-gradient(
    to bottom,
    rgb(255 255 255 / 18%) 0%,
    rgb(255 255 255 / 8%) 48%,
    transparent 100%
  );
  content: "";
  pointer-events: none;
  -webkit-backdrop-filter: blur(18px);
  backdrop-filter: blur(18px);
  mask-image: linear-gradient(to bottom, black 0%, black 52%, transparent 100%);
  -webkit-mask-image: linear-gradient(to bottom, black 0%, black 52%, transparent 100%);
}

.masonry-topbar__inner {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  min-height: 2.5rem;
  max-width: 100%;
}

.masonry-topbar__identity {
  display: flex;
  min-width: 0;
  flex: 1 1 0%;
  align-items: center;
  gap: 0.625rem;
  pointer-events: auto;
}

.masonry-topbar__status {
  position: absolute;
  top: 1.5rem;
  left: 50%;
  display: flex;
  max-width: 28%;
  transform: translate(-50%, -65%);
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.1rem;
  overflow: hidden;
  text-align: center;
  pointer-events: none;
}

.masonry-topbar__status.is-scrolled {
  top: 1.5rem;
  transform: translate(-50%, -65%);
}

.masonry-topbar__context {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.1rem;
}

.topbar-status-enter-active,
.topbar-status-leave-active {
  transition:
    opacity 180ms ease,
    transform 220ms ease;
}

.topbar-status-enter-from {
  opacity: 0;
  transform: translateY(0.3rem);
}

.topbar-status-leave-to {
  opacity: 0;
  transform: translateY(-0.3rem);
}

.masonry-topbar__slogan {
  overflow: hidden;
  color: rgb(63 63 70 / 88%);
  font-family: Pacifico, cursive;
  font-size: 0.9rem;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.masonry-topbar__date {
  color: rgb(23 23 23 / 88%);
  font-size: 0.85rem;
  font-weight: 650;
  line-height: 1.2;
  white-space: nowrap;
}

.masonry-topbar__location {
  max-width: 100%;
  overflow: hidden;
  color: rgb(82 82 91 / 78%);
  font-size: 0.68rem;
  line-height: 1.2;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dark .masonry-topbar__slogan {
  color: rgb(244 244 245 / 68%);
}

.dark .masonry-topbar__date {
  color: rgb(250 250 250 / 90%);
}

.dark .masonry-topbar__location {
  color: rgb(212 212 216 / 72%);
}

.dark .masonry-topbar {
  background: transparent;
}

.dark .masonry-topbar::before {
  background: linear-gradient(
    to bottom,
    rgb(9 9 11 / 14%) 0%,
    rgb(9 9 11 / 6%) 48%,
    transparent 100%
  );
}

/* ── 大页头（embedded / fixed）────────────────────────────── */

.masonry-hero {
  position: relative;
  z-index: 40;
  padding: 2.5rem 1rem 1.5rem;
}

/* fixed：吸在顶部。背后的照片从页头下面穿过去，所以垫一层模糊的白。 */
.masonry-hero.is-fixed {
  position: sticky;
  top: 0;
}

.masonry-hero.is-fixed::before {
  position: absolute;
  inset: 0;
  z-index: 0;
  border-bottom: 1px solid rgb(0 0 0 / 6%);
  background: rgb(255 255 255 / 82%);
  content: "";
  pointer-events: none;
  -webkit-backdrop-filter: blur(18px);
  backdrop-filter: blur(18px);
  transition: background-color 200ms ease;
}

.dark .masonry-hero.is-fixed::before {
  border-bottom-color: rgb(255 255 255 / 8%);
  background: rgb(9 9 11 / 78%);
}

.masonry-hero__inner {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.35rem;
  text-align: center;
}

.masonry-hero__avatar {
  width: 4.25rem;
  height: 4.25rem;
  margin-bottom: 0.35rem;
  overflow: hidden;
  border-radius: 9999px;
  object-fit: cover;
  box-shadow: 0 8px 26px rgb(0 0 0 / 12%);
  transition:
    width 240ms ease,
    height 240ms ease,
    margin 240ms ease,
    opacity 160ms ease;
}

.dark .masonry-hero__avatar {
  box-shadow: 0 8px 26px rgb(0 0 0 / 45%);
  outline: 1px solid rgb(255 255 255 / 12%);
}

.masonry-hero__title {
  color: rgb(23 23 23);
  font-size: clamp(1.45rem, 1.1rem + 1.6vw, 2.15rem);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.15;
  transition: font-size 240ms ease;
}

.dark .masonry-hero__title {
  color: rgb(250 250 250);
}

.masonry-hero__slogan {
  color: rgb(63 63 70 / 82%);
  font-family: Pacifico, cursive;
  font-size: 0.95rem;
  line-height: 1.3;
}

.dark .masonry-hero__slogan {
  color: rgb(228 228 231 / 72%);
}

.masonry-hero__meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 0.3rem;
  color: rgb(82 82 91 / 88%);
  font-size: 0.78rem;
  line-height: 1.3;
}

.dark .masonry-hero__meta {
  color: rgb(212 212 216 / 72%);
}

.masonry-hero__actions {
  margin-top: 0.5rem;
}

/* 滚过首屏之后收成一行：头像、标语和统计让位，只留下站名和按钮 ——
   站名靠左，按钮靠右，和紧凑顶栏对齐。 */
.masonry-hero.is-compact {
  padding: 0.55rem 0.75rem 0.6rem;
}

.masonry-hero.is-compact .masonry-hero__inner {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  text-align: left;
}

.masonry-hero.is-compact .masonry-hero__avatar,
.masonry-hero.is-compact .masonry-hero__slogan,
.masonry-hero.is-compact .masonry-hero__meta {
  display: none;
}

.masonry-hero.is-compact .masonry-hero__title {
  margin: 0;
  overflow: hidden;
  font-size: 1rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.masonry-hero.is-compact .masonry-hero__actions {
  margin-top: 0;
  flex: 0 0 auto;
}

@media (max-width: 900px) {
  .masonry-topbar__status {
    display: none;
  }
}

@media (max-width: 768px) {
  .masonry-topbar {
    padding: 0.3rem 0.2rem 0.1rem 0.2rem;
  }

  .masonry-topbar::before {
    height: 2.8rem;
  }

  .masonry-topbar__inner {
    min-height: 2.4rem;
    align-items: flex-start;
    gap: 0.4rem;
  }

  .masonry-topbar__identity {
    gap: 0.4rem;
  }

  .masonry-topbar__identity img {
    width: 1.75rem;
    height: 1.75rem;
  }

  .masonry-topbar__slogan {
    font-size: 0.72rem;
  }

  .masonry-hero {
    padding: 2rem 0.85rem 1.25rem;
  }

  .masonry-hero__avatar {
    width: 3.5rem;
    height: 3.5rem;
  }
}

@media (max-width: 640px) {
  .masonry-topbar__author {
    display: none;
  }

  .masonry-topbar__identity {
    flex: 1 1 auto;
  }

  .masonry-hero__author,
  .masonry-hero__dates {
    display: none;
  }
}
</style>
