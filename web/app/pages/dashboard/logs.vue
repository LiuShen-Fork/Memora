<script setup lang="ts">
definePageMeta({
  layout: 'dashboard',
})

/**
 * One line of the log, in the three parts the server gives it in.
 *
 * The server writes `2026-10-18T03:44:18.231+08:00 [queue] processed photo …`,
 * and a line that came from somewhere else — a log file written by an older
 * version, say — has no stamp. Such a line keeps its text and is shown without
 * a time rather than with the time the page happened to open.
 */
interface LogEntry {
  time: string
  scope: string
  message: string
  messageLower: string
  raw: string
}

const STAMPED_LINE =
  /^(\d{4}-\d{2}-\d{2}T[\d:.]+(?:Z|[+-]\d{2}:?\d{2}))\s+\[([^\]]*)\]\s*([\s\S]*)$/
const SCOPED_LINE = /^\[([^\]]*)\]\s*([\s\S]*)$/

const logs = ref<LogEntry[]>([])
const searchQuery = ref('')
const selectedScopes = ref<string[]>([])
const autoScroll = ref(true)
const isConnected = ref(false)
const connectionState = ref<
  'idle' | 'connecting' | 'loadingHistory' | 'live' | 'error'
>('idle')
const logContainer = ref<HTMLElement>()
const isInitialLoading = ref(false)
const loadingProgress = ref(0)
const normalizedSearchQuery = computed(() =>
  searchQuery.value.trim().toLowerCase(),
)
const scrollTop = ref(0)
const containerHeight = ref(0)

// 批处理队列
const batchQueue = ref<LogEntry[]>([])
const isBatchProcessing = ref(false)
const BATCH_SIZE = 100 // 每批处理的日志条数
const BATCH_DELAY = 8 // 每批处理间隔（毫秒）
const INITIAL_LOG_LINES = 'all'
const ROW_HEIGHT = 28
const VIRTUAL_OVERSCAN = 20
const VIRTUAL_BOTTOM_PADDING = 8

// 一行日志的高度。
//
// 窄屏上时间与来源挪到正文上面一行（见模板），一条日志于是占两行。虚拟滚动
// 是按固定行高算位置的，而这个高度要跟着布局走，所以下面那一串计算都读
// rowHeight，不再直接读 ROW_HEIGHT。
const ROW_HEIGHT_NARROW = 46
const isNarrow = useMediaQuery('(max-width: 640px)')
const rowHeight = computed(() => (isNarrow.value ? ROW_HEIGHT_NARROW : ROW_HEIGHT))

let resizeObserver: ResizeObserver | null = null

// EventSource 连接
let eventSource: EventSource | null = null

// 批处理添加日志
const addLogEntry = (logEntry: LogEntry) => {
  batchQueue.value.push(logEntry)
  if (!isBatchProcessing.value) {
    processBatch()
  }
}

// 处理批队列
const processBatch = async () => {
  if (isBatchProcessing.value || batchQueue.value.length === 0) return

  isBatchProcessing.value = true

  while (batchQueue.value.length > 0) {
    const batch = batchQueue.value.splice(0, BATCH_SIZE)
    logs.value.push(...batch)

    // 更新加载进度（只在初始加载时显示）
    if (isInitialLoading.value) {
      loadingProgress.value = Math.min(
        95,
        loadingProgress.value + batch.length * 0.05,
      )
    }

    // 自动滚动到底部
    if (autoScroll.value) {
      await scrollToBottom()
    }

    // 如果还有更多批次，延迟处理以避免阻塞UI
    if (batchQueue.value.length > 0) {
      await new Promise((resolve) => setTimeout(resolve, BATCH_DELAY))
    }
  }

  isBatchProcessing.value = false

  // 注意：初始加载完成现在由消息超时检测控制，不在这里处理
}

/**
 * 拆一行日志：先是时间戳（可能没有），再是方括号里的来源，最后是正文。
 *
 * 这个顺序不是猜的，是后端 writing 的顺序：LogBuffer.Add 写的是
 * `<时间> [<来源>] <正文>`。认不出来的行整行当正文，宁可少一列，也不要凭空
 * 编出一个时间。
 */
const parseLogLine = (line: string): LogEntry => {
  const stamped = STAMPED_LINE.exec(line)
  const scoped = stamped ? null : SCOPED_LINE.exec(line)
  const message = stamped?.[3] ?? scoped?.[2] ?? line
  return {
    time: stamped?.[1] ?? '',
    scope: stamped?.[2] ?? scoped?.[1] ?? '',
    message,
    messageLower: message.toLowerCase(),
    raw: line,
  }
}

