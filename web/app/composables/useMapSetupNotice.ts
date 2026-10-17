type MapSetupNoticeState = {
  open: boolean
  dismissedPath: string
}

// 地图没有配置好时，地球页会说明原因，并指向设置页。这个状态是共享的，因为
// 打开和关闭它的是两个不同的组件：地图自己，以及它渲染出来的提示框。
export function useMapSetupNotice() {
  const route = useRoute()
  const { user } = useUserSession()
  const state = useState<MapSetupNoticeState>('map-setup-notice', () => ({
    open: false,
    dismissedPath: '',
  }))

  const isAdmin = computed(() => Boolean(user.value?.isAdmin))

  const isOpen = computed({
    get: () => state.value.open,
    set: (value: boolean) => {
      if (value) {
        state.value.open = true
        return
      }
      if (!state.value.open) return
      state.value.open = false
      // 同一个页面上不再提第二次，换一页再提。
      state.value.dismissedPath = route.fullPath
    },
  })

  function show() {
    if (state.value.dismissedPath === route.fullPath) return
    state.value.open = true
  }

  // 地图能画出来了，提示与它一起消失，并且清掉“这一页不再提”，否则这一页上
  // 刚配好的地图再出问题时，反而什么都不会说。
  function hide() {
    state.value.open = false
    state.value.dismissedPath = ''
  }

  function dismiss() {
    isOpen.value = false
  }

  return { isOpen, isAdmin, show, hide, dismiss }
}
