<script setup lang="ts">
import { h, resolveComponent } from 'vue'
import type { TableColumn } from '@nuxt/ui'

const UButton = resolveComponent('UButton')

definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('dashboard.queue.title'),
})

const toast = useToast()

// 状态管理
const isLoading = ref(false)
const selectedTasks = ref<number[]>([])
const statusFilter = ref<string>('all')
const typeFilter = ref<string>('all')
const page = ref(1)
const pageSize = 20

// 数据获取
const { data: queueData, refresh: refreshQueue } = await useFetch(
  '/api/queue/task/list',
  {
    query: computed(() => ({
      page: page.value,
      pageSize,
      ...(statusFilter.value !== 'all' && { status: statusFilter.value }),
      ...(typeFilter.value !== 'all' && { type: typeFilter.value }),
    })),
  },
)

// 队列统计数据
const queueStats = computed(() => {
  if (!queueData.value?.data)
    return { pending: 0, processing: 0, completed: 0, failed: 0 }

  if (queueData.value.stats) {
    return {
      pending: queueData.value.stats.pending || 0,
      processing: queueData.value.stats['in-stages'] || 0,
      completed: queueData.value.stats.completed || 0,
      failed: queueData.value.stats.failed || 0,
    }
  }

  const stats = { pending: 0, processing: 0, completed: 0, failed: 0 }
  queueData.value.data.forEach((task) => {
    if (task.status === 'pending') stats.pending++
    else if (task.status === 'in-stages') stats.processing++
    else if (task.status === 'completed') stats.completed++
    else if (task.status === 'failed') stats.failed++
  })
  return stats
})

// 刷新数据
const refreshData = async () => {
  isLoading.value = true
  try {
    await refreshQueue()
    selectedTasks.value = []
  } finally {
    isLoading.value = false
  }
}
watch([statusFilter, typeFilter], () => {
  page.value = 1
})

// 队列的并发数由服务端设置保存，修改后立即调整正在运行的 worker，
// 因此积压的任务不需要重启就能开始处理。
const { data: concurrencyState, refresh: refreshConcurrency } = await useFetch<{
  concurrency: number
  min: number
  max: number
  workers: number
}>('/api/queue/concurrency')

const concurrencyDraft = ref(concurrencyState.value?.concurrency ?? 5)
const isSavingConcurrency = ref(false)
const concurrencyDirty = computed(
  () => concurrencyDraft.value !== concurrencyState.value?.concurrency,
)

watch(concurrencyState, (state) => {
  if (state) concurrencyDraft.value = state.concurrency
})

const saveConcurrency = async () => {
  if (isSavingConcurrency.value || !concurrencyDirty.value) return
  isSavingConcurrency.value = true
  try {
    await $fetch('/api/queue/concurrency', {
      method: 'PUT',
      body: { concurrency: concurrencyDraft.value },
    })
    await refreshConcurrency()
    toast.add({
      title: $t('dashboard.queue.concurrency.saved', {
        count: concurrencyDraft.value,
      }),
      color: 'success',
    })
  } catch (error: any) {
    console.error('Update queue concurrency failed:', error)
    toast.add({
      title: $t('dashboard.queue.messages.operationFailed'),
      description: error?.message,
      color: 'error',
    })
  } finally {
    isSavingConcurrency.value = false
  }
}

// 清理非活跃任务
const clearNonActiveTasks = async () => {
  try {
    isLoading.value = true
    const result = await $fetch('/api/queue/task/clear', {
      method: 'DELETE',
      query: {
        includeCompleted: 'true',
        includeFailed: 'true',
      },
    })

    toast.add({
      title: $t('dashboard.queue.messages.clearSuccess'),
      description: $t('dashboard.queue.messages.clearSuccessDescription', { count: result.deletedCount }),
      color: 'success',
    })

    await refreshData()
  } catch (error: any) {
    console.error('Clear tasks failed:', error)
    toast.add({
      title: $t('dashboard.queue.messages.operationFailed'),
      description: error?.message || $t('dashboard.queue.messages.clearFailed'),
      color: 'error',
    })
  } finally {
    isLoading.value = false
  }
}

// 重试单个任务
const retryTask = async (taskId: number) => {
  try {
    await $fetch('/api/queue/task/retry', {
      method: 'POST',
      body: { taskId },
    })

    toast.add({
      title: $t('dashboard.queue.messages.retrySuccess'),
      color: 'success',
    })

    await refreshData()
  } catch (error: any) {
    console.error('Retry task failed:', error)
    toast.add({
      title: $t('dashboard.queue.messages.operationFailed'),
      description: error?.message || $t('dashboard.queue.messages.retryFailed'),
      color: 'error',
    })
  }
}

