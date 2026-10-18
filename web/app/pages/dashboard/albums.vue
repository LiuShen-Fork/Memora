<script lang="ts" setup>
import type { Album, Photo } from '~~/shared/types/domain'
import type { FormSubmitEvent, FormError } from '@nuxt/ui'

definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.albums'),
})

interface AlbumItem extends Album {
  photoCount?: number
  photoIds?: string[]
  coverPhoto?: Photo | null
}

interface AlbumFormState {
  title: string
  description: string
  isHidden: boolean
  passwordEnabled: boolean
  password: string
}

const albums = ref<AlbumItem[]>([])
const isLoadingAlbums = ref(false)
const { photos: allPhotos } = usePhotos()

// 一次就够。每次要弹提示时现调 useToast()，在没有组件实例的上下文里调用
// 会拿到 undefined 并报警告——事件回调里能成，异步续上的那一半不一定。
const toast = useToast()

const isAlbumSlideoverOpen = ref(false)
const isDeleteConfirmOpen = ref(false)
const isPhotoSelectorOpen = ref(false)

const currentAlbum = ref<AlbumItem | null>(null)

const formData = reactive<AlbumFormState>({
  title: '',
  description: '',
  isHidden: false,
  passwordEnabled: false,
  password: '',
})

// A password is only as good as the media route behind it. With a provider
// that serves objects directly, the album page stops listing the photos but
// the files stay reachable by anyone holding the link, so the form says so
// rather than letting the checkbox imply more than it delivers.
const { data: albumProtection } = useFetch<{ protectsMedia: boolean }>(
  '/api/albums/protection',
)
const albumPasswordProtectsMedia = computed(
  () => albumProtection.value?.protectsMedia !== false,
)

const formRef = ref()
const isSubmittingForm = ref(false)

const selectedPhotoIds = ref<string[]>([])
const coverPhotoId = ref('')

const draftSelectedPhotoIds = ref<string[]>([])
const draftCoverPhotoId = ref('')
const {
  filteredPhotos: unifiedFilteredPhotos,
  selectedCounts,
  hasActiveFilters,
  clearAllFilters,
} = usePhotoFilters()

const isSelectorFilterOpen = ref(false)

const totalSelectedFilters = computed(() => {
  return Object.values(selectedCounts.value).reduce(
    (total, count) => total + count,
    0,
  )
})

const syncAlbumCovers = () => {
  for (const album of albums.value) {
    // 相簿没挑封面时用它自己的第一张照片顶上，只有空相簿才没有封面。
    const coverId = album.coverPhotoId || album.photoIds?.[0]
    album.coverPhoto = coverId
      ? allPhotos.value.find((photo) => photo.id === coverId) || null
      : null
  }
}

const validateForm = (state: any): FormError[] => {
  const errors: FormError[] = []
  if (!state.title?.trim()) {
    errors.push({
      name: 'title',
      message: $t('dashboard.albums.form.titleRequired'),
    })
  }
  if (state.passwordEnabled) {
    // An empty field on an album that already has a password means "keep the
    // one it has", so it is only missing when there is nothing to keep.
    if (!state.password && !currentAlbum.value?.hasPassword) {
      errors.push({
        name: 'password',
        message: $t('dashboard.albums.form.passwordRequired'),
      })
    } else if (state.password && state.password.length < 6) {
      errors.push({
        name: 'password',
        message: $t('dashboard.albums.form.passwordTooShort'),
      })
    }
  }
  return errors
}

const loadAlbums = async () => {
  isLoadingAlbums.value = true
  try {
    const response = await $fetch('/api/albums')
    albums.value = (response as any[]).map((album) => ({
      ...album,
      photoCount: album.photoIds?.length || 0,
    }))
    syncAlbumCovers()
  } catch (error) {
    console.error('Failed to load albums:', error)
    toast.add({
      title: $t('dashboard.albums.messages.loadError'),
      color: 'error',
    })
  } finally {
    isLoadingAlbums.value = false
  }
}

watch(allPhotos, syncAlbumCovers, { deep: false })

const openCreateSlideover = () => {
  currentAlbum.value = null
  formData.title = ''
  formData.description = ''
  formData.isHidden = false
  formData.passwordEnabled = false
  formData.password = ''
  selectedPhotoIds.value = []
  coverPhotoId.value = ''
  formRef.value?.clear()
  isAlbumSlideoverOpen.value = true
}

const openEditSlideover = async (album: AlbumItem) => {
  currentAlbum.value = album
  try {
    const albumDetail = (await $fetch(`/api/albums/${album.id}`)) as any
    formData.title = album.title
    formData.description = album.description || ''
    formData.isHidden = album.isHidden || false
    formData.passwordEnabled = album.hasPassword || false
    formData.password = ''
    selectedPhotoIds.value = (albumDetail.photos || []).map((p: Photo) => p.id)
    coverPhotoId.value = album.coverPhotoId || ''
    formRef.value?.clear()
  } catch (error) {
    console.error('Failed to load album details:', error)
    toast.add({
      title: $t('dashboard.albums.messages.loadDetailError'),
      color: 'error',
    })
  }
  isAlbumSlideoverOpen.value = true
}

const openDeleteConfirm = (album: AlbumItem) => {
  currentAlbum.value = album
  isDeleteConfirmOpen.value = true
}

const onFormSubmit = async (event: FormSubmitEvent<AlbumFormState>) => {
  isSubmittingForm.value = true
  // On an album that already has one, a blank field keeps the password it has;
  // switching the checkbox off is how the protection is removed.
  const password = event.data.passwordEnabled
    ? event.data.password || undefined
    : ''
  try {
    if (currentAlbum.value) {
      await $fetch(`/api/albums/${currentAlbum.value.id}`, {
        method: 'PUT',
        body: {
          title: event.data.title,
          description: event.data.description || undefined,
          coverPhotoId: coverPhotoId.value || undefined,
          photoIds: selectedPhotoIds.value,
          isHidden: event.data.isHidden,
          password,
        },
      })

      toast.add({
        title: $t('dashboard.albums.messages.updateSuccess'),
        color: 'success',
      })

      isAlbumSlideoverOpen.value = false
    } else {
      await $fetch('/api/albums', {
        method: 'POST',
        body: {
          title: event.data.title,
          description: event.data.description || undefined,
          coverPhotoId: coverPhotoId.value || undefined,
          photoIds: selectedPhotoIds.value,
          isHidden: event.data.isHidden,
          password,
        },
      })

      toast.add({
        title: $t('dashboard.albums.messages.createSuccess'),
        color: 'success',
      })

      isAlbumSlideoverOpen.value = false
    }

    // The password has been stored, so the copy on screen has no reason to
    // stay there.
    formData.password = ''
    await loadAlbums()
  } catch (error) {
    console.error('Failed to save album:', error)
    toast.add({
      title: currentAlbum.value
        ? $t('dashboard.albums.messages.updateError')
        : $t('dashboard.albums.messages.createError'),
      color: 'error',
    })
  } finally {
    isSubmittingForm.value = false
  }
}

