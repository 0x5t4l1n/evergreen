package vegetation

import (
	"math"
	"sync"

	"evergreen/internal/mesh"
	"evergreen/internal/procedural"
)

type v3 = mesh.Vec3

// Kind identifies a vegetation prototype family.
type Kind int

// Prototype families.
const (
	TreeOak Kind = iota
	TreePine
	TreeRain
	TreeAncient
	TreeBirch
	Shrub
	Fern
	Rock
	Boulder
	Grass
	Flower
	Mushroom
	GlowMushroom
	Crystal
	Log
	Reed
	numKinds
)

// Variants reports how many distinct meshes exist for a kind.
func Variants(k Kind) int {
	switch k {
	case TreeOak, TreePine, TreeRain, TreeAncient, TreeBirch:
		return 3
	case Grass, Flower, Rock, Boulder, Shrub:
		return 2
	}
	return 1
}

// IsTree reports whether the kind is a tree.
func (k Kind) IsTree() bool { return k <= TreeBirch }

var (
	cacheMu sync.Mutex
	cache   = map[[3]int]*mesh.Mesh{}
)

// Get returns the (cached) mesh for kind/variant at the given level of detail
// (0 = full, 1 = reduced). Meshes are shared and must be treated as read-only.
func Get(k Kind, variant, lod int) *mesh.Mesh {
	if n := Variants(k); n > 0 {
		variant = ((variant % n) + n) % n
	}
	if lod > 1 {
		lod = 1
	}
	key := [3]int{int(k), variant, lod}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	if m, ok := cache[key]; ok {
		return m
	}
	m := build(k, variant, lod)
	cache[key] = m
	return m
}

func build(k Kind, variant, lod int) *mesh.Mesh {
	seed := uint64(k)*7919 + uint64(variant)*104729 + 17
	switch k {
	case TreeOak:
		return oak(seed, lod)
	case TreePine:
		return pine(seed, lod)
	case TreeRain:
		return rainTree(seed, lod)
	case TreeAncient:
		return AncientTree(seed, lod)
	case TreeBirch:
		return birch(seed, lod)
	case Shrub:
		return shrub(seed, lod)
	case Fern:
		return fern(seed)
	case Rock:
		return rock(seed, 1.0, false)
	case Boulder:
		return rock(seed, 2.2, true)
	case Grass:
		return grass(seed)
	case Flower:
		return flower(seed)
	case Mushroom:
		return mushroom(seed, false)
	case GlowMushroom:
		return mushroom(seed, true)
	case Crystal:
		return crystal(seed)
	case Log:
		return logMesh(seed)
	case Reed:
		return reed(seed)
	}
	return mesh.NewBuilder("empty", 0).Finish()
}

var (
	barkCol  = [3]float32{0.30, 0.21, 0.14}
	leafCol  = [3]float32{0.07, 0.36, 0.14}
	leafDark = [3]float32{0.04, 0.24, 0.10}
	stoneCol = [3]float32{0.42, 0.42, 0.40}
	mossCol  = [3]float32{0.10, 0.30, 0.10}
)

func leafStyle(h float64, c [3]float32) mesh.Style {
	return mesh.Style{Color: c, Mat: mesh.MatFoliage, SwayK: 0.55, SwayH: h, Jitter: 0.08}
}

func barkStyle(h float64, c [3]float32) mesh.Style {
	return mesh.Style{Color: c, Mat: mesh.MatBark, SwayK: 0.05, SwayH: h, Jitter: 0.04}
}

func detail(lod, hi, lo int) int {
	if lod == 0 {
		return hi
	}
	return lo
}

// branch grows a curved limb and returns its tip.
func branch(b *mesh.Builder, from v3, dir v3, length, r0 float64, steps, sides int, st mesh.Style, droop float64) v3 {
	path := make([]v3, steps+1)
	radii := make([]float64, steps+1)
	p := from
	d := dir.Norm()
	for i := 0; i <= steps; i++ {
		path[i] = p
		radii[i] = math.Max(0.02, r0*(1-0.85*float64(i)/float64(steps)))
		p = p.Add(d.Scale(length / float64(steps)))
		d = v3{X: d.X, Y: d.Y - droop/float64(steps), Z: d.Z}.Norm()
	}
	b.Tube(path, radii, sides, st, false)
	return path[steps]
}

