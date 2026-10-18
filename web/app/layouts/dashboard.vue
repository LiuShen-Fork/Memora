<script lang="ts" setup>
import type { NavigationMenuItem } from '@nuxt/ui'

const route = useRoute()
const router = useRouter()
const { loggedIn, user } = useUserSession()
const settingsStore = useSettingsStore()
const colorMode = useColorMode()

const appTitle = computed(() => {
  const value = settingsStore.getSetting('app:title')
  return value ? String(value) : $t('title.dashboard')
})

useHead({
  title: () => $t('title.dashboard'),
  titleTemplate: (title) => `${title ? `${title} | ` : ''}${appTitle.value}`,
  // The dashboard's typeface, colours and surfaces are scoped to this class.
  // It goes on <body> rather than on the layout's own root because Nuxt UI
  // teleports popovers, dropdowns and toasts to the end of <body>: scoped to
  // a div, every menu would sit outside the scope and open in the old
  // palette, over a page that had already changed.
  bodyAttrs: { class: 'app-dashboard' },
})

const isDark = computed(() => colorMode.value === 'dark')
function toggleColorMode() {
  colorMode.preference = isDark.value ? 'light' : 'dark'
}

/**
 * True when the page on screen is this one.
 *
 * Matching by prefix alone marks every item current on every page, since all
 * of these routes begin with /dashboard. So the overview has to match itself
 * exactly, and the rest match themselves or a page below them.
 */
function isCurrent(to: string, exact: boolean) {
  return (
    route.path === to || (!exact && route.path.startsWith(`${to}/`))
  )
}

/**
 * The mark on the current item.
 *
 * It is drawn with ::after because the component uses ::before for the
 * background it fills an item with when it is current — taking the other
 * pseudo-element means the mark and the fill never argue over the same box.
 * It is only ever on one item, which is why it is set per item rather than on
 * the menu.
 */
const CURRENT =
  'text-highlighted after:absolute after:start-0 after:top-1/2 after:-translate-y-1/2 after:h-4 after:w-0.5 after:rounded-full after:bg-primary'

function entry(
  label: string,
  icon: string,
  to: string,
  exact = false,
): NavigationMenuItem {
  return {
    label,
    icon,
    to,
    ui: isCurrent(to, exact) ? { link: CURRENT } : undefined,
  }
}

function heading(key: string): NavigationMenuItem {
  return { type: 'label', label: $t(key) }
}

/**
 * Everything the dashboard can do, in one list.
 *
 * It used to be five loose items beside a "site settings" parent that folded
 * six children away behind a chevron, which is a lot of hierarchy for the
 * least-used corner of the app and almost none for the rest. The items are
 * the same; what changed is that the settings are now visible as themselves,
 * and the three groupings say what kind of thing each one is.
 */
