// Package terrain turns a world seed into heights, water, biomes and chunk
// meshes. All functions are pure and deterministic: the same seed and
// coordinates always give the same result.
package terrain

import (
	"math"

	"evergreen/internal/procedural"
	"evergreen/internal/vegetation"
	"evergreen/internal/zones"
)

// Chunk geometry constants shared across the engine.
const (
	ChunkSize  = 64.0
	ChunkCells = 32
	CellSize   = ChunkSize / ChunkCells
	LakeLevel  = 3.4
)

// Sample is everything the engine needs to know about one point of the world.
type Sample struct {
	Height   float64
	WaterY   float64
	MaxDepth float64
	HasWater bool // a water level exists here (it may still be below the ground)
	Moisture float64
	River    float64 // 0..1 river mask
	Biome    vegetation.Biome
	ZoneID   zones.ID
	ZoneW    float64 // 0..1 strength of the zone's biome influence
	Hole     bool
	Mount    float64 // metres of mountain added to the base terrain
}

// Wet reports whether water covers the ground at this sample.
func (s Sample) Wet() bool {
	d := s.WaterY - s.Height
	return s.HasWater && d > 0.02 && d <= s.MaxDepth
}

// Depth is the water depth above the ground (negative on dry land).
func (s Sample) Depth() float64 { return s.WaterY - s.Height }

// Generator produces terrain for one seed.
type Generator struct {
	Seed                           uint64
	Atlas                          *zones.Atlas
	sH, sD, sM, sR, sV, sW, sL, sB uint64
}

// New builds a generator for a numeric seed.
func New(seed uint64) *Generator {
	g := &Generator{Seed: seed}
	r := procedural.NewRNG(seed ^ 0xE7E4C0DE)
	g.sH, g.sD, g.sM, g.sR, g.sV, g.sW, g.sL, g.sB = r.Next(), r.Next(), r.Next(), r.Next(), r.Next(), r.Next(), r.Next(), r.Next()
	g.Atlas = zones.NewAtlas(seed, func(x, z float64) float64 {
		h, _, _, _ := g.natural(x, z)
		return h
	})
	return g
}

func realmDistance(x, z float64) float64 {
	rx, rz := zones.RealmOf(x, z)
	return math.Hypot(x-float64(rx)*zones.RealmSize, z-float64(rz)*zones.RealmSize)
}

// natural returns terrain without zone influence: height, mountain metres,
// river mask and the river/lake surface height.
func (g *Generator) natural(x, z float64) (h, mount, river, surface float64) {
	low := procedural.FBM(g.sH, x*0.0045, z*0.0045, 2, 2, 0.5)
	hills := 14*procedural.FBM(g.sH, x*0.0045, z*0.0045, 4, 2, 0.5) + 3.5*procedural.Noise2(g.sD, x*0.02, z*0.02) + 0.7*procedural.Noise2(g.sD+1, x*0.09, z*0.09)
	h = 12 + hills
	mm := procedural.SmoothStep(0.05, 0.5, procedural.FBM(g.sM, x*0.0006, z*0.0006, 3, 2, 0.5))
	far := procedural.SmoothStep(520, 1050, realmDistance(x, z))
	ridge := procedural.Ridged(g.sR, x*0.0016, z*0.0016, 5)
	mount = mm * far * (35 + 175*math.Pow(ridge, 1.4))
	h += mount

	wx := x + 140*procedural.Noise2(g.sV+9, x*0.0027, z*0.0027)
	wz := z + 140*procedural.Noise2(g.sV+10, x*0.0027, z*0.0027)
	rv := math.Abs(procedural.FBM(g.sV, wx*0.0011, wz*0.0011, 3, 2, 0.5))
	river = 1 - procedural.SmoothStep(0.012, 0.05, rv)
	river *= 1 - procedural.SmoothStep(55, 130, mount)
	surface = 12 + 14*low - 1.7
	if river > 0 {
		bed := surface - 0.5 - 2.7*river
		h = procedural.Mix(h, bed, procedural.SmoothStep(0, 0.7, river))
	}
	lk := procedural.FBM(g.sL, x*0.0008+50, z*0.0008-20, 3, 2, 0.5)
	basin := procedural.SmoothStep(-0.18, -0.42, lk) * (1 - procedural.SmoothStep(30, 90, mount))
	if basin > 0 {
		h = procedural.Mix(h, math.Min(h, LakeLevel-0.6-3*basin), procedural.SmoothStep(0, 0.6, basin))
		if basin > river {
			river, surface = basin*0.9, LakeLevel
		}
	}
	return
}