func oak(seed uint64, lod int) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	h := r.Range(9, 13)
	b := mesh.NewBuilder("oak", seed)
	b.M.TwoSided = true
	bark := barkStyle(h, barkCol)
	sides := detail(lod, 8, 5)
	steps := detail(lod, 5, 3)
	trunkH := h * 0.55
	path := make([]v3, steps+1)
	radii := make([]float64, steps+1)
	lean := v3{X: r.Range(-0.5, 0.5), Z: r.Range(-0.5, 0.5)}
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		path[i] = v3{X: lean.X * t * t, Y: trunkH * t, Z: lean.Z * t * t}
		radii[i] = 0.42 - 0.22*t
		if i == 0 {
			radii[i] = 0.55
		}
	}
	b.Tube(path, radii, sides, bark, true)
	top := path[steps]
	nb := detail(lod, 4, 3)
	for i := 0; i < nb; i++ {
		a := float64(i)/float64(nb)*2*math.Pi + r.Range(-0.4, 0.4)
		from := path[steps-1+(i%2)*0].Add(v3{Y: -0.3 * float64(i%2)})
		dir := v3{X: math.Cos(a), Y: r.Range(0.5, 0.9), Z: math.Sin(a)}
		tip := branch(b, from, dir, r.Range(2.6, 3.8), 0.17, 3, 5, bark, 0.3)
		rings, segs := detail(lod, 6, 4), detail(lod, 9, 6)
		rad := v3{X: r.Range(2.0, 2.8), Y: r.Range(1.5, 2.1), Z: r.Range(2.0, 2.8)}
		b.Blob(tip.Add(v3{Y: 0.9}), rad, rings, segs, 0.32, 1.3, leafStyle(h, leafCol))
	}
	b.Blob(top.Add(v3{Y: 1.6}), v3{X: 3.0, Y: 2.4, Z: 3.0}, detail(lod, 7, 4), detail(lod, 10, 6), 0.3, 1.2, leafStyle(h, leafDark))
	return b.Finish()
}

func pine(seed uint64, lod int) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	h := r.Range(13, 19)
	b := mesh.NewBuilder("pine", seed)
	b.M.TwoSided = true
	b.Tube([]v3{{}, {Y: h * 0.4}, {Y: h * 0.9}}, []float64{0.4, 0.25, 0.08}, detail(lod, 7, 5), barkStyle(h, barkCol), true)
	tiers := detail(lod, 8, 5)
	sides := detail(lod, 9, 6)
	for i := 0; i < tiers; i++ {
		t := float64(i) / float64(tiers)
		rad := (1-t)*2.9 + 0.5
		y := h*0.16 + t*h*0.76
		col := [3]float32{0.04, 0.27 + float32(t)*0.05, 0.12}
		b.Cone(v3{Y: y}, h*0.24, rad, sides, leafStyle(h, col))
	}
	return b.Finish()
}

