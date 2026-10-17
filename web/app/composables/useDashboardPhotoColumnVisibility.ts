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

// 这三列是每一行本身：缩略图、编号和操作，少了它们这张表就不成表了。
const alwaysVisible = ['thumbnailUrl', 'id', 'actions'] as const

export type DashboardPhotoColumnVisibility = Record<
  (typeof columnIds)[number],
  boolean
>

const allVisible = (): DashboardPhotoColumnVisibility =>
  Object.fromEntries(columnIds.map((id) => [id, true])) as DashboardPhotoColumnVisibility

// 存下来的可能出自列更少的版本，所以以现在的列表为准：认识的布尔值照搬，其余各归各位。
function mergeStoredColumnVisibility(
  stored: DashboardPhotoColumnVisibility,
  fallback: DashboardPhotoColumnVisibility,
): DashboardPhotoColumnVisibility {
  const merged = { ...fallback }
  for (const id of columnIds) {
    if (typeof stored?.[id] === 'boolean') merged[id] = stored[id]
  }
  for (const id of alwaysVisible) merged[id] = true
  return merged
}

export function useDashboardPhotoColumnVisibility() {
  return useLocalStorage<DashboardPhotoColumnVisibility>(storageKey, allVisible, {
    mergeDefaults: mergeStoredColumnVisibility,
    initOnMounted: true,
  })
}