// Height returns the final terrain height at (x,z).
func (g *Generator) Height(x, z float64) float64 {
	return g.Sample(x, z).Height
}

// Sample evaluates the world at (x,z).
func (g *Generator) Sample(x, z float64) Sample {
	return g.SampleWith(g.Atlas.Nearby(x, z, 0), x, z)
}

// SampleWith is Sample with the nearby zone list supplied by the caller, which
// avoids repeated lookups while filling a chunk.
func (g *Generator) SampleWith(zs []zones.Zone, x, z float64) Sample {
	h, mount, river, surface := g.natural(x, z)
	var s Sample
	s.Mount = mount
	supp := 0.0
	for i := range zs {
		if v := zs[i].SuppressRivers(x, z); v > supp {
			supp = v
		}
	}
	if supp > 0 && river > 0 {
		// recompute without the river carve where a zone claims the ground
		hn, _, _, _ := g.naturalNoRiver(x, z)
		river *= 1 - supp
		h = procedural.Mix(hn, h, 1-supp)
	}
	s.River = river
	s.HasWater = river > 0.28
	s.WaterY = surface
	s.MaxDepth = 7
	s.Moisture = 0.5 + 0.5*procedural.FBM(g.sW, x*0.002, z*0.002, 3, 2, 0.5)

	bestW := 0.0
	for i := range zs {
		zn := &zs[i]
		if w, rel := zn.Shape(x, z); w > 0 {
			h = procedural.Mix(h, zn.BaseY+rel, w)
		}
		if zn.Hole(x, z) {
			s.Hole = true
		}
		if bw := zn.BiomeWeight(x, z); bw > bestW {
			bestW = bw
			s.ZoneID, s.ZoneW = zn.ID, bw
		}
		if lvl, md, ok := zn.WaterLevel(x, z); ok {
			s.HasWater, s.WaterY, s.MaxDepth = true, zn.BaseY+lvl, md
		}
	}
	if supp > 0.01 {
		// lakes inside zones are driven only by the zone's own water definition
		if _, _, ok := zoneWater(zs, x, z); !ok {
			s.HasWater = false
		}
	}
	s.Height = h
	s.Biome = g.biome(&s, h, mount)
	return s
}

func zoneWater(zs []zones.Zone, x, z float64) (float64, float64, bool) {
	for i := range zs {
		if lvl, md, ok := zs[i].WaterLevel(x, z); ok {
			return zs[i].BaseY + lvl, md, true
		}
	}
	return 0, 0, false
}

func (g *Generator) naturalNoRiver(x, z float64) (h, mount, river, surface float64) {
	hills := 14*procedural.FBM(g.sH, x*0.0045, z*0.0045, 4, 2, 0.5) + 3.5*procedural.Noise2(g.sD, x*0.02, z*0.02) + 0.7*procedural.Noise2(g.sD+1, x*0.09, z*0.09)
	mm := procedural.SmoothStep(0.05, 0.5, procedural.FBM(g.sM, x*0.0006, z*0.0006, 3, 2, 0.5))
	far := procedural.SmoothStep(520, 1050, realmDistance(x, z))
	ridge := procedural.Ridged(g.sR, x*0.0016, z*0.0016, 5)
	mount = mm * far * (35 + 175*math.Pow(ridge, 1.4))
	return 12 + hills + mount, mount, 0, 0
}

func (g *Generator) biome(s *Sample, h, mount float64) vegetation.Biome {
	if s.ZoneW > 0.5 {
		return zones.InfoFor(s.ZoneID).Biome
	}
	switch {
	case h > 125:
		return vegetation.Alpine
	case h > 78:
		return vegetation.Highland
	case s.HasWater && s.WaterY-h > -2.5 && s.Moisture > 0.38:
		return vegetation.Wetland
	case s.Moisture > 0.6:
		return vegetation.Rainforest
	case s.Moisture > 0.44:
		return vegetation.Forest
	}
	return vegetation.Meadow
}

// GroundHeight returns the walkable ground height at (x,z) (terrain only).
func (g *Generator) GroundHeight(x, z float64) float64 { return g.Height(x, z) }

// WaterAt returns the water surface height at (x,z) if water covers the ground.
func (g *Generator) WaterAt(x, z float64) (float64, bool) {
	s := g.Sample(x, z)
	if s.Wet() {
		return s.WaterY, true
	}
	return 0, false
}
