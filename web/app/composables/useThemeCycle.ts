/**
 * 主题在「跟随系统 → 浅色 → 深色」之间循环。
 *
 * 首页顶栏和相册页首页都要放这个按钮，图标和循环顺序只写一份，免得两处
 * 各改一半、点了之后图标和实际主题对不上。
 */
export function useThemeCycle() {
  const colorMode = useColorMode()

  // 图标说的是眼睛现在看到的东西，不是点下去会变成什么：深色配月亮，
  // 浅色配太阳。跟随系统没有自己的样子，留一个显示器图标，好让人知道自己
  // 处在三态里的哪一态。
  const themeIcon = computed(() => {
    if (colorMode.preference === 'dark') return 'tabler:moon'
    if (colorMode.preference === 'light') return 'tabler:sun'
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