// 批量重试失败任务
const retryAllFailedTasks = async () => {
  try {
    isLoading.value = true
    const result = await $fetch('/api/queue/task/retry-batch', {
      method: 'POST',
      body: { retryAll: true },
    })

    toast.add({
      title: $t('dashboard.queue.messages.batchRetrySuccess'),
      description: $t('dashboard.queue.messages.batchRetrySuccessDescription', { count: result.retriedCount }),
      color: 'success',
    })

    await refreshData()
  } catch (error: any) {
    console.error('Batch retry failed:', error)
    toast.add({
      title: $t('dashboard.queue.messages.operationFailed'),
      description: error?.message || $t('dashboard.queue.messages.batchRetryFailed'),
      color: 'error',
    })
  } finally {
    isLoading.value = false
  }
}

// 删除单个任务（仅适用于失败任务）
const deleteTask = async (taskId: number) => {
  try {
    await $fetch(`/api/queue/failed/${taskId}`, {
      method: 'DELETE',
    })

    toast.add({
      title: $t('dashboard.queue.messages.deleteSuccess'),
      color: 'success',
    })

    await refreshData()
  } catch (error: any) {
    console.error('Delete task failed:', error)
    toast.add({
      title: $t('dashboard.queue.messages.operationFailed'),
      description: error?.message || $t('dashboard.queue.messages.deleteFailed'),
      color: 'error',
    })
  }
}

// 获取状态颜色
const getStatusColor = (
  status: 'pending' | 'in-stages' | 'completed' | 'failed',
) => {
  switch (status) {
    case 'pending':
      return 'warning'
    case 'in-stages':
      return 'info'
    case 'completed':
      return 'success'
    case 'failed':
      return 'error'
    default:
      return 'neutral'
  }
}

// 状态选项
const statusOptions = computed(() => [
  { label: $t('dashboard.queue.filters.all'), value: 'all' },
  { label: $t('dashboard.queue.status.pending'), value: 'pending' },
  { label: $t('dashboard.queue.status.in-stages'), value: 'in-stages' },
  { label: $t('dashboard.queue.status.completed'), value: 'completed' },
  { label: $t('dashboard.queue.status.failed'), value: 'failed' },
])

// 类型选项
const typeOptions = computed(() => [
  { label: $t('dashboard.queue.filters.all'), value: 'all' },
  { label: $t('dashboard.queue.types.photo'), value: 'photo' },
  {
    label: $t('dashboard.queue.types.photo-metadata-update'),
    value: 'photo-metadata-update',
  },
  {
    label: $t('dashboard.queue.types.live-photo-video'),
    value: 'live-photo-video',
  },
  {
    label: $t('dashboard.queue.types.photo-reverse-geocoding'),
    value: 'photo-reverse-geocoding',
  },
  {
    label: $t('dashboard.queue.types.photo-erase-location'),
    value: 'photo-erase-location',
  },
])

// 全站对「窄屏」的说法就是这个断点，这里跟着用。
const isMobile = useMediaQuery('(max-width: 768px)')

// 展开行状态
const expanded = ref<Record<string, boolean>>({})

// 表格列定义
const columns = computed<TableColumn<any>[]>(() => [
  {
    id: 'expand',
    cell: ({ row }) =>
      h(UButton, {
        color: 'neutral',
        variant: 'ghost',
        icon: 'tabler:chevron-down',
        square: true,
        'aria-label': $t('dashboard.queue.table.expandAria'),
        ui: {
          leadingIcon: [
            'transition-transform',
            row.getIsExpanded() ? 'duration-200 rotate-180' : '',
          ],
        },
        onClick: () => row.toggleExpanded(),
      }),
    enableSorting: false,
    enableHiding: false,
  },
  {
    accessorKey: 'id',
    header: $t('dashboard.queue.table.id'),
  },
  {
    id: 'type',
    accessorFn: (row) => row.payload.type,
    header: $t('dashboard.queue.table.type'),
  },
  {
    accessorKey: 'status',
    header: $t('dashboard.queue.table.status'),
  },
  {
    accessorKey: 'attempts',
    header: $t('dashboard.queue.table.attempts'),
  },
  {
    accessorKey: 'priority',
    header: $t('dashboard.queue.table.priority'),
  },
  {
    accessorKey: 'statusStage',
    header: $t('dashboard.queue.table.stage'),
  },
  {
    accessorKey: 'createdAt',
    header: $t('dashboard.queue.table.createdAt'),
  },
  {
    id: 'actions',
    header: $t('dashboard.queue.table.actions'),
  },
].filter(
  // 窄屏上只留下认得出这个任务的那几列：类型、状态和操作。编号、优先级、阶段
  // 和创建时间硬挤进来只会把表撑出卡片边缘，排到看不见的地方去——编号在展开行
  // 里本来就有。
  (column) =>
    !isMobile.value ||
    ['expand', 'type', 'status', 'actions'].includes(
      String(column.id ?? column.accessorKey ?? ''),
    ),
))

