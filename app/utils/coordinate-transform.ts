const PI = Math.PI
const AXIS = 6378245
const EE = 0.006693421622965943

const outOfChina = (lng: number, lat: number) =>
  lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271

const transformLat = (lng: number, lat: number) => {
  let value =
    -100 +
    2 * lng +
    3 * lat +
    0.2 * lat * lat +
    0.1 * lng * lat +
    0.2 * Math.sqrt(Math.abs(lng))
  value += ((20 * Math.sin(6 * lng * PI) + 20 * Math.sin(2 * lng * PI)) * 2) / 3
  value += ((20 * Math.sin(lat * PI) + 40 * Math.sin((lat / 3) * PI)) * 2) / 3
  value +=
    ((160 * Math.sin((lat / 12) * PI) + 320 * Math.sin((lat * PI) / 30)) * 2) /
    3
  return value
}

const transformLng = (lng: number, lat: number) => {
  let value =
    300 +
    lng +
    2 * lat +
    0.1 * lng * lng +
    0.1 * lng * lat +
    0.1 * Math.sqrt(Math.abs(lng))
  value += ((20 * Math.sin(6 * lng * PI) + 20 * Math.sin(2 * lng * PI)) * 2) / 3
  value += ((20 * Math.sin(lng * PI) + 40 * Math.sin((lng / 3) * PI)) * 2) / 3
  value +=
    ((150 * Math.sin((lng / 12) * PI) + 300 * Math.sin((lng / 30) * PI)) * 2) /
    3
  return value
}

export function wgs84ToGcj02(lng: number, lat: number): [number, number] {
  if (outOfChina(lng, lat)) return [lng, lat]
  const radLat = (lat / 180) * PI
  let magic = 1 - EE * Math.sin(radLat) ** 2
  const sqrtMagic = Math.sqrt(magic)
  const dLat =
    (transformLat(lng - 105, lat - 35) * 180) /
    (((AXIS * (1 - EE)) / (magic * sqrtMagic)) * PI)
  const dLng =
    (transformLng(lng - 105, lat - 35) * 180) /
    ((AXIS / sqrtMagic) * Math.cos(radLat) * PI)
  return [lng + dLng, lat + dLat]
}

export function gcj02ToWgs84(lng: number, lat: number): [number, number] {
  if (outOfChina(lng, lat)) return [lng, lat]
  const [mgLng, mgLat] = wgs84ToGcj02(lng, lat)
  return [lng * 2 - mgLng, lat * 2 - mgLat]
}

export function transformCoordinate(
  lng: number,
  lat: number,
  provider: string,
): [number, number] {
  return provider === 'amap' ? wgs84ToGcj02(lng, lat) : [lng, lat]
}