function navItems(collapsed: boolean): NavigationMenuItem[] {
  const items: NavigationMenuItem[] = [
    entry($t('title.dashboard'), 'tabler:layout-dashboard', '/dashboard', true),
  ]

  const groups: [string, NavigationMenuItem[]][] = [
    [
      'dashboard.nav.content',
      [
        entry($t('title.photos'), 'tabler:photo', '/dashboard/photos'),
        entry($t('title.albums'), 'tabler:album', '/dashboard/albums'),
      ],
    ],
    [
      'dashboard.nav.operations',
      [
        entry($t('title.queue'), 'tabler:list-check', '/dashboard/queue'),
        entry($t('title.logs'), 'tabler:file-text', '/dashboard/logs'),
      ],
    ],
    [
      'dashboard.nav.settings',
      [
        entry($t('title.generalSettings'), 'tabler:adjustments-horizontal', '/dashboard/settings/general'),
        entry($t('title.storageSettings'), 'tabler:database', '/dashboard/settings/storage'),
        entry($t('title.privacySettings'), 'tabler:shield-lock', '/dashboard/settings/privacy'),
        entry($t('title.mapAndLocation'), 'tabler:map-pin', '/dashboard/settings/map'),
        entry($t('title.systemSettings'), 'tabler:cpu', '/dashboard/settings/system'),
        entry($t('title.analyticsSettings'), 'tabler:chart-histogram', '/dashboard/settings/analytics'),
      ],
    ],
  ]

  // Collapsed, there is no room for a heading and nothing for it to head:
  // four characters of Chinese do not fit beside an icon in 64px.
  for (const [key, children] of groups) {
    if (!collapsed) items.push(heading(key))
    items.push(...children)
  }

  return items
}

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
      :min-size="13"
      :max-size="20"
      :default-size="16"
      :ui="{
        header: 'px-3 gap-2',
        body: 'px-3 pt-1',
        footer: 'border-t border-default px-3 py-2.5',
      }"
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
        <NuxtLink
          to="/"
          class="flex min-w-0 items-center gap-2.5"
          :class="collapsed ? 'mx-auto' : ''"
        >
          <img
            src="/favicon.svg"
            class="size-7 shrink-0"
            alt=""
          />
          <span
            v-if="!collapsed"
            class="truncate text-[15px] font-medium tracking-tight text-highlighted"
          >
            {{ appTitle }}
          </span>
        </NuxtLink>
      </template>

      <template #default="{ collapsed }">
        <UNavigationMenu
          :collapsed="collapsed"
          :items="navItems(collapsed)"
          orientation="vertical"
          color="neutral"
          :highlight="false"
          :ui="{
            list: 'flex flex-col gap-0.5',
            link: 'text-toned',
            label: 'mt-4 mb-1 px-2.5 text-[11px] font-medium text-dimmed',
          }"
        />
      </template>

      <template #footer="{ collapsed }">
        <!-- 昵称和它那三个键并成一行：名字占满剩下的宽度，所以这一行不会
             留出一块空的；三个键各是一个圆，图标在正中间。竖成图标栏以后
             一行放不下四个，就叠成一列。 -->
        <div
          class="flex w-full items-center gap-1"
          :class="collapsed ? 'flex-col' : ''"
        >
          <UButton
            :avatar="{
              src: user?.avatar || '',
              alt: user?.username || user?.email || 'User Avatar',
              icon: 'tabler:user',
            }"
            :label="collapsed ? undefined : user?.username || 'User'"
            to="/dashboard/settings/general"
            size="sm"
            color="neutral"
            variant="ghost"
            :ui="{ label: 'truncate' }"
            class="h-8 shrink-0 rounded-full px-1.5"
            :class="
              collapsed
                ? 'w-8 justify-center'
                : 'min-w-0 flex-1 justify-start'
            "
          />

          <div
            class="flex items-center gap-1.5"
            :class="collapsed ? 'flex-col' : ''"
          >
            <LanguageSwitcher
              compact
              button-class="size-8 shrink-0 justify-center rounded-full p-0"
            />
            <UTooltip
              :text="$t('ui.action.theme.tooltip')"
              ignore-non-keyboard-focus
            >
              <UButton
                :icon="isDark ? 'tabler:sun' : 'tabler:moon'"
                color="neutral"
                variant="ghost"
                square
                class="size-8 shrink-0 rounded-full p-0"
                :aria-label="$t('ui.action.theme.tooltip')"
                @click="toggleColorMode"
              />
            </UTooltip>
            <UTooltip
              text="LiuShen-Fork/Memora"
              ignore-non-keyboard-focus
            >
              <UButton
                icon="tabler:brand-github"
                to="https://github.com/LiuShen-Fork/Memora"
                target="_blank"
                rel="noopener noreferrer"
                color="neutral"
                variant="ghost"
                square
                class="size-8 shrink-0 rounded-full p-0"
                aria-label="LiuShen-Fork/Memora"
              />
            </UTooltip>
          </div>
        </div>
      </template>
    </UDashboardSidebar>

    <NuxtPage />
  </UDashboardGroup>
</template>
