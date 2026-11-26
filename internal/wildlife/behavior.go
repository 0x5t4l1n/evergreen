package wildlife

import (
	"math"

	"evergreen/internal/engine"
	"evergreen/internal/procedural"
)

// Species enumerates every animal.
type Species int

// Species list.
const (
	Bird Species = iota
	Butterfly
	Dragonfly
	Fish
	Deer
	Rabbit
	Frog
	Squirrel
	Beetle
	NumSpecies
)

var speciesNames = [...]string{"bird", "butterfly", "dragonfly", "fish", "deer", "rabbit", "frog", "squirrel", "beetle"}

func (s Species) String() string { return speciesNames[s] }

// State is a behaviour state.
type State int

// Behaviour states.
const (
	Wander State = iota
	Graze
	Rest
	Flee
	Hover
	Dart
	Hop
	Sit
)

// Vec3 is the world vector type.
type Vec3 = engine.Vec3

// Env is what animals perceive about the world.
type Env interface {
	Ground(x, z float64) float64
	Water(x, z float64) (surface, depth float64, ok bool)
}

// Conditions describe the current environment for behaviour decisions.
type Conditions struct {
	Player  Vec3
	Rain    float64
	Night   float64
	Wind    float64
	Thunder bool // a bolt struck this frame
	Cold    float64
}

// Entity is one animal.
type Entity struct {
	Species Species
	Pos     Vec3
	Vel     Vec3
	Yaw     float64
	State   State
	Timer   float64
	Home    Vec3
	Target  Vec3
	Phase   float64
	Flock   *Flock
	Slot    int
	Scale   float64
	Tint    float64
	Anim    float64 // animation frequency (rad/s)
	Gait    float64 // accumulated animation phase (rad), wrapped to [0,2pi)
	Fade    float64 // 0..1 appear/disappear
	Leaving bool
	Alive   bool
	rng     procedural.RNG
}

// Flock coordinates birds flying in formation.
type Flock struct {
	Center  Vec3
	Heading float64
	Speed   float64
	Height  float64
	Members int
}

// Event is a sound-worthy occurrence.
type Event struct {
	Kind string // chirp, croak, splash
	Pos  Vec3
}

func (e *Entity) toward(dx, dz float64) { e.Yaw = turn(e.Yaw, math.Atan2(dz, dx), 0.2) }

func turn(cur, want, rate float64) float64 {
	return cur + math.Remainder(want-cur, 2*math.Pi)*math.Min(1, rate)
}

// Init prepares a freshly spawned entity.
func (e *Entity) Init(s Species, pos Vec3, seed uint64) {
	*e = Entity{Species: s, Pos: pos, Home: pos, Alive: true, Scale: 1, rng: *procedural.NewRNG(seed)}
	e.Yaw = e.rng.Range(0, 2*math.Pi)
	e.Phase = e.rng.Range(0, 6.28)
	e.Tint = e.rng.Float()
	e.Timer = e.rng.Range(0.5, 3)
	switch s {
	case Bird:
		e.Scale = e.rng.Range(0.9, 1.2)
		e.Anim = 11
	case Butterfly:
		e.Scale = e.rng.Range(0.8, 1.3)
		e.Anim = 16
	case Dragonfly:
		e.Scale = e.rng.Range(0.9, 1.2)
		e.Anim = 45
		e.State = Hover
	case Fish:
		e.Scale = e.rng.Range(0.6, 1.5)
		e.Anim = 8
	case Deer:
		e.Scale = e.rng.Range(0.85, 1.1)
		e.State = Graze
	case Rabbit:
		e.State = Sit
		e.Scale = e.rng.Range(0.85, 1.15)
	case Frog:
		e.State = Sit
		e.Scale = e.rng.Range(0.8, 1.4)
	case Squirrel:
		e.State = Sit
		e.Scale = e.rng.Range(0.9, 1.1)
	case Beetle:
		e.Scale = e.rng.Range(0.05, 0.09)
	}
}