func rainTree(seed uint64, lod int) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	h := r.Range(22, 31)
	b := mesh.NewBuilder("raintree", seed)
	b.M.TwoSided = true
	bark := barkStyle(h, [3]float32{0.27, 0.2, 0.14})
	steps := detail(lod, 7, 4)
	path := make([]v3, steps+1)
	radii := make([]float64, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		path[i] = v3{X: math.Sin(t*3+float64(seed%5)) * 0.5, Y: h * 0.82 * t, Z: math.Cos(t*2.6) * 0.4}
		radii[i] = 0.5 + 0.9*math.Pow(1-t, 3) + 0.15*(1-t)
	}
	b.Tube(path, radii, detail(lod, 9, 6), bark, true)
	nb := detail(lod, 5, 3)
	for i := 0; i < nb; i++ {
		a := float64(i)/float64(nb)*2*math.Pi + r.Range(0, 1)
		d := v3{X: math.Cos(a), Z: math.Sin(a)}
		b.Tube([]v3{d.Scale(0.4).Add(v3{Y: 2.4}), d.Scale(1.4).Add(v3{Y: 0.8}), d.Scale(2.8).Add(v3{Y: -0.1})},
			[]float64{0.07, 0.22, 0.28}, 5, bark, false)
	}
	top := path[steps]
	nc := detail(lod, 6, 3)
	for i := 0; i < nc; i++ {
		a := float64(i)/float64(nc)*2*math.Pi + r.Range(0, 1)
		off := v3{X: math.Cos(a), Z: math.Sin(a)}.Scale(r.Range(3, 6))
		b.Blob(top.Add(off).Add(v3{Y: r.Range(-3, 1)}), v3{X: r.Range(4.5, 6.5), Y: r.Range(1.6, 2.4), Z: r.Range(4.5, 6.5)},
			detail(lod, 5, 4), detail(lod, 9, 6), 0.3, 1.0, leafStyle(h, leafCol))
	}
	b.Blob(top.Add(v3{Y: 1.5}), v3{X: 6, Y: 2.5, Z: 6}, detail(lod, 6, 4), detail(lod, 10, 6), 0.3, 1.0, leafStyle(h, leafDark))
	if lod == 0 {
		vs := leafStyle(h, [3]float32{0.08, 0.32, 0.12})
		for i := 0; i < 5; i++ {
			a := r.Range(0, 2*math.Pi)
			rad := r.Range(2.5, 6)
			l := r.Range(5, 11)
			x, z := math.Cos(a)*rad, math.Sin(a)*rad
			y0 := top.Y - 1.5
			w := 0.16
			b.QuadP(v3{X: x - w, Y: y0 - l, Z: z}, v3{X: x + w, Y: y0 - l, Z: z}, v3{X: x + w, Y: y0, Z: z}, v3{X: x - w, Y: y0, Z: z}, vs)
		}
	}
	return b.Finish()
}

// AncientTree builds a colossal moss-covered tree; also used by landmarks.
func AncientTree(seed uint64, lod int) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	h := r.Range(38, 50)
	b := mesh.NewBuilder("ancient", seed)
	b.M.TwoSided = true
	bark := barkStyle(h, [3]float32{0.25, 0.2, 0.15})
	moss := barkStyle(h, [3]float32{0.16, 0.26, 0.11})
	steps := detail(lod, 10, 5)
	path := make([]v3, steps+1)
	radii := make([]float64, steps+1)
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		path[i] = v3{X: math.Sin(t*2.4+float64(seed%7)) * 1.2, Y: h * 0.7 * t, Z: math.Cos(t*2.0) * 1.0}
		radii[i] = 1.7 + 1.5*math.Pow(1-t, 4) - 0.6*t
	}
	b.Tube(path, radii, detail(lod, 12, 7), moss, true)
	nr := detail(lod, 7, 4)
	for i := 0; i < nr; i++ {
		a := float64(i)/float64(nr)*2*math.Pi + r.Range(-0.3, 0.3)
		d := v3{X: math.Cos(a), Z: math.Sin(a)}
		l := r.Range(4, 7)
		b.Tube([]v3{d.Scale(0.8).Add(v3{Y: 5}), d.Scale(l * 0.35).Add(v3{Y: 1.8}), d.Scale(l * 0.7).Add(v3{Y: 0.5}), d.Scale(l).Add(v3{Y: -0.4})},
			[]float64{0.6, 0.8, 0.6, 0.3}, detail(lod, 7, 4), bark, false)
	}
	top := path[steps]
	nb := detail(lod, 6, 3)
	for i := 0; i < nb; i++ {
		a := float64(i)/float64(nb)*2*math.Pi + r.Range(-0.3, 0.3)
		dir := v3{X: math.Cos(a), Y: r.Range(0.35, 0.8), Z: math.Sin(a)}
		from := path[steps-2]
		tip := branch(b, from, dir, r.Range(8, 13), 0.7, 4, detail(lod, 7, 4), bark, 0.15)
		b.Blob(tip.Add(v3{Y: 2}), v3{X: r.Range(7, 10), Y: r.Range(4.5, 6), Z: r.Range(7, 10)}, detail(lod, 7, 4), detail(lod, 10, 6), 0.3, 0.8, leafStyle(h, leafCol))
	}
	b.Blob(top.Add(v3{Y: 3}), v3{X: 11, Y: 6, Z: 11}, detail(lod, 8, 4), detail(lod, 12, 6), 0.3, 0.8, leafStyle(h, leafDark))
	if lod == 0 {
		vs := leafStyle(h, [3]float32{0.1, 0.3, 0.1})
		for i := 0; i < 8; i++ {
			a := r.Range(0, 2*math.Pi)
			rad := r.Range(5, 11)
			x, z := math.Cos(a)*rad, math.Sin(a)*rad
			y0 := top.Y
			l := r.Range(10, 22)
			b.QuadP(v3{X: x - 0.2, Y: y0 - l, Z: z}, v3{X: x + 0.2, Y: y0 - l, Z: z}, v3{X: x + 0.2, Y: y0, Z: z}, v3{X: x - 0.2, Y: y0, Z: z}, vs)
		}
	}
	return b.Finish()
}

