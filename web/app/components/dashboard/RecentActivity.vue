<script lang="ts" setup>
import ThumbImage from '~/components/ui/ThumbImage.vue'

type ActivityStatus = 'pending' | 'in-stages' | 'completed' | 'failed'

type ActivityItem = {
  id: number
  type: string
  status: ActivityStatus
  statusStage: string | null
  errorMessage: string | null
  createdAt: string
  completedAt: string | null
  fileName: string | null
  photo: {
    id: string
    title: string | null
    thumbnailUrl: string | null
    thumbnailHash: string | null
    originalUrl: string | null
  } | null
}

const dayjs = useDayjs()

const { data, refresh } = useFetch<{ tasks: ActivityItem[] }>(
  '/api/queue/recent',
)

useIntervalFn(refresh, 5000)

const items = computed<ActivityItem[]>(() => data.value?.tasks || [])

const getStatusColor = (status: ActivityStatus) => {
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

// A task names itself after the photo it worked on, and falls back to the file
// that was uploaded when there is no photo to name.
const getItemTitle = (item: ActivityItem) => {
  return (
    item.photo?.title ||
    item.fileName ||
    $t(`dashboard.queue.types.${item.type}`)
  )
}

const getItemMeta = (item: ActivityItem) => {
  const parts: string[] = []
  const titleIsTaskType = !item.photo?.title && !item.fileName

  if (!titleIsTaskType) {
    parts.push($t(`dashboard.queue.types.${item.type}`))
  }

  if (item.status === 'in-stages' && item.statusStage) {
    parts.push($t(`dashboard.queue.stages.${item.statusStage}`))
  }

  const timestamp = item.completedAt || item.createdAt
  if (timestamp) {
    parts.push(dayjs(timestamp).fromNow())
  }

  return parts.join(' · ')
}
</script>

<template>
  <UCard :ui="{ body: 'px-4 py-4 sm:px-6 sm:py-4' }">
    <template #header>
      <DashboardSectionHeader
        :title="$t('dashboard.overview.section.recentActivity.title')"
        icon="tabler:activity"
      >
        <UButton
          to="/dashboard/queue"
          size="xs"
          variant="ghost"
          color="neutral"
          trailing-icon="tabler:arrow-right"
        >
          {{ $t('dashboard.overview.section.recentActivity.viewQueue') }}
        </UButton>
      </DashboardSectionHeader>
    </template>

    <div
      v-if="items.length === 0"
      class="flex flex-col items-center justify-center gap-2 py-6 text-center"
    >
      <UIcon
        name="tabler:inbox"
        class="size-6 text-dimmed"
      />
      <p class="text-xs text-muted">
        {{ $t('dashboard.overview.section.recentActivity.empty') }}
      </p>
    </div>

    <ul
      v-else
      class="divide-y divide-default"
    >
      <li
        v-for="item in items"
        :key="item.id"
        class="flex items-center gap-3 py-2.5 first:pt-0 last:pb-0"
      >
        <NuxtLink
          :to="item.photo ? '/dashboard/photos' : '/dashboard/queue'"
          class="shrink-0"
        >
          <ThumbImage
            v-if="item.photo?.thumbnailUrl || item.photo?.originalUrl"
            :src="item.photo.thumbnailUrl || item.photo.originalUrl || ''"
            :alt="getItemTitle(item)"
            :thumbhash="item.photo.thumbnailHash || ''"
            class="size-9 rounded-md bg-elevated"
            :lazy="false"
          />
          <div
            v-else
            class="flex size-9 items-center justify-center rounded-md bg-elevated"
          >
            <UIcon
              name="tabler:list-check"
              class="size-4 text-dimmed"
            />
          </div>
        </NuxtLink>

        <div class="min-w-0 flex-1">
          <NuxtLink
            to="/dashboard/queue"
            class="block truncate text-sm leading-5 transition-colors hover:text-primary"
          >
            {{ getItemTitle(item) }}
          </NuxtLink>
          <p class="mt-0.5 truncate text-xs leading-4 text-muted">
            {{ getItemMeta(item) }}
          </p>
          <p
            v-if="item.status === 'failed' && item.errorMessage"
            class="mt-0.5 truncate text-xs text-error"
          >
            {{ item.errorMessage }}
          </p>
        </div>

        <UBadge
          :label="$t(`dashboard.queue.status.${item.status}`)"
          variant="soft"
          :color="getStatusColor(item.status)"
          size="xs"
          class="shrink-0 font-normal"
        />
      </li>
    </ul>
  </UCard>
</template>
