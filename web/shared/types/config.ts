import type { StorageConfig } from './storage'

export interface ChronoConfig {
  storage: StorageConfig
}

export interface MatomoConfig {
  enabled: boolean
  url: string
  siteId: string
}

export interface AnalyticsConfig {
  matomo: MatomoConfig
}
