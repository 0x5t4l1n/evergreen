// Package wildlife defines the animal species of Evergreen: their procedural
// meshes, behaviours (flocking, grazing, hopping, schooling) and reactions to
// weather, time of day and the visitor.
//
// Models face +X, wings/legs spread along ±Z, units are metres.
package wildlife

import (
	"math"
	"sync"

	"evergreen/internal/mesh"
)

type v3 = mesh.Vec3

var (
	meshMu    sync.Mutex
	meshCache = map[Species]*mesh.Mesh{}
)

// Mesh returns the (cached) mesh for a species.
func Mesh(s Species) *mesh.Mesh {
	meshMu.Lock()
	defer meshMu.Unlock()
	if m, ok := meshCache[s]; ok {
		return m
	}
	var m *mesh.Mesh
	switch s {
	case Bird:
		m = birdMesh()
	case Butterfly:
		m = butterflyMesh()
	case Dragonfly:
		m = dragonflyMesh()
	case Fish:
		m = fishMesh()
	case Deer:
		m = deerMesh()
	case Rabbit:
		m = rabbitMesh()
	case Frog:
		m = frogMesh()
	case Squirrel:
		m = squirrelMesh()
	case Beetle:
		m = beetleMesh()
	}
	m.TwoSided = true
	meshCache[s] = m
	return m
}

func st(c [3]float32) mesh.Style { return mesh.Style{Color: c, Mat: mesh.MatStandard, Jitter: 0.03} }

// wing adds a flapping wing triangle fan on side sgn (+1/-1).
func wing(b *mesh.Builder, root v3, span, chord float64, sgn float64, col [3]float32, mat float32) {
	n := v3{Y: 1}
	r0 := b.Vert(root.Add(v3{X: chord * 0.5}), n, col, 0, 0, 0, mat)
	r1 := b.Vert(root.Add(v3{X: -chord * 0.5}), n, col, 0, 1, 0, mat)
	m0 := b.Vert(root.Add(v3{X: chord * 0.35, Z: sgn * span * 0.55}), n, col, -0.55, 0, 0.5, mat)
	m1 := b.Vert(root.Add(v3{X: -chord * 0.5, Z: sgn * span * 0.55}), n, col, -0.55, 1, 0.5, mat)
	t := b.Vert(root.Add(v3{X: -chord * 0.1, Z: sgn * span}), n, col, -1, 0.5, 1, mat)
	if sgn > 0 {
		b.Quad(r0, m0, m1, r1)
		b.Tri(m0, t, m1)
	} else {
		b.Quad(r0, r1, m1, m0)
		b.Tri(m0, m1, t)
	}
}

func birdMesh() *mesh.Mesh {
	b := mesh.NewBuilder("bird", 11)
	body := st([3]float32{0.16, 0.15, 0.17})
	b.Blob(v3{}, v3{X: 0.2, Y: 0.07, Z: 0.07}, 4, 7, 0, 1, body)
	b.Blob(v3{X: 0.2, Y: 0.03}, v3{X: 0.06, Y: 0.05, Z: 0.05}, 4, 6, 0, 1, body)
	b.Cone(v3{X: 0.26, Y: 0.03}, 0.07, 0.02, 4, st([3]float32{0.9, 0.7, 0.2}))
	wc := [3]float32{0.2, 0.19, 0.21}
	wing(b, v3{X: 0.02, Y: 0.05, Z: 0.04}, 0.42, 0.18, 1, wc, mesh.MatStandard)
	wing(b, v3{X: 0.02, Y: 0.05, Z: -0.04}, 0.42, 0.18, -1, wc, mesh.MatStandard)
	b.TriP(v3{X: -0.18, Y: 0.02}, v3{X: -0.36, Y: 0.0, Z: 0.07}, v3{X: -0.36, Y: 0.0, Z: -0.07}, body)
	return b.Finish()
}

func butterflyMesh() *mesh.Mesh {
	b := mesh.NewBuilder("butterfly", 12)
	b.Tube([]v3{{X: -0.05}, {X: 0.05}}, []float64{0.012, 0.01}, 4, st([3]float32{0.1, 0.08, 0.06}), true)
	wing(b, v3{Z: 0.01}, 0.14, 0.12, 1, [3]float32{1, 1, 1}, mesh.MatFlower)
	wing(b, v3{Z: -0.01}, 0.14, 0.12, -1, [3]float32{1, 1, 1}, mesh.MatFlower)
	return b.Finish()
}

