<script lang="ts" setup>
import { onMounted, onUnmounted, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    photo: Photo
    index: number
    animate?: boolean
    animationDelay?: number
  }>(),
  {
    animate: false,
    animationDelay: 0,
  },
)

const emit = defineEmits<{
  animationComplete: []
  'visibility-change': [
    { index: number; isVisible: boolean; date: string | Date },
  ]
  openViewer: [number]
}>()

const root = ref<HTMLElement>()
const isNearViewport = ref(false)
let observer: IntersectionObserver | undefined

const aspectRatio = computed(() => {
  if (props.photo.aspectRatio) return props.photo.aspectRatio
  if (props.photo.width && props.photo.height) {
    return props.photo.width / props.photo.height
  }
  return 1.2
})

onMounted(() => {
  if (typeof IntersectionObserver === 'undefined') {
    isNearViewport.value = true
    return
  }

  observer = new IntersectionObserver(
    ([entry]) => {
      isNearViewport.value = Boolean(entry?.isIntersecting)
    },
    {
      // Keep a modest render-ahead window so fast scrolling does not reveal
      // empty placeholders, while distant cards stay out of the DOM.
      rootMargin: '600px 0px',
      threshold: 0,
    },
  )
  observer.observe(root.value!)
})

onUnmounted(() => observer?.disconnect())
</script>

<template>
  <div
    ref="root"
    class="w-full"
    :style="{ aspectRatio }"
  >
    <MasonryItem
      v-if="isNearViewport"
      :photo="photo"
      :index="index"
      :animate="animate"
      :animation-delay="animationDelay"
      @animation-complete="emit('animationComplete')"
      @visibility-change="emit('visibility-change', $event)"
      @open-viewer="emit('openViewer', $event)"
    />
  </div>
</template>

<style scoped></style>