func birch(seed uint64, lod int) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	h := r.Range(8, 11.5)
	b := mesh.NewBuilder("birch", seed)
	b.M.TwoSided = true
	bk := mesh.Style{Color: [3]float32{0.82, 0.82, 0.76}, Mat: mesh.MatBark, SwayK: 0.08, SwayH: h, Jitter: 0.12}
	b.Tube([]v3{{}, {X: 0.2, Y: h * 0.4}, {X: -0.1, Y: h * 0.8}}, []float64{0.2, 0.14, 0.06}, detail(lod, 6, 4), bk, true)
	ls := leafStyle(h, [3]float32{0.28, 0.5, 0.14})
	n := detail(lod, 4, 2)
	for i := 0; i < n; i++ {
		a := float64(i) / float64(n) * 2 * math.Pi
		b.Blob(v3{X: math.Cos(a) * 1.1, Y: h*0.72 + r.Range(0, 2), Z: math.Sin(a) * 1.1}, v3{X: 1.6, Y: 1.4, Z: 1.6}, detail(lod, 5, 4), detail(lod, 8, 5), 0.3, 1.5, ls)
	}
	b.Blob(v3{Y: h*0.95 + 0.5}, v3{X: 1.7, Y: 1.5, Z: 1.7}, detail(lod, 5, 4), detail(lod, 8, 5), 0.3, 1.5, ls)
	return b.Finish()
}

func shrub(seed uint64, lod int) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("shrub", seed)
	b.M.TwoSided = true
	ls := leafStyle(1.2, [3]float32{0.1, 0.36, 0.13})
	for i := 0; i < 4; i++ {
		b.Blob(v3{X: r.Range(-0.5, 0.5), Y: r.Range(0.35, 0.6), Z: r.Range(-0.5, 0.5)}, v3{X: r.Range(0.55, 0.85), Y: r.Range(0.4, 0.6), Z: r.Range(0.55, 0.85)}, 4, 7, 0.3, 2, ls)
	}
	return b.Finish()
}

func frond(b *mesh.Builder, a, length, width, lift float64, st mesh.Style) {
	const segs = 4
	dir := v3{X: math.Cos(a), Z: math.Sin(a)}
	side := v3{X: -dir.Z, Z: dir.X}
	var prev [2]uint32
	for i := 0; i <= segs; i++ {
		t := float64(i) / segs
		p := dir.Scale(length * t).Add(v3{Y: lift*math.Sin(t*2.2) - 0.55*t*t*length*0.5 + 0.05})
		w := width * (1 - t*t) * math.Sin(math.Pi*math.Min(1, t*1.4+0.2))
		n := v3{Y: 1}
		l := b.Vert(p.Add(side.Scale(-w)), n, st.Color, st.SwayK*float32(0.3+0.7*t), 0, float32(t), st.Mat)
		rr := b.Vert(p.Add(side.Scale(w)), n, st.Color, st.SwayK*float32(0.3+0.7*t), 1, float32(t), st.Mat)
		if i > 0 {
			b.Quad(prev[0], prev[1], rr, l)
		}
		prev = [2]uint32{l, rr}
	}
}

func fern(seed uint64) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("fern", seed)
	b.M.TwoSided = true
	st := mesh.Style{Color: [3]float32{0.1, 0.42, 0.14}, Mat: mesh.MatFoliage, SwayK: 0.6, SwayH: 1, Jitter: 0.06}
	for i := 0; i < 9; i++ {
		frond(b, float64(i)/9*2*math.Pi+r.Range(-0.2, 0.2), r.Range(0.9, 1.5), 0.22, r.Range(0.4, 0.8), st)
	}
	return b.Finish()
}

