package zones

import (
	"math"

	"evergreen/internal/mesh"
	"evergreen/internal/procedural"
)

type v3 = mesh.Vec3

var (
	stoneS   = mesh.Style{Color: [3]float32{0.47, 0.47, 0.43}, Mat: mesh.MatStone, Jitter: 0.06}
	stoneDk  = mesh.Style{Color: [3]float32{0.3, 0.31, 0.3}, Mat: mesh.MatStone, Jitter: 0.05}
	woodS    = mesh.Style{Color: [3]float32{0.45, 0.31, 0.18}, Mat: mesh.MatWood, Jitter: 0.06}
	woodDk   = mesh.Style{Color: [3]float32{0.28, 0.19, 0.11}, Mat: mesh.MatWood, Jitter: 0.04}
	ropeS    = mesh.Style{Color: [3]float32{0.62, 0.52, 0.34}, Mat: mesh.MatStandard, Jitter: 0.03}
	metalS   = mesh.Style{Color: [3]float32{0.34, 0.37, 0.4}, Mat: mesh.MatMetal, Jitter: 0.02}
	metalDk  = mesh.Style{Color: [3]float32{0.17, 0.18, 0.2}, Mat: mesh.MatMetal}
	vineS    = mesh.Style{Color: [3]float32{0.09, 0.3, 0.1}, Mat: mesh.MatFoliage, SwayK: 0.7, Jitter: 0.06}
	mossS    = mesh.Style{Color: [3]float32{0.12, 0.32, 0.1}, Mat: mesh.MatFoliage, Jitter: 0.06}
	caveRock = mesh.Style{Color: [3]float32{0.2, 0.21, 0.23}, Mat: mesh.MatCave, Jitter: 0.05}
	interior = mesh.Style{Color: [3]float32{0.34, 0.33, 0.31}, Mat: mesh.MatInterior, Jitter: 0.03}
)

func emissive(c [3]float32, variant float32) mesh.Style {
	return mesh.Style{Color: c, Mat: mesh.MatEmissive, Variant: variant}
}

type lmb struct {
	z   Zone
	rng *procedural.RNG
	op  *mesh.Builder // opaque geometry
	tr  *mesh.Builder // transparent geometry
	lm  *Landmark
}

func newLMB(z Zone) *lmb {
	l := &lmb{z: z, rng: procedural.NewRNG(z.Seed), op: mesh.NewBuilder(z.Key, z.Seed), tr: mesh.NewBuilder(z.Key+"-glass", z.Seed+1), lm: &Landmark{Zone: z}}
	l.lm.init()
	l.op.M.TwoSided = true
	l.tr.M.Transparent, l.tr.M.TwoSided, l.tr.M.NoShadow = true, true, true
	return l
}

func (l *lmb) finish(radius float64) *Landmark {
	l.lm.Parts = append([]Part{{Mesh: l.op.Finish()}}, l.lm.Parts...)
	if len(l.tr.M.I) > 0 {
		l.lm.Parts = append(l.lm.Parts, Part{Mesh: l.tr.Finish()})
	}
	l.lm.Radius = radius
	lo, hi := l.op.M.Bounds()
	l.lm.MinY, l.lm.MaxY = lo.Y, hi.Y
	return l.lm
}

func (l *lmb) light(u, y, v float64, r, g, b float32, radius, flicker float64) {
	l.lm.Lights = append(l.lm.Lights, Light{U: u, Y: y, V: v, R: r, G: g, B: b, Radius: radius, Flicker: flicker})
}

func (l *lmb) circle(u, v, r, y0, y1 float64, group int) {
	l.lm.Colliders = append(l.lm.Colliders, Collider{CX: u, CZ: v, R: r, Y0: y0, Y1: y1, Group: group})
}

func (l *lmb) box(u, v, hx, hz, y0, y1 float64, group int) {
	l.lm.Colliders = append(l.lm.Colliders, Collider{CX: u, CZ: v, HX: hx, HZ: hz, Y0: y0, Y1: y1, Group: group})
}

