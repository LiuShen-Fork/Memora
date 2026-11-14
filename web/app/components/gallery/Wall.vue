<script lang="ts" setup>
// 照片墙的整屏外壳。查看器页面把它当背景（布局里的 slot 叠在上面），
// 首页则直接在它和相册列表之间二选一。
const { photos } = usePhotos()
</script>

<template>
  <div class="relative h-screen">
    <div class="h-svh">
      <ClientOnly>
        <MasonryRoot
          :photos="photos"
          columns="auto"
        />
        <template #fallback>
          <div
            class="fixed inset-0 flex flex-col gap-4 items-center justify-center"
          >
            <LoaderAvatar />
            <span class="loading-scan-wrapper">
              <span class="text-base font-medium loading-scan-text">
                {{ $t('ui.loading') }}
              </span>
            </span>
          </div>
        </template>
      </ClientOnly>
      <slot />
    </div>
  </div>
</template>

<style scoped>
.loading-scan-wrapper {
  display: inline-block;
  position: relative;
}

.loading-scan-text {
  display: inline-block;
  background: linear-gradient(
    90deg,
    var(--ui-text-highlighted) 0%,
    var(--ui-text-highlighted) 30%,
    var(--ui-text-muted) 50%,
    var(--ui-text-highlighted) 70%,
    var(--ui-text-highlighted) 100%
  );
  background-size: 200% 100%;
  background-position: 200% 0;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  color: transparent;
  animation: scan-x-text 1.2s linear infinite;
}

@keyframes scan-x-text {
  0% {
    background-position: 200% 0;
  }
  100% {
    background-position: -200% 0;
  }
}
</style>
