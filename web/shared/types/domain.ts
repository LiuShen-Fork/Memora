import type { NeededExif } from './photo'

export interface User {
  id: number
  username: string
  email: string
  password?: string | null
  avatar?: string | null
  createdAt?: string | number | Date
  isAdmin: number | boolean
}

export interface Photo {
  id: string
  title?: string | null
  description?: string | null
  width?: number | null
  height?: number | null
  aspectRatio?: number | null
  dateTaken?: string | null
  storageKey?: string | null
  thumbnailKey?: string | null
  fileSize?: number | null
  lastModified?: string | null
  originalUrl?: string | null
  thumbnailUrl?: string | null
  thumbnailHash?: string | null
  tags?: string[] | null
  exif?: NeededExif | Record<string, unknown> | null
  latitude?: number | null
  longitude?: number | null
  country?: string | null
  city?: string | null
  locationName?: string | null
  isLivePhoto?: number | boolean | null
  livePhotoVideoUrl?: string | null
  livePhotoVideoKey?: string | null
}

export interface Album {
  id: number
  title: string
  description?: string | null
  coverPhotoId?: string | null
  isHidden: boolean
  createdAt?: string | number | Date
  updatedAt?: string | number | Date
  photoIds?: string[]
  photos?: Photo[]
}

export interface AlbumPhoto {
  id: number
  albumId: number
  photoId: string
  position: number
  addedAt?: string | number | Date
}

export type QueueStatus = 'pending' | 'in-stages' | 'completed' | 'failed'
export type QueueStage =
  | 'preprocessing'
  | 'metadata'
  | 'thumbnail'
  | 'exif'
  | 'motion-photo'
  | 'reverse-geocoding'
  | 'live-photo'
  | 'location-erase'
  | null

export interface PipelineQueueItem {
  id: number
  payload: Record<string, unknown>
  priority: number
  attempts: number
  maxAttempts: number
  status: QueueStatus
  statusStage?: QueueStage
  errorMessage?: string | null
  createdAt?: string | number | Date
  completedAt?: string | number | Date | null
}

export interface PhotoReaction {
  id: number
  photoId: string
  reactionType: string
  fingerprint: string
  ipAddress?: string | null
  userAgent?: string | null
  createdAt?: string | number | Date
  updatedAt?: string | number | Date
}

export interface StorageProviderSetting {
  id: number
  name: string
  provider: 's3' | 'local' | 'openlist'
  config: Record<string, unknown>
  createdAt?: string | number | Date
  updatedAt?: string | number | Date
}