func (l *lmb) wall(u0, v0, u1, v1, r, y0, y1 float64) {
	d := math.Hypot(u1-u0, v1-v0)
	n := int(math.Ceil(d / (r * 1.4)))
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(max(n, 1))
		l.circle(u0+(u1-u0)*t, v0+(v1-v0)*t, r, y0, y1, 0)
	}
}

func (l *lmb) walkSeg(u0, v0, u1, v1, halfW, y0, y1 float64) {
	l.lm.Walk = append(l.lm.Walk, Walkable{X0: u0, Z0: v0, X1: u1, Z1: v1, R: halfW, Y0: y0, Y1: y1})
}

func (l *lmb) walkDisc(u, v, r, y float64) {
	l.lm.Walk = append(l.lm.Walk, Walkable{Disc: true, X0: u, Z0: v, R: r, Y0: y})
}

func (l *lmb) lantern(u, y, v float64, warm bool) {
	c := [3]float32{1, 0.7, 0.32}
	if !warm {
		c = [3]float32{0.5, 0.9, 1}
	}
	l.op.Blob(v3{X: u, Y: y, Z: v}, v3{X: 0.2, Y: 0.28, Z: 0.2}, 4, 6, 0, 1, emissive(c, 0.3))
	l.op.Tube([]v3{{X: u, Y: y + 0.3, Z: v}, {X: u, Y: y + 0.7, Z: v}}, []float64{0.025, 0.025}, 4, ropeS, false)
	l.light(u, y, v, c[0], c[1], c[2], 11, 0.25)
}

func (l *lmb) hangingVines(cx, cy, cz float64, n int, spread, minLen, maxLen float64) {
	for i := 0; i < n; i++ {
		x := cx + l.rng.Range(-spread, spread)
		z := cz + l.rng.Range(-spread, spread)
		ln := l.rng.Range(minLen, maxLen)
		w := l.rng.Range(0.12, 0.22)
		st := vineS
		st.SwayH = ln
		a := l.rng.Range(0, math.Pi)
		dx, dz := math.Cos(a)*w, math.Sin(a)*w
		// vertex sway grows toward the free end
		b := l.op
		v0 := b.Vert(v3{X: x - dx, Y: cy, Z: z - dz}, v3{Y: 0, X: dz, Z: -dx}.Norm(), st.Color, 0, 0, 0, st.Mat)
		v1 := b.Vert(v3{X: x + dx, Y: cy, Z: z + dz}, v3{X: dz, Z: -dx}.Norm(), st.Color, 0, 1, 0, st.Mat)
		v2 := b.Vert(v3{X: x + dx*0.7, Y: cy - ln*0.55, Z: z + dz*0.7}, v3{X: dz, Z: -dx}.Norm(), st.Color, 0.6, 1, 0.5, st.Mat)
		v3_ := b.Vert(v3{X: x - dx*0.7, Y: cy - ln*0.55, Z: z - dz*0.7}, v3{X: dz, Z: -dx}.Norm(), st.Color, 0.6, 0, 0.5, st.Mat)
		v4 := b.Vert(v3{X: x, Y: cy - ln, Z: z}, v3{X: dz, Z: -dx}.Norm(), st.Color, 1, 0.5, 1, st.Mat)
		b.Quad(v0, v1, v2, v3_)
		b.Tri(v3_, v2, v4)
	}
}

// rockPile scatters mossy boulders around a point.
func (l *lmb) rockPile(cx, cz, spread float64, n int, minR, maxR float64, groundY func(u, v float64) float64) {
	for i := 0; i < n; i++ {
		u := cx + l.rng.Range(-spread, spread)
		v := cz + l.rng.Range(-spread, spread)
		r := l.rng.Range(minR, maxR)
		y := 0.0
		if groundY != nil {
			y = groundY(u, v)
		}
		l.op.Blob(v3{X: u, Y: y + r*0.35, Z: v}, v3{X: r, Y: r * 0.7, Z: r * l.rng.Range(0.8, 1.2)}, 7, 10, 0.35, 1.5, stoneS)
		l.circle(u, v, r*0.85, y, y+r*1.1, 0)
	}
}

