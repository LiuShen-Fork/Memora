<script setup lang="ts">
import type { Driver, DriveStep } from 'driver.js'
import { tourIds, type TourId } from '~~/shared/types/tours'

const route = useRoute()
const { user } = useUserSession()
const { records, ready, busy, revision, load, update } = useTours()
const settings = useSettingsStore()
const { t, locale } = useI18n()

let active: Driver | undefined
// Bumped whenever the guide on screen is no longer wanted. Every await in
// start() compares against it so a slow step cannot resurrect a stale guide.
let generation = 0

// Settings arrive after the first render. Reading them before that would both
// warn and misjudge whether the site is still being set up.
const settingsReady = computed(() => settings.isReady)

const current = computed<TourId | undefined>(() => {
  if (route.path === '/') return 'home'
  if (route.path === '/dashboard') return 'dashboard'
  const name = route.path
    .replace('/dashboard/settings/', '')
    .replace('/dashboard/', '')
  return route.path.startsWith('/dashboard/') &&
    tourIds.includes(name as TourId)
    ? (name as TourId)
    : undefined
})

function stop() {
  active?.destroy()
  active = undefined
}

async function start() {
  const token = ++generation
  stop()
  if (
    !user.value?.isAdmin ||
    !settingsReady.value ||
    settings.getSetting('system:firstLaunch') ||
    !ready.value ||
    busy.value
  )
    return
  const id = current.value
  if (!id || records.value[id] !== 'pending') return
  await nextTick()
  // The guide is anchored to controls that appear with the page, so wait for a
  // painted frame before measuring them.
  await new Promise((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(resolve)),
  )
  if (token !== generation) return

  const field = (key: string) => `[data-tour-setting="${key}"]`
  const plans: Record<TourId, [string, string?][]> = {
    home: [
      ['home'],
      ['homeAvatar', '[data-tour="home-avatar"]'],
      ['homeDashboard', '[data-tour="home-dashboard"]'],
    ],
    dashboard: [
      ['dashboard', '[data-tour="dashboard-photos"]'],
      ['queueDetails', '[data-tour="dashboard-queue"]'],
      ['shareSite', '[data-tour="dashboard-share"]'],
    ],
    photos: [
      ['photoUpload', '[data-tour="photo-upload"]'],
      ['queueDetails', '[data-tour="photo-queue"]'],
      ['photoFilter', '[data-tour="photo-filter"]'],
    ],
    albums: [
      ['albumCreate', '[data-tour="album-create"]'],
      ['albumActions', '[data-tour="album-actions"]'],
    ],
    general: [
      ['avatar', field('app:avatarUrl')],
      ['general', field('app:title')],
      ['appearance', field('app:appearance.theme')],
      ['saveSettings', 'button[form="appearanceSettingsForm"][type="submit"]'],
    ],
    storage: [
      ['storage', '[data-tour="storage-default"]'],
      ['storageAdd', '[data-tour="storage-add"]'],
    ],
    map: [
      ['map', field('map:provider')],
      [
        'mapStyle',
        `${field('map:mapbox.style')}, ${field('map:maplibre.style')}`,
      ],
      [
        'mapToken',
        `${field('map:mapbox.token')}, ${field('map:maplibre.token')}`,
      ],
      ['location', field('location:language')],
      ['locationService', field('location:nominatim.baseUrl')],
      ['saveSettings', 'button[form="mapSettingsForm"][type="submit"]'],
    ],
    privacy: [
      ['privacy', field('privacy:upload.autoEraseLocation')],
      ['saveSettings', 'button[form="privacySettingsForm"][type="submit"]'],
    ],
    system: [
      ['githubLogin', field('system:auth.github.enabled')],
      ['uploadLimit', field('system:upload.maxFileSize')],
      ['duplicateCheck', field('system:upload.duplicateCheck.enabled')],
      ['resetTours', '[data-tour="reset"]'],
    ],
  }

  const visibleTarget = (selector: string) =>
    Array.from(document.querySelectorAll<HTMLElement>(selector)).find(
      (element) => element.getClientRects().length > 0,
    )
  // Settings arrive asynchronously. Wait for the first real control rather than
  // the skeleton that stands in for it.
  const firstSelector = plans[id][0]?.[1]
  if (firstSelector && !visibleTarget(firstSelector)) {
    await new Promise<void>((resolve) => {
      const observer = new MutationObserver(() => {
        if (visibleTarget(firstSelector)) finishWait()
      })
      const timeout = window.setTimeout(finishWait, 5000)
      function finishWait() {
        observer.disconnect()
        clearTimeout(timeout)
        resolve()
      }
      observer.observe(document.body, {
        childList: true,
        subtree: true,
        attributes: true,
      })
    })
  }
  if (token !== generation) return

  // A step whose anchor is missing is dropped rather than shown pointing at
  // nothing.
  const steps: DriveStep[] = plans[id].flatMap(([key, selector]) => {
    const target = selector ? visibleTarget(selector) : undefined
    if (selector && !target) return []
    return [
      {
        element: target,
        popover: {
          title: t(`tours.${key}.title`),
          description: t(`tours.${key}.description`),
          popoverClass:
            key === 'home' ? 'memora-tour memora-tour-welcome' : 'memora-tour',
        },
      },
    ]
  })
  if (!steps.length) return

  const [{ driver }] = await Promise.all([
    import('driver.js'),
    import('driver.js/dist/driver.css'),
  ])
  if (token !== generation) return

  let saving = false
  async function finish(status: 'completed' | 'skipped') {
    if (saving) return
    saving = true
    // Dismiss first so a slow status request cannot keep the overlay up.
    stop()
    await update(id as TourId, status)
  }

  active = driver({
    steps,
    showProgress: true,
    showButtons: ['next', 'previous', 'close'],
    allowClose: true,
    overlayOpacity: 0.45,
    stagePadding: 6,
    stageRadius: 10,
    popoverOffset: 12,
    overlayClickBehavior: 'none',
    closeBtnLabel: t('tours.skip'),
    onCloseClick: () => {
      void finish('skipped')
    },
    onDestroyStarted: () => {
      void finish('skipped')
    },
    disableActiveInteraction: true,
    animate: !window.matchMedia('(prefers-reduced-motion: reduce)').matches,
    nextBtnText: t('tours.next'),
    prevBtnText: t('tours.previous'),
    doneBtnText: t('tours.done'),
    progressText: '{{current}} / {{total}}',
    onDoneClick: () => {
      void finish('completed')
    },
    onNextClick: () => {
      if (active?.hasNextStep()) active.moveNext()
      else void finish('completed')
    },
    onPopoverRender: (popover) => {
      popover.closeButton.textContent = t('tours.skip')
      popover.closeButton.classList.add('memora-tour-skip')
      popover.footerButtons.prepend(popover.closeButton)
    },
  })
  active.drive()
}