func (e *Entity) rand(a, b float64) float64 { return e.rng.Range(a, b) }

func (e *Entity) threat(c Conditions, radius float64) (dx, dz, d float64, ok bool) {
	dx, dz = e.Pos.X-c.Player.X, e.Pos.Z-c.Player.Z
	d = math.Hypot(dx, dz)
	return dx, dz, d, d < radius
}

// Update advances one entity. Events are appended to ev.
func (e *Entity) Update(dt float64, env Env, c Conditions, ev *[]Event) {
	if e.Leaving {
		e.Fade = math.Max(0, e.Fade-dt)
		if e.Fade <= 0 {
			e.Alive = false
		}
	} else if e.Fade < 1 {
		e.Fade = math.Min(1, e.Fade+dt*0.8)
	}
	switch e.Species {
	case Bird:
		e.updateBird(dt, c)
	case Butterfly:
		e.updateButterfly(dt, env, c)
	case Dragonfly:
		e.updateDragonfly(dt, env, c)
	case Fish:
		e.updateFish(dt, env, c)
	case Deer:
		e.updateDeer(dt, env, c)
	case Rabbit:
		e.updateRabbit(dt, env, c)
	case Frog:
		e.updateFrog(dt, env, c, ev)
	case Squirrel:
		e.updateSquirrel(dt, env, c)
	case Beetle:
		e.updateBeetle(dt, env)
	}
	e.Gait = math.Mod(e.Gait+dt*e.Anim, 2*math.Pi)
}

// formationSlot returns a V-formation offset for slot i relative to the heading.
func formationSlot(i int) (back, side float64) {
	if i == 0 {
		return 0, 0
	}
	rank := float64((i + 1) / 2)
	s := 1.0
	if i%2 == 0 {
		s = -1
	}
	return -rank * 2.4, s * rank * 2.1
}

// UpdateFlock moves the shared flock centre.
func (f *Flock) Update(dt float64, player Vec3, t float64, rain float64) {
	// slow turning with a pull back toward the visitor so flocks stay in view
	toP := math.Atan2(player.Z-f.Center.Z, player.X-f.Center.X)
	d := math.Hypot(player.X-f.Center.X, player.Z-f.Center.Z)
	pull := procedural.SmoothStep(120, 260, d)
	wob := 0.25 * math.Sin(t*0.13+f.Height)
	f.Heading = turn(f.Heading+wob*dt, toP, pull*dt*0.8)
	f.Center.X += math.Cos(f.Heading) * f.Speed * dt * (1 - 0.4*rain)
	f.Center.Z += math.Sin(f.Heading) * f.Speed * dt * (1 - 0.4*rain)
	f.Center.Y = f.Height + 3*math.Sin(t*0.21+f.Height)
}

func (e *Entity) updateBird(dt float64, c Conditions) {
	if e.Flock == nil {
		return
	}
	back, side := formationSlot(e.Slot)
	ch, sh := math.Cos(e.Flock.Heading), math.Sin(e.Flock.Heading)
	jx := math.Sin(e.Phase*1.7+e.Timer*0.0) * 0.4
	want := Vec3{
		X: e.Flock.Center.X + ch*back - sh*side + jx,
		Y: e.Flock.Center.Y + 0.35*math.Sin(e.Phase),
		Z: e.Flock.Center.Z + sh*back + ch*side,
	}
	k := math.Min(1, dt*1.8)
	prev := e.Pos
	e.Pos = e.Pos.Lerp(want, k)
	e.Phase += dt * 1.3
	mv := e.Pos.Sub(prev)
	if mv.LenXZ() > 1e-4 {
		e.Yaw = turn(e.Yaw, math.Atan2(mv.Z, mv.X), 0.3)
	}
	e.Anim = 10 + 2*math.Sin(e.Phase*2) + c.Wind*2
	if mv.Y > 0.02 {
		e.Anim += 3
	}
}