// stairs builds steps plus a walkable ramp.
func (l *lmb) stairs(u0, v0, y0, u1, v1, y1, width float64, steps int, st mesh.Style) {
	for i := 0; i < steps; i++ {
		t := (float64(i) + 0.5) / float64(steps)
		u, v := u0+(u1-u0)*t, v0+(v1-v0)*t
		y := y0 + (y1-y0)*float64(i+1)/float64(steps)
		yaw := math.Atan2(u1-u0, v1-v0) // box long axis is local x; rotate so it spans the width
		h := (y - (y0 - 0.5)) / 2
		stepLen := math.Hypot(u1-u0, v1-v0) / float64(steps) / 2
		l.op.BoxYaw(v3{X: u, Y: (y + y0 - 0.5) / 2, Z: v}, v3{X: width, Y: h, Z: stepLen + 0.02}, yaw, st)
	}
	l.walkSeg(u0, v0, u1, v1, width, y0+0.1, y1)
}

// Build constructs the landmark for a placed zone.
func Build(z Zone) *Landmark {
	switch z.ID {
	case Entrance:
		return buildEntrance(z)
	case Wilderness:
		return buildWilderness(z)
	case Falls:
		return buildFalls(z)
	case Cave:
		return buildCave(z)
	case Treehouse:
		return buildTreehouse(z)
	case Ruins:
		return buildRuins(z)
	case Observatory:
		return buildObservatory(z)
	case Station:
		return buildStation(z)
	case WorldTree:
		return buildWorldTree(z)
	}
	l := newLMB(z)
	return l.finish(10)
}

func buildEntrance(z Zone) *Landmark {
	l := newLMB(z)
	b := l.op
	// timber gate
	for _, s := range []float64{-5.5, 5.5} {
		b.Tube([]v3{{X: s, Y: -0.4, Z: 0}, {X: s, Y: 4.5, Z: 0}, {X: s * 0.94, Y: 8.6, Z: 0}}, []float64{0.62, 0.55, 0.45}, 8, woodDk, true)
		b.Box(v3{X: s, Y: 0.15, Z: 0}, v3{X: 0.95, Y: 0.3, Z: 0.95}, stoneS)
		l.circle(s, 0, 0.8, -1, 9, 0)
	}
	b.Beam(v3{X: -6.4, Y: 8.4, Z: 0}, v3{X: 6.4, Y: 8.4, Z: 0}, 0.38, woodS)
	b.Beam(v3{X: -5.8, Y: 6.9, Z: 0}, v3{X: 5.8, Y: 6.9, Z: 0}, 0.25, woodS)
	for i := 0; i < 6; i++ {
		x := -4.5 + float64(i)*1.8
		b.Beam(v3{X: x, Y: 6.9, Z: 0}, v3{X: x, Y: 8.4, Z: 0}, 0.1, woodDk)
	}
	for _, x := range []float64{-3.2, 3.2} {
		l.lantern(x, 6.1, 0.3, true)
	}
	l.hangingVines(0, 8.3, 0, 9, 5.5, 1.2, 3.8)
	// signpost toward the World Tree (+x)
	b.Tube([]v3{{X: 8, Y: 0, Z: 6}, {X: 8, Y: 2.6, Z: 6}}, []float64{0.14, 0.12}, 6, woodDk, true)
	b.BoxYaw(v3{X: 8.5, Y: 2.1, Z: 6}, v3{X: 0.9, Y: 0.22, Z: 0.05}, 0, woodS)
	b.TriP(v3{X: 9.4, Y: 1.88, Z: 6.06}, v3{X: 9.4, Y: 2.32, Z: 6.06}, v3{X: 10.1, Y: 2.1, Z: 6.06}, woodS)
	l.circle(8, 6, 0.3, 0, 3, 0)
	l.rockPile(-9, 7, 4, 5, 0.5, 1.1, nil)
	l.rockPile(10, -8, 5, 4, 0.6, 1.2, nil)
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "pollen", U: 0, Y: 1.5, V: 0, Spread: 40, Rate: 20})
	return l.finish(40)
}

