export default defineAppConfig({
  ui: {
    colors: {
      primary: 'pink',
      secondary: 'purple',
      neutral: 'neutral',
      info: 'sky',
      success: 'emerald',
      warning: 'amber',
      danger: 'rose',
    },
    // Nuxt UI's own components ask for lucide icons unless told otherwise, and
    // the generated frontend is served as static files with no icon service
    // behind it: an icon the build did not bundle is drawn as empty space, so
    // a select drew no chevron and a toast drew no close button. The app draws
    // itself in tabler, so every default is answered in tabler, and the icon
    // bundle scan is pointed at this file — it does not read .ts on its own.
    icons: {
      arrowDown: 'tabler:arrow-down',
      arrowLeft: 'tabler:arrow-left',
      arrowRight: 'tabler:arrow-right',
      arrowUp: 'tabler:arrow-up',
      caution: 'tabler:alert-circle',
      check: 'tabler:check',
      chevronDoubleLeft: 'tabler:chevrons-left',
      chevronDoubleRight: 'tabler:chevrons-right',
      chevronDown: 'tabler:chevron-down',
      chevronLeft: 'tabler:chevron-left',
      chevronRight: 'tabler:chevron-right',
      chevronUp: 'tabler:chevron-up',
      close: 'tabler:x',
      copy: 'tabler:copy',
      copyCheck: 'tabler:copy-check',
      dark: 'tabler:moon',
      drag: 'tabler:grip-vertical',
      ellipsis: 'tabler:dots',
      error: 'tabler:circle-x',
      external: 'tabler:arrow-up-right',
      eye: 'tabler:eye',
      eyeOff: 'tabler:eye-off',
      file: 'tabler:file',
      folder: 'tabler:folder',
      folderOpen: 'tabler:folder-open',
      hash: 'tabler:hash',
      info: 'tabler:info-circle',
      light: 'tabler:sun',
      loading: 'tabler:loader-2',
      menu: 'tabler:menu-2',
      minus: 'tabler:minus',
      panelClose: 'tabler:layout-sidebar-left-collapse',
      panelOpen: 'tabler:layout-sidebar-left-expand',
      plus: 'tabler:plus',
      reload: 'tabler:reload',
      search: 'tabler:search',
      stop: 'tabler:square',
      success: 'tabler:circle-check',
      system: 'tabler:device-desktop',
      tip: 'tabler:bulb',
      upload: 'tabler:upload',
      warning: 'tabler:alert-triangle',
    },
    button: {
      slots: {
        base: 'cursor-pointer leading-none',
      },
    },
    popover: {
      slots: {
        content:
          'bg-white/70 dark:bg-neutral-900/50 backdrop-blur-3xl ring-0 shadow-lg border border-neutral-300/50 dark:border-neutral-500/50 rounded-lg',
      },
    },
    card: {
      slots: {
        header: 'font-semibold',
      },
      variants: {
        variant: {
          glassmorphism: {
            root: 'shadow-lg divide-y-0 divide-neutral-300/50 dark:divide-neutral-500/50',
            header: 'p-2 sm:p-2 pb-0!',
            body: 'p-2 sm:p-2',
            footer: 'p-2 sm:p-2',
          },
        },
      },
    },
    formField: {
      slots: {
        label: 'mb-1',
        help: 'mt-0.5',
      },
    },
  },
})
