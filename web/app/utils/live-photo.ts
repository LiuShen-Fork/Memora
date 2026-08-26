import type { Photo } from '~~/shared/types/domain'

/**
 * Use the Go media route when a storage key is available. This keeps video
 * loading same-origin, which is required by the client-side MOV preparation
 * step when a CDN does not expose CORS headers.
 */
export function livePhotoVideoSource(photo: Photo): string | null {
  const key = photo.livePhotoVideoKey?.trim()
  if (key) {
    const encodedKey = key
      .replaceAll('\\', '/')
      .split('/')
      .filter(Boolean)
      .map((part) => encodeURIComponent(part))
      .join('/')
    return encodedKey ? `/image/${encodedKey}` : null
  }

  return photo.livePhotoVideoUrl?.trim() || null
}
