<script lang="ts" setup>
interface Props {
  stats?: {
    total: number;
  };
  dateRangeText: string;
  locations?: string;
  isScrolled?: boolean;
}

withDefaults(defineProps<Props>(), {
  stats: () => ({ total: 0 }),
  locations: "",
  isScrolled: false,
});

const router = useRouter();
const route = useRoute();
const colorMode = useColorMode();
const { locale } = useI18n();

const photoCountLabel = computed(() => {
  switch (locale.value) {
    case 'zh-Hant-TW':
    case 'zh-Hant-HK':
      return '張作品';
    case 'en':
      return ' works';
    case 'ja':
      return '作品';
    case 'ru':
      return ' работ';
    default:
      return '张作品';
  }
});

const isDark = computed({
  get: () => colorMode.value === "dark",
  set: (value) => {
    colorMode.preference = value ? "dark" : "light";
  },
});

const defaultFooterLinkUrl = "https://github.com/LiuShen-Fork/Memora";
const footerLinkText = computed(() => {
  const value = getSetting("app:footerLinkText");
  return String(value || "").trim() || "Memora";
});
const footerLinkUrl = computed(() => {
  const value = String(getSetting("app:footerLinkUrl") || "").trim();
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

const openLogin = () => router.push("/signin");
</script>

<template>
  <header class="masonry-topbar">
    <div class="masonry-topbar__inner">
      <div class="masonry-topbar__identity">
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
            <span
              v-if="getSetting('app:author')"
              class="masonry-topbar__author"
            >
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

      <AuthState>
        <template #default="{ loggedIn, clear }">
          <div class="masonry-topbar__actions">
            <div class="masonry-topbar__group masonry-topbar__functional-group">
              <UTooltip :text="$t('ui.action.theme.tooltip')">
                <UButton
                  variant="ghost"
                  color="neutral"
                  class="masonry-topbar__button"
                  :icon="isDark ? 'tabler:sun' : 'tabler:moon'"
                  size="sm"
                  :aria-label="$t('ui.action.theme.tooltip')"
                  @click="isDark = !isDark"
                />
              </UTooltip>
              <LanguageSwitcher compact button-class="masonry-topbar__button" />
              <MasonryItemTopbarSortMenu />
              <MasonryItemTopbarFilterMenu />
            </div>

            <div class="masonry-topbar__compact-functions masonry-topbar__group">
              <UPopover>
              <UButton
                variant="ghost"
                color="neutral"
                class="masonry-topbar__button"
                icon="tabler:dots-vertical"
                size="sm"
                aria-label="More actions"
              />
              <template #content>
                <UCard
                  variant="glassmorphism"
                  class="max-w-[calc(100vw-1rem)]"
                  :ui="{ body: 'p-1 sm:p-1' }"
                >
                  <div class="masonry-topbar__compact-function-row">
                    <UButton
                      variant="ghost"
                      color="neutral"
                      class="masonry-topbar__button"
                      :icon="isDark ? 'tabler:sun' : 'tabler:moon'"
                      size="sm"
                      :aria-label="$t('ui.action.theme.tooltip')"
                      @click="isDark = !isDark"
                    />
                    <LanguageSwitcher compact button-class="masonry-topbar__button" :tooltip="false" />
                    <MasonryItemTopbarSortMenu />
                    <MasonryItemTopbarFilterMenu />
                  </div>
                </UCard>
              </template>
              </UPopover>
            </div>

            <div class="masonry-topbar__group masonry-topbar__types">
              <UTooltip :text="$t('ui.action.globe.tooltip')">
                <UButton
                  variant="ghost"
                  color="neutral"
                  class="masonry-topbar__button"
                  icon="tabler:map-pin-2"
                  size="sm"
                  :aria-label="$t('ui.action.globe.tooltip')"
                  to="/globe"
                />
              </UTooltip>
              <UTooltip :text="$t('title.albums')">
                <UButton
                  variant="ghost"
                  color="neutral"
                  class="masonry-topbar__button"
                  icon="tabler:photo"
                  size="sm"
                  :aria-label="$t('title.albums')"
                  to="/albums"
                />
              </UTooltip>
            </div>

            <div class="masonry-topbar__group">
              <template v-if="loggedIn">
                <UTooltip :text="$t('ui.action.dashboard.tooltip')">
                  <UButton
                    variant="ghost"
                    color="neutral"
                    class="masonry-topbar__button"
                    icon="tabler:dashboard"
                    size="sm"
                    :aria-label="$t('ui.action.dashboard.tooltip')"
                    to="/dashboard"
                  />
                </UTooltip>
                <UTooltip :text="$t('ui.action.logout.tooltip')">
                  <UButton
                    variant="ghost"
                    color="neutral"
                    class="masonry-topbar__button"
                    icon="tabler:logout"
                    size="sm"
                    :aria-label="$t('ui.action.logout.tooltip')"
                    @click="clear"
                  />
                </UTooltip>
              </template>
              <UTooltip v-else :text="$t('auth.form.signin.title')">
                <UButton
                  variant="ghost"
                  color="neutral"
                  class="masonry-topbar__button"
                  icon="tabler:login"
                  size="sm"
                  :aria-label="$t('auth.form.signin.title')"
                  @click="openLogin"
                />
              </UTooltip>
            </div>
          </div>
        </template>
      </AuthState>
    </div>
  </header>
  <a
    v-if="route.path === '/'"
    :href="footerLinkUrl"
    target="_blank"
    rel="noopener noreferrer"
    class="masonry-page-footer"
  >
    <Icon name="tabler:brand-github" class="size-3.5" />
    <span class="max-w-36 truncate">{{ footerLinkText }}</span>
  </a>
</template>

<style scoped>
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

.masonry-topbar__actions {
  display: flex;
  flex: 1 1 0%;
  justify-content: flex-end;
  gap: 0.35rem;
  min-width: 0;
  pointer-events: auto;
}

.masonry-topbar__group {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.1rem;
  padding: 0.12rem;
  border: 1px solid rgb(255 255 255 / 35%);
  border-radius: 0.55rem;
  background: rgb(255 255 255 / 62%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 5%);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
}

.dark .masonry-topbar__group {
  border-color: rgb(255 255 255 / 10%);
  background: rgb(24 24 27 / 55%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 18%);
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

:deep(.masonry-topbar__button) {
  width: 1.8rem;
  height: 1.8rem;
  flex: 0 0 auto;
  cursor: pointer;
  justify-content: center;
  border-radius: 0.4rem;
  background: transparent !important;
  padding: 0 !important;
  transition: background-color 150ms ease, color 150ms ease;
}

:deep(.masonry-topbar__button:hover),
:deep(.masonry-topbar__button:focus-visible),
:deep(.masonry-topbar__button[data-state='open']) {
  background: rgb(0 0 0 / 8%) !important;
}

.dark :deep(.masonry-topbar__button:hover),
.dark :deep(.masonry-topbar__button:focus-visible),
.dark :deep(.masonry-topbar__button[data-state='open']) {
  background: rgb(255 255 255 / 12%) !important;
}

.masonry-topbar__compact-functions {
  display: none;
}

.masonry-topbar__compact-function-row {
  display: flex;
  align-items: center;
  gap: 0.15rem;
  white-space: nowrap;
}

.masonry-page-footer {
  position: fixed;
  z-index: 35;
  left: 0.75rem;
  bottom: 0.75rem;
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  max-width: 10rem;
  padding: 0.35rem 0.6rem;
  border: 1px solid rgb(255 255 255 / 30%);
  border-radius: 999px;
  background: rgb(255 255 255 / 48%);
  color: rgb(63 63 70 / 92%);
  font-size: 0.68rem;
  font-weight: 500;
  box-shadow: 0 5px 18px rgb(0 0 0 / 6%);
  pointer-events: auto;
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
}

.dark .masonry-page-footer {
  border-color: rgb(255 255 255 / 10%);
  background: rgb(24 24 27 / 58%);
  color: rgb(228 228 231 / 82%);
}

@media (max-width: 900px) {
  .masonry-topbar__status {
    display: none;
  }
}

@media (max-width: 768px) {
  .masonry-topbar {
    padding: 0.2rem 0.5rem;
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

  .masonry-topbar__actions {
    max-width: 58%;
    gap: 0.2rem;
    overflow-x: auto;
    padding-bottom: 1px;
    scrollbar-width: none;
  }

  .masonry-topbar__actions::-webkit-scrollbar {
    display: none;
  }

  .masonry-topbar__group {
    padding: 0.12rem;
    border-radius: 0.5rem;
  }

  :deep(.masonry-topbar__button) {
    width: 1.65rem;
    height: 1.65rem;
  }

  .masonry-page-footer {
    left: 0.5rem;
    bottom: 0.5rem;
  }
}

@media (max-width: 640px) {
  .masonry-topbar__author {
    display: none;
  }

  .masonry-topbar__functional-group {
    display: none;
  }

  .masonry-topbar__compact-functions {
    display: inline-flex;
  }

  .masonry-topbar__actions {
    max-width: 62%;
  }
}
</style>
