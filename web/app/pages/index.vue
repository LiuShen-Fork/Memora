<script lang="ts" setup>
// 首页不放布局：照片墙是整屏容器，相册列表自己滚动，两种首页的外壳不一样，
// 所以由页面自己决定画哪一个。
definePageMeta({ layout: false })

useHead({ title: '' })

const settings = useSettingsStore()
const homeView = useSettingRef('app:homeView')

// 设置到达之前不知道首页是哪一种，先给一个等待画面，
// 免得先画照片墙再跳成相册。
const ready = computed(
  () => settings.isInitialized || Boolean(settings.error),
)
const showsAlbums = computed(() => ready.value && homeView.value === 'albums')
</script>

<template>
  <AlbumListing
    v-if="showsAlbums"
    home
  />
  <GalleryWall v-else-if="ready" />
  <div
    v-else
    class="fixed inset-0 flex items-center justify-center"
  >
    <LoaderAvatar />
  </div>
</template>
