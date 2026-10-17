type MapServiceConfig = Record<string, unknown> | null | undefined

const text = (config: MapServiceConfig, key: string) =>
  String(config?.[key] ?? '').trim()

/**
 * Whether the configured provider can actually draw a map.
 *
 * The default MapLibre style calls MapTiler, and Mapbox styles need an access
 * token; a MapLibre style URL of one's own loads without a MapTiler token. AMap
 * brings its own key. Nothing here means the map stays blank, which is what the
 * globe says until an administrator fills one of them in.
 */
export function isMapServiceConfigured(config: MapServiceConfig): boolean {
  const provider = text(config, 'provider') || 'maplibre'

  if (provider === 'mapbox') {
    return text(config, 'mapbox.token').length > 0
  }

  if (provider === 'amap') {
    return text(config, 'amap.key').length > 0
  }

  return (
    text(config, 'maplibre.token').length > 0 ||
    text(config, 'maplibre.style').length > 0
  )
}
