/**
 * 主题在「跟随系统 → 浅色 → 深色」之间循环。
 *
 * 首页顶栏和相册页首页都要放这个按钮，图标和循环顺序只写一份，免得两处
 * 各改一半、点了之后图标和实际主题对不上。
 */
export function useThemeCycle() {
  const colorMode = useColorMode()

  const themeIcon = computed(() => {
    if (colorMode.preference === 'dark') return 'tabler:sun'
    if (colorMode.preference === 'light') return 'tabler:moon'
    return 'tabler:device-desktop'
  })

  const cycleTheme = () => {
    const nextTheme = {
      system: 'light',
      light: 'dark',
      dark: 'system',
    } as const
    colorMode.preference =
      nextTheme[colorMode.preference as keyof typeof nextTheme] || 'system'
  }

  return { themeIcon, cycleTheme }
}
