<script lang="ts" setup>
import type { AsyncDataRequestStatus } from '#app'

const props = defineProps<{
  photos: Photo[]
  refresh: () => Promise<void>
  loadMore: () => Promise<void>
  ensurePhoto: (id: string) => Promise<void>
  hasMore: boolean
  isLoadingMore: boolean
  status: AsyncDataRequestStatus
  totalCount?: number
}>()

const photosRef = toRef(props, 'photos')
const status = toRef(props, 'status')
const refresh = props.refresh
providePhotos(photosRef, status, refresh, toRef(props, 'totalCount'), {
  loadMore: props.loadMore,
  ensurePhoto: props.ensurePhoto,
  hasMore: toRef(props, 'hasMore'),
  isLoadingMore: toRef(props, 'isLoadingMore'),
})
</script>

<template>
  <slot />
</template>

<style scoped></style>
