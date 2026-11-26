package world

import (
	"math"

	"evergreen/internal/procedural"
	"evergreen/internal/terrain"
	"evergreen/internal/vegetation"
	"evergreen/internal/zones"
)

// trunkRadius is the collider radius for each tree kind at scale 1.
var trunkRadius = map[vegetation.Kind]float64{
	vegetation.TreeOak: 0.5, vegetation.TreePine: 0.42, vegetation.TreeRain: 1.0,
	vegetation.TreeAncient: 2.3, vegetation.TreeBirch: 0.28, vegetation.Boulder: 1.2, vegetation.Rock: 0.55,
}

type placer struct {
	c    *Chunk
	gr   *terrain.Grid
	g    *terrain.Generator
	seed uint64
	cfg  Config
}

func (p *placer) rng(salt uint64) *procedural.RNG {
	return procedural.NewRNG(procedural.Hash3(p.seed, int32(p.c.CX), int32(p.c.CZ), int32(salt)))
}

// sampleAt returns the grid sample nearest to world (x,z).
func (p *placer) sampleAt(x, z float64) (*terrain.Sample, int, int) {
	i := int(math.Round((x - p.gr.X0) / p.gr.Step))
	j := int(math.Round((z - p.gr.Z0) / p.gr.Step))
	i, j = max(-1, min(terrain.ChunkCells+1, i)), max(-1, min(terrain.ChunkCells+1, j))
	return p.gr.At(i, j), i, j
}

// ok reports whether vegetation may grow at (x,z) and returns the sample.
func (p *placer) ok(x, z float64, maxSlope float64) (*terrain.Sample, bool) {
	s, i, j := p.sampleAt(x, z)
	if s.Hole || s.River > 0.22 || (s.HasWater && s.WaterY-s.Height > -0.15) {
		return s, false
	}
	if p.gr.PathAt(i, j) > 0.25 {
		return s, false
	}
	if p.gr.Slope(i, j) > maxSlope {
		return s, false
	}
	for k := range p.gr.Near {
		if p.gr.Near[k].Excluded(x, z) {
			return s, false
		}
	}
	return s, true
}

func (p *placer) add(l Layer, k vegetation.Kind, v int, x, z, y, scale, yaw, tint float64) {
	p.c.Layers[l] = append(p.c.Layers[l], Item{X: float32(x), Y: float32(y - 0.05*scale), Z: float32(z), Scale: float32(scale), Yaw: float32(yaw), Tint: float32(tint), Kind: k, Variant: uint8(v)})
	if r, ok := trunkRadius[k]; ok {
		p.c.Colliders = append(p.c.Colliders, Circle{X: float32(x), Z: float32(z), R: float32(r * scale)})
	}
}

func (p *placer) height(x, z float64) float64 { return p.gr.HeightBilinear(x, z) }

// trees places forest trees, ancient giants and GitHub repository trees.
func (p *placer) trees() {
	r := p.rng(1)
	x0, z0 := p.gr.X0, p.gr.Z0
	const cell = 4.0
	n := int(math.Floor(terrain.ChunkSize / cell))
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			x := x0 + (float64(i)+r.Range(0.1, 0.9))*cell
			z := z0 + (float64(j)+r.Range(0.1, 0.9))*cell
			s, ok := p.ok(x, z, 0.42)
			roll, u, kindRoll := r.Float(), r.Float(), r.Float()
			if !ok {
				continue
			}
			prof := vegetation.ProfileFor(s.Biome)
			if prof.TreeChance == 0 {
				continue
			}
			ratio := cell / prof.TreeSpacing
			chance := prof.TreeChance * ratio * ratio * p.cfg.Influence.Density
			// thin out toward tree line and near water edges
			if s.Height > 100 {
				chance *= math.Max(0, 1-(s.Height-100)/60)
			}
			if roll > chance {
				continue
			}
			kind := prof.PickTree(kindRoll)
			variant := int(u * 3)
			scale := 0.8 + 0.5*r.Float()
			p.add(LayerTrees, kind, variant, x, z, p.height(x, z), scale, r.Range(0, 2*math.Pi), r.Float())
		}
	}
	// ancient trees on a coarse grid
	const acell = 30.0
	an := int(math.Floor(terrain.ChunkSize / acell))
	for j := 0; j <= an; j++ {
		for i := 0; i <= an; i++ {
			x := x0 + (float64(i)+r.Range(0.15, 0.85))*acell
			z := z0 + (float64(j)+r.Range(0.15, 0.85))*acell
			roll, u := r.Float(), r.Float()
			if x >= x0+terrain.ChunkSize || z >= z0+terrain.ChunkSize {
				continue
			}
			s, ok := p.ok(x, z, 0.35)
			if !ok {
				continue
			}
			prof := vegetation.ProfileFor(s.Biome)
			if roll > prof.AncientChance*p.cfg.Influence.Density {
				continue
			}
			p.add(LayerTrees, vegetation.TreeAncient, int(u*3), x, z, p.height(x, z), 0.85+0.5*r.Float(), r.Range(0, 2*math.Pi), r.Float())
		}
	}
	// repository trees around the forest entrance
	if len(p.cfg.Influence.Trees) > 0 {
		e := p.g.Atlas.Find(zones.Entrance, 0, 0)
		for _, t := range p.cfg.Influence.Trees {
			x, z := e.X+t.DX, e.Z+t.DZ
			if x < x0 || x >= x0+terrain.ChunkSize || z < z0 || z >= z0+terrain.ChunkSize {
				continue
			}
			if _, ok := p.ok(x, z, 0.5); !ok {
				continue
			}
			p.add(LayerTrees, t.Kind, int(procedural.Hash2(p.seed, int32(x), int32(z))%3), x, z, p.height(x, z), t.Scale, 0, 0.5)
		}
	}
}