const deleteAlbum = async () => {
  if (!currentAlbum.value) return

  try {
    await $fetch(`/api/albums/${currentAlbum.value.id}`, {
      method: 'DELETE',
    })

    toast.add({
      title: $t('dashboard.albums.messages.deleteSuccess'),
      color: 'success',
    })

    isDeleteConfirmOpen.value = false
    await loadAlbums()
  } catch (error) {
    console.error('Failed to delete album:', error)
    toast.add({
      title: $t('dashboard.albums.messages.deleteError'),
      color: 'error',
    })
  }
}

const togglePhotoSelection = (photoId: string) => {
  const index = selectedPhotoIds.value.indexOf(photoId)
  if (index > -1) {
    selectedPhotoIds.value.splice(index, 1)
    if (coverPhotoId.value === photoId) {
      coverPhotoId.value = ''
    }
  } else {
    selectedPhotoIds.value.push(photoId)
  }
}

const selectorRangeSelection = useShiftRangeSelection()

// 清空选择后重新开始一段范围，否则下一次 Shift 点击会跨过中间已取消的选择
watch(
  () => draftSelectedPhotoIds.value.length === 0,
  (isEmpty) => {
    if (isEmpty) selectorRangeSelection.resetAnchor()
  },
)

const openPhotoSelector = () => {
  draftSelectedPhotoIds.value = [...selectedPhotoIds.value]
  draftCoverPhotoId.value =
    coverPhotoId.value && selectedPhotoIds.value.includes(coverPhotoId.value)
      ? coverPhotoId.value
      : ''
  selectorRangeSelection.resetAnchor()
  isSelectorFilterOpen.value = false
  isPhotoSelectorOpen.value = true
}

const closePhotoSelector = () => {
  isSelectorFilterOpen.value = false
  isPhotoSelectorOpen.value = false
}

const confirmPhotoSelection = () => {
  selectedPhotoIds.value = [...draftSelectedPhotoIds.value]
  coverPhotoId.value = draftSelectedPhotoIds.value.includes(
    draftCoverPhotoId.value,
  )
    ? draftCoverPhotoId.value
    : ''
  isPhotoSelectorOpen.value = false
}

// 按住 Shift 点击时，选中（或取消）从上一个点到这一个点之间的所有照片
const toggleDraftPhotoSelection = (photoId: string, event: MouseEvent) => {
  const shouldSelect = !draftSelectedPhotoIds.value.includes(photoId)
  const rangeIds = selectorRangeSelection.getRangeIds(
    selectorFilteredPhotos.value.map((photo) => photo.id),
    photoId,
    event.shiftKey,
  )

  if (shouldSelect) {
    const selected = new Set(draftSelectedPhotoIds.value)
    for (const id of rangeIds) {
      if (!selected.has(id)) {
        draftSelectedPhotoIds.value.push(id)
      }
    }
    return
  }

  const removed = new Set(rangeIds)
  draftSelectedPhotoIds.value = draftSelectedPhotoIds.value.filter(
    (id) => !removed.has(id),
  )
  if (removed.has(draftCoverPhotoId.value)) {
    draftCoverPhotoId.value = ''
  }
}

const setDraftCoverPhoto = (photoId: string) => {
  if (!draftSelectedPhotoIds.value.includes(photoId)) {
    draftSelectedPhotoIds.value.push(photoId)
  }
  draftCoverPhotoId.value = photoId
}

const getDraftPhotoOrder = (photoId: string) => {
  const index = draftSelectedPhotoIds.value.indexOf(photoId)
  return index >= 0 ? index + 1 : null
}

const removeDraftPhoto = (photoId: string) => {
  const index = draftSelectedPhotoIds.value.indexOf(photoId)
  if (index < 0) return
  draftSelectedPhotoIds.value.splice(index, 1)
  if (draftCoverPhotoId.value === photoId) {
    draftCoverPhotoId.value = ''
  }
}

// The order of draftSelectedPhotoIds is the order the album stores, so moving a
// photo is a splice within that list rather than a separate value to keep in
// sync.
const moveDraftPhoto = (photoId: string, offset: number) => {
  const ids = draftSelectedPhotoIds.value
  const index = ids.indexOf(photoId)
  const target = index + offset
  if (index < 0 || target < 0 || target >= ids.length) return
  const [moved] = ids.splice(index, 1)
  ids.splice(target, 0, moved as string)
}

const isReorderingAlbums = ref(false)

/**
 * 把当前顺序交给服务器。新顺序是调用方先摆在屏幕上的，写不进去就退回
 * previous 那一份。
 */
const persistAlbumOrder = async (previous: AlbumItem[]) => {
  isReorderingAlbums.value = true

  try {
    await $fetch('/api/albums/reorder', {
      method: 'PUT',
      body: { albumIds: albums.value.map((album) => album.id) },
    })
  } catch (error) {
    console.error('Failed to reorder albums:', error)
    albums.value = previous
    toast.add({
      title: $t('dashboard.albums.messages.reorderError'),
      color: 'error',
    })
  } finally {
    isReorderingAlbums.value = false
  }
}

const moveAlbum = async (index: number, offset: number) => {
  const target = index + offset
  if (isReorderingAlbums.value || target < 0 || target >= albums.value.length) {
    return
  }

  const previous = albums.value
  const reordered = [...albums.value]
  const [moved] = reordered.splice(index, 1)
  reordered.splice(target, 0, moved as AlbumItem)
  // Show the new order straight away; a failure puts the old one back.
  albums.value = reordered
  await persistAlbumOrder(previous)
}

