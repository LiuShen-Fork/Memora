<script lang="ts" setup>
// 首页要一层空布局：照片墙是整屏容器，相册列表自己滚动，两种首页的外壳不一样，
// 所以外壳由页面自己画。但这层布局不能省成 `layout: false`——从没有布局的页面
// 跳到有布局的页面（比如 /dashboard）时，Nuxt 会把旧页面拆两遍，第二遍里已经
// 从 DOM 上摘掉的 teleport 内容再去遍历就抛出 nextSibling 的 TypeError。
definePageMeta({ layout: 'blank' })

useHead({ title: '' })

const settings = useSettingsStore()
const homeView = useSettingRef('app:homeView')

// 设置到达之前不知道首页是哪一种，先给一个等待画面，
// 免得先画照片墙再跳成相册。
const ready = computed(() => settings.isInitialized || Boolean(settings.error))
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
