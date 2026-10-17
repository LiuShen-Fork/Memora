import type { TableRow } from '@nuxt/ui'

type RowSelectionState = Record<string, boolean>

// The part of a table this needs: the rows as they are ordered on screen, and a
// way to replace the selection in one go.
interface RangeSelectableTable<T> {
  getRowModel(): { rows: TableRow<T>[] }
  setRowSelection(
    updater: (previous: RowSelectionState) => RowSelectionState,
  ): void
}

/**
 * 按住 Shift 点击时选中一段范围。
 *
 * 选择是一个锚点加一个落点：普通点击把锚点落在刚点的这一项上，按住 Shift
 * 点击则选中锚点与落点之间的所有项。锚点在选择被清空时重置，下一次点击重新
 * 开始一段范围。
 */
export function useShiftRangeSelection() {
  let anchorId: string | null = null
  let pendingShiftKey = false

  // 从锚点到落点的一段，从锚点这一头数起；没有锚点时只有落点自己。
  function getRangeIds(
    orderedIds: readonly string[],
    targetId: string,
    shiftKey: boolean,
  ): string[] {
    const anchorIndex =
      shiftKey && anchorId !== null ? orderedIds.indexOf(anchorId) : -1
    const targetIndex = orderedIds.indexOf(targetId)

    anchorId = targetId

    if (anchorIndex === -1 || targetIndex === -1) {
      return [targetId]
    }

    return anchorIndex <= targetIndex
      ? orderedIds.slice(anchorIndex, targetIndex + 1)
      : orderedIds.slice(targetIndex, anchorIndex + 1).reverse()
  }

  function resetAnchor() {
    anchorId = null
  }

  // 复选框的 model 更新发生在点击之后，此时鼠标事件已经过去，所以按下时先记下
  // Shift 是否按着。
  function captureModifiers(event: MouseEvent) {
    pendingShiftKey = event.shiftKey
  }

  // 按住 Shift 点击会顺带选中页面上的文字，这里先挡掉。
  function preventShiftTextSelection(event: MouseEvent) {
    if (event.shiftKey) {
      event.preventDefault()
    }
  }

  function toggleTableRow<T>(
    table: RangeSelectableTable<T>,
    row: TableRow<T>,
    value: boolean,
  ) {
    const shiftKey = pendingShiftKey
    pendingShiftKey = false

    const rows = table.getRowModel().rows
    const rangeIds = new Set(
      getRangeIds(
        rows.map((r) => r.id),
        row.id,
        shiftKey,
      ),
    )

    if (rangeIds.size === 1) {
      row.toggleSelected(value)
      return
    }

    table.setRowSelection((previous) => {
      const next: RowSelectionState = { ...previous }
      for (const rangeRow of rows) {
        if (!rangeIds.has(rangeRow.id) || !rangeRow.getCanSelect()) continue
        if (value) {
          next[rangeRow.id] = true
        } else {
          delete next[rangeRow.id]
        }
      }
      return next
    })
  }

  return {
    getRangeIds,
    resetAnchor,
    captureModifiers,
    preventShiftTextSelection,
    toggleTableRow,
  }
}