func dragonflyMesh() *mesh.Mesh {
	b := mesh.NewBuilder("dragonfly", 13)
	body := mesh.Style{Color: [3]float32{0.1, 0.5, 0.75}, Mat: mesh.MatMetal}
	b.Tube([]v3{{X: -0.22}, {X: -0.05}, {X: 0.06}, {X: 0.1}}, []float64{0.004, 0.008, 0.012, 0.014}, 5, body, true)
	for _, x := range []float64{0.04, 0.0} {
		wing(b, v3{X: x, Y: 0.012, Z: 0.01}, 0.16, 0.05, 1, [3]float32{0.8, 0.9, 1}, mesh.MatStandard)
		wing(b, v3{X: x, Y: 0.012, Z: -0.01}, 0.16, 0.05, -1, [3]float32{0.8, 0.9, 1}, mesh.MatStandard)
	}
	return b.Finish()
}

func fishMesh() *mesh.Mesh {
	b := mesh.NewBuilder("fish", 14)
	body := mesh.Style{Color: [3]float32{0.75, 0.55, 0.25}, Mat: mesh.MatStandard, Jitter: 0.04}
	// body as a tapered tube whose rear vertices wag (sway 3+w)
	var path []v3
	var rad []float64
	var sways []float32
	for i := 0; i <= 5; i++ {
		t := float64(i) / 5
		path = append(path, v3{X: 0.18 - 0.42*t})
		rad = append(rad, 0.07*math.Sin(math.Pi*(0.15+0.8*(1-t)))+0.012)
		sways = append(sways, float32(3+math.Max(0, 0.2+0.8*t)))
	}
	start := uint32(len(b.M.V) / mesh.Stride)
	b.Tube(path, rad, 6, body, false)
	for i := range path {
		for s := 0; s <= 6; s++ {
			b.M.V[(int(start)+i*7+s)*mesh.Stride+9] = sways[i]
		}
	}
	fin := st([3]float32{0.85, 0.4, 0.15})
	t := func(p v3, w float32) uint32 { return b.Vert(p, v3{Z: 1}, fin.Color, w, 0, 0, mesh.MatStandard) }
	a, c, d := t(v3{X: -0.22}, 3.8), t(v3{X: -0.36, Y: 0.1}, 4), t(v3{X: -0.36, Y: -0.1}, 4)
	b.Tri(a, c, d)
	b.Tri(a, d, c)
	return b.Finish()
}

func leg(b *mesh.Builder, x, z, h, r float64, col [3]float32) {
	st := mesh.Style{Color: col, Mat: mesh.MatStandard}
	path := []v3{{X: x, Y: h, Z: z}, {X: x, Y: h * 0.5, Z: z}, {X: x, Y: 0, Z: z}}
	start := uint32(len(b.M.V) / mesh.Stride)
	b.Tube(path, []float64{r, r * 0.7, r * 0.6}, 5, st, false)
	// legs swing from the hip: weight grows toward the foot
	for i, w := range []float32{2.0, 2.5, 3.0 - 1e-3} {
		for s := 0; s <= 5; s++ {
			b.M.V[(int(start)+i*6+s)*mesh.Stride+9] = w
		}
	}
}

func deerMesh() *mesh.Mesh {
	b := mesh.NewBuilder("deer", 15)
	fur := st([3]float32{0.46, 0.3, 0.17})
	belly := st([3]float32{0.78, 0.68, 0.52})
	b.Blob(v3{Y: 0.95}, v3{X: 0.62, Y: 0.3, Z: 0.24}, 6, 9, 0.03, 1, fur)
	b.Blob(v3{X: 0.05, Y: 0.8}, v3{X: 0.5, Y: 0.16, Z: 0.22}, 4, 8, 0, 1, belly)
	b.Tube([]v3{{X: 0.5, Y: 1.05}, {X: 0.7, Y: 1.4}, {X: 0.82, Y: 1.62}}, []float64{0.13, 0.1, 0.085}, 6, fur, false)
	b.Blob(v3{X: 0.95, Y: 1.66}, v3{X: 0.18, Y: 0.1, Z: 0.09}, 4, 7, 0, 1, fur)
	b.Blob(v3{X: 1.12, Y: 1.62}, v3{X: 0.04, Y: 0.04, Z: 0.04}, 3, 5, 0, 1, st([3]float32{0.05, 0.04, 0.04}))
	for _, s := range []float64{-1, 1} {
		b.TriP(v3{X: 0.85, Y: 1.72, Z: s * 0.05}, v3{X: 0.78, Y: 1.95, Z: s * 0.15}, v3{X: 0.9, Y: 1.78, Z: s * 0.1}, fur)
		b.Tube([]v3{{X: 0.85, Y: 1.72, Z: s * 0.06}, {X: 0.82, Y: 2.0, Z: s * 0.16}, {X: 0.8, Y: 2.2, Z: s * 0.1}}, []float64{0.02, 0.016, 0.008}, 4, st([3]float32{0.7, 0.62, 0.5}), false)
	}
	b.Blob(v3{X: -0.62, Y: 1.0}, v3{X: 0.08, Y: 0.1, Z: 0.07}, 3, 5, 0, 1, belly)
	fc := [3]float32{0.35, 0.22, 0.13}
	leg(b, 0.38, 0.13, 0.78, 0.055, fc)
	leg(b, 0.38, -0.13, 0.78, 0.055, fc)
	leg(b, -0.4, 0.13, 0.78, 0.06, fc)
	leg(b, -0.4, -0.13, 0.78, 0.06, fc)
	return b.Finish()
}

