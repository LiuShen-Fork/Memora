<script lang="ts" setup>

defineProps<{
  stats?: {
    total: number
    dateRange: {
      start: string | undefined
      end: string | undefined
    } | null
  }
  dateRangeText: string
}>()

const router = useRouter()
// const config = useRuntimeConfig()
const colorMode = useColorMode()

const isDark = computed({
  get() {
    return colorMode.value === 'dark'
  },
  set(_isDark) {
    colorMode.preference = _isDark ? 'dark' : 'light'
  },
})

const handleOpenLogin = () => {
  router.push('/signin')
}

const { hasActiveFilters, selectedCounts } = usePhotoFilters()

const {
  currentSortLabel,
  currentSortIcon,
  currentSortOption,
  availableSorts,
  setSortOption,
} = usePhotoSort()

const totalSelectedFilters = computed(() => {
  return Object.values(selectedCounts.value).reduce(
    (total, count) => total + count,
    0,
  )
})

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
    // Empty or invalid custom URLs use the project repository.
  }
  return defaultFooterLinkUrl
})
</script>

<template>
  <div class="w-full relative overflow-hidden bg-white dark:bg-neutral-800">
    <div class="relative flex flex-col items-center py-6 pb-0 gap-2">
      <div class="absolute top-3 left-3 z-10">
        <LanguageSwitcher compact />
      </div>
      <AuthState>
        <template #default="{ loggedIn, clear }">
          <div class="flex flex-col items-center gap-2">
            <div class="relative mx-auto">
              <div
                v-if="loggedIn"
                class="absolute -bottom-0.5 -right-0.5 bg-amber-500 text-white rounded-full flex items-center justify-center size-5 text-xs drop-shadow-lg drop-shadow-amber-500/30"
              >
                <Icon name="tabler:star-filled" />
              </div>
              <img
                :src="
                  (getSetting('app:avatarUrl') as string) ||
                  '/web-app-manifest-192x192.png'
                "
                class="size-16 rounded-full object-cover"
                :class="!loggedIn && 'cursor-pointer'"
                :alt="$t('ui.photo.avatarAlt')"
                @click="!loggedIn && handleOpenLogin()"
              />
            </div>
            <h1
              class="text-2xl font-bold text-neutral-900 dark:text-white/90 mb-2"
            >
              {{ getSetting('app:title') }}
            </h1>
          </div>
          <div
            class="text-neutral-600 dark:text-white/30 space-y-1 text-center"
          >
            <p
              v-if="stats?.total"
              class="text-xs font-medium"
            >
              {{
                $t('ui.stats.totalPhotosWithRange', {
                  range: dateRangeText,
                  count: stats?.total,
                })
              }}
            </p>
            <p
              v-else
              class="text-xs font-medium"
            >
              {{ $t('ui.stats.noPhotosTip') }}
            </p>
            <p
              v-if="getSetting('app:slogan')"
              class="font-[Pacifico]"
            >
              {{ getSetting('app:slogan') }}
            </p>
          </div>
          <div
            class="flex items-center gap-0 p-1 bg-neutral-100 dark:bg-neutral-700 rounded-full"
          >
            <UTooltip :text="$t('ui.action.globe.tooltip')">
              <UButton
                variant="soft"
                color="neutral"
                class="bg-transparent rounded-full cursor-pointer"
                icon="tabler:map-pin-2"
                size="sm"
                to="/globe"
              />
            </UTooltip>
            <UTooltip :text="$t('title.albums')">
              <UButton
                variant="soft"
                color="neutral"
                class="bg-transparent rounded-full cursor-pointer"
                icon="tabler:photo"
                size="sm"
                to="/albums"
              />
            </UTooltip>
            <UPopover>
              <UTooltip :text="$t('ui.action.filter.tooltip')">
                <UChip
                  inset
                  size="sm"
                  color="info"
                  :show="totalSelectedFilters > 0"
                >
                  <UButton
                    variant="soft"
                    :color="hasActiveFilters ? 'info' : 'neutral'"
                    class="bg-transparent rounded-full cursor-pointer relative"
                    icon="tabler:filter"
                    size="sm"
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
                  variant="soft"
                  :color="
                    currentSortOption?.key === 'dateTaken-desc'
                      ? 'neutral'
                      : 'info'
                  "
                  class="bg-transparent rounded-full cursor-pointer"
                  :icon="currentSortIcon"
                  size="sm"
                />
              </UTooltip>

              <template #content>
                <UCard
                  variant="glassmorphism"
                  class="w-3xs"
                >
                  <template #header>
                    <h3 class="font-bold text-sm p-1">
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
            <UTooltip :text="$t('ui.action.theme.tooltip')">
              <UButton
                variant="soft"
                color="neutral"
                class="bg-transparent rounded-full cursor-pointer"
                :icon="isDark ? 'tabler:sun' : 'tabler:moon'"
                size="sm"
                @click="isDark = !isDark"
              />
            </UTooltip>
            <UTooltip
              v-if="loggedIn"
              :text="$t('ui.action.dashboard.tooltip')"
            >
              <UButton
                size="sm"
                color="neutral"
                variant="soft"
                class="size-8 bg-transparent rounded-full cursor-pointer p-0 justify-center"
                icon="tabler:dashboard"
                to="/dashboard"
              />
            </UTooltip>
            <UTooltip
              v-if="loggedIn"
              :text="$t('ui.action.logout.tooltip')"
            >
              <UButton
                size="sm"
                color="neutral"
                variant="soft"
                class="size-8 bg-transparent rounded-full cursor-pointer p-0 justify-center"
                icon="tabler:logout"
                @click="clear"
            /></UTooltip>
          </div>
        </template>
      </AuthState>
      <div
        class="w-full px-2 pb-1 pt-1.5 bg-neutral-100 dark:bg-neutral-800 flex justify-between items-center gap-2"
      >
        <div
          v-if="getSetting('app:author') || getSetting('app:title')"
          class="text-xs text-neutral-500/80 dark:text-neutral-500 font-medium truncate"
        >
          © {{ $dayjs().format('YYYY') }}
          {{ getSetting('app:author') || getSetting('app:title') }}
        </div>
        <a
          :href="footerLinkUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="max-w-[50%] truncate text-xs text-neutral-500/60 dark:text-neutral-500/80 font-medium hover:underline"
        >
          {{ footerLinkText }}
        </a>
      </div>
    </div>
  </div>
</template>

<style scoped></style>
