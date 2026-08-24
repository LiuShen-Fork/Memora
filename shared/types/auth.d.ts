import type { User as DBUser } from './domain'

declare module '#auth-utils' {
  interface User extends DBUser {}
}

export {}
