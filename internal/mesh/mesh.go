// Package mesh is a small procedural geometry toolkit. Every mesh uses one
// interleaved vertex layout shared with the GLSL pipeline:
//
//	position(3) normal(3) colour(3) sway(1) uv(2) material(1)  = 13 floats
//
// sway encodes per-vertex animation: (0,1] wind sway weight, (-1,0) wing
// flap weight, [2,3) leg swing weight+2, [3,4) swim wag weight+3.
package mesh

import (
	"math"

	"evergreen/internal/engine"
	"evergreen/internal/procedural"
)

// Stride is the number of float32 values per vertex.
const Stride = 13

// Material identifiers understood by the fragment shader.
const (
	MatStandard float32 = 0
	MatFoliage  float32 = 1
	MatEmissive float32 = 2
	MatScreen   float32 = 3 // + variant*0.1
	MatGlass    float32 = 4
	MatFall     float32 = 5
	MatCrystal  float32 = 6
	MatBark     float32 = 7
	MatStone    float32 = 8
	MatTerrain  float32 = 9
	MatWater    float32 = 10
	MatMetal    float32 = 11
	MatGlow     float32 = 12
	MatWood     float32 = 13
	MatFlower   float32 = 14 // petal colour comes from the instance tint palette
	MatGrass    float32 = 15
	MatCave     float32 = 16 // wet cave rock, lit only by local lights
	MatInterior float32 = 17 // indoor surfaces: no sun, warm local lights
)

// Vec3 is shorthand for the engine vector.
type Vec3 = engine.Vec3

// Mesh is CPU-side geometry. The renderer caches GPU buffers keyed by pointer.
type Mesh struct {
	Name        string
	V           []float32
	I           []uint32
	Transparent bool // drawn in the blended pass
	TwoSided    bool // backface culling disabled
	NoShadow    bool // excluded from shadow map
	Min, Max    Vec3
	boundsOK    bool
}

// VertexCount returns the number of vertices.
func (m *Mesh) VertexCount() int { return len(m.V) / Stride }

// Bounds returns the axis-aligned bounding box.
func (m *Mesh) Bounds() (Vec3, Vec3) {
	if !m.boundsOK {
		m.Min = Vec3{math.Inf(1), math.Inf(1), math.Inf(1)}
		m.Max = Vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
		for i := 0; i < len(m.V); i += Stride {
			p := Vec3{float64(m.V[i]), float64(m.V[i+1]), float64(m.V[i+2])}
			m.Min = Vec3{math.Min(m.Min.X, p.X), math.Min(m.Min.Y, p.Y), math.Min(m.Min.Z, p.Z)}
			m.Max = Vec3{math.Max(m.Max.X, p.X), math.Max(m.Max.Y, p.Y), math.Max(m.Max.Z, p.Z)}
		}
		if len(m.V) == 0 {
			m.Min, m.Max = Vec3{}, Vec3{}
		}
		m.boundsOK = true
	}
	return m.Min, m.Max
}

// Radius is the bounding radius around the origin in the XZ plane.
func (m *Mesh) Radius() float64 {
	lo, hi := m.Bounds()
	return math.Max(math.Max(math.Abs(lo.X), math.Abs(hi.X)), math.Max(math.Abs(lo.Z), math.Abs(hi.Z)))
}

// Style describes the surface properties applied to newly added shapes.
type Style struct {
	Color   [3]float32
	Mat     float32
	SwayK   float32 // wind sway at height SwayH
	SwayH   float64
	Jitter  float32 // per-vertex colour noise amplitude
	Variant float32
}

// Builder accumulates geometry into a Mesh.
type Builder struct {
	M    *Mesh
	seed uint64
	n    uint64
}

// NewBuilder starts an empty mesh.
func NewBuilder(name string, seed uint64) *Builder {
	return &Builder{M: &Mesh{Name: name}, seed: seed}
}

// Vert appends a vertex and returns its index.
func (b *Builder) Vert(p, n Vec3, col [3]float32, sway, u, v, mat float32) uint32 {
	idx := uint32(len(b.M.V) / Stride)
	b.M.V = append(b.M.V, float32(p.X), float32(p.Y), float32(p.Z), float32(n.X), float32(n.Y), float32(n.Z),
		col[0], col[1], col[2], sway, u, v, mat)
	b.M.boundsOK = false
	return idx
}

func (b *Builder) jit(st Style) [3]float32 {
	if st.Jitter == 0 {
		return st.Color
	}
	b.n++
	j := float32(procedural.HashFloat2(b.seed, int32(b.n), 7)-0.5) * st.Jitter
	return [3]float32{st.Color[0] + j, st.Color[1] + j*0.8, st.Color[2] + j*0.6}
}

func (st Style) sway(y float64) float32 {
	if st.SwayK == 0 || st.SwayH <= 0 {
		return st.SwayK
	}
	t := math.Min(1, math.Max(0, y/st.SwayH))
	return st.SwayK * float32(t*t)
}

// SV adds a vertex using style st, with sway derived from height.
func (b *Builder) SV(p, n Vec3, st Style, u, v float64) uint32 {
	return b.Vert(p, n, b.jit(st), st.sway(p.Y), float32(u), float32(v), st.Mat+st.Variant)
}