const areAllFilteredPhotosSelected = computed(() => {
  return (
    selectorFilteredPhotos.value.length > 0 &&
    selectorFilteredPhotos.value.every((photo) =>
      draftSelectedPhotoIds.value.includes(photo.id),
    )
  )
})

const areSomeFilteredPhotosSelected = computed(() => {
  const selectedInFiltered = selectorFilteredPhotos.value.filter((photo) =>
    draftSelectedPhotoIds.value.includes(photo.id),
  ).length
  return (
    selectedInFiltered > 0 &&
    selectedInFiltered < selectorFilteredPhotos.value.length
  )
})

const toggleAllFilteredPhotos = () => {
  if (areAllFilteredPhotosSelected.value) {
    draftSelectedPhotoIds.value = draftSelectedPhotoIds.value.filter(
      (id) => !selectorFilteredPhotos.value.some((photo) => photo.id === id),
    )
    if (
      draftCoverPhotoId.value &&
      !draftSelectedPhotoIds.value.includes(draftCoverPhotoId.value)
    ) {
      draftCoverPhotoId.value = ''
    }
    return
  }

  const merged = new Set(draftSelectedPhotoIds.value)
  for (const photo of selectorFilteredPhotos.value) {
    merged.add(photo.id)
  }
  draftSelectedPhotoIds.value = [...merged]
}

const selectorFilteredPhotos = computed(() => {
  if (allPhotos.value.length === 0) return []

  const ids = new Set(allPhotos.value.map((photo) => photo.id))
  return unifiedFilteredPhotos.value.filter((photo) => ids.has(photo.id))
})

const selectedPhotosPreview = computed(() => {
  return draftSelectedPhotoIds.value
    .map((id) => allPhotos.value.find((photo) => photo.id === id))
    .filter((photo): photo is Photo => Boolean(photo))
    .slice(0, 8)
})

const selectedPhotosOverflowCount = computed(() => {
  return Math.max(
    draftSelectedPhotoIds.value.length - selectedPhotosPreview.value.length,
    0,
  )
})

onMounted(async () => {
  await loadAlbums()
})

const dayjs = useDayjs()

const slideoverTitle = computed(() => {
  return currentAlbum.value
    ? $t('dashboard.albums.slideover.edit.title')
    : $t('dashboard.albums.slideover.create.title')
})

const slideoverDescription = computed(() => {
  return currentAlbum.value
    ? $t('dashboard.albums.slideover.edit.description')
    : $t('dashboard.albums.slideover.create.description')
})

const submitButtonLabel = computed(() => {
  return currentAlbum.value
    ? $t('dashboard.albums.slideover.submitEdit')
    : $t('dashboard.albums.slideover.submitCreate')
})

/* -------------------------------------------------------------------------
 * 排列顺序
 *
 * 卡片墙里的上下箭头没有意义——一张卡片不在「上一张的下面」，它可能在右边、
 * 也可能换了一行。所以顺序是另外一层：点「排序」进到排序状态，卡片各自报出
 * 自己是第几张，然后可以用鼠标拖，也可以点 ◀ ▶ 一格一格挪。离开这个状态就
 * 回到平时的样子，不会误触。
 * ---------------------------------------------------------------------- */
const isOrdering = ref(false)
/** 拖动前的顺序，写回失败时退回去。 */
let orderBeforeDrag: AlbumItem[] | null = null

const moveAlbumTo = (from: number, to: number) => {
  const reordered = [...albums.value]
  const [moved] = reordered.splice(from, 1)
  reordered.splice(to, 0, moved as AlbumItem)
  albums.value = reordered
}

const {
  containerRef: albumGridRef,
  draggingIndex,
  ghostStyle,
  startDrag,
} = useGridDragSort({
  onMove: moveAlbumTo,
  onStart: () => {
    orderBeforeDrag = albums.value
  },
  // 拖一次可能换过十几个位置，但服务器只该收到一遍。
  onCommit: () => {
    if (orderBeforeDrag) void persistAlbumOrder(orderBeforeDrag)
    orderBeforeDrag = null
  },
})

const draggingAlbum = computed(() =>
  draggingIndex.value === null ? null : albums.value[draggingIndex.value],
)

const isDragging = (index: number) => draggingIndex.value === index