func rabbitMesh() *mesh.Mesh {
	b := mesh.NewBuilder("rabbit", 16)
	fur := st([3]float32{0.55, 0.45, 0.35})
	b.Blob(v3{Y: 0.16}, v3{X: 0.19, Y: 0.14, Z: 0.12}, 5, 8, 0.03, 1, fur)
	b.Blob(v3{X: 0.2, Y: 0.22}, v3{X: 0.09, Y: 0.08, Z: 0.07}, 4, 7, 0, 1, fur)
	for _, s := range []float64{-1, 1} {
		b.Blob(v3{X: 0.18, Y: 0.36, Z: s * 0.035}, v3{X: 0.025, Y: 0.1, Z: 0.02}, 4, 6, 0, 1, fur)
	}
	b.Blob(v3{X: -0.19, Y: 0.18}, v3{X: 0.05, Y: 0.05, Z: 0.05}, 3, 6, 0, 1, st([3]float32{0.95, 0.95, 0.92}))
	b.Blob(v3{X: 0.27, Y: 0.23, Z: 0}, v3{X: 0.015, Y: 0.015, Z: 0.015}, 3, 5, 0, 1, st([3]float32{0.04, 0.03, 0.03}))
	return b.Finish()
}

func frogMesh() *mesh.Mesh {
	b := mesh.NewBuilder("frog", 17)
	skin := st([3]float32{0.2, 0.5, 0.17})
	b.Blob(v3{Y: 0.04}, v3{X: 0.06, Y: 0.04, Z: 0.045}, 4, 7, 0.05, 1, skin)
	for _, s := range []float64{-1, 1} {
		b.Blob(v3{X: 0.045, Y: 0.08, Z: s * 0.03}, v3{X: 0.014, Y: 0.014, Z: 0.014}, 3, 5, 0, 1, st([3]float32{0.9, 0.8, 0.2}))
		b.Blob(v3{X: -0.03, Y: 0.025, Z: s * 0.05}, v3{X: 0.04, Y: 0.015, Z: 0.012}, 3, 5, 0, 1, skin)
	}
	return b.Finish()
}

func squirrelMesh() *mesh.Mesh {
	b := mesh.NewBuilder("squirrel", 18)
	fur := st([3]float32{0.5, 0.3, 0.16})
	b.Blob(v3{Y: 0.15}, v3{X: 0.13, Y: 0.1, Z: 0.07}, 4, 7, 0.02, 1, fur)
	b.Blob(v3{X: 0.14, Y: 0.22}, v3{X: 0.06, Y: 0.055, Z: 0.05}, 4, 6, 0, 1, fur)
	b.Tube([]v3{{X: -0.1, Y: 0.15}, {X: -0.22, Y: 0.28}, {X: -0.22, Y: 0.45}, {X: -0.1, Y: 0.52}}, []float64{0.03, 0.06, 0.06, 0.04}, 6, st([3]float32{0.58, 0.38, 0.2}), true)
	return b.Finish()
}

func beetleMesh() *mesh.Mesh {
	b := mesh.NewBuilder("beetle", 19)
	b.Blob(v3{Y: 0.5}, v3{X: 1.2, Y: 0.6, Z: 0.9}, 4, 7, 0.02, 1, mesh.Style{Color: [3]float32{0.1, 0.25, 0.15}, Mat: mesh.MatMetal})
	return b.Finish()
}