/**
 * 窄屏上这几列各自占多宽。桌面上列宽交给内容，几百像素的窗口里得先分好：
 * 状态和操作是固定的两小块，类型拿去剩下的。三列都不带宽度的话，各占三分之一，
 * 类型那格就装不下"照片元数据更新"和后面那枚状态徽章了。
 */
const mobileColumnWidths: Record<string, string> = {
  expand: '36px',
  status: '86px',
  actions: '64px',
}

const sizedColumns = computed<TableColumn<any>[]>(() =>
  columns.value.map((column) => {
    const width = isMobile.value
      ? mobileColumnWidths[String(column.id ?? column.accessorKey ?? '')]
      : undefined
    if (!width) return column
    return { ...column, meta: { ...column.meta, style: { th: { width } } } }
  }),
)

// 表格自己的那身衣服也随宽度换：桌面上列宽随内容，窄屏上先分好再截断。
// 一格左右各留 2.5 而不是 3 个间距——九列攒下来差出三十多个像素，正是这张表
// 在一千像素宽上还要横着挪一点点的那点差额。
const tableUi = computed(() => ({
  root: 'table-scroll overflow-auto',
  base: isMobile.value ? 'min-w-full table-fixed' : 'min-w-full',
  thead: 'bg-muted whitespace-nowrap',
  tr: 'data-[selected=true]:bg-primary/5',
  th: 'px-2.5 py-2.5 text-xs font-medium text-muted',
  td: 'px-2.5 py-2.5 text-sm text-default',
  empty: 'py-12 text-sm text-muted',
}))

/**
 * 每十秒拉一次队列，标签页在后台时不动。
 *
 * 后台拉来的结果没有人看，而回到这个页面时本来就该是新的——所以定时器照常走，
 * 只是轮到它的时候先问一句页面还在不在。
 *
 * 这一轮不走 refreshData：那个会点亮"刷新"按钮上的转圈，于是每分钟六次，三个
 * 按钮一起转一下。定时刷新没有人在等它。
 */
