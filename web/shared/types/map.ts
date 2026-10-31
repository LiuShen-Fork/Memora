import type { Map as _MaplibreMap } from 'maplibre-gl'
import type { Map as _MapboxMap } from 'mapbox-gl'

export type MaplibreMap = _MaplibreMap
export type MapboxMap = _MapboxMap
export type AMapMap = any
// Tencent's JS API and Tianditu's have no type packages, and the parts the app
// touches are a handful of methods that are called through `any` anyway.
export type TencentMap = any
export type TiandituMap = any

export type MapInstance =
  | MaplibreMap
  | MapboxMap
  | AMapMap
  | TencentMap
  | TiandituMap

export const isMaplibreMap = (
  map: MapInstance,
  provider: string,
): map is MaplibreMap => {
  return provider === 'maplibre'
}

export const isMapboxMap = (
  map: MapInstance,
  provider: string,
): map is MapboxMap => {
  return provider === 'mapbox'
}

export const isAMapMap = (
  map: MapInstance,
  provider: string,
): map is AMapMap => provider === 'amap'

export const isTencentMap = (
  map: MapInstance,
  provider: string,
): map is TencentMap => provider === 'tencent'

export const isTiandituMap = (
  map: MapInstance,
  provider: string,
): map is TiandituMap => provider === 'tianditu'

export interface PhotoMarker {
  id: string
  latitude: number
  longitude: number
  title?: string
  thumbnailUrl?: string
  thumbnailHash?: string
  dateTaken?: string
  city?: string
  exif?: any
}

export interface ClusterPoint {
  type: 'Feature'
  properties: {
    marker?: PhotoMarker
    cluster?: boolean
    point_count?: number
    point_count_abbreviated?: string
    clusteredPhotos?: PhotoMarker[]
  }
  geometry: {
    type: 'Point'
    coordinates: [number, number]
  }
}