watch(
  () => user.value?.isAdmin,
  async (admin) => {
    ++generation
    stop()
    ready.value = false
    if (admin) {
      await load()
      void start()
    }
  },
  { immediate: true },
)
watch(
  [revision, locale, settingsReady],
  () => {
    void start()
  },
  { flush: 'post' },
)
watch(
  () => route.path,
  async () => {
    ++generation
    stop()
    if (user.value?.isAdmin) {
      await load()
      void start()
    }
  },
  { flush: 'post' },
)
const removePageFinish = useNuxtApp().hook('page:finish', () => {
  void start()
})
let revisionBeforeUpdate = revision.value
watch(
  busy,
  (value) => {
    if (value) revisionBeforeUpdate = revision.value
    else if (revision.value !== revisionBeforeUpdate) void start()
  },
  { flush: 'post' },
)
onUnmounted(() => {
  removePageFinish()
  ++generation
  stop()
})
</script>

<template><span hidden /></template>

<style>
.driver-overlay {
  z-index: 10050 !important;
}

.driver-popover.memora-tour {
  --tour-surface: var(--ui-bg);
  z-index: 10051 !important;
  min-width: min(300px, calc(100vw - 32px));
  max-width: min(360px, calc(100vw - 32px));
  padding: 16px;
  border: 1px solid var(--ui-border);
  border-radius: 16px;
  background: var(--tour-surface);
  color: var(--ui-text);
  font-family: var(--font-sans);
  box-shadow:
    0 16px 48px rgb(0 0 0 / 16%),
    0 2px 8px rgb(0 0 0 / 6%);
}

