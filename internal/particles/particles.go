// Package particles contains two particle systems: stateless GPU-driven
// ambient particles (rain, snow, pollen, leaves, fireflies, dust, spores),
// whose counts are planned here, and a pooled CPU system for localised
// effects such as waterfall mist, splashes and glowing spores.
package particles

import (
	"math"

	"evergreen/internal/engine"
	"evergreen/internal/procedural"
)

// Plan is how many GPU particles of each kind to draw this frame.
type Plan struct {
	Rain, Snow, Pollen, Dust, Leaves, Fireflies, Spores int
}

// PlanInput describes the environment for planning.
type PlanInput struct {
	Rain          float64 // 0..1
	Wind          float64
	Day           float64 // 0..1
	Season        int     // 0 spring .. 3 winter
	LeafFall      float64
	Fog           float64
	StarsBoost    float64 // 0..1 from GitHub stars
	Glow          float64 // 0..1 how bioluminescent the surroundings are (cave, grove)
	ReducedMotion bool
}

// PlanFor chooses particle counts, scaled by the graphics quality.
func PlanFor(q engine.Quality, in PlanInput) Plan {
	s := q.ParticleScale
	night := 1 - in.Day
	var p Plan
	winter := in.Season == 3
	precip := in.Rain
	if winter {
		p.Snow = int(5200 * precip * s)
	} else {
		p.Rain = int(9000 * precip * s)
	}
	warm := 1.0
	if winter {
		warm = 0.15
	} else if in.Season == 2 {
		warm = 0.6
	}
	p.Pollen = int(1800 * in.Day * (1 - math.Min(1, in.Rain*2)) * warm * s)
	p.Dust = int(900 * in.Day * (1 - in.Fog) * s)
	p.Leaves = int(520 * (0.25 + in.LeafFall) * (0.4 + in.Wind) * s)
	p.Fireflies = int((500*night*warm + 900*in.StarsBoost*night) * (1 - math.Min(1, in.Rain*2)) * s)
	p.Spores = int(1200 * in.Glow * s)
	if in.ReducedMotion {
		p.Rain, p.Snow = int(float64(p.Rain)*0.5), int(float64(p.Snow)*0.5)
		p.Leaves = 0
		p.Pollen /= 3
		p.Dust /= 3
	}
	return p
}

// Total is the sum of all GPU particles.
func (p Plan) Total() int {
	return p.Rain + p.Snow + p.Pollen + p.Dust + p.Leaves + p.Fireflies + p.Spores
}

// ---- CPU particles --------------------------------------------------------------

// Kind of localised particle.
type Kind uint8

// Kinds.
const (
	Mist Kind = iota
	Splash
	Glow
	Steam
	Pollen
)

// KindFromName maps zone emitter names to kinds.
func KindFromName(n string) (Kind, bool) {
	switch n {
	case "mist":
		return Mist, true
	case "splash":
		return Splash, true
	case "glow", "spark":
		return Glow, true
	case "steam":
		return Steam, true
	case "pollen":
		return Pollen, true
	}
	return 0, false
}

// P is a single particle.
type P struct {
	X, Y, Z    float32
	VX, VY, VZ float32
	Age, Life  float32
	Size       float32
	Kind       Kind
	Hue        float32
}

// Source is an active emitter in world space.
type Source struct {
	Kind    Kind
	X, Y, Z float64
	Spread  float64
	Rate    float64
}

// System is a fixed-capacity particle pool.
type System struct {
	p     []P
	n     int
	rng   *procedural.RNG
	carry map[int]float64
}

// New creates a system able to hold cap particles.
func New(capacity int, seed uint64) *System {
	return &System{p: make([]P, capacity), rng: procedural.NewRNG(seed), carry: map[int]float64{}}
}

// Count is the number of live particles.
func (s *System) Count() int { return s.n }

// Capacity is the pool size.
func (s *System) Capacity() int { return len(s.p) }

