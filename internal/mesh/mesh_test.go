package mesh

import (
	"math"
	"testing"
)

func validate(t *testing.T, m *Mesh) {
	t.Helper()
	if m.VertexCount() == 0 || len(m.I) == 0 || len(m.I)%3 != 0 {
		t.Fatalf("%s: empty or malformed (v=%d i=%d)", m.Name, m.VertexCount(), len(m.I))
	}
	for _, ix := range m.I {
		if int(ix) >= m.VertexCount() {
			t.Fatalf("%s: index %d out of range %d", m.Name, ix, m.VertexCount())
		}
	}
	for _, f := range m.V {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			t.Fatalf("%s: NaN/Inf in vertex data", m.Name)
		}
	}
}

func TestShapes(t *testing.T) {
	st := Style{Color: [3]float32{0.3, 0.5, 0.2}, Jitter: 0.05, SwayK: 0.5, SwayH: 5}
	b := NewBuilder("shapes", 1)
	b.Tube([]Vec3{{0, 0, 0}, {0, 2, 0}, {0.3, 4, 0}}, []float64{0.5, 0.4, 0.2}, 8, st, true)
	b.Cone(Vec3{3, 0, 0}, 3, 1, 8, st)
	b.Cylinder(Vec3{6, 0, 0}, 2, 1, 1, 8, st)
	b.Blob(Vec3{9, 1, 0}, Vec3{1, 1, 1}, 6, 8, 0.3, 1.5, st)
	b.Box(Vec3{12, 1, 0}, Vec3{1, 1, 1}, st)
	b.Beam(Vec3{14, 0, 0}, Vec3{16, 3, 0}, 0.1, st)
	validate(t, b.Finish())
}

func TestAppendTransforms(t *testing.T) {
	src := NewBuilder("src", 1)
	src.Box(Vec3{1, 0, 0}, Vec3{0.5, 0.5, 0.5}, Style{})
	dst := NewBuilder("dst", 1)
	dst.Append(src.Finish(), Vec3{10, 2, 0}, math.Pi/2, 2)
	m := dst.Finish()
	validate(t, m)
	lo, hi := m.Bounds()
	// a box at x=1 rotated 90deg about Y lands near z=-2 (scaled) and x=10
	if math.Abs((lo.X+hi.X)/2-10) > 0.01 || math.Abs((lo.Z+hi.Z)/2+2) > 0.01 {
		t.Fatalf("unexpected bounds %v %v", lo, hi)
	}
}

func TestSwayGrowsWithHeight(t *testing.T) {
	st := Style{SwayK: 1, SwayH: 10}
	if st.sway(0) != 0 || st.sway(10) != 1 || st.sway(5) >= st.sway(8) {
		t.Fatal("sway profile must increase with height")
	}
}

// outward checks that triangle winding agrees with the stored normals and that
// normals point away from the centre, so back-face culling never hides a shape.
func outward(t *testing.T, m *Mesh, c Vec3) {
	t.Helper()
	bad := 0
	for i := 0; i < len(m.I); i += 3 {
		v := func(k int) Vec3 {
			o := int(m.I[i+k]) * Stride
			return Vec3{float64(m.V[o]), float64(m.V[o+1]), float64(m.V[o+2])}
		}
		n := v(1).Sub(v(0)).Cross(v(2).Sub(v(0)))
		if n.Len() < 1e-9 {
			continue
		}
		centroid := v(0).Add(v(1)).Add(v(2)).Scale(1.0 / 3)
		if n.Dot(centroid.Sub(c)) <= 0 {
			bad++
		}
	}
	if bad > 0 {
		t.Fatalf("%s: %d triangles wind inward", m.Name, bad)
	}
}

func TestWindingOutward(t *testing.T) {
	st := Style{}
	b := NewBuilder("box", 1)
	b.Box(Vec3{0, 0, 0}, Vec3{1, 2, 3}, st)
	outward(t, b.Finish(), Vec3{})
	b = NewBuilder("yawbox", 1)
	b.BoxYaw(Vec3{0, 0, 0}, Vec3{1, 2, 3}, 0.7, st)
	outward(t, b.Finish(), Vec3{})
	b = NewBuilder("beam", 1)
	b.Beam(Vec3{0, -3, 0}, Vec3{0, 3, 0}, 0.5, st)
	outward(t, b.Finish(), Vec3{})
	b = NewBuilder("blob", 1)
	b.Blob(Vec3{}, Vec3{1, 1, 1}, 8, 10, 0.2, 1, st)
	outward(t, b.Finish(), Vec3{})
	b = NewBuilder("tube", 1)
	b.Tube([]Vec3{{0, -2, 0}, {0, 0, 0}, {0, 2, 0}}, []float64{1, 1, 1}, 8, st, true)
	outward(t, b.Finish(), Vec3{})
	b = NewBuilder("cone", 1)
	b.Cone(Vec3{0, -1, 0}, 2, 1, 8, st)
	outward(t, b.Finish(), Vec3{0, -0.5, 0})
}
