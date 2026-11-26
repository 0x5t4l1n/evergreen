package zones

import (
	"math"

	"evergreen/internal/procedural"
)

func sm(a, b, x float64) float64 { return procedural.SmoothStep(a, b, x) }

// Cave geometry constants shared by terrain, collision and the landmark mesh.
const (
	CaveHoleRadius  = 58.0
	CaveShellRadius = 60.0
	CaveInnerRadius = 50.0
	CaveLakeU       = -14.0
	CaveLakeV       = 10.0
	CaveLakeLevel   = -0.35
	FallsUpperWater = 12.0
	FallsLowerWater = -14.2
)

func caveLakeDepth(u, v float64) float64 {
	return 2.6 * (1 - sm(8, 13, math.Hypot(u-CaveLakeU, v-CaveLakeV)))
}

// CaveFloor is the interior floor height relative to the zone origin.
func CaveFloor(u, v float64) float64 { return -caveLakeDepth(u, v) }

// Shape returns the blend weight (0..1) with which this zone overrides natural
// terrain at world (x,z), and the target height relative to the zone BaseY.
func (z *Zone) Shape(x, zz float64) (w, rel float64) {
	u, v := x-z.X, zz-z.Z
	d := math.Hypot(u, v)
	R := z.Radius
	switch z.ID {
	case Entrance:
		w = 1 - sm(0.5*R, 0.98*R, d)
		rel = 0.9*math.Sin(u*0.05)*math.Cos(v*0.04) + 0.4
	case Falls:
		w = 1 - sm(0.72*R, R, d)
		rel = fallsRel(u, v)
	case Cave:
		w = 1 - sm(0.55*R, R, d)
		rel = CaveFloor(u, v)
		if d > CaveHoleRadius {
			rel += 0.5 * math.Sin(u*0.07) * math.Cos(v*0.06) * sm(CaveHoleRadius, CaveHoleRadius+20, d)
		}
	case Treehouse:
		w = 1 - sm(0.45*R, R, d)
		rel = 0.7 * math.Sin(u*0.045) * math.Cos(v*0.05)
	case Ruins:
		w = 1 - sm(0.4*R, 0.95*R, d)
		rel = 4.2*(1-sm(34, 68, d)) + 0.35*math.Sin(u*0.2)*math.Cos(v*0.17)*sm(30, 50, d)
	case Observatory:
		w = 1 - sm(0.82*R, R, d)
		rel = 70*(1-sm(16, 142, d)) + 3.5*math.Sin(u*0.09+1)*math.Sin(v*0.11)*sm(18, 60, d)
	case Station:
		w = 1 - sm(0.45*R, 0.95*R, d)
		rel = 0.4*math.Sin(u*0.06)*math.Cos(v*0.05) + 0.3
	case WorldTree:
		w = 1 - sm(0.5*R, R, d)
		rel = 3.5*(1-sm(10, 140, d)) + 0.5*math.Sin(u*0.08)*math.Cos(v*0.07)
	}
	return w, rel
}

func fallsRel(u, v float64) float64 {
	chanW := sm(10, 3.5, math.Abs(v))
	upper := 13 - 4*chanW // banks at +13, river bed at +9
	rel := procedural.Mix(upper, -13, sm(-3, 3, u))
	pool := 1 - sm(18, 34, math.Hypot(u-46, v))
	rel -= 8 * pool
	rel -= 3.5 * chanW * sm(40, 62, u)
	rel += 1.1 * math.Sin(u*0.06+1) * math.Cos(v*0.05) * (1 - chanW) * (1 - pool)
	return rel
}

// FallsProfile exposes the cliff height at the river centre line for mesh building.
func FallsProfile(u float64) float64 { return fallsRel(u, 0) }

// WaterLevel returns a relative water surface height and the maximum depth to
// draw it at, if this zone defines water at (x,z). Limiting depth keeps the
// upstream river from hanging in mid-air over the waterfall cliff.
func (z *Zone) WaterLevel(x, zz float64) (rel, maxDepth float64, ok bool) {
	u, v := x-z.X, zz-z.Z
	d := math.Hypot(u, v)
	switch z.ID {
	case Falls:
		if d > 0.9*z.Radius {
			return 0, 0, false
		}
		if u < -2.2 && math.Abs(v) < 16 {
			return FallsUpperWater, 3.8, true
		}
		if u > 1.5 {
			return FallsLowerWater, 9, true
		}
	case Cave:
		if d < CaveInnerRadius {
			return CaveLakeLevel, 4, true
		}
	}
	return 0, 0, false
}

// Hole reports whether terrain triangles must not be drawn at (x,z) because a
// landmark mesh replaces the ground there.
func (z *Zone) Hole(x, zz float64) bool {
	return z.ID == Cave && math.Hypot(x-z.X, zz-z.Z) < CaveHoleRadius
}

// SuppressRivers is 0..1: how strongly this zone removes natural rivers at a point.
func (z *Zone) SuppressRivers(x, zz float64) float64 {
	d := math.Hypot(x-z.X, zz-z.Z)
	if z.ID == Falls {
		return 1 - sm(0.8*z.Radius, 1.05*z.Radius, d)
	}
	return 1 - sm(0.35*z.Radius, 0.9*z.Radius, d)
}

// BiomeWeight is the 0..1 strength of this zone's biome at a point.
func (z *Zone) BiomeWeight(x, zz float64) float64 {
	return 1 - sm(0.55*z.Radius, 0.95*z.Radius, math.Hypot(x-z.X, zz-z.Z))
}

// Excluded reports whether natural vegetation must stay clear of (x,z).
func (z *Zone) Excluded(x, zz float64) bool {
	u, v := x-z.X, zz-z.Z
	d := math.Hypot(u, v)
	switch z.ID {
	case Entrance:
		return d < 16 && math.Abs(u) < 14
	case Falls:
		return math.Abs(u) < 10 && math.Abs(v) < 14 || u > 10 && math.Hypot(u-46, v) < 30 || u < 0 && math.Abs(v) < 12
	case Cave:
		return d < CaveShellRadius+4 || (u > 40 && math.Abs(v) < 10)
	case Treehouse:
		return d < 20
	case Ruins:
		return d < 46
	case Observatory:
		return d < 26
	case Station:
		return math.Abs(u) < 17 && math.Abs(v) < 12
	case WorldTree:
		return d < 26
	}
	return false
}
