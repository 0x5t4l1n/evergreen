package app

import (
	"math"

	"evergreen/internal/zones"
)

// ArrivalPoint returns where the visitor appears when navigating to a zone
// from the menu, and the direction they face (yaw, radians).
func ArrivalPoint(z zones.Zone) (x, zz, yaw float64) {
	switch z.ID {
	case zones.Entrance:
		return z.X - 10, z.Z + 26, -math.Pi / 2
	case zones.Wilderness:
		return z.X + 8, z.Z + 44, -math.Pi / 2
	case zones.Falls:
		return z.X - 24, z.Z + 14, 0
	case zones.Cave:
		return z.X + 86, z.Z, math.Pi
	case zones.Treehouse:
		return z.X + 22, z.Z + 14, math.Pi + 0.5
	case zones.Ruins:
		return z.X, z.Z + 40, -math.Pi / 2
	case zones.Observatory:
		return z.X + 15, z.Z + 8, 0.4
	case zones.Station:
		return z.X + 4, z.Z + 24, -math.Pi / 2
	case zones.WorldTree:
		return z.X, z.Z + 48, -math.Pi / 2
	}
	return z.X, z.Z, 0
}
