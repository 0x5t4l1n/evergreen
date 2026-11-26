// Package ecosystem owns the living population: it spawns, updates and removes
// animals around the visitor according to biome, weather, season and time of
// day, and exposes them as GPU instance buffers.
package ecosystem

import (
	"math"

	"evergreen/internal/engine"
	"evergreen/internal/procedural"
	"evergreen/internal/vegetation"
	"evergreen/internal/wildlife"
)

// Env extends the wildlife environment with biome knowledge.
type Env interface {
	wildlife.Env
	Biome(x, z float64) vegetation.Biome
}

// Settings drive population targets.
type Settings struct {
	Cap    int // maximum simultaneous animals (from graphics preset)
	Season int // 0 spring .. 3 winter
	Cond   wildlife.Conditions
	Day    float64 // 0..1
}

// Manager spawns and updates wildlife.
type Manager struct {
	ents    []*wildlife.Entity
	pool    *engine.Pool[wildlife.Entity]
	flocks  []*wildlife.Flock
	rng     *procedural.RNG
	counter uint64
	seed    uint64
	t       float64
	Events  []wildlife.Event
	scratch [wildlife.NumSpecies][]float32
}

// New creates an empty ecosystem.
func New(seed uint64) *Manager {
	return &Manager{
		seed: seed,
		rng:  procedural.NewRNG(seed ^ 0xFA17),
		pool: engine.NewPool(func() *wildlife.Entity { return new(wildlife.Entity) }, func(e *wildlife.Entity) { *e = wildlife.Entity{} }),
	}
}

// Count returns the number of live animals of species s.
func (m *Manager) Count(s wildlife.Species) int {
	n := 0
	for _, e := range m.ents {
		if e.Species == s && e.Alive && !e.Leaving {
			n++
		}
	}
	return n
}

// Total is the number of live animals.
func (m *Manager) Total() int { return len(m.ents) }

// Live reports entities checked out from the pool (for leak tests).
func (m *Manager) Live() int { return m.pool.Live() }

func (m *Manager) targets(s Settings, env Env) [wildlife.NumSpecies]int {
	var t [wildlife.NumSpecies]int
	scale := float64(s.Cap) / 90
	day := s.Day
	dry := 1 - procedural.Clamp(s.Cond.Rain*2.5, 0, 1)
	warm := 1 - s.Cond.Cold
	biome := env.Biome(s.Cond.Player.X, s.Cond.Player.Z)
	lush := 1.0
	switch biome {
	case vegetation.Alpine:
		lush = 0.15
	case vegetation.Highland:
		lush = 0.5
	}
	_, _, nearWater := m.nearWater(env, s.Cond.Player)
	n := func(v float64) int { return int(math.Round(v * scale * lush)) }
	t[wildlife.Bird] = n(14 * day * (1 - procedural.Clamp(s.Cond.Rain*1.6, 0, 1)))
	t[wildlife.Butterfly] = n(16 * day * dry * warm * (map[bool]float64{true: 1.0, false: 0.6}[biome == vegetation.Meadow]))
	t[wildlife.Deer] = n(6 * (0.5 + 0.5*day))
	t[wildlife.Rabbit] = n(7 * (0.4 + 0.6*day) * dry)
	t[wildlife.Squirrel] = n(5 * day * dry)
	t[wildlife.Beetle] = n(14 * warm)
	if nearWater {
		t[wildlife.Fish] = n(14)
		t[wildlife.Dragonfly] = n(8 * day * dry * warm)
		t[wildlife.Frog] = n(6 * (0.4 + 0.6*(1-day+s.Cond.Rain)) * warm)
	}
	return t
}

// nearWater finds water close to p by probing a ring; returns a water point.
func (m *Manager) nearWater(env Env, p engine.Vec3) (x, z float64, ok bool) {
	for r := 6.0; r <= 36; r += 10 {
		for k := 0; k < 8; k++ {
			a := float64(k) * math.Pi / 4
			wx, wz := p.X+math.Cos(a)*r, p.Z+math.Sin(a)*r
			if _, d, w := env.Water(wx, wz); w && d > 0.7 {
				return wx, wz, true
			}
		}
	}
	if _, d, w := env.Water(p.X, p.Z); w && d > 0.7 {
		return p.X, p.Z, true
	}
	return 0, 0, false
}

func (m *Manager) spawn(s wildlife.Species, pos engine.Vec3) *wildlife.Entity {
	e := m.pool.Get()
	m.counter++
	e.Init(s, pos, m.seed^m.counter*0x9E3779B97F4A7C15)
	m.ents = append(m.ents, e)
	return e
}

func (m *Manager) ringPoint(p engine.Vec3, rmin, rmax float64) (float64, float64) {
	a := m.rng.Range(0, 2*math.Pi)
	r := m.rng.Range(rmin, rmax)
	return p.X + math.Cos(a)*r, p.Z + math.Sin(a)*r
}

