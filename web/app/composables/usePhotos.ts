import type { AsyncDataRequestStatus } from '#app'

interface PhotosContext {
  photos: Ref<Photo[]>
  status: Ref<AsyncDataRequestStatus>
  refresh: () => Promise<void>
  loadMore: () => Promise<void>
  ensurePhoto: (id: string) => Promise<void>
  hasMore: ComputedRef<boolean>
  isLoadingMore: Ref<boolean>
  getPhotoById: (id: string) => Photo | undefined
  filterPhotos: (predicate: (photo: Photo) => boolean) => Photo[]
  totalCount: ComputedRef<number>
}

const PhotosContextKey = Symbol('PhotosContext') as InjectionKey<PhotosContext>

export function providePhotos(
  photos: Ref<Photo[]>,
  status: Ref<AsyncDataRequestStatus>,
  refresh: () => Promise<void>,
  totalCount?: Ref<number | undefined> | ComputedRef<number | undefined>,
  paging?: {
    loadMore: () => Promise<void>
    ensurePhoto: (id: string) => Promise<void>
    hasMore: ComputedRef<boolean>
    isLoadingMore: Ref<boolean>
  },
) {
  const context: PhotosContext = {
    photos,
    status,
    refresh,
    loadMore: paging?.loadMore ?? (async () => {}),
    ensurePhoto: paging?.ensurePhoto ?? (async () => {}),
    hasMore: paging?.hasMore ?? computed(() => false),
    isLoadingMore: paging?.isLoadingMore ?? ref(false),
    getPhotoById: (id: string) => {
      return photos.value.find((photo) => photo.id === id)
    },
    filterPhotos: (predicate: (photo: Photo) => boolean) => {
      return photos.value.filter(predicate)
    },
    totalCount: totalCount
      ? computed(() => totalCount.value ?? photos.value.length)
      : computed(() => photos.value.length),
  }

  provide(PhotosContextKey, context)

  return context
}

export function usePhotos(): PhotosContext {
  const context = inject(PhotosContextKey)

  if (!context) {
    throw new Error('usePhotos must be used within a PhotosProvider')
  }

  return context
}