// 过滤后的日志
const filteredLogs = computed(() => {
  let filtered = logs.value

  // 按来源过滤
  if (selectedScopes.value.length > 0) {
    const selected = new Set(selectedScopes.value)
    filtered = filtered.filter((log) => selected.has(log.scope))
  }

  // 按搜索词过滤
  if (normalizedSearchQuery.value) {
    const query = normalizedSearchQuery.value
    filtered = filtered.filter((log) => {
      return log.messageLower.includes(query) || log.scope.includes(query)
    })
  }

  return filtered
})

const availableScopes = computed(() => {
  const scopes = new Set<string>()
  for (const log of logs.value) {
    if (log.scope) {
      scopes.add(log.scope)
    }
  }

  return Array.from(scopes)
    .sort((a, b) => a.localeCompare(b))
    .map((scope) => ({
      label: scope,
      value: scope,
    }))
})

const totalVirtualHeight = computed(
  () => filteredLogs.value.length * rowHeight.value + VIRTUAL_BOTTOM_PADDING,
)

const virtualStart = computed(() => {
  const start = Math.floor(scrollTop.value / rowHeight.value) - VIRTUAL_OVERSCAN
  return Math.max(0, start)
})

const virtualEnd = computed(() => {
  const visibleCount =
    Math.ceil(containerHeight.value / rowHeight.value) + VIRTUAL_OVERSCAN * 2
  return Math.min(
    filteredLogs.value.length,
    virtualStart.value + Math.max(visibleCount, 1),
  )
})

const virtualOffset = computed(() => virtualStart.value * rowHeight.value)

const visibleLogs = computed(() => {
  return filteredLogs.value.slice(virtualStart.value, virtualEnd.value)
})

/**
 * 把一行正文按搜索词切成片段，命中的那几段标记出来。
 *
 * 切分而不是替换成一段 HTML：日志正文里有文件名、相册名，谁都能往里写一个
 * `<`，而 v-html 会把它们当标记渲染。这里是模板自己在插值，没有这回事。
 */
function highlightSegments(text: string): { text: string; hit: boolean }[] {
  const query = normalizedSearchQuery.value
  if (!query) return [{ text, hit: false }]

  const lower = text.toLowerCase()
  const segments: { text: string; hit: boolean }[] = []
  let at = 0
  for (;;) {
    const hit = lower.indexOf(query, at)
    if (hit < 0) break
    if (hit > at) segments.push({ text: text.slice(at, hit), hit: false })
    segments.push({ text: text.slice(hit, hit + query.length), hit: true })
    at = hit + query.length
  }
  if (at < text.length) segments.push({ text: text.slice(at), hit: false })
  return segments.length ? segments : [{ text, hit: false }]
}

const getConnectionStatusColor = ():
  | 'error'
  | 'info'
  | 'success'
  | 'primary'
  | 'secondary'
  | 'warning'
  | 'neutral' => {
  if (connectionState.value === 'live') {
    return 'success'
  }
  if (
    connectionState.value === 'connecting' ||
    connectionState.value === 'loadingHistory'
  ) {
    return 'info'
  }
  if (connectionState.value === 'error') {
    return 'error'
  }
  return 'warning'
}

// 切换自动滚动
const toggleAutoScroll = () => {
  autoScroll.value = !autoScroll.value
  if (autoScroll.value) {
    scrollToBottom()
  }
}

// 滚动到底部
const scrollToBottom = async () => {
  await nextTick()
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight
  }
}

// 处理滚动事件
const handleScroll = () => {
  if (!logContainer.value) return

  const {
    scrollTop: currentScrollTop,
    scrollHeight,
    clientHeight,
  } = logContainer.value
  scrollTop.value = currentScrollTop
  containerHeight.value = clientHeight
  const isNearBottom = currentScrollTop + clientHeight >= scrollHeight - 50 // 距离底部50px以内
  const isAtTop = currentScrollTop + clientHeight < scrollHeight - 200 // 距离底部200px以上

  // 如果滚动到接近底部，自动开启自动滚动
  if (isNearBottom && !autoScroll.value) {
    autoScroll.value = true
  }
  // 如果用户手动滚动到较高位置，暂停自动滚动
  else if (isAtTop && autoScroll.value) {
    autoScroll.value = false
  }
}