func buildWilderness(z Zone) *Landmark {
	l := newLMB(z)
	b := l.op
	// colossal fallen trunk, hollow at one end
	path := []v3{}
	rad := []float64{}
	for i := 0; i <= 12; i++ {
		t := float64(i) / 12
		path = append(path, v3{X: -22 + 44*t, Y: 1.8 + 0.5*math.Sin(t*3), Z: 6 + 1.5*math.Sin(t*4)})
		rad = append(rad, 2.3-0.9*t)
	}
	b.Tube(path, rad, 12, woodDk, true)
	for i := 0; i < 9; i++ {
		t := l.rng.Range(0.1, 0.9)
		p := path[int(t*12)]
		b.Blob(v3{X: p.X, Y: p.Y + 2, Z: p.Z}, v3{X: l.rng.Range(1.5, 3), Y: 0.4, Z: 1.2}, 4, 8, 0.3, 2, mossS)
	}
	l.wall(-22, 6, 22, 7.5, 1.9, -1, 4)
	l.rockPile(0, -10, 14, 9, 1.2, 3.0, nil)
	// giant stone arch with vines
	b.Blob(v3{X: -14, Y: 3, Z: -22}, v3{X: 2.5, Y: 4, Z: 2.2}, 7, 10, 0.3, 1.2, stoneS)
	b.Blob(v3{X: -6, Y: 3, Z: -22}, v3{X: 2.5, Y: 4, Z: 2.2}, 7, 10, 0.3, 1.2, stoneS)
	b.Blob(v3{X: -10, Y: 6.8, Z: -22}, v3{X: 6.2, Y: 1.6, Z: 2.0}, 6, 10, 0.25, 1.2, stoneS)
	l.hangingVines(-10, 6.2, -22, 14, 3.5, 2, 6)
	l.circle(-14, -22, 2.4, 0, 8, 0)
	l.circle(-6, -22, 2.4, 0, 8, 0)
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "mist", U: 0, Y: 1, V: 0, Spread: 55, Rate: 25}, Emitter{Kind: "glow", U: -10, Y: 1.5, V: -20, Spread: 6, Rate: 6})
	l.light(-10, 2, -20, 0.4, 1, 0.7, 12, 0.2)
	return l.finish(60)
}