const albumMenu = (album: AlbumItem) => [
  [
    {
      label: $t('dashboard.albums.menu.view'),
      icon: 'tabler:external-link',
      to: `/albums/${album.id}`,
      target: '_blank',
    },
    {
      label: $t('common.actions.edit'),
      icon: 'tabler:edit',
      onSelect: () => openEditSlideover(album),
    },
  ],
  [
    {
      label: $t('common.actions.delete'),
      icon: 'tabler:trash',
      color: 'error' as const,
      onSelect: () => openDeleteConfirm(album),
    },
  ],
]
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <UDashboardNavbar :title="$t('title.albums')">
        <template #right>
          <UButton
            v-if="albums.length > 0"
            :icon="isOrdering ? 'tabler:check' : 'tabler:arrows-sort'"
            :variant="isOrdering ? 'solid' : 'soft'"
            :color="isOrdering ? 'primary' : 'neutral'"
            @click="isOrdering = !isOrdering"
          >
            {{
              isOrdering
                ? $t('dashboard.albums.order.done')
                : $t('dashboard.albums.order.start')
            }}
          </UButton>
          <UButton
            v-if="!isOrdering"
            data-tour="album-create"
            icon="tabler:plus"
            variant="soft"
            @click="openCreateSlideover"
          >
            {{ $t('dashboard.albums.createButton') }}
          </UButton>
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="flex flex-col gap-6">
        <template v-if="albums.length > 0">
          <!-- 排序状态才出现的一行说明：这个状态下卡片的样子变了，得说一句
               为什么变。 -->
          <div
            v-if="isOrdering"
            class="flex items-center gap-2.5 rounded-lg border border-primary/25 bg-primary/5 px-3.5 py-2.5"
          >
            <UIcon
              name="tabler:arrows-sort"
              class="size-4 shrink-0 text-primary"
            />
            <p class="min-w-0 flex-1 text-sm text-toned">
              {{ $t('dashboard.albums.order.hint') }}
            </p>
          </div>

          <div
            ref="albumGridRef"
            :class="draggingIndex !== null ? 'album-grid-dragging' : ''"
          >
            <TransitionGroup
              tag="ul"
              name="album-card"
              class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-3"
            >
              <li
                v-for="(album, index) in albums"
                :key="album.id"
                data-sort-item
                class="group relative flex flex-col overflow-hidden rounded-lg border border-default bg-default"
                :class="[
                  isOrdering
                    ? 'select-none'
                    : 'cursor-pointer transition-[border-color,box-shadow] hover:border-accented hover:shadow-sm',
                  // 被拖的那张留在格子里当空位，跟着手指走的是别处那块预览。
                  isDragging(index) ? 'opacity-30' : '',
                ]"
                @click="isOrdering ? undefined : openEditSlideover(album)"
                @pointerdown="
                  isOrdering ? startDrag(index, $event) : undefined
                "
              >
                <div class="relative aspect-[16/10] w-full overflow-hidden bg-elevated">
                  <img
                    v-if="album.coverPhoto"
                    :src="album.coverPhoto.thumbnailUrl || ''"
                    :alt="album.title"
                    class="size-full object-cover"
                    :class="
                      isOrdering
                        ? ''
                        : 'transition-transform duration-500 group-hover:scale-[1.04]'
                    "
                  />
                  <div
                    v-else
                    class="flex size-full items-center justify-center text-dimmed"
                  >
                    <UIcon
                      name="tabler:photo"
                      class="size-8"
                    />
                  </div>

                  <!-- 排序时卡片报出自己的名次，光有箭头看不出现在排到哪了。 -->
                  <span
                    v-if="isOrdering"
                    class="absolute start-2 top-2 flex size-6 items-center justify-center rounded-full bg-inverted/75 text-xs font-medium tabular text-inverted backdrop-blur-sm"
                  >
                    {{ index + 1 }}
                  </span>

                  <!-- 平时这张卡片上只有一个控件：编辑和删除都在里面。
                       一直看得见，手指上没有悬停这回事。 -->
                  <div
                    v-else
                    class="absolute end-2 top-2"
                    :data-tour="index === 0 ? 'album-actions' : undefined"
                    @click.stop
                  >
                    <UDropdownMenu
                      :items="albumMenu(album)"
                      :content="{ align: 'end' }"
                      size="sm"
                    >
                      <UButton
                        icon="tabler:dots-vertical"
                        color="neutral"
                        variant="solid"
                        size="xs"
                        square
                        class="rounded-full bg-default/85 text-toned backdrop-blur-sm hover:bg-default"
                        :aria-label="$t('dashboard.albums.menu.label')"
                      />
                    </UDropdownMenu>
                  </div>

                  <div
                    v-if="album.isHidden || album.hasPassword"
                    class="absolute start-2 bottom-2 flex items-center gap-1"
                  >
                    <span
                      v-if="album.isHidden"
                      class="rounded-full bg-inverted/75 px-1.5 py-0.5 text-[11px] leading-4 text-inverted backdrop-blur-sm"
                    >
                      {{ $t('dashboard.albums.form.isHidden') }}
                    </span>
                    <span
                      v-if="album.hasPassword"
                      class="flex items-center gap-1 rounded-full bg-inverted/75 px-1.5 py-0.5 text-[11px] leading-4 text-inverted backdrop-blur-sm"
                    >
                      <UIcon
                        name="tabler:lock"
                        class="size-3"
                      />
                      {{ $t('dashboard.albums.form.password') }}
                    </span>
                  </div>
                </div>

                <div class="flex flex-1 flex-col p-3.5">
                  <h3
                    class="truncate font-medium text-highlighted"
                    :title="album.title"
                  >
                    {{ album.title }}
                  </h3>

                  <!-- 说明占固定的三行：每张卡片一样高，写多长都一样，多出来
                       的收成省略号。要看全文就点开这一张。 -->
                  <p
                    v-if="album.description"
                    class="mt-1 h-15 line-clamp-3 text-[13px] leading-5 text-muted"
                    :title="album.description"
                  >
                    {{ album.description }}
                  </p>
                  <p
                    v-else
                    class="mt-1 h-15 text-[13px] leading-5 text-dimmed"
                  >
                    —
                  </p>

                  <div
                    class="mt-auto flex items-center gap-2 pt-3 text-xs text-dimmed"
                  >
                    <span class="tabular">
                      {{
                        $t('dashboard.albums.photoCount', {
                          count: album.photoCount || 0,
                        })
                      }}
                    </span>
                    <span aria-hidden="true">·</span>
                    <span class="tabular">
                      {{ dayjs(album.createdAt).format('YYYY-MM-DD') }}
                    </span>
                  </div>
                </div>

                <!-- 排序状态下的两件事：把手（触屏上只有它能拖），和把名次
                     挪一格的两个按钮。 -->
                <div
                  v-if="isOrdering"
                  class="flex items-center justify-between gap-2 border-t border-default px-2 py-1.5"
                >
                  <span
                    data-sort-grip
                    aria-hidden="true"
                    class="flex size-7 cursor-grab touch-none items-center justify-center rounded-md text-dimmed active:cursor-grabbing"
                  >
                    <UIcon
                      name="tabler:grip-horizontal"
                      class="size-4"
                    />
                  </span>

                  <div class="flex items-center gap-0.5">
                    <UButton
                      icon="tabler:chevron-left"
                      color="neutral"
                      variant="ghost"
                      size="xs"
                      square
                      :disabled="index === 0 || isReorderingAlbums"
                      :title="$t('dashboard.albums.order.moveEarlier')"
                      :aria-label="$t('dashboard.albums.order.moveEarlier')"
                      @click="moveAlbum(index, -1)"
                    />
                    <UButton
                      icon="tabler:chevron-right"
                      color="neutral"
                      variant="ghost"
                      size="xs"
                      square
                      :disabled="
                        index === albums.length - 1 || isReorderingAlbums
                      "
                      :title="$t('dashboard.albums.order.moveLater')"
                      :aria-label="$t('dashboard.albums.order.moveLater')"
                      @click="moveAlbum(index, 1)"
                    />
                  </div>
                </div>
              </li>
            </TransitionGroup>
          </div>

          <!-- 跟着指针走的那一块。用 teleport 挪到 body：它得贴着视口定位，
               而且要躲开列表自己的移动动画。 -->
          <Teleport to="body">
            <div
              v-if="draggingAlbum"
              :style="ghostStyle"
              class="pointer-events-none fixed z-50 overflow-hidden rounded-lg border border-default bg-default shadow-xl"
            >
              <div class="relative aspect-[16/10] w-full overflow-hidden bg-elevated">
                <img
                  v-if="draggingAlbum.coverPhoto"
                  :src="draggingAlbum.coverPhoto.thumbnailUrl || ''"
                  alt=""
                  class="size-full object-cover"
                />
                <div
                  v-else
                  class="flex size-full items-center justify-center text-dimmed"
                >
                  <UIcon
                    name="tabler:photo"
                    class="size-8"
                  />
                </div>
              </div>
              <div class="p-3.5">
                <h3 class="truncate font-medium text-highlighted">
                  {{ draggingAlbum.title }}
                </h3>
              </div>
            </div>
          </Teleport>
        </template>

        <DashboardPanel
          v-else-if="!isLoadingAlbums"
          class="items-center text-center"
          :ui="{ body: 'flex flex-col items-center gap-2 px-4 py-16 sm:px-4' }"
        >
          <Icon
            name="tabler:album"
            class="size-10 text-dimmed"
          />
          <h3 class="text-sm font-semibold text-highlighted">
            {{ $t('dashboard.albums.noAlbums') }}
          </h3>
          <p class="mb-2 max-w-md text-sm text-muted">
            {{ $t('dashboard.albums.noAlbumsTip') }}
          </p>
          <UButton
            icon="tabler:plus"
            @click="openCreateSlideover"
          >
            {{ $t('dashboard.albums.createButton') }}
          </UButton>
        </DashboardPanel>
        <div
          v-else
          class="flex items-center justify-center py-12"
        >
          <Icon
            name="tabler:loader"
            class="size-8 animate-spin text-primary"
          />
        </div>

        <USlideover
          v-model:open="isAlbumSlideoverOpen"
          :title="slideoverTitle"
          :description="slideoverDescription"
          :ui="{ footer: 'justify-end', body: 'p-0 sm:p-0 space-y-4' }"
        >
          <template #body>
            <div
              v-if="coverPhotoId"
              class="relative w-full aspect-video bg-gray-100 dark:bg-neutral-800 overflow-hidden"
            >
              <ThumbImage
                :src="
                  allPhotos.find((p) => p.id === coverPhotoId)?.thumbnailUrl ||
                  ''
                "
                :alt="coverPhotoId"
                class="absolute inset-0 w-full h-full object-cover"
              />
              <button
                class="absolute inset-0 bg-black/50 opacity-0 hover:opacity-100 transition-opacity flex items-center justify-center"
                @click="coverPhotoId = ''"
              >
                <Icon
                  name="tabler:x"
                  class="text-white"
                />
              </button>
            </div>
            <button
              v-else
              class="w-full h-48 bg-gray-100 dark:bg-neutral-800 flex flex-col items-center justify-center text-gray-500 hover:text-gray-600 dark:hover:text-gray-400 hover:bg-gray-200 dark:hover:bg-neutral-700 transition-colors cursor-pointer"
              @click="openPhotoSelector"
            >
              <Icon
                name="tabler:photo"
                size="40"
                class="mb-2"
              />
              <p class="text-sm font-medium">
                {{ $t('dashboard.albums.form.addCoverPhoto') }}
              </p>
            </button>
            <div class="space-y-4 px-4">
              <UForm
                ref="formRef"
                :state="formData"
                :validate="validateForm"
                class="space-y-4"
                @submit="onFormSubmit"
              >
                <UFormField
                  :label="$t('dashboard.albums.form.title')"
                  name="title"
                  required
                >
                  <UInput
                    v-model="formData.title"
                    class="w-full"
                    :placeholder="$t('dashboard.albums.form.titlePlaceholder')"
                  />
                </UFormField>

                <UFormField
                  :label="$t('dashboard.albums.form.description')"
                  name="description"
                >
                  <UTextarea
                    v-model="formData.description"
                    class="w-full"
                    :placeholder="
                      $t('dashboard.albums.form.descriptionPlaceholder')
                    "
                    :rows="3"
                  />
                </UFormField>

                <UFormField
                  :label="$t('dashboard.albums.form.isHidden')"
                  name="isHidden"
                  :hint="$t('dashboard.albums.form.isHiddenHint')"
                >
                  <UCheckbox
                    v-model="formData.isHidden"
                    :label="$t('dashboard.albums.form.isHidden')"
                  />
                </UFormField>

                <!-- Password protection -->
                <div
                  class="space-y-3 rounded-xl border p-4 transition-colors"
                  :class="
                    formData.passwordEnabled
                      ? 'border-primary-200 bg-primary-50/40 dark:border-primary-800/60 dark:bg-primary-950/20'
                      : 'border-gray-200 dark:border-neutral-700'
                  "
                >
                  <UCheckbox
                    v-model="formData.passwordEnabled"
                    :label="$t('dashboard.albums.form.passwordProtection')"
                    :description="
                      $t('dashboard.albums.form.passwordProtectionHint')
                    "
                  />

                  <template v-if="formData.passwordEnabled">
                    <UAlert
                      v-if="!albumPasswordProtectsMedia"
                      color="warning"
                      variant="subtle"
                      icon="tabler:alert-triangle"
                      :title="$t('dashboard.albums.form.passwordWarningTitle')"
                      :description="
                        $t('dashboard.albums.form.passwordWarningDescription')
                      "
                    />

                    <UFormField
                      name="password"
                      :label="$t('dashboard.albums.form.password')"
                      :hint="
                        currentAlbum?.hasPassword
                          ? $t('dashboard.albums.form.passwordKeepHint')
                          : undefined
                      "
                    >
                      <UInput
                        v-model="formData.password"
                        type="password"
                        class="w-full"
                        autocomplete="new-password"
                        :placeholder="
                          $t('dashboard.albums.form.passwordPlaceholder')
                        "
                      />
                    </UFormField>
                  </template>
                </div>
              </UForm>

              <!-- 照片选择部分 -->
              <div class="space-y-3">
                <UButton
                  variant="outline"
                  color="primary"
                  icon="tabler:photo-plus"
                  size="lg"
                  class="w-full"
                  @click="openPhotoSelector"
                >
                  {{
                    selectedPhotoIds.length > 0
                      ? $t('dashboard.albums.form.editPhotos')
                      : $t('dashboard.albums.form.selectPhotos')
                  }}
                </UButton>

                <div
                  v-if="selectedPhotoIds.length > 0"
                  class="space-y-2"
                >
                  <div class="flex items-center justify-between">
                    <label
                      class="text-sm font-medium text-gray-700 dark:text-gray-300"
                      >{{
                        $t('dashboard.albums.form.selectedCount', {
                          count: selectedPhotoIds.length,
                        })
                      }}</label
                    >
                    <UButton
                      variant="ghost"
                      color="neutral"
                      size="xs"
                      icon="tabler:trash"
                      @click="selectedPhotoIds = []"
                    >
                      {{ $t('dashboard.albums.form.clearAll') }}
                    </UButton>
                  </div>

                  <div
                    class="grid grid-cols-4 gap-2 p-3 bg-gray-50 dark:bg-neutral-800/50 rounded-lg border border-gray-200 dark:border-neutral-700"
                  >
                    <div
                      v-for="photoId in selectedPhotoIds"
                      :key="photoId"
                      class="relative group aspect-square rounded-lg overflow-hidden bg-gray-200 dark:bg-neutral-700"
                    >
                      <img
                        :src="
                          allPhotos.find((p) => p.id === photoId)
                            ?.thumbnailUrl || ''
                        "
                        :alt="photoId"
                        class="w-full h-full object-cover"
                      />

                      <div
                        v-if="coverPhotoId === photoId"
                        class="absolute top-1 left-1 bg-primary-500 text-white px-1.5 py-0.5 rounded text-xs font-medium"
                      >
                        {{ $t('dashboard.albums.modal.setCover') }}
                      </div>

                      <button
                        class="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center"
                        @click="togglePhotoSelection(photoId)"
                      >
                        <Icon
                          name="tabler:x"
                          class="text-white"
                        />
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </template>

          <template #footer="{ close }">
            <UButton
              variant="ghost"
              color="neutral"
              @click="close"
            >
              {{ $t('dashboard.albums.slideover.cancel') }}
            </UButton>
            <UButton
              icon="tabler:check"
              :loading="isSubmittingForm"
              @click="formRef?.submit()"
            >
              {{ submitButtonLabel }}
            </UButton>
          </template>
        </USlideover>

        <UModal
          v-model:open="isPhotoSelectorOpen"
          portal
          scrollable
          :ui="{
            wrapper: 'z-220',
            overlay: 'z-220',
            content: 'z-221 w-full max-w-6xl overflow-hidden',
          }"
        >
          <template #content>
            <div class="flex h-[88vh] max-h-[88vh] flex-col">
              <div
                class="shrink-0 border-b border-gray-200 bg-white/80 p-4 backdrop-blur-sm dark:border-neutral-700 dark:bg-neutral-900/80 sm:p-5"
              >
                <div class="flex items-center justify-between gap-2">
                  <div>
                    <h2 class="text-lg font-semibold sm:text-xl">
                      {{ $t('dashboard.albums.modal.selectPhotos') }}
                    </h2>
                  </div>
                  <UButton
                    icon="tabler:x"
                    color="neutral"
                    variant="ghost"
                    @click="closePhotoSelector"
                  />
                </div>

                <div class="mt-4 space-y-3">
                  <div class="flex flex-wrap items-center gap-2">
                    <UPopover
                      v-model:open="isSelectorFilterOpen"
                      :content="{
                        side: 'bottom',
                        align: 'start',
                        sideOffset: 8,
                      }"
                      :ui="{ content: 'z-230' }"
                    >
                      <UButton
                        icon="tabler:filter"
                        :color="hasActiveFilters ? 'info' : 'neutral'"
                        :variant="hasActiveFilters ? 'soft' : 'outline'"
                        size="sm"
                      >
                        {{ $t('ui.action.filter.title') }}
                        <UBadge
                          v-if="totalSelectedFilters > 0"
                          size="xs"
                          color="info"
                          variant="solid"
                          class="ml-1"
                        >
                          {{ totalSelectedFilters }}
                        </UBadge>
                      </UButton>

                      <template #content>
                        <UCard variant="glassmorphism">
                          <OverlayFilterPanel />
                        </UCard>
                      </template>
                    </UPopover>

                    <UButton
                      v-if="hasActiveFilters"
                      icon="tabler:filter-x"
                      color="neutral"
                      variant="ghost"
                      size="sm"
                      @click="clearAllFilters()"
                    >
                      {{ $t('ui.action.filter.clearAll') }}
                    </UButton>

                    <UButton
                      color="neutral"
                      variant="soft"
                      size="sm"
                      class="justify-center"
                      :icon="
                        areAllFilteredPhotosSelected
                          ? 'tabler:checkbox'
                          : areSomeFilteredPhotosSelected
                            ? 'tabler:minus'
                            : 'tabler:square'
                      "
                      @click="toggleAllFilteredPhotos"
                    >
                      {{ $t('dashboard.albums.modal.selectAll') }}
                    </UButton>

                    <div class="ml-auto flex flex-wrap items-center gap-1.5">
                      <UBadge
                        color="primary"
                        variant="soft"
                      >
                        {{
                          $t('dashboard.albums.modal.selectedPhotos', {
                            count: draftSelectedPhotoIds.length,
                          })
                        }}
                      </UBadge>
                      <UBadge
                        v-if="draftCoverPhotoId"
                        color="warning"
                        variant="soft"
                        icon="tabler:star-filled"
                      >
                        {{ $t('dashboard.albums.modal.coverSetInfo') }}
                      </UBadge>
                    </div>
                  </div>

                  <p
                    class="hidden items-center gap-1.5 text-xs text-neutral-500 pointer-fine:flex dark:text-neutral-400"
                  >
                    <Icon
                      name="tabler:info-circle"
                      class="size-3.5 shrink-0"
                    />
                    {{ $t('ui.action.select.rangeTip') }}
                  </p>

                  <div class="flex flex-wrap gap-1">
                    <UBadge
                      v-if="selectedCounts.tags"
                      size="xs"
                      color="neutral"
                      variant="outline"
                    >
                      {{ $t('ui.action.filter.tabs.tags') }}:
                      {{ selectedCounts.tags }}
                    </UBadge>
                    <UBadge
                      v-if="selectedCounts.cameras"
                      size="xs"
                      color="neutral"
                      variant="outline"
                    >
                      {{ $t('ui.action.filter.tabs.cameras') }}:
                      {{ selectedCounts.cameras }}
                    </UBadge>
                    <UBadge
                      v-if="selectedCounts.lenses"
                      size="xs"
                      color="neutral"
                      variant="outline"
                    >
                      {{ $t('ui.action.filter.tabs.lenses') }}:
                      {{ selectedCounts.lenses }}
                    </UBadge>
                    <UBadge
                      v-if="selectedCounts.cities"
                      size="xs"
                      color="neutral"
                      variant="outline"
                    >
                      {{ $t('ui.action.filter.tabs.cities') }}:
                      {{ selectedCounts.cities }}
                    </UBadge>
                    <UBadge
                      v-if="selectedCounts.ratings"
                      size="xs"
                      color="neutral"
                      variant="outline"
                    >
                      {{ $t('ui.action.filter.tabs.ratings') }}
                    </UBadge>
                  </div>

                  <UCard
                    variant="subtle"
                    :ui="{
                      body: 'p-2 sm:p-2',
                    }"
                  >
                    <div>
                      <div
                        v-if="selectedPhotosPreview.length > 0"
                        class="flex h-full items-center gap-2 overflow-x-auto"
                      >
                        <button
                          v-for="photo in selectedPhotosPreview"
                          :key="photo.id"
                          class="relative h-12 w-12 shrink-0 overflow-hidden rounded-md border-2 transition"
                          :class="
                            draftCoverPhotoId === photo.id
                              ? 'border-warning-500'
                              : 'border-transparent hover:border-gray-300 dark:hover:border-neutral-500'
                          "
                          @click="setDraftCoverPhoto(photo.id)"
                        >
                          <ThumbImage
                            :src="photo.thumbnailUrl || ''"
                            :alt="photo.title || $t('ui.photo.altFallback')"
                            class="h-full w-full object-cover"
                          />
                          <div
                            v-if="draftCoverPhotoId === photo.id"
                            class="absolute inset-x-0 bottom-0 flex items-center justify-center bg-warning-500/90 py-0.5"
                          >
                            <Icon
                              name="tabler:star-filled"
                              size="12"
                              class="text-white"
                            />
                          </div>
                        </button>

                        <UBadge
                          v-if="selectedPhotosOverflowCount > 0"
                          variant="soft"
                          color="neutral"
                          class="shrink-0"
                        >
                          +{{ selectedPhotosOverflowCount }}
                        </UBadge>
                      </div>
                      <div
                        v-else
                        class="flex h-full items-center"
                      >
                        <div
                          class="flex h-12 w-full items-center gap-2 rounded-md border border-dashed border-gray-300 bg-gray-50/80 px-3 text-xs text-gray-600 dark:border-neutral-700 dark:bg-neutral-800/60 dark:text-gray-300"
                        >
                          <Icon
                            name="tabler:photo-plus"
                            size="14"
                            class="shrink-0 text-gray-500 dark:text-gray-400"
                          />
                          <span class="truncate">
                            {{ $t('dashboard.albums.form.selectPhotos') }}
                          </span>
                        </div>
                      </div>
                    </div>
                  </UCard>
                </div>
              </div>

              <div class="flex-1 overflow-y-auto p-3 sm:p-5">
                <div
                  v-if="selectorFilteredPhotos.length > 0"
                  class="grid grid-cols-3 gap-1.5 sm:grid-cols-4 sm:gap-2 lg:grid-cols-5 xl:grid-cols-6"
                >
                  <button
                    v-for="photo in selectorFilteredPhotos"
                    :key="photo.id"
                    class="group text-left rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-400/60"
                    @mousedown="
                      selectorRangeSelection.preventShiftTextSelection
                    "
                    @click="toggleDraftPhotoSelection(photo.id, $event)"
                  >
                    <div
                      class="relative aspect-square overflow-hidden rounded-lg border bg-gray-200/90 transition-all duration-200 dark:bg-neutral-700/80"
                      :class="{
                        'border-primary-400 ring-1 ring-primary-300/60 dark:ring-primary-700/50':
                          draftSelectedPhotoIds.includes(photo.id),
                        'border-gray-200/70 hover:border-gray-300/90 dark:border-neutral-700 dark:hover:border-neutral-500':
                          !draftSelectedPhotoIds.includes(photo.id),
                      }"
                    >
                      <ThumbImage
                        :src="photo.thumbnailUrl || ''"
                        :alt="photo.title || $t('ui.photo.altFallback')"
                        class="h-full w-full object-cover transition-transform duration-300 group-hover:scale-[1.02]"
                      />

                      <div
                        class="absolute inset-x-0 bottom-0 h-10 bg-linear-to-t from-black/55 via-black/10 to-transparent"
                      />

                      <div
                        class="absolute inset-x-2 bottom-1.5 flex items-end justify-between gap-2"
                      >
                        <div class="min-w-0">
                          <p
                            class="truncate text-[10px] font-medium text-white/92"
                          >
                            {{
                              photo.title ||
                              photo.storageKey ||
                              $t('ui.photo.untitled')
                            }}
                          </p>
                          <p class="truncate text-[9px] text-white/72">
                            {{
                              photo.city
                                ? `${photo.city} · ${dayjs(photo.createdAt).format('MM-DD')}`
                                : dayjs(photo.createdAt).format('YYYY-MM-DD')
                            }}
                          </p>
                        </div>
                      </div>

                      <div
                        v-if="draftSelectedPhotoIds.includes(photo.id)"
                        class="absolute left-1.5 top-1.5 flex items-center gap-1"
                      >
                        <div
                          class="flex h-5 min-w-5 items-center justify-center rounded-full border border-white/85 bg-primary-500 px-1 text-white shadow-sm"
                        >
                          <span
                            v-if="getDraftPhotoOrder(photo.id)"
                            class="text-[10px] font-semibold"
                          >
                            {{ getDraftPhotoOrder(photo.id) }}
                          </span>
                          <Icon
                            v-else
                            name="tabler:check"
                            size="14"
                          />
                        </div>

                        <!-- The badge above is the album position; these step
                             the photo one place earlier or later in it. -->
                        <div
                          class="flex gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                        >
                          <button
                            type="button"
                            class="flex h-5 w-5 cursor-pointer items-center justify-center rounded-full border border-white/70 bg-black/55 text-white transition hover:bg-black/80 disabled:cursor-not-allowed disabled:opacity-35"
                            :title="
                              $t('dashboard.albums.form.movePhotoEarlier')
                            "
                            :disabled="getDraftPhotoOrder(photo.id) === 1"
                            @click.stop="moveDraftPhoto(photo.id, -1)"
                          >
                            <Icon
                              name="tabler:chevron-left"
                              size="12"
                            />
                          </button>
                          <button
                            type="button"
                            class="flex h-5 w-5 cursor-pointer items-center justify-center rounded-full border border-white/70 bg-black/55 text-white transition hover:bg-black/80 disabled:cursor-not-allowed disabled:opacity-35"
                            :title="$t('dashboard.albums.form.movePhotoLater')"
                            :disabled="
                              getDraftPhotoOrder(photo.id) ===
                              draftSelectedPhotoIds.length
                            "
                            @click.stop="moveDraftPhoto(photo.id, 1)"
                          >
                            <Icon
                              name="tabler:chevron-right"
                              size="12"
                            />
                          </button>
                        </div>
                      </div>

                      <UButton
                        v-if="draftCoverPhotoId !== photo.id"
                        size="xs"
                        color="warning"
                        variant="solid"
                        icon="tabler:star"
                        class="absolute right-1.5 top-1.5 opacity-100 transition sm:opacity-0 sm:group-hover:opacity-100"
                        @click.stop="setDraftCoverPhoto(photo.id)"
                      />

                      <UBadge
                        v-else
                        color="warning"
                        variant="solid"
                        icon="tabler:star-filled"
                        class="absolute right-1.5 top-1.5"
                      >
                        {{ $t('dashboard.albums.modal.setCover') }}
                      </UBadge>
                    </div>
                  </button>
                </div>

                <div
                  v-else
                  class="flex h-64 flex-col items-center justify-center text-gray-500"
                >
                  <Icon
                    name="tabler:image-off"
                    size="48"
                    class="mb-3 opacity-50"
                  />
                  <p class="font-medium">
                    {{
                      hasActiveFilters
                        ? $t('dashboard.albums.modal.noResults')
                        : $t('dashboard.albums.modal.noPhotos')
                    }}
                  </p>
                  <p
                    v-if="hasActiveFilters"
                    class="mt-1 text-sm"
                  >
                    {{ $t('dashboard.albums.modal.tryOtherKeywords') }}
                  </p>
                </div>
              </div>

              <div
                class="shrink-0 border-t border-gray-200 bg-white p-3 dark:border-neutral-700 dark:bg-neutral-900 sm:p-4"
              >
                <div
                  class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-end"
                >
                  <UButton
                    variant="outline"
                    color="neutral"
                    class="w-full sm:w-auto"
                    @click="closePhotoSelector"
                  >
                    {{ $t('dashboard.albums.slideover.cancel') }}
                  </UButton>
                  <UButton
                    icon="tabler:check"
                    color="primary"
                    class="w-full sm:w-auto"
                    @click="confirmPhotoSelection"
                  >
                    {{
                      $t('dashboard.albums.modal.confirm', {
                        count: draftSelectedPhotoIds.length,
                      })
                    }}
                  </UButton>
                </div>
              </div>
            </div>
          </template>
        </UModal>

        <UModal v-model:open="isDeleteConfirmOpen">
          <template #content>
            <div class="p-6 space-y-4">
              <div class="flex items-center gap-3">
                <div
                  class="shrink-0 w-10 h-10 bg-error-100 dark:bg-error-900/30 rounded-full flex items-center justify-center"
                >
                  <Icon
                    name="tabler:alert-circle"
                    class="text-error-500"
                  />
                </div>
                <div>
                  <h3 class="text-lg font-semibold">
                    {{ $t('dashboard.albums.delete.title') }}
                  </h3>
                  <p class="text-sm text-gray-600 dark:text-gray-400 mt-1">
                    {{
                      $t('dashboard.albums.delete.message', {
                        title: currentAlbum?.title,
                      })
                    }}
                  </p>
                </div>
              </div>

              <div class="flex justify-end gap-2 pt-4">
                <UButton
                  variant="ghost"
                  color="neutral"
                  @click="isDeleteConfirmOpen = false"
                >
                  {{ $t('dashboard.albums.delete.cancel') }}
                </UButton>
                <UButton
                  color="error"
                  icon="tabler:trash"
                  @click="deleteAlbum"
                >
                  {{ $t('dashboard.albums.delete.confirm') }}
                </UButton>
              </div>
            </div>
          </template>
        </UModal>
      </div>
    </template>
  </UDashboardPanel>
</template>

<style scoped>
/* 卡片换位的时候滑过去，不要「啪」地跳一下。<TransitionGroup> 会把这一
 * 条挂到位置变了的卡片上，过渡读完再摘掉。 */
.album-card-move {
  transition: transform 220ms cubic-bezier(0.2, 0, 0, 1);
}

/* 拖动当中不做过场。指针落在哪张卡片上是按卡片量出来的矩形判的，而过场期间
 * 量到的是半路上的坐标——卡片正从旧格子滑向新格子，中间会盖住指针，于是又
 * 被判成「要挪到这张」，两张卡片来回跳。让它在拖动中直接落位，量到什么就是
 * 什么。松手之后（不在拖动中）换位照旧滑过去。 */
.album-grid-dragging .album-card-move {
  transition: none;
}

@media (prefers-reduced-motion: reduce) {
  .album-card-move {
    transition: none;
  }
}
</style>