const visibility = useDocumentVisibility()
useIntervalFn(
  () => {
    if (visibility.value === 'visible') void refreshQueue()
  },
  10000,
)
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <UDashboardNavbar :title="$t('dashboard.queue.title')">
        <template #right>
          <!-- 窄屏上只留图标：三颗按钮带着全称排不下，最后一颗会被推出屏幕。
               名字挪到 title 里，指上去还在。 -->
          <UButton
            icon="tabler:refresh"
            variant="soft"
            :loading="isLoading"
            :title="$t('dashboard.queue.actions.refresh')"
            @click="refreshData"
          >
            <span class="hidden sm:inline">
              {{ $t('dashboard.queue.actions.refresh') }}
            </span>
          </UButton>
          <UButton
            icon="tabler:refresh"
            color="warning"
            variant="soft"
            :loading="isLoading"
            :title="$t('dashboard.queue.actions.retryAll')"
            @click="retryAllFailedTasks"
          >
            <span class="hidden sm:inline">
              {{ $t('dashboard.queue.actions.retryAll') }}
            </span>
          </UButton>
          <UButton
            icon="tabler:trash"
            color="error"
            variant="soft"
            :loading="isLoading"
            :title="$t('dashboard.queue.actions.clearNonActive')"
            @click="clearNonActiveTasks"
          >
            <span class="hidden sm:inline">
              {{ $t('dashboard.queue.actions.clearNonActive') }}
            </span>
          </UButton>
        </template>
      </UDashboardNavbar>
    </template>

    <template #body>
      <div class="flex flex-col gap-6">
        <!-- 状态指示器 -->
        <div class="grid grid-cols-2 lg:grid-cols-4 gap-4">
          <DashboardIndicator
            :title="$t('dashboard.queue.indicator.pending')"
            icon="tabler:clock"
            tone="attention"
            :value="queueStats.pending"
          />
          <DashboardIndicator
            :title="$t('dashboard.queue.indicator.processing')"
            icon="tabler:loader"
            tone="accent"
            :value="queueStats.processing"
          />
          <DashboardIndicator
            :title="$t('dashboard.queue.indicator.completed')"
            icon="tabler:check"
            tone="positive"
            :value="queueStats.completed"
          />
          <DashboardIndicator
            :title="$t('dashboard.queue.indicator.failed')"
            icon="tabler:alert-triangle"
            tone="negative"
            :value="queueStats.failed"
          />
        </div>

        <!-- 筛选器和表格 -->
        <DashboardPanel
          :title="$t('dashboard.queue.taskListTitle')"
          flush
          :ui="{ header: 'flex-wrap gap-y-2 px-4 py-3 sm:px-4' }"
        >
          <template #actions>
            <div class="flex flex-wrap items-center justify-end gap-2">
                <!-- 并发数：队列同时处理多少个任务，改完立即生效 -->
                <UPopover :content="{ align: 'end' }">
                  <UButton
                    icon="tabler:adjustments-horizontal"
                    variant="soft"
                    color="neutral"
                    size="sm"
                  >
                    {{ $t('dashboard.queue.concurrency.button') }}
                    <span class="font-semibold tabular-nums">
                      {{ concurrencyState?.concurrency }}
                    </span>
                  </UButton>

                  <template #content>
                    <div class="w-72 space-y-4 p-1">
                      <div class="space-y-1">
                        <p
                          class="text-sm font-semibold text-highlighted"
                        >
                          {{ $t('dashboard.queue.concurrency.title') }}
                        </p>
                        <p
                          class="text-xs leading-relaxed text-muted"
                        >
                          {{ $t('dashboard.queue.concurrency.description') }}
                        </p>
                      </div>

                      <div class="space-y-2">
                        <USlider
                          v-model="concurrencyDraft"
                          :min="concurrencyState?.min ?? 1"
                          :max="concurrencyState?.max ?? 8"
                          :step="1"
                        />
                        <div
                          class="flex items-center justify-between text-xs text-muted"
                        >
                          <span>
                            {{
                              $t('dashboard.queue.concurrency.running', {
                                count: concurrencyState?.workers ?? 0,
                              })
                            }}
                          </span>
                          <span
                            class="font-semibold tabular-nums text-highlighted"
                          >
                            {{ concurrencyDraft }}
                          </span>
                        </div>
                      </div>

                      <UButton
                        block
                        size="sm"
                        :loading="isSavingConcurrency"
                        :disabled="!concurrencyDirty"
                        @click="saveConcurrency"
                      >
                        {{ $t('common.actions.save') }}
                      </UButton>
                    </div>
                  </template>
                </UPopover>

                <USelectMenu
                  v-model="statusFilter"
                  :items="statusOptions"
                  value-key="value"
                  size="sm"
                  variant="soft"
                  class="w-32"
                  :search-input="false"
                />
                <USelectMenu
                  v-model="typeFilter"
                  :items="typeOptions"
                  value-key="value"
                  size="sm"
                  variant="soft"
                  class="w-32"
                  :search-input="false"
                />
            </div>
          </template>

          <div>
            <!-- 任务表格 -->
            <UTable
              v-model:expanded="expanded"
              :data="queueData?.data || []"
              :columns="sizedColumns"
              :loading="isLoading"
              :empty-state="{
                icon: 'tabler:inbox',
                label: $t('dashboard.queue.messages.noTasks'),
              }"
              class="w-full"
              :ui="tableUi"
            >
              <!-- 任务类型：一列里一个颜色只说明"这是哪种任务"，而这张表里颜色已经
                   被状态用掉了，所以类型一律素色。 -->
              <template #type-cell="{ row }">
                <UBadge
                  :label="
                    $t(`dashboard.queue.types.${row.original.payload.type}`)
                  "
                  variant="soft"
                  color="neutral"
                  size="sm"
                />
              </template>

              <!-- 状态 -->
              <template #status-cell="{ row }">
                <div class="flex flex-col items-start gap-1">
                  <UBadge
                    :label="$t(`dashboard.queue.status.${row.original.status}`)"
                    variant="soft"
                    :color="getStatusColor(row.original.status)"
                    size="sm"
                  />
                  <!-- 任务正在等待重试延迟结束，说明回来的时间 -->
                  <span
                    v-if="row.original.availableAt"
                    class="inline-flex items-center gap-1 text-xs text-muted"
                  >
                    <UIcon
                      name="tabler:clock-hour-4"
                      class="size-3"
                    />
                    {{
                      $t('dashboard.queue.table.retryAt', {
                        time: $dayjs(row.original.availableAt).format(
                          'MM-DD HH:mm:ss',
                        ),
                      })
                    }}
                  </span>
                </div>
              </template>

              <!-- 尝试次数 -->
              <template #attempts-cell="{ row }">
                <span class="text-sm">
                  {{ row.original.attempts }}/{{ row.original.maxAttempts }}
                </span>
              </template>

              <!-- 处理阶段 -->
              <template #statusStage-cell="{ row }">
                <span
                  v-if="row.original.statusStage"
                  class="text-xs text-toned"
                >
                  {{ $t(`dashboard.queue.stages.${row.original.statusStage}`) }}
                </span>
                <span
                  v-else
                  class="text-xs text-dimmed"
                  >—</span
                >
              </template>

              <!-- 创建时间 -->
              <template #createdAt-cell="{ row }">
                <span class="text-sm">{{
                  $dayjs(row.original.createdAt).format('MM-DD HH:mm:ss')
                }}</span>
              </template>

              <!-- 操作按钮：只有失败的任务能重试，也只有失败的任务能单独删掉，
                   队列接口本来就只收这一种（DELETE /api/queue/failed/<id>）。 -->
              <template #actions-cell="{ row }">
                <div class="flex items-center gap-1">
                  <!-- 窄屏上这两颗只剩图标：一格里的两个词排出来比整列还宽，
                       表就被撑得横着滚动，为的是看两个谁都知道是什么的图标。 -->
                  <template v-if="row.original.status === 'failed'">
                    <UButton
                      icon="tabler:refresh"
                      size="xs"
                      variant="soft"
                      color="warning"
                      :square="isMobile"
                      :title="$t('dashboard.queue.buttons.retry')"
                      @click="retryTask(row.original.id)"
                    >
                      <span class="hidden sm:inline">
                        {{ $t('dashboard.queue.buttons.retry') }}
                      </span>
                    </UButton>
                    <UButton
                      icon="tabler:trash"
                      size="xs"
                      variant="ghost"
                      color="error"
                      :square="isMobile"
                      :title="$t('dashboard.queue.buttons.delete')"
                      @click="deleteTask(row.original.id)"
                    >
                      <span class="hidden sm:inline">
                        {{ $t('dashboard.queue.buttons.delete') }}
                      </span>
                    </UButton>
                  </template>
                  <span
                    v-else
                    class="text-xs text-dimmed"
                    >—</span
                  >
                </div>
              </template>

              <!-- 展开行内容 -->
              <template #expanded="{ row }">
                <div class="px-2">
                  <div class="space-y-3">
                    <!-- 任务ID和类型 -->
                    <div class="flex flex-wrap gap-4">
                      <div>
                        <p class="text-xs text-dimmed">{{ $t('dashboard.queue.table.detail.photoId') }}</p>
                        <p class="text-sm capitalize">
                          {{ row.original.payload.photoId || '-' }}
                        </p>
                      </div>
                      <div>
                        <p class="text-xs text-dimmed">
                          {{ $t('dashboard.queue.table.id') }}
                        </p>
                        <p class="font-mono text-sm">{{ row.original.id }}</p>
                      </div>
                      <div>
                        <p class="text-xs text-dimmed">
                          {{ $t('dashboard.queue.table.type') }}
                        </p>
                        <p class="text-sm capitalize">
                          {{ $t(`dashboard.queue.types.${row.original.payload.type}`) }}
                        </p>
                      </div>
                    </div>

                    <!-- 错误信息（仅在失败状态显示） -->
                    <div
                      v-if="
                        row.original.status === 'failed' &&
                        row.original.errorMessage
                      "
                      class="mt-3"
                    >
                      <p class="text-xs text-dimmed mb-1">
                        {{ $t('dashboard.queue.table.errorMessage') }}
                      </p>
                      <div
                        class="rounded border border-error/25 bg-error/5 p-3"
                      >
                        <p
                          class="font-mono text-sm wrap-break-word text-error"
                        >
                          {{ row.original.errorMessage }}
                        </p>
                      </div>
                    </div>

                    <!-- Payload 信息 -->
                    <div
                      v-if="row.original.payload"
                      class="mt-3"
                    >
                      <p class="text-xs text-dimmed mb-1">{{ $t('dashboard.queue.table.detail.payload') }}</p>
                      <pre
                        class="overflow-x-auto rounded bg-elevated/60 p-2 text-xs text-toned"
                        >{{
                          JSON.stringify(row.original.payload, null, 2)
                        }}</pre
                      >
                    </div>
                  </div>
                </div>
              </template>
            </UTable>
            <div class="flex justify-center border-t border-default p-3">
              <UPagination
                v-model:page="page"
                :total="queueData?.total || 0"
                :items-per-page="pageSize"
                show-edges
              />
            </div>
          </div>
        </DashboardPanel>
      </div>
    </template>
  </UDashboardPanel>
</template>
