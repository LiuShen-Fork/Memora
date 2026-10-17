<script lang="ts" setup>
import '@/assets/css/heatmap.css'
import type { CalendarItem } from '~/components/ui/CalendarHeatmap/Heatmap'

definePageMeta({
  layout: 'dashboard',
})

useHead({
  title: () => $t('title.dashboard'),
})

const dayjs = useDayjs()
const config = useRuntimeConfig()

const { data: dashboardStats, refresh: refreshStats } =
  await useFetch('/api/system/stats')

// The calendar counts photos by the day they were taken, which is a question
// about the collection rather than about the photos the gallery happens to
// have loaded. It used to be answered from the browser's copy of the
// collection, so a cold visit to the dashboard drew an empty year, and a warm
// one drew a year counted from a sample. One query answers it properly.
const { data: timeline } = await useFetch<{
  days: { date: string; count: number }[]
}>('/api/photos/timeline')

const timelineDays = computed(() => timeline.value?.days || [])

// 年份选择器相关状态
const selectedYear = ref<number | 'recent'>('recent')

// A dashboard is left open. Re-reading every five seconds costs the server
// twelve requests a minute for a page nobody is looking at, so the poll is
// slower and stops while the tab is in the background.
const visibility = useDocumentVisibility()
useIntervalFn(
  () => {
    if (visibility.value === 'visible') void refreshStats()
  },
  10000,
)

const memoryUsage = computed(() => {
  const memory = dashboardStats.value?.memory
  if (!memory || memory.total <= 0) return null
  return Math.min(100, Math.max(0, (memory.used / memory.total) * 100))
})

const systemStatus = computed(() => {
  if (memoryUsage.value === null) return 'unknown'
  if (memoryUsage.value > 90) return 'critical'
  if (memoryUsage.value > 70) return 'warning'
  return 'healthy'
})

const queueState = computed(() => {
  if (!dashboardStats.value) return 'unknown'
  const pool = dashboardStats.value.workerPool
  if (!pool?.totalWorkers) return 'stopped'
  if (pool.activeWorkers > 0) return 'processing'
  // Work that has been waiting well past its turn is worth a second look even
  // while the queue is idle.
  if ((dashboardStats.value.queue?.oldestWaitSeconds || 0) >= 300)
    return 'attention'
  return 'idle'
})

const queueNotice = computed(() => {
  const queue = dashboardStats.value?.queue
  if (!queue) return ''
  if (queue.pending > 0 && queueState.value === 'stopped')
    return $t('dashboard.overview.section.queue.stoppedNotice', {
      count: queue.pending,
    })
  if (queue.oldestWaitSeconds >= 300)
    return $t('dashboard.overview.section.queue.waitNotice', {
      minutes: Math.floor(queue.oldestWaitSeconds / 60),
    })
  if (queue.failed > 0)
    return $t('dashboard.overview.section.queue.failedNotice', {
      count: queue.failed,
    })
  return ''
})

const hasQueueActivity = computed(() => {
  const pool = dashboardStats.value?.workerPool
  return (pool?.totalProcessed || 0) > 0 || (pool?.totalErrors || 0) > 0
})

const queueSuccessRate = computed(() =>
  Math.min(
    100,
    Math.max(0, dashboardStats.value?.workerPool?.averageSuccessRate || 0),
  ),
)

const queueSuccessColor = computed(() => {
  const pool = dashboardStats.value?.workerPool
  if (!pool || !hasQueueActivity.value) return 'neutral'
  if (pool.averageSuccessRate > 90) return 'success'
  if (pool.averageSuccessRate > 70) return 'warning'
  return 'error'
})

const queueMetrics = computed(() => {
  const pool = dashboardStats.value?.workerPool
  const failed = pool?.totalErrors || 0
  return [
    {
      icon: 'tabler:activity',
      label: $t('dashboard.overview.section.queue.workers'),
      value: `${pool?.activeWorkers || 0}/${pool?.totalWorkers || 0}`,
      isError: false,
    },
    {
      icon: 'tabler:hourglass',
      label: $t('dashboard.overview.section.queue.pending'),
      value: dashboardStats.value?.queue?.pending || 0,
      isError: false,
    },
    {
      icon: 'tabler:circle-check',
      label: $t('dashboard.overview.section.queue.totalProcessed'),
      value: pool?.totalProcessed || 0,
      isError: false,
    },
    {
      icon: 'tabler:circle-x',
      label: $t('dashboard.overview.section.queue.totalFailed'),
      value: failed,
      isError: failed > 0,
    },
  ]
})

