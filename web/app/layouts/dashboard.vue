<script lang="ts" setup>
import type { NavigationMenuItem } from '@nuxt/ui'

const route = useRoute()
const router = useRouter()
const { loggedIn, user } = useUserSession()
const settingsStore = useSettingsStore()

const appTitle = computed(() => {
  const value = settingsStore.getSetting('app:title')
  return value ? String(value) : $t('title.dashboard')
})

const navItems = computed<NavigationMenuItem[]>(() => [
  {
    label: $t('title.dashboard'),
    icon: 'tabler:dashboard',
    to: '/dashboard',
  },
  {
    label: $t('title.photos'),
    icon: 'tabler:photo-cog',
    to: '/dashboard/photos',
  },
  {
    label: $t('title.albums'),
    icon: 'tabler:album',
    to: '/dashboard/albums',
  },
  {
    label: $t('title.queue'),
    icon: 'tabler:list-check',
    to: '/dashboard/queue',
  },
  {
    label: $t('title.logs'),
    icon: 'tabler:file-text',
    to: '/dashboard/logs',
  },
  {
    label: $t('title.siteAdministration'),
    icon: 'tabler:settings',
    defaultOpen: route.path.startsWith('/dashboard/settings'),
    children: [
      {
        label: $t('title.generalSettings'),
        icon: 'tabler:settings-2',
        to: '/dashboard/settings/general',
      },
      {
        label: $t('title.storageSettings'),
        icon: 'tabler:database',
        to: '/dashboard/settings/storage',
      },
      {
        label: $t('title.privacySettings'),
        icon: 'tabler:shield-lock',
        to: '/dashboard/settings/privacy',
      },
      {
        label: $t('title.mapAndLocation'),
        icon: 'tabler:map-pin',
        to: '/dashboard/settings/map',
      },
      {
        label: $t('title.systemSettings'),
        icon: 'tabler:cpu',
        to: '/dashboard/settings/system',
      },
      {
        label: $t('title.analyticsSettings'),
        icon: 'tabler:chart-bar',
        to: '/dashboard/settings/analytics',
      },
    ],
  },
])

useHead({
  title: () => $t('title.dashboard'),
  titleTemplate: (title) => `${title ? `${title} | ` : ''}${appTitle.value}`,
})

const handleLogin = () => {
  router.push({
    path: '/signin',
    query: { redirect: route.fullPath },
  })
}
</script>

<template>
  <!-- TODO: unified error page -->
  <div
    v-if="!loggedIn || !user?.isAdmin"
    class="h-svh flex flex-col gap-4 items-center justify-center px-4"
  >
    <Icon
      name="tabler:alert-triangle"
      class="size-12 text-primary"
    />
    <p class="text-gray-500 text-center">
      {{
        !user?.isAdmin
          ? $t('dashboard.access.pleaseLogin')
          : $t('dashboard.access.noAccess')
      }}
    </p>
    <UButton @click="handleLogin">{{ $t('auth.form.signin.title') }}</UButton>
  </div>
  <UDashboardGroup v-else>
    <UDashboardSidebar
      id="cframe-dashboard-sidebar"
      resizable
      collapsible
      mode="drawer"
      :min-size="8"
      :max-size="12"
      :ui="{ footer: 'border-t border-default' }"
      :toggle="{
        color: 'primary',
        variant: 'subtle',
        class: 'rounded-full',
      }"
    >
      <template #toggle>
        <UDashboardSidebarToggle variant="soft" />
      </template>

      <template #header="{ collapsed }">
        <div
          v-if="!collapsed"
          class="flex items-center gap-2"
        >
          <img
            src="/favicon.svg"
            class="h-8 w-auto shrink-0"
          />
          <div class="flex flex-col overflow-hidden">
            <NuxtLink
              to="/"
              class="text-lg font-medium line-clamp-1"
            >
              {{ appTitle }}
            </NuxtLink>
          </div>
        </div>
        <img
          v-else
          src="/favicon.svg"
          class="size-8 mx-auto"
        />
      </template>

      <template #default="{ collapsed }">
        <UNavigationMenu
          :collapsed="collapsed"
          :items="navItems"
          orientation="vertical"
        />
      </template>

      <template #footer="{ collapsed }">
        <div
          class="w-full gap-1"
          :class="collapsed ? 'flex flex-col' : 'grid grid-cols-3 items-center'"
        >
          <LanguageSwitcher
            compact
            block
          />
          <UButton
            :avatar="{
              src: user?.avatar || '',
              alt: user?.username || user?.email || 'User Avatar',
              icon: 'tabler:user',
            }"
            :label="collapsed ? undefined : user?.username || 'User'"
            size="sm"
            color="neutral"
            variant="ghost"
            class="h-9 w-full min-w-0 justify-start px-2"
            :class="collapsed ? 'justify-center' : ''"
          />
          <UButton
            icon="tabler:brand-github"
            :label="collapsed ? undefined : 'Memora'"
            to="https://github.com/LiuShen-Fork/Memora"
            target="_blank"
            rel="noopener noreferrer"
            size="sm"
            color="neutral"
            variant="ghost"
            class="h-9 w-full min-w-0 justify-start px-2"
            :class="collapsed ? 'justify-center' : ''"
          />
        </div>
      </template>
    </UDashboardSidebar>

    <NuxtPage />
  </UDashboardGroup>
</template>

<style scoped></style>
