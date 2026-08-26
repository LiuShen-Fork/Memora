import { useSettingsStore } from '~/stores/settings'

export default defineNuxtRouteMiddleware(async (to, from) => {
  const settingsStore = useSettingsStore()
  const isInitialNavigation = !from.name

  // The initial navigation must wait so the onboarding redirect is reliable.
  // Subsequent navigations should not keep the previous page on screen while
  // the shared settings request is still in flight.
  if (!settingsStore.isReady) {
    if (isInitialNavigation) {
      try {
        await settingsStore.initSettings()
      } catch (e) {
        console.error('Failed to load settings in middleware', e)
      }
    } else {
      void settingsStore.initSettings().catch((e) => {
        console.error('Failed to load settings in middleware', e)
      })
    }
  }

  // A client-side navigation can continue while settings are loading. There
  // is no safe first-launch decision to make until the initial request ends.
  if (!settingsStore.isReady) {
    return
  }

  const isFirstLaunch = settingsStore.getSetting('system:firstLaunch')
  const isOnboarding = to.path.startsWith('/onboarding')

  if (isFirstLaunch === true) {
    if (!isOnboarding) {
      return navigateTo('/onboarding')
    }
  } else {
    if (isOnboarding) {
      // ignore
    }
  }
})
