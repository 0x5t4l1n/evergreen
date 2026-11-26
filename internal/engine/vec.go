// Package engine holds renderer-agnostic engine primitives: vector and matrix
// math, configuration, quality presets and small utilities.
package engine

import "math"

// Vec3 is a 3D vector in world space (metres).
type Vec3 struct{ X, Y, Z float64 }

func (a Vec3) Add(b Vec3) Vec3             { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a Vec3) Sub(b Vec3) Vec3             { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a Vec3) Scale(s float64) Vec3        { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func (a Vec3) Dot(b Vec3) float64          { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a Vec3) Len() float64                { return math.Sqrt(a.Dot(a)) }
func (a Vec3) LenXZ() float64              { return math.Hypot(a.X, a.Z) }
func (a Vec3) Lerp(b Vec3, t float64) Vec3 { return a.Add(b.Sub(a).Scale(t)) }

func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}

// Norm returns the unit vector; the zero vector is returned unchanged.
func (a Vec3) Norm() Vec3 {
	l := a.Len()
	if l < 1e-12 {
		return a
	}
	return a.Scale(1 / l)
}

// Mat4 is a column-major 4x4 float32 matrix, laid out for direct GPU upload.
type Mat4 [16]float32

// Identity returns the identity matrix.
func Identity() Mat4 { return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }

// Mul returns a*b.
func (a Mat4) Mul(b Mat4) Mat4 {
	var r Mat4
	for c := 0; c < 4; c++ {
		for rr := 0; rr < 4; rr++ {
			var s float32
			for k := 0; k < 4; k++ {
				s += a[k*4+rr] * b[c*4+k]
			}
			r[c*4+rr] = s
		}
	}
	return r
}

// Perspective builds an OpenGL-style projection matrix (fovY in radians).
func Perspective(fovY, aspect, near, far float64) Mat4 {
	f := 1 / math.Tan(fovY/2)
	var m Mat4
	m[0] = float32(f / aspect)
	m[5] = float32(f)
	m[10] = float32((far + near) / (near - far))
	m[11] = -1
	m[14] = float32(2 * far * near / (near - far))
	return m
}

// Ortho builds an orthographic projection.
func Ortho(l, r, b, t, n, f float64) Mat4 {
	m := Identity()
	m[0] = float32(2 / (r - l))
	m[5] = float32(2 / (t - b))
	m[10] = float32(-2 / (f - n))
	m[12] = float32(-(r + l) / (r - l))
	m[13] = float32(-(t + b) / (t - b))
	m[14] = float32(-(f + n) / (f - n))
	return m
}

// LookAt builds a right-handed view matrix.
func LookAt(eye, target, up Vec3) Mat4 {
	f := target.Sub(eye).Norm()
	s := f.Cross(up).Norm()
	u := s.Cross(f)
	return Mat4{
		float32(s.X), float32(u.X), float32(-f.X), 0,
		float32(s.Y), float32(u.Y), float32(-f.Y), 0,
		float32(s.Z), float32(u.Z), float32(-f.Z), 0,
		float32(-s.Dot(eye)), float32(-u.Dot(eye)), float32(f.Dot(eye)), 1,
	}
}

// Frustum is six inward-facing planes (a,b,c,d) extracted from a view-projection matrix.
type Frustum [6][4]float64

// NewFrustum extracts frustum planes (Gribb/Hartmann).
func NewFrustum(vp Mat4) Frustum {
	row := func(i int) [4]float64 {
		return [4]float64{float64(vp[i]), float64(vp[4+i]), float64(vp[8+i]), float64(vp[12+i])}
	}
	r0, r1, r2, r3 := row(0), row(1), row(2), row(3)
	var f Frustum
	for i := 0; i < 4; i++ {
		f[0][i] = r3[i] + r0[i]
		f[1][i] = r3[i] - r0[i]
		f[2][i] = r3[i] + r1[i]
		f[3][i] = r3[i] - r1[i]
		f[4][i] = r3[i] + r2[i]
		f[5][i] = r3[i] - r2[i]
	}
	for i := range f {
		l := math.Sqrt(f[i][0]*f[i][0] + f[i][1]*f[i][1] + f[i][2]*f[i][2])
		if l > 0 {
			for j := 0; j < 4; j++ {
				f[i][j] /= l
			}
		}
	}
	return f
}

// IntersectsAABB reports whether the box overlaps the frustum.
func (f Frustum) IntersectsAABB(min, max Vec3) bool {
	for _, p := range f {
		x, y, z := min.X, min.Y, min.Z
		if p[0] > 0 {
			x = max.X
		}
		if p[1] > 0 {
			y = max.Y
		}
		if p[2] > 0 {
			z = max.Z
		}
		if p[0]*x+p[1]*y+p[2]*z+p[3] < 0 {
			return false
		}
	}
	return true
}
