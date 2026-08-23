interface SessionUser {
  id: number
  username: string
  email: string
  avatar: string
  isAdmin: number
}

interface SessionState {
  user: Ref<SessionUser | null>
  loggedIn: ComputedRef<boolean>
  fetch: () => Promise<void>
  clear: () => Promise<void>
}

const sessionKey = Symbol('go-session') as InjectionKey<SessionState>

export function useUserSession(): SessionState {
  const existing = inject(sessionKey, null)
  if (existing) return existing

  const user = useState<SessionUser | null>('go-session-user', () => null)
  const loaded = useState<boolean>('go-session-loaded', () => false)
  const loggedIn = computed(() => user.value !== null)

  const fetch = async () => {
    try {
      user.value = await $fetch<SessionUser | null>('/api/profile')
    } catch {
      user.value = null
    } finally {
      loaded.value = true
    }
  }

  const clear = async () => {
    try {
      await $fetch('/api/logout')
    } finally {
      user.value = null
      loaded.value = true
    }
  }

  const state = { user, loggedIn, fetch, clear }
  provide(sessionKey, state)
  return state
}
