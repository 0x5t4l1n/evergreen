// Package water implements the interactive water surface simulation: a damped
// wave-equation ripple field that follows the camera, driven by rain, wading
// and splashes, and uploaded to the GPU as a half-float height texture.
package water

import (
	"math"

	"evergreen/internal/procedural"
)

// Ripples is a square wave-equation grid centred (and snapped) around a world point.
type Ripples struct {
	N        int
	Cell     float64 // metres per texel
	OriginX  float64 // world position of texel (0,0)
	OriginZ  float64
	h, v     []float32
	tmp      []float32
	half     []uint16
	acc      float64
	rng      *procedural.RNG
	RainRate float64 // drops per second per full rain
}

// NewRipples creates an n*n grid covering n*cell metres.
func NewRipples(n int, cell float64, seed uint64) *Ripples {
	return &Ripples{N: n, Cell: cell, h: make([]float32, n*n), v: make([]float32, n*n), tmp: make([]float32, n*n),
		half: make([]uint16, n*n), rng: procedural.NewRNG(seed), RainRate: 900}
}

// Span is the world size covered by the grid.
func (r *Ripples) Span() float64 { return float64(r.N) * r.Cell }

// Recenter shifts the grid so it stays centred on (x,z), preserving ripples.
func (r *Ripples) Recenter(x, z float64) {
	nx := math.Floor(x/r.Cell)*r.Cell - r.Span()/2
	nz := math.Floor(z/r.Cell)*r.Cell - r.Span()/2
	sx, sz := int(math.Round((nx-r.OriginX)/r.Cell)), int(math.Round((nz-r.OriginZ)/r.Cell))
	r.OriginX, r.OriginZ = nx, nz
	if sx == 0 && sz == 0 {
		return
	}
	if abs(sx) >= r.N || abs(sz) >= r.N {
		clear(r.h)
		clear(r.v)
		return
	}
	shift := func(a []float32) {
		for j := 0; j < r.N; j++ {
			for i := 0; i < r.N; i++ {
				si, sj := i+sx, j+sz
				if si >= 0 && si < r.N && sj >= 0 && sj < r.N {
					r.tmp[j*r.N+i] = a[sj*r.N+si]
				} else {
					r.tmp[j*r.N+i] = 0
				}
			}
		}
		copy(a, r.tmp)
	}
	shift(r.h)
	shift(r.v)
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// Drop adds a circular disturbance at world (x,z).
func (r *Ripples) Drop(x, z, strength, radius float64) {
	cx, cz := (x-r.OriginX)/r.Cell, (z-r.OriginZ)/r.Cell
	rad := math.Max(1, radius/r.Cell)
	for j := int(cz - rad - 1); j <= int(cz+rad+1); j++ {
		for i := int(cx - rad - 1); i <= int(cx+rad+1); i++ {
			if i < 1 || j < 1 || i >= r.N-1 || j >= r.N-1 {
				continue
			}
			d := math.Hypot(float64(i)-cx, float64(j)-cz) / rad
			if d < 1 {
				r.v[j*r.N+i] -= float32(strength * (0.5 + 0.5*math.Cos(d*math.Pi)))
			}
		}
	}
}

// Step advances the simulation by dt seconds; rain in [0,1] seeds raindrops.
func (r *Ripples) Step(dt, rain float64) {
	if rain > 0.02 {
		n := rain * r.RainRate * dt
		whole := int(n)
		if r.rng.Float() < n-float64(whole) {
			whole++
		}
		for k := 0; k < whole; k++ {
			x := r.OriginX + r.rng.Float()*r.Span()
			z := r.OriginZ + r.rng.Float()*r.Span()
			r.Drop(x, z, 0.5+rain*0.5, r.Cell*1.2)
		}
	}
	r.acc += dt
	const fixed = 1.0 / 60
	for steps := 0; r.acc >= fixed && steps < 4; steps++ {
		r.acc -= fixed
		r.step()
	}
	if r.acc > fixed*4 {
		r.acc = 0
	}
}

func (r *Ripples) step() {
	n := r.N
	const c, damp = 0.45, 0.992
	for j := 1; j < n-1; j++ {
		for i := 1; i < n-1; i++ {
			k := j*n + i
			avg := (r.h[k-1] + r.h[k+1] + r.h[k-n] + r.h[k+n]) * 0.25
			r.v[k] = (r.v[k] + (avg-r.h[k])*c*2) * damp
		}
	}
	for k := range r.h {
		r.h[k] += r.v[k] * 0.9
		if r.h[k] > 1.5 {
			r.h[k] = 1.5
		} else if r.h[k] < -1.5 {
			r.h[k] = -1.5
		}
	}
	// absorbing borders
	for i := 0; i < n; i++ {
		for _, k := range []int{i, (n-1)*n + i, i * n, i*n + n - 1} {
			r.h[k] *= 0.5
			r.v[k] = 0
		}
	}
}

// Height returns the ripple height at world (x,z).
func (r *Ripples) Height(x, z float64) float64 {
	i, j := int((x-r.OriginX)/r.Cell), int((z-r.OriginZ)/r.Cell)
	if i < 0 || j < 0 || i >= r.N || j >= r.N {
		return 0
	}
	return float64(r.h[j*r.N+i])
}

// Energy is the summed absolute height and velocity; used by tests and idle detection.
func (r *Ripples) Energy() float64 {
	var e float64
	for i, v := range r.h {
		e += math.Abs(float64(v)) + math.Abs(float64(r.v[i]))
	}
	return e
}

// Half returns the height field as IEEE half floats for texImage2D(HALF_FLOAT).
func (r *Ripples) Half() []uint16 {
	for i, v := range r.h {
		r.half[i] = Float32ToHalf(v * 0.25)
	}
	return r.half
}

// Float32ToHalf converts to IEEE 754 binary16 (round to nearest, flush denormals).
func Float32ToHalf(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b>>16) & 0x8000
	exp := int((b>>23)&0xff) - 127 + 15
	mant := b & 0x7fffff
	switch {
	case exp <= 0:
		return sign
	case exp >= 31:
		return sign | 0x7c00
	}
	h := sign | uint16(exp)<<10 | uint16(mant>>13)
	if mant&0x1000 != 0 {
		h++
	}
	return h
}

// HalfToFloat32 converts binary16 back to float32 (for tests).
func HalfToFloat32(h uint16) float32 {
	sign := uint32(h&0x8000) << 16
	exp := uint32(h>>10) & 0x1f
	mant := uint32(h & 0x3ff)
	switch exp {
	case 0:
		return math.Float32frombits(sign)
	case 31:
		return math.Float32frombits(sign | 0x7f800000 | mant<<13)
	}
	return math.Float32frombits(sign | (exp-15+127)<<23 | mant<<13)
}