// Emit spawns one particle of the given kind near (x,y,z).
func (s *System) Emit(k Kind, x, y, z, spread float64) {
	if s.n >= len(s.p) {
		return
	}
	r := s.rng
	a := r.Range(0, 2*math.Pi)
	d := math.Sqrt(r.Float()) * spread
	pt := P{X: float32(x + math.Cos(a)*d), Y: float32(y + r.Range(-0.3, 0.6)), Z: float32(z + math.Sin(a)*d), Kind: k, Hue: float32(r.Float())}
	switch k {
	case Mist:
		pt.VY, pt.Life, pt.Size = float32(r.Range(0.3, 1.1)), float32(r.Range(3.5, 7)), float32(r.Range(2.5, 5))
		pt.VX, pt.VZ = float32(r.Range(-0.4, 0.4)), float32(r.Range(-0.4, 0.4))
	case Splash:
		pt.VY, pt.Life, pt.Size = float32(r.Range(2, 6)), float32(r.Range(0.6, 1.3)), float32(r.Range(0.12, 0.3))
		pt.VX, pt.VZ = float32(r.Range(-1.5, 1.5)), float32(r.Range(-1.5, 1.5))
	case Glow:
		pt.VY, pt.Life, pt.Size = float32(r.Range(-0.1, 0.35)), float32(r.Range(5, 11)), float32(r.Range(0.12, 0.28))
		pt.VX, pt.VZ = float32(r.Range(-0.25, 0.25)), float32(r.Range(-0.25, 0.25))
	case Steam:
		pt.VY, pt.Life, pt.Size = float32(r.Range(0.6, 1.4)), float32(r.Range(1.5, 3)), float32(r.Range(0.5, 1.2))
	case Pollen:
		pt.VY, pt.Life, pt.Size = float32(r.Range(-0.05, 0.2)), float32(r.Range(8, 14)), float32(r.Range(0.05, 0.1))
		pt.VX, pt.VZ = float32(r.Range(-0.3, 0.3)), float32(r.Range(-0.3, 0.3))
	}
	s.p[s.n] = pt
	s.n++
}

// Update emits from sources (rate scaled by quality) and advances all particles.
func (s *System) Update(dt float64, windX, windZ float64, sources []Source, scale float64) {
	for i, src := range sources {
		acc := s.carry[i] + src.Rate*scale*dt
		n := int(acc)
		s.carry[i] = acc - float64(n)
		for k := 0; k < n; k++ {
			s.Emit(src.Kind, src.X, src.Y, src.Z, src.Spread)
		}
	}
	for k := range s.carry {
		if k >= len(sources) {
			delete(s.carry, k)
		}
	}
	d := float32(dt)
	wx, wz := float32(windX), float32(windZ)
	for i := 0; i < s.n; {
		p := &s.p[i]
		p.Age += d
		if p.Age >= p.Life {
			s.n--
			s.p[i] = s.p[s.n]
			continue
		}
		switch p.Kind {
		case Splash:
			p.VY -= 9.8 * d
		case Mist, Steam:
			p.VX += (wx*0.5 - p.VX) * d * 0.5
			p.VZ += (wz*0.5 - p.VZ) * d * 0.5
		case Glow:
			p.VX += float32(math.Sin(float64(p.Age)*1.3+float64(p.Hue)*9)) * d * 0.3
			p.VZ += float32(math.Cos(float64(p.Age)*1.1+float64(p.Hue)*7)) * d * 0.3
		case Pollen:
			p.VX += (wx - p.VX) * d * 0.4
			p.VZ += (wz - p.VZ) * d * 0.4
		}
		p.X += p.VX * d
		p.Y += p.VY * d
		p.Z += p.VZ * d
		i++
	}
}

// Fill writes the live particles as 8 floats each: x,y,z,size, r,g,b,a.
// It returns the number of particles written (at most len(dst)/8).
func (s *System) Fill(dst []float32) int {
	n := min(s.n, len(dst)/8)
	for i := 0; i < n; i++ {
		p := &s.p[i]
		t := p.Age / p.Life
		fade := float32(math.Min(1, float64(t)*6)) * (1 - t)
		var r, g, b, a float32
		size := p.Size
		switch p.Kind {
		case Mist:
			r, g, b, a = 0.85, 0.92, 0.95, 0.22*fade
			size *= 0.6 + t
		case Splash:
			r, g, b, a = 0.9, 0.97, 1, 0.8*fade
		case Glow:
			// green/cyan/violet palette keyed by hue
			switch {
			case p.Hue < 0.45:
				r, g, b = 0.3, 1, 0.75
			case p.Hue < 0.8:
				r, g, b = 0.4, 0.75, 1
			default:
				r, g, b = 0.8, 0.5, 1
			}
			a = fade * (0.6 + 0.4*float32(math.Sin(float64(p.Age)*3+float64(p.Hue)*20)))
		case Steam:
			r, g, b, a = 0.9, 0.9, 0.92, 0.18*fade
			size *= 0.7 + t*1.2
		case Pollen:
			r, g, b, a = 1, 0.95, 0.6, 0.7*fade
		}
		o := dst[i*8 : i*8+8]
		o[0], o[1], o[2], o[3], o[4], o[5], o[6], o[7] = p.X, p.Y, p.Z, size, r, g, b, a
	}
	return n
}

// Clear removes all particles.
func (s *System) Clear() { s.n = 0 }