// 连接日志流
const connectLogStream = () => {
  if (eventSource) {
    eventSource.close()
  }

  // 重置状态
  logs.value = []
  batchQueue.value = []
  isInitialLoading.value = true
  loadingProgress.value = 5

  connectionState.value = 'connecting'
  eventSource = new EventSource(`/api/system/logs?initial=${INITIAL_LOG_LINES}`)

  let initialLoadCompleteTimer: NodeJS.Timeout | null = null
  const MESSAGE_TIMEOUT = 2000 // 消息间隔超时时间（毫秒）

  eventSource.onopen = () => {
    isConnected.value = true
    // 浏览器的 EventSource 会自行重连，服务端也会从上次中断处继续发送，
    // 因此重连时不需要重新加载历史，只需要把状态切回已连接
    connectionState.value = isInitialLoading.value ? 'loadingHistory' : 'live'
  }

  eventSource.onmessage = (event) => {
    const logLine = event.data
    if (logLine && logLine.trim()) {
      const logEntry = parseLogLine(logLine)

      // 清除之前的定时器
      if (initialLoadCompleteTimer) {
        clearTimeout(initialLoadCompleteTimer)
        initialLoadCompleteTimer = null
      }

      if (isInitialLoading.value) {
        addLogEntry(logEntry)
        loadingProgress.value = Math.min(90, loadingProgress.value + 0.2)

        // 设置新的定时器，如果在指定时间内没有新消息，认为初始加载完成
        initialLoadCompleteTimer = setTimeout(() => {
          if (isInitialLoading.value) {
            connectionState.value = 'live'
            // 让加载指示器显示完成状态后再隐藏
            setTimeout(() => {
              isInitialLoading.value = false
              loadingProgress.value = 100
              autoScroll.value = true
              scrollToBottom()
            }, 500)
          }
        }, MESSAGE_TIMEOUT)
      } else {
        // 实时日志直接添加
        addLogEntry(logEntry)
      }
    }
  }

  eventSource.onerror = (error) => {
    isConnected.value = false
    connectionState.value = 'error'
    console.error('EventSource error:', error)
  }
}

watch(
  [selectedScopes, searchQuery],
  () => {
    if (autoScroll.value) {
      scrollToBottom()
    }
  },
  { deep: true },
)

onMounted(() => {
  if (logContainer.value) {
    containerHeight.value = logContainer.value.clientHeight
  }

  if (typeof ResizeObserver !== 'undefined' && logContainer.value) {
    resizeObserver = new ResizeObserver((entries) => {
      const [entry] = entries
      if (!entry) return
      containerHeight.value = entry.contentRect.height
    })
    resizeObserver.observe(logContainer.value)
  }

  connectLogStream()
})

