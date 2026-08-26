import { WebGLImageViewer } from '@memora/webgl-image'
import '@memora/webgl-image/style'

export default defineNuxtPlugin((nuxtApp) => {
  nuxtApp.vueApp.component('WebGLImageViewer', WebGLImageViewer)
})