func buildFalls(z Zone) *Landmark {
	l := newLMB(z)
	// waterfall sheet hugging the cliff: parameterised along u
	const n = 16
	const half = 5.0
	nrm := v3{X: -0.7, Y: 0.7}.Norm()
	wb := l.tr
	var rows [][2]uint32
	for i := 0; i <= n; i++ {
		t := float64(i) / n
		u := -2.7 + 6.4*t
		y := fallsRel(u, 0) + 0.7 + 2.3*(1-sm(-2.5, 0.5, u))
		if i == n {
			y = FallsLowerWater - 0.2
		}
		var row [2]uint32
		for j := 0; j < 2; j++ {
			s := float64(j*2 - 1)
			row[j] = wb.Vert(v3{X: u, Y: y, Z: s * half}, nrm, [3]float32{0.75, 0.95, 0.95}, 0, float32(j), float32(t), mesh.MatFall)
		}
		rows = append(rows, row)
		if i > 0 {
			p := rows[i-1]
			wb.Quad(p[0], p[1], row[1], row[0])
		}
	}
	// pool foam disc
	{
		c := wb.Vert(v3{X: 4.3, Y: FallsLowerWater + 0.05, Z: 0}, v3{Y: 1}, [3]float32{1, 1, 1}, 0, 0.5, 0.5, mesh.MatFall+0.1)
		var ring []uint32
		for s := 0; s <= 24; s++ {
			a := float64(s) / 24 * 2 * math.Pi
			ring = append(ring, wb.Vert(v3{X: 4.3 + math.Cos(a)*8, Y: FallsLowerWater + 0.05, Z: math.Sin(a) * 8}, v3{Y: 1}, [3]float32{1, 1, 1}, 0, float32(0.5+0.5*math.Cos(a)), float32(0.5+0.5*math.Sin(a)), mesh.MatFall+0.1))
		}
		for s := 0; s < 24; s++ {
			wb.Tri(c, ring[s+1], ring[s])
		}
	}
	// rope bridge over the upper river
	b := l.op
	const bu = -42.0
	deck := 13.7
	nPl := 22
	for i := 0; i < nPl; i++ {
		t := float64(i) / float64(nPl-1)
		v := -14 + 28*t
		b.Box(v3{X: bu, Y: deck + 0.1*math.Sin(t*math.Pi), Z: v}, v3{X: 1.2, Y: 0.07, Z: 0.5}, woodS)
	}
	for _, s := range []float64{-1.2, 1.2} {
		var path []v3
		var radii []float64
		for i := 0; i <= 10; i++ {
			t := float64(i) / 10
			path = append(path, v3{X: bu + s, Y: deck + 1.0 + 0.1*math.Sin(t*math.Pi) - 0.0, Z: -14 + 28*t})
			radii = append(radii, 0.05)
		}
		b.Tube(path, radii, 4, ropeS, false)
		for i := 0; i <= 4; i++ {
			v := -14 + 28*float64(i)/4
			b.Beam(v3{X: bu + s, Y: deck, Z: v}, v3{X: bu + s, Y: deck + 1.1, Z: v}, 0.08, woodDk)
		}
	}
	for _, v := range []float64{-14.5, 14.5} {
		for _, s := range []float64{-1.2, 1.2} {
			b.Tube([]v3{{X: bu + s, Y: deck - 2.5, Z: v}, {X: bu + s, Y: deck + 1.6, Z: v}}, []float64{0.18, 0.15}, 6, woodDk, true)
		}
		l.lantern(bu, deck+1.8, v, true)
	}
	l.walkSeg(bu, -14.8, bu, 14.8, 1.3, deck+0.05, deck+0.05)
	// moss boulders around the plunge pool
	l.rockPile(46, -2, 30, 16, 1.0, 2.8, func(u, v float64) float64 {
		if math.Hypot(u-46, v) < 22 {
			return -99 // skip inside the pool
		}
		return fallsRel(u, v)
	})
	l.lm.Emitters = append(l.lm.Emitters,
		Emitter{Kind: "mist", U: 4.5, Y: FallsLowerWater + 1, V: 0, Spread: 7, Rate: 160},
		Emitter{Kind: "splash", U: 3.6, Y: FallsLowerWater + 0.3, V: 0, Spread: 4.5, Rate: 120},
		Emitter{Kind: "mist", U: -2, Y: 9, V: 0, Spread: 4, Rate: 30})
	return l.finish(110)
}

// ---- Cave ---------------------------------------------------------------

func caveNoise(seed uint64, a, r float64) float64 {
	return procedural.Value3(seed, math.Cos(a)*3, math.Sin(a)*3, r*0.05)
}

// caveOuter is the height of the outer rock dome.
func caveOuter(seed uint64, r, a float64) float64 {
	t := math.Max(0, 1-(r/CaveShellRadius)*(r/CaveShellRadius))
	return 30 * math.Pow(t, 0.7) * (1 + 0.16*caveNoise(seed, a, r))
}

// caveCeil is the height of the inner chamber ceiling.
func caveCeil(seed uint64, r, a float64) float64 {
	t := math.Max(0, 1-(r/CaveInnerRadius)*(r/CaveInnerRadius))
	return 19*math.Sqrt(t)*(1+0.12*caveNoise(seed+5, a, r)) + 2.2*math.Sin(a*5+r*0.2)*t
}

type tunnel struct{ angle, r0, r1, w, h float64 }

func (t tunnel) inside(u, v, y float64) bool {
	ca, sa := math.Cos(t.angle), math.Sin(t.angle)
	along := u*ca + v*sa
	lat := -u*sa + v*ca
	if along < t.r0 || along > t.r1 {
		return false
	}
	return (lat/t.w)*(lat/t.w)+(y/t.h)*(y/t.h) < 1
}