func rock(seed uint64, size float64, big bool) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("rock", seed)
	st := mesh.Style{Color: stoneCol, Mat: mesh.MatStone, Jitter: 0.05}
	b.Blob(v3{Y: size * 0.28}, v3{X: size * r.Range(0.8, 1.1), Y: size * r.Range(0.5, 0.75), Z: size * r.Range(0.7, 1.0)}, 8, 11, 0.38, 1.6, st)
	if big {
		b.Blob(v3{X: size * 0.7, Y: size * 0.18, Z: size * 0.3}, v3{X: size * 0.5, Y: size * 0.36, Z: size * 0.45}, 6, 8, 0.35, 1.8, st)
	}
	return b.Finish()
}

func grass(seed uint64) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("grass", seed)
	b.M.TwoSided = true
	b.M.NoShadow = true
	base := [3]float32{0.05, 0.2, 0.06}
	mid := [3]float32{0.14, 0.44, 0.12}
	tip := [3]float32{0.42, 0.68, 0.2}
	for i := 0; i < 6; i++ {
		a := r.Range(0, 2*math.Pi)
		rad := r.Range(0, 0.22)
		ox, oz := math.Cos(a)*rad, math.Sin(a)*rad
		ya := r.Range(0, math.Pi)
		dx, dz := math.Cos(ya), math.Sin(ya)
		h := r.Range(0.5, 0.95)
		w := r.Range(0.035, 0.055)
		lean := r.Range(0.1, 0.4)
		n := v3{X: -dz, Y: 0.3, Z: dx}.Norm()
		bl := b.Vert(v3{X: ox - dx*w, Z: oz - dz*w}, n, base, 0, 0, 0, mesh.MatGrass)
		br := b.Vert(v3{X: ox + dx*w, Z: oz + dz*w}, n, base, 0, 1, 0, mesh.MatGrass)
		ml := b.Vert(v3{X: ox - dx*w*0.7 + dz*lean*0.2, Y: h * 0.55, Z: oz - dz*w*0.7 - dx*lean*0.2}, n, mid, 0.55, 0, 0.55, mesh.MatGrass)
		mr := b.Vert(v3{X: ox + dx*w*0.7 + dz*lean*0.2, Y: h * 0.55, Z: oz + dz*w*0.7 - dx*lean*0.2}, n, mid, 0.55, 1, 0.55, mesh.MatGrass)
		tp := b.Vert(v3{X: ox + dz*lean, Y: h, Z: oz - dx*lean}, n, tip, 1, 0.5, 1, mesh.MatGrass)
		b.Quad(bl, br, mr, ml)
		b.Tri(ml, mr, tp)
	}
	return b.Finish()
}

func flower(seed uint64) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("flower", seed)
	b.M.TwoSided = true
	b.M.NoShadow = true
	h := r.Range(0.35, 0.6)
	stem := mesh.Style{Color: [3]float32{0.1, 0.38, 0.1}, Mat: mesh.MatFoliage, SwayK: 0.8, SwayH: h}
	b.Tube([]v3{{}, {X: 0.03, Y: h * 0.5}, {X: 0.05, Y: h}}, []float64{0.012, 0.01, 0.01}, 4, stem, false)
	pet := mesh.Style{Color: [3]float32{1, 1, 1}, Mat: mesh.MatFlower, SwayK: 0.8, SwayH: h}
	np := 6 + int(seed%3)
	for i := 0; i < np; i++ {
		a := float64(i) / float64(np) * 2 * math.Pi
		c, s := math.Cos(a), math.Sin(a)
		cx, cz := 0.05, 0.0
		b.TriP(v3{X: cx, Y: h, Z: cz}, v3{X: cx + c*0.09 - s*0.03, Y: h + 0.03, Z: cz + s*0.09 + c*0.03}, v3{X: cx + c*0.09 + s*0.03, Y: h + 0.03, Z: cz + s*0.09 - c*0.03}, pet)
	}
	ctr := mesh.Style{Color: [3]float32{0.95, 0.75, 0.15}, Mat: mesh.MatStandard, SwayK: 0.8, SwayH: h}
	b.Blob(v3{X: 0.05, Y: h + 0.015}, v3{X: 0.025, Y: 0.02, Z: 0.025}, 3, 5, 0, 1, ctr)
	return b.Finish()
}