// 存储服务的名称：从面板配置的带上它自己的名字，从环境变量起的只有类型。
const activeStorage = computed(() => {
  const active = dashboardStats.value?.storage?.activeProvider
  if (!active) return '—'
  const label = $t(`settings.storage.providers.${active.provider}`)
  return active.name ? `${label}(${active.name})` : label
})

// Every day the collection has a photo on is a day the calendar can offer.
const availableYears = computed(() => {
  const years = new Set<number>()
  for (const day of timelineDays.value) {
    const year = Number(day.date.slice(0, 4))
    if (Number.isFinite(year) && year > 0) years.add(year)
  }
  return [...years].sort((a, b) => b - a)
})

// Days are compared as text because that is how they are stored: YYYY-MM-DD
// sorts the same way it reads.
const range = computed(() =>
  selectedYear.value === 'recent'
    ? {
        from: dayjs().subtract(1, 'year').format('YYYY-MM-DD'),
        to: dayjs().add(1, 'day').format('YYYY-MM-DD'),
      }
    : {
        from: `${selectedYear.value}-01-01`,
        to: `${selectedYear.value}-12-31`,
      },
)

const heatmapData = computed(() =>
  timelineDays.value.filter(
    (day) => day.date >= range.value.from && day.date <= range.value.to,
  ),
)

const heatmapStartDate = computed(() => dayjs(range.value.from).toDate())
const heatmapEndDate = computed(() => dayjs(range.value.to).toDate())

const yearOptions = computed(() => {
  const options: Array<{ label: string; value: number | 'recent' }> = [
    {
      label: $t('common.heatmap.legend.recentlyYear'),
      value: 'recent' as const,
    },
  ]

  availableYears.value.forEach((year) => {
    options.push({ label: year.toString(), value: year })
  })

  return options
})

const onShareSite = () => {
  const discussionParams = new URLSearchParams({
    category: 'showcases',
    title: `Show: ${config.public.app.title}`,
    body: `## Description / Motto\n\n${config.public.app.slogan}\n\n## URL\n\n[${window.location.origin}](${window.location.origin})`,
  })
  window.open(
    `https://github.com/LiuShen-Fork/Memora/discussions/new?${discussionParams}`,
    '_blank',
  )
}
</script>