func (m *Manager) trySpawn(sp wildlife.Species, env Env, p engine.Vec3) {
	switch sp {
	case wildlife.Bird:
		// birds arrive as a flock of 3..8 in formation
		x, z := m.ringPoint(p, 90, 150)
		n := 3 + m.rng.Intn(6)
		f := &wildlife.Flock{Center: engine.Vec3{X: x, Y: 40, Z: z}, Heading: math.Atan2(p.Z-z, p.X-x) + m.rng.Range(-0.8, 0.8), Speed: m.rng.Range(7, 11), Height: m.rng.Range(28, 55), Members: n}
		m.flocks = append(m.flocks, f)
		for i := 0; i < n; i++ {
			b, sd := formationSlotOffset(i)
			e := m.spawn(wildlife.Bird, engine.Vec3{X: x + b, Y: f.Height, Z: z + sd})
			e.Flock, e.Slot = f, i
		}
	case wildlife.Fish, wildlife.Dragonfly, wildlife.Frog:
		wx, wz, ok := m.nearWater(env, p)
		if !ok {
			return
		}
		if sp == wildlife.Frog {
			// frogs sit on the shoreline: dry ground with water within a couple of metres
			for k := 0; k < 16; k++ {
				x, z := wx+m.rng.Range(-14, 14), wz+m.rng.Range(-14, 14)
				if _, d, w := env.Water(x, z); (!w || d < 0.3) && hasWaterNear(env, x, z) {
					m.spawn(sp, engine.Vec3{X: x, Y: env.Ground(x, z), Z: z})
					return
				}
			}
			return
		}
		x, z := wx+m.rng.Range(-6, 6), wz+m.rng.Range(-6, 6)
		surf, depth, w := env.Water(x, z)
		switch sp {
		case wildlife.Fish:
			if w && depth > 0.9 {
				m.spawn(sp, engine.Vec3{X: x, Y: surf - depth*0.4, Z: z})
			}
		case wildlife.Dragonfly:
			if w {
				m.spawn(sp, engine.Vec3{X: x, Y: surf + 1, Z: z})
			}
		}
	default:
		x, z := m.ringPoint(p, 18, 70)
		if sp == wildlife.Beetle {
			x, z = m.ringPoint(p, 2, 12)
		}
		if _, d, w := env.Water(x, z); w && d > 0.3 {
			return
		}
		n := 1
		if sp == wildlife.Deer {
			n = 1 + m.rng.Intn(3)
		}
		for i := 0; i < n; i++ {
			ox, oz := x+m.rng.Range(-3, 3), z+m.rng.Range(-3, 3)
			if _, d, w := env.Water(ox, oz); w && d > 0.3 {
				continue
			}
			m.spawn(sp, engine.Vec3{X: ox, Y: env.Ground(ox, oz), Z: oz})
		}
	}
}

func formationSlotOffset(i int) (float64, float64) {
	rank := float64((i + 1) / 2)
	s := 1.0
	if i%2 == 0 {
		s = -1
	}
	if i == 0 {
		return 0, 0
	}
	return -rank * 2.4, s * rank * 2.1
}

func hasWaterNear(env Env, x, z float64) bool {
	for k := 0; k < 8; k++ {
		a := float64(k) * math.Pi / 4
		if _, d, w := env.Water(x+math.Cos(a)*2.5, z+math.Sin(a)*2.5); w && d > 0.4 {
			return true
		}
	}
	return false
}

// Update advances the whole ecosystem by dt seconds.
func (m *Manager) Update(dt float64, env Env, s Settings) {
	m.t += dt
	m.Events = m.Events[:0]
	p := s.Cond.Player
	for _, f := range m.flocks {
		f.Update(dt, p, m.t, s.Cond.Rain)
	}
	despawnR := 230.0
	for _, e := range m.ents {
		e.Update(dt, env, s.Cond, &m.Events)
		d := math.Hypot(e.Pos.X-p.X, e.Pos.Z-p.Z)
		lim := despawnR
		if e.Species != wildlife.Bird {
			lim = 110
		}
		if d > lim && !e.Leaving {
			e.Leaving = true
		}
	}
	// compact dead entities back into the pool
	live := m.ents[:0]
	for _, e := range m.ents {
		if e.Alive {
			live = append(live, e)
		} else {
			m.pool.Put(e)
		}
	}
	for i := len(live); i < len(m.ents); i++ {
		m.ents[i] = nil
	}
	m.ents = live
	// drop empty flocks
	fl := m.flocks[:0]
	for _, f := range m.flocks {
		used := false
		for _, e := range m.ents {
			if e.Flock == f {
				used = true
				break
			}
		}
		if used {
			fl = append(fl, f)
		}
	}
	m.flocks = fl

	tg := m.targets(s, env)
	var have [wildlife.NumSpecies]int
	for _, e := range m.ents {
		if !e.Leaving {
			have[e.Species]++
		}
	}
	if len(m.ents) < s.Cap+8 {
		spawned := 0
		for sp := wildlife.Species(0); sp < wildlife.NumSpecies && spawned < 2; sp++ {
			if have[sp] < tg[sp] {
				m.trySpawn(sp, env, p)
				spawned++
			}
		}
	}
	// surplus (weather turned, night fell): retire the farthest animals first
	for sp := wildlife.Species(0); sp < wildlife.NumSpecies; sp++ {
		for have[sp] > tg[sp]+1 || (tg[sp] == 0 && have[sp] > 0) {
			var far *wildlife.Entity
			fd := -1.0
			for _, e := range m.ents {
				if e.Species == sp && !e.Leaving {
					if d := math.Hypot(e.Pos.X-p.X, e.Pos.Z-p.Z); d > fd {
						far, fd = e, d
					}
				}
			}
			if far == nil {
				break
			}
			far.Leaving = true
			have[sp]--
		}
	}
}

// Instances returns the packed instance data (8 floats per animal) for species sp.
func (m *Manager) Instances(sp wildlife.Species) []float32 {
	buf := m.scratch[sp][:0]
	for _, e := range m.ents {
		if e.Species == sp {
			buf = e.Instance(buf)
		}
	}
	m.scratch[sp] = buf
	return buf
}

// Clear removes every animal (used when the seed changes).
func (m *Manager) Clear() {
	for _, e := range m.ents {
		m.pool.Put(e)
	}
	m.ents = m.ents[:0]
	m.flocks = m.flocks[:0]
}