func mushroom(seed uint64, glow bool) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	name := "mushroom"
	capMat := mesh.MatStandard
	capCol := [3]float32{0.62, 0.18, 0.12}
	if glow {
		name, capMat, capCol = "glowshroom", mesh.MatGlow, [3]float32{0.3, 0.9, 0.8}
	}
	b := mesh.NewBuilder(name, seed)
	b.M.NoShadow = glow
	stem := mesh.Style{Color: [3]float32{0.88, 0.84, 0.74}, Mat: mesh.MatStandard}
	cap := mesh.Style{Color: capCol, Mat: capMat, Jitter: 0.04}
	for i := 0; i < 3; i++ {
		x, z := r.Range(-0.18, 0.18), r.Range(-0.18, 0.18)
		h := r.Range(0.12, 0.3)
		cr := r.Range(0.09, 0.17)
		b.Tube([]v3{{X: x, Z: z}, {X: x, Y: h, Z: z}}, []float64{cr * 0.35, cr * 0.28}, 6, stem, false)
		b.Blob(v3{X: x, Y: h, Z: z}, v3{X: cr, Y: cr * 0.55, Z: cr}, 4, 8, 0.05, 2, cap)
	}
	return b.Finish()
}

func crystal(seed uint64) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("crystal", seed)
	b.M.NoShadow = true
	st := mesh.Style{Color: [3]float32{0.4, 0.85, 1}, Mat: mesh.MatCrystal, Jitter: 0.05}
	for i := 0; i < 5; i++ {
		a := r.Range(0, 2*math.Pi)
		rad := r.Range(0, 0.5)
		h := r.Range(0.8, 2.4)
		w := r.Range(0.12, 0.28)
		tx, tz := r.Range(-0.3, 0.3), r.Range(-0.3, 0.3)
		o := v3{X: math.Cos(a) * rad, Z: math.Sin(a) * rad}
		b.Tube([]v3{o, o.Add(v3{X: tx * 0.5, Y: h * 0.78, Z: tz * 0.5}), o.Add(v3{X: tx, Y: h, Z: tz})}, []float64{w, w * 0.85, 0.01}, 6, st, false)
	}
	return b.Finish()
}

func logMesh(seed uint64) *mesh.Mesh {
	b := mesh.NewBuilder("log", seed)
	b.Tube([]v3{{X: -1.8, Y: 0.32}, {X: 0, Y: 0.36}, {X: 1.8, Y: 0.3}}, []float64{0.34, 0.36, 0.32}, 8, barkStyle(1, [3]float32{0.28, 0.2, 0.13}), true)
	b.Blob(v3{X: 0.2, Y: 0.62}, v3{X: 1.2, Y: 0.14, Z: 0.3}, 3, 8, 0.2, 2, mesh.Style{Color: mossCol, Mat: mesh.MatFoliage, Jitter: 0.06})
	return b.Finish()
}

func reed(seed uint64) *mesh.Mesh {
	r := procedural.NewRNG(seed)
	b := mesh.NewBuilder("reed", seed)
	b.M.TwoSided = true
	b.M.NoShadow = true
	st := mesh.Style{Color: [3]float32{0.25, 0.45, 0.16}, Mat: mesh.MatFoliage, SwayK: 0.7, SwayH: 2}
	head := mesh.Style{Color: [3]float32{0.32, 0.18, 0.1}, Mat: mesh.MatStandard, SwayK: 0.7, SwayH: 2}
	for i := 0; i < 7; i++ {
		a := r.Range(0, 2*math.Pi)
		rad := r.Range(0, 0.35)
		h := r.Range(1.1, 1.9)
		o := v3{X: math.Cos(a) * rad, Z: math.Sin(a) * rad}
		l := r.Range(-0.2, 0.2)
		b.Tube([]v3{o, o.Add(v3{X: l * 0.5, Y: h * 0.5}), o.Add(v3{X: l, Y: h})}, []float64{0.018, 0.014, 0.008}, 4, st, false)
		if i%3 == 0 {
			b.Tube([]v3{o.Add(v3{X: l * 0.8, Y: h * 0.8}), o.Add(v3{X: l, Y: h * 0.97})}, []float64{0.04, 0.035}, 5, head, true)
		}
	}
	return b.Finish()
}