// understory places shrubs, ferns, rocks, fungi, logs and reeds.
func (p *placer) understory() {
	r := p.rng(2)
	const cell = 3.0
	n := int(math.Floor(terrain.ChunkSize / cell))
	x0, z0 := p.gr.X0, p.gr.Z0
	d := p.cfg.Density
	boost := 1 + 0.6*p.cfg.Influence.PlantBoost
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			x := x0 + (float64(i)+r.Range(0.1, 0.9))*cell
			z := z0 + (float64(j)+r.Range(0.1, 0.9))*cell
			s, ok := p.ok(x, z, 0.6)
			a, b, c2, e, f := r.Float(), r.Float(), r.Float(), r.Float(), r.Float()
			if !ok {
				// reeds grow right at the waterline
				if s.HasWater && !s.Hole && math.Abs(s.WaterY-s.Height) < 0.5 && a < 0.35*d {
					p.add(LayerSmall, vegetation.Reed, 0, x, z, p.height(x, z), 0.8+0.6*b, r.Range(0, 6), c2)
				}
				continue
			}
			prof := vegetation.ProfileFor(s.Biome)
			y := p.height(x, z)
			switch {
			case a < prof.ShrubChance*d*boost:
				p.add(LayerSmall, vegetation.Shrub, int(b*2), x, z, y, 0.7+0.8*c2, r.Range(0, 6), e)
			case a < (prof.ShrubChance+prof.FernChance)*d*boost:
				p.add(LayerSmall, vegetation.Fern, 0, x, z, y, 0.7+0.8*c2, r.Range(0, 6), e)
			case a < (prof.ShrubChance+prof.FernChance+prof.RockChance)*d:
				if b < 0.12 {
					p.add(LayerSmall, vegetation.Boulder, int(c2*2), x, z, y, 0.8+1.0*e, r.Range(0, 6), f)
				} else {
					p.add(LayerSmall, vegetation.Rock, int(c2*2), x, z, y, 0.5+0.9*e, r.Range(0, 6), f)
				}
			case a < (prof.ShrubChance+prof.FernChance+prof.RockChance+prof.MushroomChance)*d:
				if prof.GlowChance > 0 && b < prof.GlowChance {
					p.add(LayerSmall, vegetation.GlowMushroom, 0, x, z, y, 0.8+1.2*c2, r.Range(0, 6), e)
				} else {
					p.add(LayerSmall, vegetation.Mushroom, 0, x, z, y, 0.8+0.8*c2, r.Range(0, 6), e)
				}
			case a > 0.995-0.01*prof.TreeChance*d && prof.TreeChance > 0.3:
				p.add(LayerSmall, vegetation.Log, 0, x, z, y, 0.8+0.6*b, r.Range(0, 6), 0)
			}
		}
	}
}

// flowers places clustered blossoms.
func (p *placer) flowers() {
	r := p.rng(3)
	const cell = 2.5
	n := int(math.Floor(terrain.ChunkSize / cell))
	x0, z0 := p.gr.X0, p.gr.Z0
	boost := 1 + 1.2*p.cfg.Influence.FlowerBoost
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			cx := x0 + (float64(i)+r.Range(0.1, 0.9))*cell
			cz := z0 + (float64(j)+r.Range(0.1, 0.9))*cell
			s, ok := p.ok(cx, cz, 0.4)
			roll, hue := r.Float(), r.Float()
			if !ok {
				continue
			}
			prof := vegetation.ProfileFor(s.Biome)
			if roll > prof.FlowerChance*p.cfg.Density*boost*0.5 {
				continue
			}
			cnt := 3 + r.Intn(5)
			for k := 0; k < cnt; k++ {
				x, z := cx+r.Range(-1.1, 1.1), cz+r.Range(-1.1, 1.1)
				if _, ok := p.ok(x, z, 0.4); !ok {
					continue
				}
				p.add(LayerSmall, vegetation.Flower, int(r.Float()*2), x, z, p.height(x, z), 0.8+0.6*r.Float(), r.Range(0, 6), math.Mod(hue+r.Range(-0.06, 0.06)+1, 1))
			}
		}
	}
}

// grass scatters swaying grass clumps.
func (p *placer) grass() {
	r := p.rng(4)
	const cell = 1.6
	n := int(math.Floor(terrain.ChunkSize / cell))
	x0, z0 := p.gr.X0, p.gr.Z0
	boost := 1 + 0.5*p.cfg.Influence.PlantBoost
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			x := x0 + (float64(i)+r.Range(0.05, 0.95))*cell
			z := z0 + (float64(j)+r.Range(0.05, 0.95))*cell
			s, ok := p.ok(x, z, 0.5)
			a, b, c := r.Float(), r.Float(), r.Float()
			if !ok {
				continue
			}
			prof := vegetation.ProfileFor(s.Biome)
			dens := prof.GrassDensity * p.cfg.Density * boost * cell * cell
			cnt := int(dens)
			if a < dens-float64(cnt) {
				cnt++
			}
			for k := 0; k < cnt; k++ {
				gx, gz := x+(b-0.5)*cell*0.5*float64(k), z+(c-0.5)*cell*0.5*float64(k)
				p.add(LayerGrass, vegetation.Grass, int(b*2), gx, gz, p.height(gx, gz), 0.7+0.9*c, r.Range(0, 6), s.Moisture)
			}
		}
	}
}