func (e *Entity) updateButterfly(dt float64, env Env, c Conditions) {
	e.Phase += dt
	e.Timer -= dt
	if e.Timer <= 0 {
		a := e.rand(0, 2*math.Pi)
		e.Target = Vec3{X: e.Pos.X + math.Cos(a)*e.rand(2, 6), Z: e.Pos.Z + math.Sin(a)*e.rand(2, 6)}
		e.Timer = e.rand(1.5, 4)
	}
	dx, dz := e.Target.X-e.Pos.X, e.Target.Z-e.Pos.Z
	l := math.Hypot(dx, dz)
	speed := 1.4 + c.Wind
	if l > 0.1 {
		e.Vel.X += (dx/l*speed - e.Vel.X) * dt * 2
		e.Vel.Z += (dz/l*speed - e.Vel.Z) * dt * 2
	}
	ground := env.Ground(e.Pos.X, e.Pos.Z)
	wantY := ground + 1.3 + 0.7*math.Sin(e.Phase*0.5+e.Tint*9)
	e.Vel.Y += ((wantY-e.Pos.Y)*1.2 + 0.9*math.Sin(e.Phase*7)) * dt * 2
	e.Vel.Y *= math.Exp(-dt * 1.5)
	e.Pos = e.Pos.Add(e.Vel.Scale(dt))
	e.Yaw = turn(e.Yaw, math.Atan2(e.Vel.Z, e.Vel.X), 0.1)
	e.Anim = 14 + 6*math.Sin(e.Phase*0.9)
}

func (e *Entity) updateDragonfly(dt float64, env Env, c Conditions) {
	e.Phase += dt
	e.Timer -= dt
	if e.Timer <= 0 {
		if e.State == Hover {
			e.State = Dart
			a := e.rand(0, 2*math.Pi)
			r := e.rand(2, 6)
			e.Target = Vec3{X: e.Home.X + math.Cos(a)*r, Z: e.Home.Z + math.Sin(a)*r}
			e.Timer = e.rand(0.4, 0.9)
		} else {
			e.State = Hover
			e.Timer = e.rand(0.6, 2.2)
		}
	}
	if e.State == Dart {
		dx, dz := e.Target.X-e.Pos.X, e.Target.Z-e.Pos.Z
		l := math.Hypot(dx, dz)
		if l > 0.2 {
			e.Vel.X += (dx/l*7 - e.Vel.X) * math.Min(1, dt*8)
			e.Vel.Z += (dz/l*7 - e.Vel.Z) * math.Min(1, dt*8)
			e.Yaw = turn(e.Yaw, math.Atan2(dz, dx), 0.4)
		}
	} else {
		e.Vel.X *= math.Exp(-dt * 6)
		e.Vel.Z *= math.Exp(-dt * 6)
	}
	surface := env.Ground(e.Pos.X, e.Pos.Z)
	if s, _, ok := env.Water(e.Pos.X, e.Pos.Z); ok {
		surface = s
	}
	wantY := surface + 1.0 + 0.15*math.Sin(e.Phase*3)
	e.Pos.Y += (wantY - e.Pos.Y) * math.Min(1, dt*4)
	e.Pos.X += e.Vel.X * dt
	e.Pos.Z += e.Vel.Z * dt
	if math.Hypot(e.Pos.X-e.Home.X, e.Pos.Z-e.Home.Z) > 10 {
		e.Target = e.Home
		e.State = Dart
	}
}