func buildCave(z Zone) *Landmark {
	l := newLMB(z)
	seed := z.Seed
	tunnels := []tunnel{{0, 30, 70, 6.6, 8.6}, {2.3, 42, 48, 3.4, 4.6}, {4.0, 42, 48, 3.4, 4.6}}
	removed := func(p [3]v3) bool {
		cu, cy, cv := (p[0].X+p[1].X+p[2].X)/3, (p[0].Y+p[1].Y+p[2].Y)/3, (p[0].Z+p[1].Z+p[2].Z)/3
		for _, t := range tunnels {
			if t.inside(cu, cv, cy) {
				return true
			}
		}
		return false
	}
	const nr, ns = 30, 96
	// grid builder for a polar surface; flip reverses winding for interior surfaces
	surface := func(b *mesh.Builder, rmax float64, height func(r, a float64) float64, st mesh.Style, flip bool, cut bool) {
		pos := make([]v3, (nr+1)*(ns+1))
		for i := 0; i <= nr; i++ {
			r := rmax * float64(i) / nr
			for j := 0; j <= ns; j++ {
				a := 2 * math.Pi * float64(j%ns) / ns
				pos[i*(ns+1)+j] = v3{X: math.Cos(a) * r, Y: height(r, a), Z: math.Sin(a) * r}
			}
		}
		idx := make([]uint32, len(pos))
		for k, p := range pos {
			// normals from finite differences on the grid
			i, j := k/(ns+1), k%(ns+1)
			pu := pos[min(i+1, nr)*(ns+1)+j]
			pm := pos[max(i-1, 0)*(ns+1)+j]
			pn := pos[i*(ns+1)+(j+1)%(ns+1)]
			pq := pos[i*(ns+1)+(j+ns)%(ns+1)]
			n := pu.Sub(pm).Cross(pn.Sub(pq))
			if n.Y < 0 {
				n = n.Scale(-1)
			}
			if flip {
				n = n.Scale(-1)
			}
			idx[k] = b.SV(p, n.Norm(), st, p.X*0.1, p.Z*0.1)
		}
		for i := 0; i < nr; i++ {
			for j := 0; j < ns; j++ {
				a, c, d, e := i*(ns+1)+j, (i+1)*(ns+1)+j, (i+1)*(ns+1)+j+1, i*(ns+1)+j+1
				for _, tri := range [][3]int{{a, c, d}, {a, d, e}} {
					if cut && removed([3]v3{pos[tri[0]], pos[tri[1]], pos[tri[2]]}) {
						continue
					}
					if flip {
						b.Tri(idx[tri[0]], idx[tri[1]], idx[tri[2]])
					} else {
						b.Tri(idx[tri[0]], idx[tri[2]], idx[tri[1]])
					}
				}
			}
		}
	}
	outer := mesh.Style{Color: [3]float32{0.38, 0.38, 0.35}, Mat: mesh.MatStone, Jitter: 0.05}
	surface(l.op, CaveShellRadius, func(r, a float64) float64 { return caveOuter(seed, r, a) }, outer, false, true)
	surface(l.op, CaveInnerRadius, func(r, a float64) float64 { return caveCeil(seed, r, a) }, caveRock, true, true)
	// floor
	floor := caveRock
	floor.Color = [3]float32{0.16, 0.17, 0.18}
	surface(l.op, CaveInnerRadius+1.5, func(r, a float64) float64 {
		u, v := math.Cos(a)*r, math.Sin(a)*r
		return CaveFloor(u, v) + 0.05*procedural.Value3(seed+9, u*0.4, 0, v*0.4)
	}, floor, false, false)
	// tunnels as half-pipes with inward normals
	for ti, t := range tunnels {
		ca, sa := math.Cos(t.angle), math.Sin(t.angle)
		const steps, around = 8, 12
		start := uint32(len(l.op.M.V) / mesh.Stride)
		for s := 0; s <= steps; s++ {
			al := t.r0 + (t.r1-t.r0)*float64(s)/steps
			for k := 0; k <= around; k++ {
				ph := math.Pi * float64(k) / around
				lat, y := math.Cos(ph)*(t.w+0.25), math.Sin(ph)*(t.h+0.3)
				p := v3{X: al*ca - lat*sa, Y: y, Z: al*sa + lat*ca}
				n := v3{X: -math.Cos(ph) * ca / t.w, Y: -math.Sin(ph) / t.h, Z: -math.Cos(ph) * sa / t.w}.Norm()
				l.op.SV(p, n, caveRock, al*0.1, float64(k)*0.3)
			}
		}
		for s := 0; s < steps; s++ {
			for k := 0; k < around; k++ {
				a := start + uint32(s*(around+1)+k)
				l.op.Quad(a, a+1, a+1+uint32(around+1), a+uint32(around+1))
			}
		}
		if ti > 0 { // alcove end cap
			capc := v3{X: (t.r1 + 0.5) * ca, Y: 0.3, Z: (t.r1 + 0.5) * sa}
			l.op.Blob(capc.Add(v3{Y: t.h * 0.4}), v3{X: t.w, Y: t.h, Z: t.w}, 5, 8, 0.2, 1.2, caveRock)
		}
	}
	// stalactites and stalagmites
	for i := 0; i < 70; i++ {
		a := l.rng.Range(0, 2*math.Pi)
		r := math.Sqrt(l.rng.Float()) * (CaveInnerRadius - 5)
		u, v := math.Cos(a)*r, math.Sin(a)*r
		if tunnelNear(tunnels, u, v) {
			continue
		}
		cy := caveCeil(seed, r, a)
		ln := l.rng.Range(1, 4.5)
		w := l.rng.Range(0.25, 0.7)
		l.op.Tube([]v3{{X: u, Y: cy + 0.4, Z: v}, {X: u, Y: cy - ln*0.6, Z: v}, {X: u, Y: cy - ln, Z: v}}, []float64{w, w * 0.6, 0.03}, 6, caveRock, false)
		if i%3 == 0 && math.Hypot(u-CaveLakeU, v-CaveLakeV) > 15 {
			h := l.rng.Range(0.6, 2.2)
			l.op.Cone(v3{X: u + 1, Y: CaveFloor(u+1, v) - 0.1, Z: v}, h, w, 6, caveRock)
		}
	}
	// crystals + glowing fungi + lights
	palette := [][3]float32{{0.3, 0.85, 1}, {0.75, 0.35, 1}, {0.3, 1, 0.6}}
	for i := 0; i < 46; i++ {
		a := l.rng.Range(0, 2*math.Pi)
		r := l.rng.Range(14, CaveInnerRadius-3)
		u, v := math.Cos(a)*r, math.Sin(a)*r
		if tunnelNear(tunnels, u, v) || math.Hypot(u-CaveLakeU, v-CaveLakeV) < 10 {
			continue
		}
		col := palette[i%3]
		st := mesh.Style{Color: col, Mat: mesh.MatCrystal, Jitter: 0.06}
		base := v3{X: u, Y: CaveFloor(u, v), Z: v}
		for k := 0; k < 4; k++ {
			o := base.Add(v3{X: l.rng.Range(-0.5, 0.5), Z: l.rng.Range(-0.5, 0.5)})
			h := l.rng.Range(0.8, 3.2)
			w := l.rng.Range(0.14, 0.3)
			tx, tz := l.rng.Range(-0.4, 0.4), l.rng.Range(-0.4, 0.4)
			l.op.Tube([]v3{o, o.Add(v3{X: tx * 0.5, Y: h * 0.8, Z: tz * 0.5}), o.Add(v3{X: tx, Y: h, Z: tz})}, []float64{w, w * 0.8, 0.01}, 6, st, false)
		}
		if i%4 == 0 {
			l.light(u, base.Y+1.4, v, col[0], col[1], col[2], 16, 0.12)
		}
	}
	for i := 0; i < 60; i++ {
		a := l.rng.Range(0, 2*math.Pi)
		r := l.rng.Range(8, CaveInnerRadius-2)
		u, v := math.Cos(a)*r, math.Sin(a)*r
		if tunnelNear(tunnels, u, v) || math.Hypot(u-CaveLakeU, v-CaveLakeV) < 9 {
			continue
		}
		col := [3]float32{0.2, 0.95, 0.75}
		if i%3 == 1 {
			col = [3]float32{0.55, 0.4, 1}
		}
		fy := CaveFloor(u, v)
		for k := 0; k < 3; k++ {
			ox, oz := l.rng.Range(-0.3, 0.3), l.rng.Range(-0.3, 0.3)
			h := l.rng.Range(0.15, 0.5)
			l.op.Tube([]v3{{X: u + ox, Y: fy, Z: v + oz}, {X: u + ox, Y: fy + h, Z: v + oz}}, []float64{0.03, 0.025}, 5, mesh.Style{Color: [3]float32{0.8, 0.9, 0.85}, Mat: mesh.MatStandard}, false)
			l.op.Blob(v3{X: u + ox, Y: fy + h, Z: v + oz}, v3{X: h * 0.6, Y: h * 0.35, Z: h * 0.6}, 4, 7, 0.05, 2, mesh.Style{Color: col, Mat: mesh.MatGlow})
		}
	}
	l.light(CaveLakeU, 3, CaveLakeV, 0.3, 0.8, 1, 26, 0.08)
	l.light(30, 3, 0, 0.5, 0.95, 0.8, 18, 0.1)
	l.light(-30, 4, -20, 0.7, 0.4, 1, 20, 0.1)
	l.light(0, 5, -34, 0.3, 1, 0.6, 20, 0.1)
	l.light(-8, 4, 32, 0.4, 0.9, 1, 20, 0.1)
	// underground lake surface
	wb := l.tr
	c := wb.Vert(v3{X: CaveLakeU, Y: CaveLakeLevel, Z: CaveLakeV}, v3{Y: 1}, [3]float32{0.1, 0.4, 0.45}, 0, 0, 0, mesh.MatWater+0.1)
	var ring []uint32
	for s := 0; s <= 40; s++ {
		a := float64(s) / 40 * 2 * math.Pi
		ring = append(ring, wb.Vert(v3{X: CaveLakeU + math.Cos(a)*14.5, Y: CaveLakeLevel, Z: CaveLakeV + math.Sin(a)*14.5}, v3{Y: 1}, [3]float32{0.1, 0.4, 0.45}, 0, 1, 1, mesh.MatWater+0.1))
	}
	for s := 0; s < 40; s++ {
		wb.Tri(c, ring[s+1], ring[s])
	}
	// colliders: ring of boulders just inside the stone wall so players cannot leave the chamber
	for k := 0; k < 48; k++ {
		a := float64(k) / 48 * 2 * math.Pi
		if tunnelNear(tunnels[:1], math.Cos(a)*CaveInnerRadius, math.Sin(a)*CaveInnerRadius) {
			continue
		}
		l.circle(math.Cos(a)*(CaveInnerRadius+1.5), math.Sin(a)*(CaveInnerRadius+1.5), 3, -3, 25, 0)
	}
	// tunnel side walls (keep the player inside the half-pipe)
	for al := 30.0; al < 70; al += 2 {
		l.circle(al, 7.6, 1.2, -3, 10, 0)
		l.circle(al, -7.6, 1.2, -3, 10, 0)
	}
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "glow", U: 0, Y: 3, V: 0, Spread: 40, Rate: 40}, Emitter{Kind: "mist", U: CaveLakeU, Y: 0.3, V: CaveLakeV, Spread: 13, Rate: 10})
	return l.finish(80)
}

func tunnelNear(ts []tunnel, u, v float64) bool {
	for _, t := range ts {
		ca, sa := math.Cos(t.angle), math.Sin(t.angle)
		along := u*ca + v*sa
		lat := -u*sa + v*ca
		if along > t.r0-5 && along < t.r1+3 && math.Abs(lat) < t.w+3 {
			return true
		}
	}
	return false
}