// Tri appends a triangle.
func (b *Builder) Tri(a, c, d uint32) { b.M.I = append(b.M.I, a, c, d) }

// Quad appends two triangles for the quad a,b,c,d (counter-clockwise).
func (b *Builder) Quad(a, c, d, e uint32) { b.M.I = append(b.M.I, a, c, d, a, d, e) }

// QuadP adds a flat-shaded quad from four corners (counter-clockwise seen from the front).
func (b *Builder) QuadP(p0, p1, p2, p3 Vec3, st Style) {
	n := p1.Sub(p0).Cross(p3.Sub(p0)).Norm()
	a := b.SV(p0, n, st, 0, 0)
	c := b.SV(p1, n, st, 1, 0)
	d := b.SV(p2, n, st, 1, 1)
	e := b.SV(p3, n, st, 0, 1)
	b.Quad(a, c, d, e)
}

// TriP adds a flat-shaded triangle.
func (b *Builder) TriP(p0, p1, p2 Vec3, st Style) {
	n := p1.Sub(p0).Cross(p2.Sub(p0)).Norm()
	b.Tri(b.SV(p0, n, st, 0, 0), b.SV(p1, n, st, 1, 0), b.SV(p2, n, st, 0.5, 1))
}

// frame returns two unit vectors perpendicular to dir.
func frame(dir Vec3) (Vec3, Vec3) {
	up := Vec3{0, 1, 0}
	if math.Abs(dir.Y) > 0.95 {
		up = Vec3{1, 0, 0}
	}
	r := dir.Cross(up).Norm()
	return r, r.Cross(dir).Norm()
}

// Tube lofts a ring along path with per-point radii. Ends are capped when cap is true.
func (b *Builder) Tube(path []Vec3, radii []float64, sides int, st Style, cap bool) {
	if len(path) < 2 {
		return
	}
	start := uint32(len(b.M.V) / Stride)
	for i, p := range path {
		var d Vec3
		switch {
		case i == 0:
			d = path[1].Sub(path[0])
		case i == len(path)-1:
			d = path[i].Sub(path[i-1])
		default:
			d = path[i+1].Sub(path[i-1])
		}
		d = d.Norm()
		r, u := frame(d)
		for s := 0; s <= sides; s++ {
			a := 2 * math.Pi * float64(s) / float64(sides)
			n := r.Scale(math.Cos(a)).Add(u.Scale(math.Sin(a)))
			b.SV(p.Add(n.Scale(radii[i])), n, st, float64(s)/float64(sides)*3, float64(i))
		}
	}
	ring := uint32(sides + 1)
	for i := 0; i < len(path)-1; i++ {
		for s := 0; s < sides; s++ {
			a := start + uint32(i)*ring + uint32(s)
			b.Quad(a, a+ring, a+1+ring, a+1)
		}
	}
	if cap {
		for _, e := range []int{0, len(path) - 1} {
			d := path[len(path)-1].Sub(path[0]).Norm()
			if e == 0 {
				d = d.Scale(-1)
			}
			c := b.SV(path[e], d, st, 0.5, 0.5)
			base := start + uint32(e)*ring
			for s := 0; s < sides; s++ {
				if e == 0 {
					b.Tri(c, base+uint32(s), base+uint32(s+1))
				} else {
					b.Tri(c, base+uint32(s+1), base+uint32(s))
				}
			}
		}
	}
}

// Cone adds a cone whose axis is +Y from base.
func (b *Builder) Cone(base Vec3, h, r float64, sides int, st Style) {
	b.SV(base.Add(Vec3{0, h, 0}), Vec3{0, 1, 0}, st, 0.5, 1) // apex, referenced as start-1
	slope := r / h
	start := uint32(len(b.M.V) / Stride)
	for s := 0; s <= sides; s++ {
		a := 2 * math.Pi * float64(s) / float64(sides)
		n := Vec3{math.Cos(a), slope, math.Sin(a)}.Norm()
		b.SV(base.Add(Vec3{math.Cos(a) * r, 0, math.Sin(a) * r}), n, st, float64(s)/float64(sides), 0)
	}
	for s := 0; s < sides; s++ {
		b.Tri(start-1, start+uint32(s+1), start+uint32(s))
	}
}

// Cylinder adds a capped cylinder along +Y.
func (b *Builder) Cylinder(base Vec3, h, r0, r1 float64, sides int, st Style) {
	b.Tube([]Vec3{base, base.Add(Vec3{0, h, 0})}, []float64{r0, r1}, sides, st, true)
}