onUnmounted(() => {
  if (eventSource) {
    eventSource.close()
  }
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
})
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <UDashboardNavbar :title="$t('title.logs')" />
    </template>

    <template #body>
      <div
        class="relative flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-default bg-default"
      >
        <div
          class="flex flex-wrap items-center gap-x-2 gap-y-2 border-b border-default px-4 py-3"
        >
          <Icon
            name="tabler:file-text"
            class="size-4 shrink-0 text-dimmed"
          />
          <span class="font-mono text-xs text-toned">app.log</span>
          <UBadge
            v-if="connectionState !== 'idle'"
            size="sm"
            variant="subtle"
            :color="getConnectionStatusColor()"
          >
            {{ $t('dashboard.logs.connectionStatus.' + connectionState) }}
          </UBadge>
          <span class="text-xs text-dimmed">
            {{
              $t('dashboard.logs.countLabel', {
                total: logs.length,
                shown: filteredLogs.length,
              })
            }}
          </span>

          <div class="ms-auto flex flex-wrap items-center gap-2">
            <!-- Search -->
            <UInput
              v-model="searchQuery"
              :placeholder="$t('dashboard.logs.search.placeholder')"
              size="sm"
              class="w-44 sm:w-56"
              icon="tabler:search"
              :ui="{ trailing: 'pe-1' }"
            >
              <template
                v-if="searchQuery?.length"
                #trailing
              >
                <UButton
                  color="neutral"
                  variant="link"
                  size="sm"
                  icon="tabler:x"
                  :aria-label="$t('dashboard.logs.search.clearAriaLabel')"
                  @click="searchQuery = ''"
                />
              </template>
            </UInput>
            <!-- Scope -->
            <USelect
              v-model="selectedScopes"
              :items="availableScopes"
              multiple
              size="sm"
              :placeholder="$t('dashboard.logs.filter.scopePlaceholder')"
              class="w-36 sm:w-44"
              :clearable="false"
            />
            <!-- Auto scroll -->
            <UTooltip
              :text="$t('dashboard.logs.autoScroll.tooltip')"
              ignore-non-keyboard-focus
            >
              <UButton
                :icon="autoScroll ? 'tabler:arrow-bar-to-down' : 'tabler:arrow-bar-down'"
                color="neutral"
                size="sm"
                :variant="autoScroll ? 'soft' : 'outline'"
                :aria-label="$t('dashboard.logs.autoScroll.tooltip')"
                @click="toggleAutoScroll"
              />
            </UTooltip>
          </div>
        </div>

        <!-- 加载进度指示器 -->
        <div
          v-if="isInitialLoading"
          class="absolute inset-0 z-10 flex items-center justify-center bg-default/80 backdrop-blur-sm"
        >
          <div class="text-center">
            <div class="mb-4">
              <Icon
                name="tabler:loader-2"
                class="size-8 animate-spin text-primary"
              />
            </div>
            <div class="mb-2 text-sm text-muted">
              {{ $t('dashboard.logs.connectionStatus.loadingHistory') }}
            </div>
            <div class="h-1.5 w-64 overflow-hidden rounded-full bg-elevated">
              <div
                class="h-full rounded-full bg-primary transition-all duration-300 ease-out"
                :style="{ width: `${loadingProgress}%` }"
              />
            </div>
            <div class="mt-1 text-xs text-dimmed">
              {{ Math.round(loadingProgress) }}%
            </div>
          </div>
        </div>

        <div
          ref="logContainer"
          class="table-scroll relative min-h-0 flex-1 overflow-y-auto scroll-smooth font-mono"
          @scroll="handleScroll"
        >
          <div
            class="relative"
            :style="{ height: `${totalVirtualHeight}px` }"
          >
            <div
              class="absolute inset-x-0"
              :style="{ transform: `translateY(${virtualOffset}px)` }"
            >
              <div
                v-for="(log, index) in visibleLogs"
                :key="`${virtualStart + index}-${log.raw}`"
                class="flex gap-3 px-4 text-xs transition-colors hover:bg-elevated/60"
                :class="
                  isNarrow
                    ? 'flex-col justify-center gap-0.5'
                    : 'items-center'
                "
                :style="{ height: `${rowHeight}px` }"
              >
                <!-- 窄屏上时间与来源合成正文上面的一行。正文才是要读的东西，
                     而它在 390px 上只分得到一百来个像素——一行里挤不下第三
                     列。宽屏上这一层不占地方，里面的两格按原来的宽度并排。 -->
                <div class="flex shrink-0 items-center gap-3">
                  <!-- 时间 -->
                  <span
                    class="whitespace-nowrap tabular text-dimmed"
                    :class="isNarrow ? '' : 'w-[100px]'"
                  >
                    {{
                      log.time ? $dayjs(log.time).format('MM-DD HH:mm:ss') : ''
                    }}
                  </span>

                  <!-- 来源 -->
                  <span
                    v-if="log.scope"
                    class="truncate rounded bg-elevated px-1.5 py-0.5 text-center text-[11px] text-toned"
                    :class="isNarrow ? '' : 'w-[76px]'"
                  >
                    {{ log.scope }}
                  </span>
                  <span
                    v-else
                    :class="isNarrow ? 'hidden' : 'w-[76px]'"
                  />
                </div>

                <!-- 正文 -->
                <span
                  class="min-w-0 truncate text-default"
                  :class="isNarrow ? 'w-full' : 'flex-1'"
                  :title="log.message"
                >
                  <template
                    v-for="(segment, at) in highlightSegments(log.message)"
                    :key="at"
                  >
                    <mark
                      v-if="segment.hit"
                      class="rounded-xs bg-warning/30 px-0.5 text-inherit"
                    >{{ segment.text }}</mark
                    >
                    <template v-else>{{ segment.text }}</template>
                  </template>
                </span>
              </div>
            </div>

            <!-- 空状态 -->
            <div
              v-if="filteredLogs.length === 0"
              class="absolute inset-0 flex flex-col items-center justify-center gap-2 py-8 text-center text-xs text-muted"
            >
              <Icon
                name="tabler:file-off"
                class="size-6 text-dimmed"
              />
              <span>{{
                logs.length === 0
                  ? $t('dashboard.logs.empty.waiting')
                  : $t('dashboard.logs.empty.noMatch')
              }}</span>
            </div>
          </div>
        </div>
      </div>
    </template>
  </UDashboardPanel>
</template>