func (e *Entity) updateFish(dt float64, env Env, c Conditions) {
	e.Phase += dt
	speed := 0.7 + 0.5*math.Sin(e.Phase*0.4+e.Tint*7)
	if dx, dz, d, ok := e.threat(c, 4.5); ok && c.Player.Y < 9999 {
		e.Yaw = turn(e.Yaw, math.Atan2(dz, dx), 0.2)
		speed = 2.8 * (1 - d/6)
	}
	e.Timer -= dt
	if e.Timer <= 0 {
		e.Yaw += e.rand(-1.0, 1.0)
		e.Timer = e.rand(1.5, 4)
	}
	nx, nz := e.Pos.X+math.Cos(e.Yaw)*speed*dt, e.Pos.Z+math.Sin(e.Yaw)*speed*dt
	surface, depth, ok := env.Water(nx, nz)
	if !ok || depth < 0.55 {
		e.Yaw += math.Pi*0.6 + e.rand(-0.3, 0.3)
	} else {
		e.Pos.X, e.Pos.Z = nx, nz
		e.Pos.Y += (surface - depth*(0.35+0.2*math.Sin(e.Phase*0.3+e.Tint*5)) - e.Pos.Y) * math.Min(1, dt*2)
	}
	e.Anim = 6 + speed*6
}

func (e *Entity) walkable(env Env, x, z float64) bool {
	if _, depth, ok := env.Water(x, z); ok && depth > 0.5 {
		return false
	}
	return math.Abs(env.Ground(x, z)-env.Ground(e.Pos.X, e.Pos.Z)) < 0.9
}

func (e *Entity) step(dt, speed float64, env Env) bool {
	nx, nz := e.Pos.X+math.Cos(e.Yaw)*speed*dt, e.Pos.Z+math.Sin(e.Yaw)*speed*dt
	if !e.walkable(env, nx, nz) {
		return false
	}
	e.Pos.X, e.Pos.Z = nx, nz
	e.Pos.Y = env.Ground(nx, nz)
	return true
}

func (e *Entity) updateDeer(dt float64, env Env, c Conditions) {
	e.Timer -= dt
	if _, _, d, ok := e.threat(c, 14); ok && e.State != Flee {
		e.State, e.Timer = Flee, 4
		_ = d
	}
	if c.Thunder && e.State != Flee {
		e.State, e.Timer = Flee, 3
	}
	speed := 0.0
	switch e.State {
	case Flee:
		dx, dz, _, _ := e.threat(c, 1e9)
		e.Yaw = turn(e.Yaw, math.Atan2(dz, dx)+0.3*math.Sin(e.Phase), 0.08)
		speed = 8.5
		if e.Timer <= 0 {
			e.State, e.Timer = Wander, e.rand(3, 8)
		}
	case Wander:
		speed = 1.3
		if e.Timer <= 0 {
			e.State, e.Timer = Graze, e.rand(6, 14)
		}
	case Graze, Rest:
		if e.Timer <= 0 {
			e.State = Wander
			e.Timer = e.rand(4, 10)
			e.Yaw += e.rand(-1.5, 1.5)
		}
		if c.Night > 0.7 || c.Rain > 0.6 {
			e.State = Rest
		}
	}
	if speed > 0 && !e.step(dt, speed, env) {
		e.Yaw += 1.2 + e.rand(0, 1)
	}
	e.Pos.Y = env.Ground(e.Pos.X, e.Pos.Z)
	e.Phase += dt * speed
	e.Anim = speed*2.4 + 0.001
	if speed == 0 {
		e.Anim = 0
	}
}

func (e *Entity) updateRabbit(dt float64, env Env, c Conditions) {
	e.Timer -= dt
	_, _, _, scared := e.threat(c, 9)
	switch e.State {
	case Sit:
		if scared || c.Thunder {
			e.State, e.Timer = Hop, 0.3
			dx, dz, _, _ := e.threat(c, 1e9)
			e.Yaw = math.Atan2(dz, dx) + e.rand(-0.6, 0.6)
			e.Phase = 0
		} else if e.Timer <= 0 {
			e.State = Hop
			e.Yaw += e.rand(-2, 2)
			e.Phase = 0
			e.Timer = e.rand(0.4, 0.6)
		}
	case Hop:
		e.Phase += dt / math.Max(0.25, e.Timer+0.0001+0.25)
		hopSpeed := 2.6
		if scared {
			hopSpeed = 6.5
		}
		if !e.step(dt, hopSpeed, env) {
			e.Yaw += 1.5
		}
		if e.Timer <= 0 {
			if scared {
				e.Timer = 0.3
				e.Yaw += e.rand(-0.9, 0.9)
			} else {
				e.State, e.Timer = Sit, e.rand(1.5, 6)
			}
		}
	}
	hop := 0.0
	if e.State == Hop {
		hop = 0.22 * math.Abs(math.Sin(e.Phase*math.Pi*2))
	}
	e.Pos.Y = env.Ground(e.Pos.X, e.Pos.Z) + hop
	e.Anim = 0
}

