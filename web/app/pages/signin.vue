<script lang="ts" setup>
// 这扇门和门里的房间用同一套材料：同一批暖灰、同一个字。页面的底由这套
// token 自己铺，卡片才是从纸面上切下来的一张。
useHead({
  title: () => $t('auth.form.signin.title'),
  bodyAttrs: { class: 'app-dashboard' },
})

const session = useUserSession()
const config = useRuntimeConfig()
const settingsStore = useSettingsStore()
const toast = useToast()
const route = useRoute()
const router = useRouter()

const isLoading = ref(false)

// 门上的招牌取自站主自己的设置：有头像就用自己的头像，有标语就写自己的标语。
const siteTitle = useSettingRef('app:title')
const siteSlogan = useSettingRef('app:slogan')
const siteAvatar = useSettingRef('app:avatarUrl')

const title = computed(() => String(siteTitle.value || config.public.app.title))
const subtitle = computed(() =>
  String(
    siteSlogan.value ||
      $t('auth.form.signin.subtitle', [title.value]),
  ),
)

const githubOauthEnabled = computed(() => {
  const settingsValue = settingsStore.getSetting('system:auth.github.enabled')
  if (typeof settingsValue === 'boolean') {
    return settingsValue
  }

  return Boolean(config.public.oauth.github.enabled)
})

const genericOauthEnabled = computed(() =>
  settingsStore.getSetting('system:auth.oauth.enabled') === true,
)

const oauthProvider = computed(() => {
  if (genericOauthEnabled.value) {
    const name = String(
      settingsStore.getSetting('system:auth.oauth.name') || 'OAuth',
    )
    return {
      icon: 'tabler:shield-lock',
      size: 'lg',
      color: 'neutral',
      variant: 'subtle',
      block: true,
      label: $t('auth.form.signin.oauthLogin', { name }),
      to: '/api/auth/oauth',
      external: true,
    }
  }
  if (githubOauthEnabled.value) {
    return {
      icon: 'tabler:brand-github',
      size: 'lg',
      color: 'neutral',
      variant: 'subtle',
      block: true,
      label: 'GitHub',
      to: '/api/auth/github',
      external: true,
    }
  }
  return null
})

const onAuthSubmit = async (event: any) => {
  isLoading.value = true
  await $fetch('/api/login', {
    method: 'POST',
    body: event.data,
  })
    .then(async () => {
      await session.fetch()
      router.push(route.query.redirect?.toString() || '/')
    })
    .catch((error) => {
      console.error('Login error:', error)
      toast.add({
        color: 'error',
        title: $t('auth.messages.loginFailed.title'),
        description: error?.data?.message || $t('auth.messages.loginFailed.description'),
      })
    })
    .finally(() => {
      isLoading.value = false
    })
}
</script>

<template>
  <div
    class="relative flex min-h-svh w-full flex-col items-center justify-center overflow-hidden p-4"
  >
    <!-- 卡片背后的一点光，让这张小卡片看起来是浮着的，而不是贴着的 -->
    <div
      aria-hidden="true"
      class="pointer-events-none absolute left-1/2 top-1/2 size-[32rem] -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary/5 blur-3xl"
    />

    <div class="absolute end-2 top-2 z-10 sm:end-4 sm:top-4">
      <LanguageSwitcher compact />
    </div>

    <div class="relative z-10 flex w-full flex-col items-center">
      <AuthForm
        :title="title"
        :subtitle="subtitle"
        :logo="String(siteAvatar || '') || undefined"
        :loading="isLoading"
        :providers="oauthProvider ? [oauthProvider] : []"
        @submit="onAuthSubmit"
      />

      <UButton
        to="/"
        variant="link"
        color="neutral"
        size="sm"
        icon="tabler:arrow-left"
        class="mt-4"
      >
        {{ $t('auth.form.action.backToHome') }}
      </UButton>
    </div>
  </div>
</template>
