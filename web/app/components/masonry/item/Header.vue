<script lang="ts" setup>
interface Props {
  stats?: {
    total: number
  }
  dateRangeText: string
  locations?: string
  isScrolled?: boolean
}

withDefaults(defineProps<Props>(), {
  stats: () => ({ total: 0 }),
  locations: '',
  isScrolled: false,
})

const router = useRouter()
const route = useRoute()
const colorMode = useColorMode()
const { hasActiveFilters, selectedCounts } = usePhotoFilters()
const {
  currentSortLabel,
  currentSortIcon,
  currentSortOption,
  availableSorts,
  setSortOption,
} = usePhotoSort()

const isDark = computed({
  get: () => colorMode.value === 'dark',
  set: (value) => {
    colorMode.preference = value ? 'dark' : 'light'
  },
})

const totalSelectedFilters = computed(() =>
  Object.values(selectedCounts.value).reduce((total, count) => total + count, 0),
)

const defaultFooterLinkUrl = 'https://github.com/LiuShen-Fork/Memora'
const footerLinkText = computed(() => {
  const value = getSetting('app:footerLinkText')
  return String(value || '').trim() || 'Memora'
})
const footerLinkUrl = computed(() => {
  const value = String(getSetting('app:footerLinkUrl') || '').trim()
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

const openLogin = () => router.push('/signin')
</script>

<template>
  <header class="masonry-topbar">
    <div class="masonry-topbar__inner">
      <div class="masonry-topbar__identity">
        <img
          :src="
            (getSetting('app:avatarUrl') as string) ||
            '/web-app-manifest-192x192.png'
          "
          class="size-9 shrink-0 rounded-full object-cover ring-1 ring-black/10 dark:ring-white/15"
          :alt="$t('ui.photo.avatarAlt')"
        />
        <div class="min-w-0 leading-tight">
          <h1 class="truncate text-sm font-semibold text-neutral-900 dark:text-white">
            {{ getSetting('app:title') }}
          </h1>
          <p class="truncate text-[11px] text-neutral-500 dark:text-white/55">
            {{ stats.total }} {{ $t('title.photos') }}
            <span v-if="getSetting('app:author')">
              · @{{ getSetting('app:author') }}
            </span>
          </p>
        </div>
      </div>

      <div class="masonry-topbar__status" :class="{ 'is-scrolled': isScrolled }">
        <Transition
          name="topbar-status"
          mode="out-in"
        >
          <div
            v-if="isScrolled"
            key="context"
            class="masonry-topbar__context"
          >
            <span class="masonry-topbar__date">{{ dateRangeText }}</span>
            <span
              v-if="locations"
              class="masonry-topbar__location"
            >
              {{ locations }}
            </span>
          </div>
          <span
            v-else-if="getSetting('app:slogan')"
            key="slogan"
            class="masonry-topbar__slogan"
          >
            {{ getSetting('app:slogan') }}
          </span>
        </Transition>
      </div>

      <AuthState>
        <template #default="{ loggedIn, clear }">
          <div class="masonry-topbar__actions">
            <div class="masonry-topbar__group">
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
              <UPopover>
                <UTooltip :text="$t('ui.action.filter.tooltip')">
                  <UChip
                    inset
                    size="sm"
                    color="info"
                    :show="totalSelectedFilters > 0"
                  >
                    <UButton
                      variant="ghost"
                      :color="hasActiveFilters ? 'info' : 'neutral'"
                      class="masonry-topbar__button"
                      icon="tabler:filter"
                      size="sm"
                      :aria-label="$t('ui.action.filter.tooltip')"
                    />
                  </UChip>
                </UTooltip>
                <template #content>
                  <UCard variant="glassmorphism">
                    <OverlayFilterPanel />
                  </UCard>
                </template>
              </UPopover>

              <UPopover>
                <UTooltip :text="$t('ui.action.sort.tooltip')">
                  <UButton
                    variant="ghost"
                    :color="
                      currentSortOption?.key === 'dateTaken-desc'
                        ? 'neutral'
                        : 'info'
                    "
                    class="masonry-topbar__button"
                    :icon="currentSortIcon"
                    size="sm"
                    :aria-label="$t('ui.action.sort.tooltip')"
                  />
                </UTooltip>
                <template #content>
                  <UCard
                    variant="glassmorphism"
                    class="w-3xs"
                  >
                    <template #header>
                      <h3 class="p-1 text-sm font-bold">
                        {{ $t('ui.action.sort.title') }}
                      </h3>
                    </template>
                    <div class="space-y-1">
                      <UButton
                        v-for="sort in availableSorts"
                        :key="sort.key"
                        :variant="
                          currentSortLabel === sort.labelI18n ? 'soft' : 'ghost'
                        "
                        :color="
                          currentSortLabel === sort.labelI18n ? 'info' : 'neutral'
                        "
                        :icon="sort.icon"
                        size="sm"
                        block
                        class="justify-start"
                        @click="setSortOption(sort.key)"
                      >
                        {{ $t(sort.labelI18n) }}
                      </UButton>
                    </div>
                  </UCard>
                </template>
              </UPopover>
            </div>

            <div class="masonry-topbar__group">
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
              <LanguageSwitcher compact />
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
              <UTooltip
                v-else
                :text="$t('auth.form.signin.title')"
              >
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
  padding: 0.5rem 1rem 1rem;
  -webkit-backdrop-filter: blur(18px);
  backdrop-filter: blur(18px);
  mask-image: linear-gradient(to bottom, black 0%, black 58%, transparent 100%);
  -webkit-mask-image: linear-gradient(
    to bottom,
    black 0%,
    black 58%,
    transparent 100%
  );
}

.masonry-topbar__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  min-height: 3rem;
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
  top: 1.75rem;
  left: 50%;
  display: flex;
  max-width: 28%;
  transform: translate(-50%, -50%);
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 0.1rem;
  overflow: hidden;
  text-align: center;
  pointer-events: none;
}