func (e *Entity) updateFrog(dt float64, env Env, c Conditions, ev *[]Event) {
	e.Timer -= dt
	e.Phase += dt
	if e.State == Hop {
		if !e.step(dt, 1.6, env) {
			e.Yaw += 2
		}
		e.Pos.Y = env.Ground(e.Pos.X, e.Pos.Z) + 0.12*math.Abs(math.Sin(e.Phase*9))
		if e.Timer <= 0 {
			e.State, e.Timer = Sit, e.rand(2, 7)
		}
		return
	}
	e.Pos.Y = env.Ground(e.Pos.X, e.Pos.Z)
	if _, _, _, scared := e.threat(c, 3); scared {
		e.State, e.Timer = Hop, 0.5
		dx, dz, _, _ := e.threat(c, 1e9)
		e.Yaw = math.Atan2(dz, dx)
	} else if e.Timer <= 0 {
		if (c.Night > 0.4 || c.Rain > 0.3) && e.rand(0, 1) < 0.5 && ev != nil {
			*ev = append(*ev, Event{Kind: "croak", Pos: e.Pos})
		}
		if e.rand(0, 1) < 0.35 {
			e.State, e.Timer = Hop, e.rand(0.2, 0.5)
			e.Yaw += e.rand(-1.5, 1.5)
		} else {
			e.Timer = e.rand(1.5, 5)
		}
	}
}

func (e *Entity) updateSquirrel(dt float64, env Env, c Conditions) {
	e.Timer -= dt
	_, _, _, scared := e.threat(c, 10)
	switch e.State {
	case Sit:
		if scared {
			e.State, e.Timer = Flee, 2.5
			dx, dz, _, _ := e.threat(c, 1e9)
			e.Yaw = math.Atan2(dz, dx) + e.rand(-0.5, 0.5)
		} else if e.Timer <= 0 {
			e.State, e.Timer = Dart, e.rand(0.5, 1.4)
			e.Yaw += e.rand(-2.5, 2.5)
		}
	case Dart, Flee:
		sp := 3.8
		if e.State == Flee {
			sp = 6
		}
		if !e.step(dt, sp, env) {
			e.Yaw += 1.4
		}
		if e.Timer <= 0 {
			e.State, e.Timer = Sit, e.rand(1, 3.5)
		}
	}
	e.Pos.Y = env.Ground(e.Pos.X, e.Pos.Z)
	e.Anim = 0
}

func (e *Entity) updateBeetle(dt float64, env Env) {
	e.Timer -= dt
	if e.Timer <= 0 {
		e.Yaw += e.rand(-1.5, 1.5)
		e.Timer = e.rand(1, 4)
	}
	if !e.step(dt, 0.12, env) {
		e.Yaw += 2
	}
	e.Pos.Y = env.Ground(e.Pos.X, e.Pos.Z)
}

// Instance appends the 8-float GPU instance record for e.
func (e *Entity) Instance(dst []float32) []float32 {
	s := float32(e.Scale * e.Fade)
	if e.Fade <= 0.001 {
		s = 0
	}
	// birth < -500 tells the shader this is an animal: phase = -(birth+1000)
	amp := math.Min(e.Anim/12, 1.5)
	return append(dst, float32(e.Pos.X), float32(e.Pos.Y), float32(e.Pos.Z), s,
		float32(e.Yaw), float32(e.Tint), float32(-1000-e.Gait), float32(amp))
}
