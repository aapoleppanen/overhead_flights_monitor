package main

import (
	"math"
)

const (
	tileSize = 256
)

// LatLonToPixels converts latitude and longitude to pixel coordinates at a given zoom level.
func LatLonToPixels(lat, lon float64, zoom int) (float64, float64) {
	scale := math.Pow(2, float64(zoom))
	x := (lon + 180.0) / 360.0 * scale * float64(tileSize)

	latRad := lat * math.Pi / 180.0
	y := (1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * scale * float64(tileSize)

	return x, y
}

// PixelsToLatLon converts pixel coordinates at a given zoom level to latitude and longitude.
func PixelsToLatLon(x, y float64, zoom int) (float64, float64) {
	scale := math.Pow(2, float64(zoom))
	lon := (x / (scale * float64(tileSize)) * 360.0) - 180.0

	n := math.Pi - 2.0*math.Pi*y/(scale*float64(tileSize))
	lat := 180.0 / math.Pi * math.Atan(0.5*(math.Exp(n)-math.Exp(-n)))

	return lat, lon
}

// Distance calculates distance between two lat/lon points in km (Haversine formula).
func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371 // Earth radius in km
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180.0)*math.Cos(lat2*math.Pi/180.0)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}
