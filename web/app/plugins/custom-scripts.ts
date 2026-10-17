import { parseScriptTags } from '~/utils/parseScriptTags'

/**
 * 将管理员配置的 <script> 标签注入到页面 <head> 和 <body> 中。
 *
 * 读取 settings store 中的 `analytics:headScripts` 与 `analytics:bodyScripts`，
 * 通过正则解析为 useHead 接受的 script 数组。
 *
 * 与 analytics-tracker.client.ts（Matomo）独立并行，互不干扰：
 *  - 本插件负责管理员自定义脚本（GA、百度统计、Plausible、Umami、JSON-LD 等）
 *  - Matomo 插件负责 runtimeConfig 配置的 Matomo 实例
 *
 * 响应式：当 settings 通过 refreshSettings() 更新时，computed 会重算，
 * useHead 自动同步 DOM。
 */
export default defineNuxtPlugin(() => {
  // 插件比 app.vue 早一步执行，而下面这两个 computed 在 useHead 注册的当下就会被
  // 求值——那时设置请求还没发出去，读到的只会是空值。先在这里把请求发起来，
  // store 会复用同一次请求（pendingSettingsRequest），不会因此多发一次。
  // 失败由 app.vue 统一记录，这里只是不让请求悬着。
  void useSettingsStore().initSettings().catch(() => {})

  const headScriptsRef = useSettingRef('analytics:headScripts')
  const bodyScriptsRef = useSettingRef('analytics:bodyScripts')

  useHead(
    computed(() => {
      // 防御 null：未初始化时 store 返回 null
      const headParsed = parseScriptTags(String(headScriptsRef.value ?? ''))
      const bodyParsed = parseScriptTags(String(bodyScriptsRef.value ?? ''))

      // head 脚本注入到 <head>
      const headScripts = headParsed.map((s, i) => ({
        ...s,
        tagPosition: 'head' as const,
        // key 帮助 Unhead 在响应式更新时正确 diff，避免重复注入
        key: `cf-analytics-head-${i}`,
      }))

      // body 脚本注入到 <body> 开头
      const bodyScripts = bodyParsed.map((s, i) => ({
        ...s,
        tagPosition: 'bodyOpen' as const,
        key: `cf-analytics-body-${i}`,
      }))

      return {
        script: [...headScripts, ...bodyScripts],
      }
    }),
  )
})