.masonry-topbar__status.is-scrolled {
  top: 1.75rem;
}

.masonry-topbar__context {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 0.1rem;
}

.topbar-status-enter-active,
.topbar-status-leave-active {
  transition: opacity 180ms ease, transform 220ms ease;
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
  color: rgb(82 82 91 / 75%);
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
  color: rgb(82 82 91 / 62%);
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
  color: rgb(212 212 216 / 58%);
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
  padding: 0.2rem;
  border: 1px solid rgb(255 255 255 / 35%);
  border-radius: 999px;
  background: rgb(255 255 255 / 48%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 5%);
  -webkit-backdrop-filter: blur(14px);
  backdrop-filter: blur(14px);
}

.dark .masonry-topbar__group {
  border-color: rgb(255 255 255 / 10%);
  background: rgb(24 24 27 / 55%);
  box-shadow: 0 5px 18px rgb(0 0 0 / 18%);
}

.masonry-topbar__button {
  width: 2rem;
  height: 2rem;
  flex: 0 0 auto;
  cursor: pointer;
  justify-content: center;
  border-radius: 999px;
  background: transparent;
  padding: 0;
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
  color: rgb(82 82 91 / 78%);
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
  color: rgb(228 228 231 / 72%);
}

@media (max-width: 768px) {
  .masonry-topbar {
    padding: 0.35rem 0.5rem 0.9rem;
  }

  .masonry-topbar__inner {
    align-items: flex-start;
    gap: 0.4rem;
  }

  .masonry-topbar__identity {
    gap: 0.4rem;
  }

  .masonry-topbar__identity img {
    width: 2rem;
    height: 2rem;
  }

  .masonry-topbar__status {
    top: 1.45rem;
    max-width: 26%;
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
  }

  .masonry-topbar__button {
    width: 1.75rem;
    height: 1.75rem;
  }

  .masonry-page-footer {
    left: 0.5rem;
    bottom: 0.5rem;
  }
}
</style>
