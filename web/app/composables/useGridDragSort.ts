import { computed, onBeforeUnmount, ref } from 'vue'

interface Options {
  /**
   * 把第 from 张挪到第 to 张的位置。列表当场就动——格子里的空位是跟着手指
   * 走的，卡片墙自己会把别的卡片让开。
   */
  onMove: (from: number, to: number) => void
  /** 一次拖动结束，可以落盘了。中途换过多少次都只叫一次。 */
  onCommit: () => void
  /** 拖动开始，留给调用方记下拖动前的顺序好回退。 */
  onStart?: () => void
}

/** 走多远才算拖，而不是手抖。 */
const DRAG_THRESHOLD = 4

/**
 * 卡片墙的拖动排序。
 *
 * 只管「指针落在了哪一张上」和「跟着走的那张预览画在哪儿」。真正被拖的那张
 * 卡片留在原地，只是变淡成一个空位——它要是跟着指针走，就得用 transform，
 * 而列表一重排 <TransitionGroup> 会把子元素的 transform 清掉重新算，两下
 * 里打架。所以跟着指针的是别处一块预览，和被拖的那张互不相干。
 *
 * 只认指针事件，鼠标和触控笔走同一条路。触屏上不让整张卡片接管手势——那样
 * 在手机上就滑不动页面了——只有把手（带 data-sort-grip 的那一块）能拖。
 */
export function useGridDragSort({ onMove, onCommit, onStart }: Options) {
  const containerRef = ref<HTMLElement>()
  const draggingIndex = ref<number | null>(null)

  /** 预览块的位置和大小，取自按下时那张卡片的样子。 */
  const ghost = ref({ left: 0, top: 0, width: 0, height: 0 })

  /** 指针按下但还没走够距离，这时什么都不该发生。 */
  let pressed: { index: number; x: number; y: number } | null = null
  /** 抓在卡片里的哪个位置，预览块要照这个偏移跟着指针。 */
  let grabOffset = { x: 0, y: 0 }
  let releaseSwallow: (() => void) | undefined

  const cards = () =>
    Array.from(
      containerRef.value?.querySelectorAll<HTMLElement>('[data-sort-item]') ?? [],
    )

  /** 指针落在哪张卡片上；被拖着的那张不算，它是那个空位。 */
  const indexAtPoint = (x: number, y: number) => {
    const list = cards()
    for (let index = 0; index < list.length; index += 1) {
      if (index === draggingIndex.value) continue
      const rect = list[index]!.getBoundingClientRect()
      if (x >= rect.left && x < rect.right && y >= rect.top && y < rect.bottom) {
        return index
      }
    }
    return null
  }

  const moveTo = (to: number) => {
    const from = draggingIndex.value
    if (from === null || to === from) return
    onMove(from, to)
    draggingIndex.value = to
  }

  const stopListening = () => {
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
  }

  const onPointerMove = (event: PointerEvent) => {
    if (pressed && draggingIndex.value === null) {
      const travelled = Math.hypot(
        event.clientX - pressed.x,
        event.clientY - pressed.y,
      )
      if (travelled < DRAG_THRESHOLD) return
      draggingIndex.value = pressed.index
      onStart?.()
    }

    if (draggingIndex.value === null) return

    ghost.value = {
      ...ghost.value,
      left: event.clientX - grabOffset.x,
      top: event.clientY - grabOffset.y,
    }

    const target = indexAtPoint(event.clientX, event.clientY)
    if (target !== null && target !== draggingIndex.value) moveTo(target)
  }

  const onPointerUp = () => {
    stopListening()
    pressed = null
    if (draggingIndex.value === null) return

    draggingIndex.value = null
    releaseSwallow?.()

    // 松手的那一下后面多半还跟着一个 click，落在卡片下面的按钮上就会多走
    // 一步。拦掉下一个，然后就撤——不是每次拖动都有 click 跟来，留着不撤
    // 会把之后真正的点击也吃掉。
    const swallow = (event: Event) => {
      event.stopPropagation()
      event.preventDefault()
    }
    const release = () => {
      document.removeEventListener('click', swallow, true)
      clearTimeout(timer)
      releaseSwallow = undefined
    }
    document.addEventListener('click', swallow, true)
    const timer = setTimeout(release, 300)
    releaseSwallow = release

    onCommit()
  }

  const startDrag = (index: number, event: PointerEvent) => {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    // 触屏上只有把手能拖：整张卡片接管手势，页面就滑不动了。
    if (event.pointerType !== 'mouse') {
      const target = event.target
      if (!(target instanceof Element) || !target.closest('[data-sort-grip]')) {
        return
      }
    }

    const rect = cards()[index]?.getBoundingClientRect()
    if (!rect) return

    pressed = { index, x: event.clientX, y: event.clientY }
    grabOffset = { x: event.clientX - rect.left, y: event.clientY - rect.top }
    ghost.value = {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
  }

  const ghostStyle = computed(() => ({
    left: `${ghost.value.left}px`,
    top: `${ghost.value.top}px`,
    width: `${ghost.value.width}px`,
    height: `${ghost.value.height}px`,
  }))

  onBeforeUnmount(() => {
    stopListening()
    releaseSwallow?.()
    if (draggingIndex.value !== null) onCommit()
  })

  return { containerRef, draggingIndex, ghostStyle, startDrag }
}
