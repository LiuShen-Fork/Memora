/**
 * 页面滚过首屏之后，为「回到顶部」这类浮层让出位置。
 *
 * 上下两条线：往下过了 500 才出现，回到 400 以上才收起。只有一条线的时
 * 候，触控板轻轻一弹、或者照片墙加载完重新排版让页面高度变了，读数就会
 * 在线上来回跳，浮层跟着一闪一闪。两条线之间留出的这段距离，就是给它
 * 犹豫用的。
 *
 * 每帧只读一次滚动位置：一次滚动里浏览器会派发很多次 scroll，逐个处理
 * 没有意义。
 */
const SHOW_AFTER = 500
const HIDE_BEFORE = 400

export function useScrolledPast() {
  const scrolledPast = ref(false)
  let frame = 0

  const update = () => {
    frame = 0
    const scrollTop = window.pageYOffset || document.documentElement.scrollTop
    if (scrolledPast.value) {
      if (scrollTop < HIDE_BEFORE) scrolledPast.value = false
    } else if (scrollTop > SHOW_AFTER) {
      scrolledPast.value = true
    }
  }

  const handleScroll = () => {
    if (!frame) frame = requestAnimationFrame(update)
  }

  onMounted(() => {
    window.addEventListener('scroll', handleScroll, { passive: true })
  })

  onUnmounted(() => {
    window.removeEventListener('scroll', handleScroll)
    if (frame) cancelAnimationFrame(frame)
  })

  return scrolledPast
}
