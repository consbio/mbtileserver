package main

import (
	"math"
)

// Cast interface to a string if not nil, otherwise empty string
func toString(s interface{}) string {
	if s != nil {
		return s.(string)
	}
	return ""
}

// Convert a latitude and longitude to mercator coordinates, bounded to world domain.
func geoToMercator(longitude, latitude float64) (float64, float64) {
	// bound to world coordinates
	if latitude > 80 {
		latitude = 80
	} else if latitude < -80 {
		latitude = -80
	}

	origin := 6378137 * math.Pi // 6378137 is WGS84 semi-major axis
	x := longitude * origin / 180
	y := math.Log(math.Tan((90+latitude)*math.Pi/360)) / (math.Pi / 180) * (origin / 180)

	return x, y
}