// Blob adds a noise-displaced ellipsoid, useful for foliage clumps, rocks and bushes.
func (b *Builder) Blob(c, radii Vec3, rings, segs int, amp, freq float64, st Style) {
	start := uint32(len(b.M.V) / Stride)
	pos := func(ring, seg int) Vec3 {
		phi := math.Pi * float64(ring) / float64(rings)
		th := 2 * math.Pi * float64(seg) / float64(segs)
		d := Vec3{math.Sin(phi) * math.Cos(th), math.Cos(phi), math.Sin(phi) * math.Sin(th)}
		k := 1 + amp*procedural.Value3(b.seed, d.X*freq+c.X, d.Y*freq+c.Y, d.Z*freq+c.Z)
		return Vec3{c.X + d.X*radii.X*k, c.Y + d.Y*radii.Y*k, c.Z + d.Z*radii.Z*k}
	}
	for ring := 0; ring <= rings; ring++ {
		for seg := 0; seg <= segs; seg++ {
			p := pos(ring, seg%segs)
			// normal from neighbouring samples keeps displaced lighting smooth
			pu := pos(ring, (seg+1)%segs)
			pv := pos(min(ring+1, rings), seg%segs)
			if ring == rings {
				pv = pos(ring-1, seg%segs)
			}
			n := pu.Sub(p).Cross(pv.Sub(p)).Norm()
			if ring == rings {
				n = n.Scale(-1)
			}
			if n.Dot(p.Sub(c)) < 0 {
				n = n.Scale(-1)
			}
			b.SV(p, n, st, float64(seg)/float64(segs), float64(ring)/float64(rings))
		}
	}
	w := uint32(segs + 1)
	for ring := 0; ring < rings; ring++ {
		for seg := 0; seg < segs; seg++ {
			a := start + uint32(ring)*w + uint32(seg)
			b.Quad(a, a+1, a+w+1, a+w)
		}
	}
}

// Box adds an axis-aligned flat-shaded box centred at c.
func (b *Builder) Box(c, h Vec3, st Style) { b.BoxYaw(c, h, 0, st) }

// BoxYaw adds a box rotated by yaw radians about Y around its centre.
func (b *Builder) BoxYaw(c, h Vec3, yaw float64, st Style) {
	s, co := math.Sin(yaw), math.Cos(yaw)
	rot := func(x, y, z float64) Vec3 { return Vec3{c.X + x*co + z*s, c.Y + y, c.Z - x*s + z*co} }
	corner := func(sx, sy, sz float64) Vec3 { return rot(sx*h.X, sy*h.Y, sz*h.Z) }
	faces := [][4]Vec3{
		{corner(-1, -1, 1), corner(1, -1, 1), corner(1, 1, 1), corner(-1, 1, 1)},
		{corner(1, -1, -1), corner(-1, -1, -1), corner(-1, 1, -1), corner(1, 1, -1)},
		{corner(1, -1, 1), corner(1, -1, -1), corner(1, 1, -1), corner(1, 1, 1)},
		{corner(-1, -1, -1), corner(-1, -1, 1), corner(-1, 1, 1), corner(-1, 1, -1)},
		{corner(-1, 1, 1), corner(1, 1, 1), corner(1, 1, -1), corner(-1, 1, -1)},
		{corner(-1, -1, -1), corner(1, -1, -1), corner(1, -1, 1), corner(-1, -1, 1)},
	}
	for _, f := range faces {
		b.QuadP(f[0], f[1], f[2], f[3], st)
	}
}

// Beam adds a square-section beam between two points.
func (b *Builder) Beam(a, c Vec3, w float64, st Style) {
	d := c.Sub(a).Norm()
	r, u := frame(d)
	r, u = r.Scale(w), u.Scale(w)
	p := func(o Vec3, s1, s2 float64) Vec3 { return o.Add(r.Scale(s1)).Add(u.Scale(s2)) }
	cs1 := [4]float64{1, -1, -1, 1}
	cs2 := [4]float64{1, 1, -1, -1}
	for i := 0; i < 4; i++ {
		j := (i + 1) % 4
		b.QuadP(p(a, cs1[i], cs2[i]), p(c, cs1[i], cs2[i]), p(c, cs1[j], cs2[j]), p(a, cs1[j], cs2[j]), st)
	}
	b.QuadP(p(a, -1, -1), p(a, 1, -1), p(a, 1, 1), p(a, -1, 1), st)
	b.QuadP(p(c, 1, -1), p(c, -1, -1), p(c, -1, 1), p(c, 1, 1), st)
}

// Append copies src into the builder with translation, yaw and uniform scale.
func (b *Builder) Append(src *Mesh, pos Vec3, yaw, scale float64) {
	base := uint32(len(b.M.V) / Stride)
	s, co := math.Sin(yaw), math.Cos(yaw)
	for i := 0; i < len(src.V); i += Stride {
		v := src.V[i : i+Stride]
		x, y, z := float64(v[0])*scale, float64(v[1])*scale, float64(v[2])*scale
		nx, nz := float64(v[3]), float64(v[5])
		b.M.V = append(b.M.V,
			float32(pos.X+x*co+z*s), float32(pos.Y+y), float32(pos.Z-x*s+z*co),
			float32(nx*co+nz*s), v[4], float32(-nx*s+nz*co),
			v[6], v[7], v[8], v[9], v[10], v[11], v[12])
	}
	for _, ix := range src.I {
		b.M.I = append(b.M.I, base+ix)
	}
	b.M.boundsOK = false
}

// Finish returns the built mesh.
func (b *Builder) Finish() *Mesh { return b.M }