<template>
  <UDashboardPanel>
    <template #header>
      <UDashboardNavbar :title="$t('dashboard.overview.title')" />
    </template>

    <template #body>
      <div class="flex flex-col gap-4">
        <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
          <DashboardIndicator
            data-tour="dashboard-photos"
            :title="$t('dashboard.overview.indicator.totalPhotos')"
            icon="tabler:photo"
            tone="accent"
            :value="dashboardStats?.photos?.total || 0"
            :hint="
              $t('dashboard.overview.indicator.todayCount', {
                count: dashboardStats?.photos?.today || 0,
              })
            "
            clickable
            @click="$router.push('/dashboard/photos')"
          />
          <DashboardIndicator
            :title="$t('dashboard.overview.indicator.thisMonth')"
            icon="tabler:photo-plus"
            :value="dashboardStats?.photos?.thisMonth || 0"
            :hint="
              $t('dashboard.overview.indicator.weekCount', {
                count: dashboardStats?.photos?.thisWeek || 0,
              })
            "
          />
          <DashboardIndicator
            data-tour="dashboard-queue"
            :title="$t('dashboard.overview.indicator.queueStatus.title')"
            icon="tabler:loader"
            :tone="
              queueState === 'stopped'
                ? 'negative'
                : queueState === 'attention'
                  ? 'attention'
                  : queueState === 'processing'
                    ? 'accent'
                    : 'neutral'
            "
            :value="
              (dashboardStats?.workerPool?.activeWorkers || 0) > 0
                ? $t('dashboard.overview.indicator.queueStatus.processing')
                : $t('dashboard.overview.indicator.queueStatus.pending')
            "
            :hint="
              $t('dashboard.overview.indicator.queueCounts', {
                pending: dashboardStats?.queue?.pending || 0,
                failed: dashboardStats?.queue?.failed || 0,
              })
            "
            clickable
            @click="$router.push('/dashboard/queue')"
          />
          <DashboardIndicator
            :title="$t('dashboard.overview.indicator.storageUsage')"
            icon="tabler:database"
            :value="formatBytes(dashboardStats?.storage?.totalSize || 0)"
            :hint="
              $t('dashboard.overview.indicator.averageSize', {
                size: formatBytes(dashboardStats?.storage?.averageSize || 0),
              })
            "
          />
        </div>

        <!-- Both columns are stretched to the taller of the two, and the
             recent-activity card takes up the slack inside the left one, so
             the page ends on one line instead of two. -->
        <div class="grid min-h-0 gap-4 lg:grid-cols-5">
          <div class="flex min-w-0 flex-col gap-4 lg:col-span-3">
            <DashboardPanel :ui="{ body: 'px-2 py-3 sm:px-3' }">
              <div class="heatmap-container">
                <ClientOnly>
                  <CalendarHeatmap
                    theme="brand"
                    :values="heatmapData"
                    :start-date="heatmapStartDate"
                    :end-date="heatmapEndDate"
                    :round="3"
                    :tooltip-formatter="
                      (item: CalendarItem) => {
                        return $t('common.heatmap.tooltip.data', [
                          $dayjs(item.date).format('LL'),
                          item.count || 0,
                        ])
                      }
                    "
                    :tooltip-no-data-formatter="
                      (date: Date) =>
                        $t('common.heatmap.tooltip.noData', [
                          $dayjs(date).format('LL'),
                        ])
                    "
                    :locale="{
                      months: [
                        $t('common.months.jan'),
                        $t('common.months.feb'),
                        $t('common.months.mar'),
                        $t('common.months.apr'),
                        $t('common.months.may'),
                        $t('common.months.jun'),
                        $t('common.months.jul'),
                        $t('common.months.aug'),
                        $t('common.months.sep'),
                        $t('common.months.oct'),
                        $t('common.months.nov'),
                        $t('common.months.dec'),
                      ],
                      days: [
                        $t('common.days.sun'),
                        $t('common.days.mon'),
                        $t('common.days.tue'),
                        $t('common.days.wed'),
                        $t('common.days.thu'),
                        $t('common.days.fri'),
                        $t('common.days.sat'),
                      ],
                      less: $t('common.heatmap.legend.less'),
                      more: $t('common.heatmap.legend.more'),
                    }"
                    :dark-mode="$colorMode.value === 'dark'"
                  >
                    <template #vch__legend-left>
                      <USelectMenu
                        v-model="selectedYear"
                        :items="yearOptions"
                        :disabled="yearOptions.length <= 1"
                        :search-input="false"
                        value-key="value"
                        size="xs"
                        variant="soft"
                        class="w-24"
                      />
                    </template>
                  </CalendarHeatmap>
                  <template #placeholder>
                    <div class="flex h-[164.5px] items-center justify-center">
                      <Icon
                        name="svg-spinners:180-ring-with-bg"
                        class="size-8 opacity-50"
                        mode="svg"
                      />
                    </div>
                  </template>
                </ClientOnly>
              </div>
            </DashboardPanel>

            <DashboardRecentActivity class="flex-1" />
          </div>

          <!-- 右侧：运行状态 -->
          <div class="flex min-w-0 flex-col lg:col-span-2">
            <DashboardPanel
              class="@container h-full"
              :title="$t('dashboard.overview.section.queue.title')"
              icon="tabler:server"
              :ui="{
                root: 'flex h-full flex-col shadow-xs',
                body: 'flex flex-1 flex-col px-4 py-4 sm:px-4',
              }"
            >
              <template #actions>
                <span class="flex items-center gap-1.5 text-xs text-muted">
                  <span
                    class="size-1.5 rounded-full"
                    :class="{
                      'bg-success': queueState === 'processing',
                      'bg-warning': queueState === 'attention',
                      'bg-error': queueState === 'stopped',
                      'bg-neutral-400':
                        queueState === 'idle' || queueState === 'unknown',
                    }"
                  />
                  {{
                    $t('dashboard.overview.section.queue.states.' + queueState)
                  }}
                </span>
              </template>

              <div
                class="grid grid-cols-1 overflow-hidden rounded-lg border border-default @min-[440px]:grid-cols-[100px_minmax(0,1fr)]"
              >
                <div
                  class="flex items-center justify-center gap-3 border-b border-default px-3 py-3 @min-[440px]:flex-col @min-[440px]:gap-1 @min-[440px]:border-r @min-[440px]:border-b-0"
                >
                  <div class="relative h-12 w-20">
                    <svg
                      viewBox="0 0 100 60"
                      class="h-full w-full"
                      aria-hidden="true"
                    >
                      <path
                        d="M 10 50 A 40 40 0 0 1 90 50"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="7"
                        stroke-linecap="round"
                        class="text-(--ui-border-accented)"
                      />
                      <path
                        v-if="hasQueueActivity && queueSuccessRate > 0"
                        d="M 10 50 A 40 40 0 0 1 90 50"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="7"
                        stroke-linecap="round"
                        pathLength="100"
                        :stroke-dasharray="queueSuccessRate + ' 100'"
                        :class="{
                          'text-success': queueSuccessColor === 'success',
                          'text-warning': queueSuccessColor === 'warning',
                          'text-error': queueSuccessColor === 'error',
                        }"
                      />
                    </svg>
                    <span
                      class="absolute inset-x-0 bottom-0 text-center text-base leading-5 font-semibold tabular"
                      :class="{ 'text-muted': !hasQueueActivity }"
                      >{{
                        hasQueueActivity
                          ? Math.round(queueSuccessRate) + '%'
                          : '—'
                      }}</span
                    >
                  </div>
                  <span class="text-center text-xs leading-4 text-muted">{{
                    $t('dashboard.overview.section.queue.avgSuccessRate')
                  }}</span>
                </div>
                <div class="grid min-w-0 grid-cols-2 gap-px bg-border">
                  <div
                    v-for="metric in queueMetrics"
                    :key="metric.label"
                    class="grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-x-2 gap-y-1 bg-default px-3 py-2.5 @min-[440px]:grid-cols-[auto_minmax(0,1fr)_auto]"
                  >
                    <UIcon
                      :name="metric.icon"
                      class="size-4 shrink-0 text-muted"
                    />
                    <span class="min-w-0 break-words text-xs text-muted">{{
                      metric.label
                    }}</span>
                    <span
                      class="col-span-2 min-w-0 break-all text-base font-semibold tabular @min-[440px]:col-span-1 @min-[440px]:text-right"
                      :class="{ 'text-error': metric.isError }"
                      >{{ metric.value }}</span
                    >
                  </div>
                </div>
              </div>

              <NuxtLink
                v-if="queueNotice"
                to="/dashboard/queue"
                class="mt-3 flex items-start gap-2 text-xs text-muted hover:text-default"
              >
                <UIcon
                  name="tabler:alert-circle"
                  class="size-4 shrink-0 text-warning"
                />
                <span>{{ queueNotice }}</span>
                <UIcon
                  name="tabler:arrow-right"
                  class="ml-auto size-4 shrink-0"
                />
              </NuxtLink>

              <div class="mt-4 space-y-2 rounded-lg border border-default p-3">
                <div class="flex items-center justify-between gap-3">
                  <span class="flex items-center gap-2 text-xs text-muted">
                    <UIcon
                      name="tabler:cpu"
                      class="size-4 shrink-0"
                    />
                    {{ $t('dashboard.overview.section.memory.title') }}
                  </span>
                  <span class="text-sm font-semibold tabular">
                    {{
                      memoryUsage === null ? '—' : `${Math.round(memoryUsage)}%`
                    }}
                  </span>
                </div>
                <UProgress
                  :model-value="memoryUsage ?? 0"
                  :color="
                    systemStatus === 'healthy'
                      ? 'success'
                      : systemStatus === 'warning'
                        ? 'warning'
                        : systemStatus === 'critical'
                          ? 'error'
                          : 'neutral'
                  "
                  class="w-full"
                />
                <p class="text-xs text-muted tabular">
                  {{
                    memoryUsage === null
                      ? $t('dashboard.overview.memoryUnavailable')
                      : `${formatBytes(dashboardStats?.memory?.used || 0)} / ${formatBytes(dashboardStats?.memory?.total || 0)}`
                  }}
                </p>
              </div>

              <dl
                class="mt-4 divide-y divide-default rounded-lg border border-default px-3"
              >
                <div class="flex items-center justify-between gap-3 py-3">
                  <dt class="flex items-center gap-2 text-xs text-muted">
                    <UIcon
                      name="tabler:tag"
                      class="size-4 shrink-0"
                    />
                    {{ $t('dashboard.overview.section.runtimeInfo.version') }}
                  </dt>
                  <dd class="min-w-0 text-xs">
                    <NuxtLink
                      class="flex items-center gap-1 hover:text-primary"
                      target="_blank"
                      external
                      :to="`https://github.com/LiuShen-Fork/Memora/releases/tag/v${$config.public.VERSION}`"
                    >
                      v{{ $config.public.VERSION }}
                      <UIcon
                        name="tabler:external-link"
                        class="size-3.5 shrink-0 text-muted"
                      />
                    </NuxtLink>
                  </dd>
                </div>
                <div class="flex items-center justify-between gap-3 py-3">
                  <dt class="flex items-center gap-2 text-xs text-muted">
                    <UIcon
                      name="tabler:clock"
                      class="size-4 shrink-0"
                    />
                    {{ $t('dashboard.overview.section.runtimeInfo.uptime') }}
                  </dt>
                  <dd class="text-right text-xs tabular">
                    {{
                      dashboardStats?.uptime != null
                        ? $dayjs
                            .duration(dashboardStats.uptime, 'seconds')
                            .humanize()
                        : '—'
                    }}
                  </dd>
                </div>
                <div class="flex items-center justify-between gap-3 py-3">
                  <dt class="flex items-center gap-2 text-xs text-muted">
                    <UIcon
                      name="tabler:server"
                      class="size-4 shrink-0"
                    />
                    {{
                      $t('dashboard.overview.section.runtimeInfo.environment')
                    }}
                  </dt>
                  <dd class="text-right text-xs">
                    {{
                      $t(
                        `dashboard.overview.section.runtimeInfo.systems.${dashboardStats?.runningOn || 'unknown'}`,
                      )
                    }}
                  </dd>
                </div>
                <div class="flex items-start justify-between gap-3 py-3">
                  <dt
                    class="flex shrink-0 items-center gap-2 text-xs leading-5 text-muted"
                  >
                    <UIcon
                      name="tabler:database"
                      class="size-4 shrink-0"
                    />
                    {{ $t('settings.storage.form.typeLabel') }}
                  </dt>
                  <dd class="min-w-0 text-right text-xs break-words">
                    {{ activeStorage }}
                  </dd>
                </div>
                <div class="flex items-center justify-between gap-3 py-3">
                  <dt class="flex items-center gap-2 text-xs text-muted">
                    <UIcon
                      name="tabler:refresh"
                      class="size-4 shrink-0"
                    />
                    {{ $t('dashboard.overview.section.runtimeInfo.lastUpdate') }}
                  </dt>
                  <dd class="text-right text-xs tabular">
                    {{
                      dashboardStats?.timestamp
                        ? $dayjs(dashboardStats.timestamp).fromNow()
                        : '—'
                    }}
                  </dd>
                </div>
              </dl>

              <div class="mt-auto flex flex-wrap items-center justify-end gap-2 pt-4">
                <UButton
                  data-tour="dashboard-share"
                  variant="subtle"
                  size="xs"
                  color="neutral"
                  trailing-icon="tabler:external-link"
                  @click="onShareSite"
                >
                  {{ $t('dashboard.overview.shareSite.button') }}
                </UButton>
              </div>
            </DashboardPanel>
          </div>
        </div>
      </div>
    </template>
  </UDashboardPanel>
</template>

<style>
/* The calendar is wider than a phone and scrolls inside its card rather than
 * pushing the card wider than the page. */
.heatmap-container {
  min-width: 100%;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;
  scrollbar-color: var(--ui-color-neutral-300) transparent;
}

.heatmap-container::-webkit-scrollbar {
  height: 2px;
}

.heatmap-container .vch__container {
  min-width: 720px;
}

.vch__day__label,
.vch__month__label,
.vch__legend {
  font-size: var(--text-xs);
  font-weight: var(--font-weight-medium);
  color: var(--ui-color-neutral-500);
}
</style>