.memora-tour .driver-popover-title {
  color: var(--ui-text-highlighted);
  font-size: 16px;
  font-weight: 600;
  line-height: 1.5;
  letter-spacing: -0.015em;
}

.memora-tour
  .driver-popover-title[style*='block']
  + .driver-popover-description {
  margin-top: 8px;
}

.memora-tour .driver-popover-description {
  color: var(--ui-text-toned);
  font-size: 13px;
  line-height: 1.75;
  overflow-wrap: anywhere;
}

.memora-tour .driver-popover-footer {
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid var(--ui-border);
}

.memora-tour .driver-popover-progress-text {
  flex-shrink: 0;
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--ui-bg-muted);
  color: var(--ui-text-muted);
  font-size: 11px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.memora-tour .driver-popover-navigation-btns {
  gap: 6px;
}

.memora-tour .driver-popover-navigation-btns button + button {
  margin-left: 0;
}

.memora-tour .driver-popover-footer-btn,
.memora-tour .driver-popover-close-btn.memora-tour-skip {
  box-sizing: border-box;
  position: static;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: auto;
  height: auto;
  min-height: 32px;
  padding: 6px 10px;
  border: 1px solid transparent;
  border-radius: 8px;
  color: var(--ui-text);
  background: var(--ui-bg-elevated);
  font-family: inherit;
  font-size: 12px;
  font-weight: 500;
  line-height: 1.4;
  cursor: pointer;
  transition:
    background-color 150ms,
    box-shadow 150ms;
}

.memora-tour .driver-popover-close-btn.memora-tour-skip {
  color: var(--ui-text-muted);
  background: transparent;
}

.memora-tour .driver-popover-footer-btn:hover,
.memora-tour .driver-popover-close-btn.memora-tour-skip:hover {
  color: var(--ui-text-highlighted);
  background: var(--ui-bg-accented);
}

.memora-tour .driver-popover-next-btn {
  color: var(--ui-text-inverted);
  background: var(--ui-primary);
  box-shadow: 0 1px 2px rgb(0 0 0 / 8%);
}

.memora-tour .driver-popover-next-btn:hover {
  color: var(--ui-text-inverted);
  background: color-mix(
    in srgb,
    var(--ui-primary) 88%,
    var(--ui-text-highlighted)
  );
}

.memora-tour button:focus-visible {
  outline: 2px solid var(--ui-primary);
  outline-offset: 3px;
}

.memora-tour .driver-popover-footer .driver-popover-btn-disabled {
  opacity: 0.4;
}

.memora-tour .driver-popover-arrow-side-left {
  border-left-color: var(--tour-surface);
}

.memora-tour .driver-popover-arrow-side-right {
  border-right-color: var(--tour-surface);
}

.memora-tour .driver-popover-arrow-side-top {
  border-top-color: var(--tour-surface);
}

.memora-tour .driver-popover-arrow-side-bottom {
  border-bottom-color: var(--tour-surface);
}

.driver-popover.memora-tour-welcome {
  max-width: min(480px, calc(100vw - 32px));
  padding: 20px;
}

.memora-tour-welcome .driver-popover-title {
  font-size: 22px;
}

.memora-tour-welcome .driver-popover-description {
  font-size: 14px;
  line-height: 1.8;
}

@media (prefers-reduced-motion: reduce) {
  .memora-tour button {
    transition: none;
  }
}
</style>
