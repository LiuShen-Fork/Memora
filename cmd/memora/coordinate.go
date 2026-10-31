package main

import "math"

// Chinese map services draw on a shifted grid.
//
// Photographs carry GPS coordinates as recorded by the camera, which is WGS-84.
// AMap and Tencent place a point by GCJ-02, the same coordinates with a
// deliberate offset applied to them by law; handing them a WGS-84 pair puts the
// pin a few hundred metres away from the place it names. Mapbox, MapLibre and
// Tianditu draw on WGS-84, so they take the coordinates as they are.
//
// The shift is a fixed formula rather than a lookup table, and the one below is
// the published approximation the frontend already uses
// (web/app/utils/coordinate-transform.ts). Keeping the two in step matters: a
// pin drawn on the map and a place looked up for the same photo have to be at
// the same spot.

const (
	coordinateAxis     = 6378245.0
	coordinateEcc      = 0.006693421622965943
	coordinateRadians  = math.Pi
	coordinateDegrees  = 180.0
	coordinateTaiwanLn = 119.18
)

// outsideChina reports a point the shift does not apply to.
func outsideChina(lng, lat float64) bool {
	return lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271
}

// inTaiwan covers the island Tencent documents as WGS-84 even though it falls
// inside the bounding box above. The box is deliberately tight enough not to
// reach the Fujian coast towns (Putian sits at 119.0, Fuzhou at 26.0 north).
func inTaiwan(lng, lat float64) bool {
	return lng >= 119.18 && lng <= 122.5 && lat >= 21.89 && lat <= 25.30
}

func shiftLat(lng, lat float64) float64 {
	value := -100 + 2*lng + 3*lat + 0.2*lat*lat + 0.1*lng*lat + 0.2*math.Sqrt(math.Abs(lng))
	value += (20*math.Sin(6*lng*math.Pi) + 20*math.Sin(2*lng*math.Pi)) * 2 / 3
	value += (20*math.Sin(lat*math.Pi) + 40*math.Sin(lat/3*math.Pi)) * 2 / 3
	value += (160*math.Sin(lat/12*math.Pi) + 320*math.Sin(lat*math.Pi/30)) * 2 / 3
	return value
}

func shiftLng(lng, lat float64) float64 {
	value := 300 + lng + 2*lat + 0.1*lng*lng + 0.1*lng*lat + 0.1*math.Sqrt(math.Abs(lng))
	value += (20*math.Sin(6*lng*math.Pi) + 20*math.Sin(2*lng*math.Pi)) * 2 / 3
	value += (20*math.Sin(lng*math.Pi) + 40*math.Sin(lng/3*math.Pi)) * 2 / 3
	value += (150*math.Sin(lng/12*math.Pi) + 300*math.Sin(lng/30*math.Pi)) * 2 / 3
	return value
}

// wgs84ToGcj02 shifts a recorded coordinate onto the grid AMap and Tencent
// draw. Coordinates outside China, and those in Taiwan, come back unchanged.
func wgs84ToGcj02(lng, lat float64) (float64, float64) {
	if outsideChina(lng, lat) || inTaiwan(lng, lat) {
		return lng, lat
	}
	radLat := lat / coordinateDegrees * math.Pi
	magic := 1 - coordinateEcc*math.Pow(math.Sin(radLat), 2)
	sqrtMagic := math.Sqrt(magic)
	dLat := (shiftLat(lng-105, lat-35) * coordinateDegrees) /
		((coordinateAxis * (1 - coordinateEcc) / (magic * sqrtMagic)) * math.Pi)
	dLng := (shiftLng(lng-105, lat-35) * coordinateDegrees) /
		((coordinateAxis / sqrtMagic) * math.Cos(radLat) * math.Pi)
	return lng + dLng, lat + dLat
}
