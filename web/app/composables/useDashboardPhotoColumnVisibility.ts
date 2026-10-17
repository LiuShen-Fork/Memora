import { useLocalStorage } from '@vueuse/core'

// 后台照片表显示哪几列是各人自己的事，存在浏览器里，刷新之后还是挑过的那几列。
const storageKey = 'dashboard-photos-column-visibility'

const columnIds = [
  'thumbnailUrl',
  'id',
  'actions',
  'title',
  'tags',
  'rating',
  'isLivePhoto',
  'location',
  'dateTaken',
  'lastModified',
  'fileSize',
  'colorSpace',
  'reactions',
] as const

type ColumnId = (typeof columnIds)[number]

// 缩略图和操作是这一行本身，少了它们这张表就不成表了，所以它们不参与挑选。
const alwaysVisible = ['thumbnailUrl', 'actions'] as const

// 默认摆出认得出一张照片的那几列：文件名、标签、评分、拍摄时间和地点。
// 编号、体积、色彩空间、修改时间和表态都留着，但不占地方——一张十三列的表在
// 1440 宽的屏幕上必须横向滚动，而滚动出来的第九列不会有人去看。
const defaultVisible = [
  'thumbnailUrl',
  'actions',
  'title',
  'tags',
  'rating',
  'isLivePhoto',
  'dateTaken',
  'location',
] as const

export type DashboardPhotoColumnVisibility = Record<ColumnId, boolean>

const fromList = (visible: readonly ColumnId[]): DashboardPhotoColumnVisibility =>
  Object.fromEntries(columnIds.map((id) => [id, visible.includes(id)])) as
    DashboardPhotoColumnVisibility

/**
 * 老版本把十三列一股脑全打开过——那是当时的默认值，不是谁挑的。这样的记录照着新
 * 默认重来一遍，免得升级之后手里还是那张要横向滚动的表；动过列菜单的人照旧。
 */
function isUnchangedOldDefault(stored: DashboardPhotoColumnVisibility) {
  return columnIds.every((id) => stored?.[id] === true)
}

/** 列菜单里的"恢复默认"用得到，所以这套默认值也得能从外面要一份。 */
export function defaultDashboardPhotoColumnVisibility() {
  return fromList(defaultVisible)
}

export function useDashboardPhotoColumnVisibility() {
  return useLocalStorage<DashboardPhotoColumnVisibility>(
    storageKey,
    () => fromList(defaultVisible),
    {
      // 存下来的可能出自列更少的版本，所以以现在的列表为准：认识的布尔值照搬，
      // 其余各归各位；两个必显列无论如何都是显示的。
      mergeDefaults(stored, fallback) {
        if (isUnchangedOldDefault(stored)) return fromList(defaultVisible)
        const merged = { ...fallback }
        for (const id of columnIds) {
          if (typeof stored?.[id] === 'boolean') merged[id] = stored[id]
        }
        for (const id of alwaysVisible) merged[id] = true
        return merged
      },
      initOnMounted: true,
    },
  )
}
