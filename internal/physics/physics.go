// Package physics provides the player body: terrain following, step-up onto
// walkable structures, wall collisions, wading and swimming.
package physics

import "math"

// World is the environment the body moves through.
type World interface {
	// GroundAt returns the highest standable surface height at (x,z) that is at
	// most stepUp above feetY, including structures such as bridges.
	GroundAt(x, z, feetY, stepUp float64) float64
	// WaterLevel returns the water surface height at (x,z) if there is water.
	WaterLevel(x, z float64) (float64, bool)
	// Push resolves overlap with solid objects, returning a corrective offset.
	Push(x, z, radius, y float64) (dx, dz float64)
}

// Body is the player's collision capsule (feet position).
type Body struct {
	X, Y, Z    float64
	VY         float64
	Radius     float64
	EyeHeight  float64
	OnGround   bool
	Swimming   bool
	Underwater bool
	Depth      float64 // water depth at the feet
	GroundY    float64
}

// Tunables.
const (
	Gravity   = 24.0
	StepUp    = 0.75
	MaxSlope  = 1.15 // rise/run
	JumpSpeed = 7.0
	WadeDepth = 0.9
	SwimDepth = 1.5
)

// NewBody places a body standing at (x,z) on the ground.
func NewBody(w World, x, z float64) *Body {
	b := &Body{X: x, Z: z, Radius: 0.45, EyeHeight: 1.7}
	b.Y = w.GroundAt(x, z, 1e9, 1e9)
	b.GroundY = b.Y
	b.OnGround = true
	return b
}

// Eye returns the eye height in world space (lowers slightly when swimming).
func (b *Body) Eye() float64 {
	if b.Swimming {
		return b.Y + b.EyeHeight*0.35
	}
	return b.Y + b.EyeHeight
}

// Step moves the body. (vx,vz) is the desired horizontal velocity in m/s,
// vert is swim up/down input (-1..1) and jump requests a jump.
func (b *Body) Step(dt, vx, vz, vert float64, jump bool, w World) {
	if dt <= 0 {
		return
	}
	dt = math.Min(dt, 0.05)
	wl, wet := w.WaterLevel(b.X, b.Z)
	ground := w.GroundAt(b.X, b.Z, b.Y, StepUp)
	b.Depth = 0
	if wet && wl > ground {
		b.Depth = wl - ground
	}
	b.Swimming = b.Depth > SwimDepth && b.Y+0.2 < wl+0.2
	speed := 1.0
	if b.Depth > 0.25 && !b.Swimming {
		speed = 1 - 0.45*math.Min(1, b.Depth/WadeDepth)
	}
	if b.Swimming {
		speed = 0.55
	}
	nx, nz := b.X+vx*speed*dt, b.Z+vz*speed*dt
	// try full move, then axis-separated slides
	for _, c := range [][2]float64{{nx, nz}, {nx, b.Z}, {b.X, nz}} {
		if b.tryMove(c[0], c[1], w) {
			break
		}
	}
	ground = w.GroundAt(b.X, b.Z, b.Y, StepUp)
	b.GroundY = ground
	wl, wet = w.WaterLevel(b.X, b.Z)
	b.Depth = 0
	if wet && wl > ground {
		b.Depth = wl - ground
	}
	b.Swimming = b.Depth > SwimDepth && b.Y < wl-0.2+0.5
	if b.Swimming {
		b.VY += vert * 6 * dt
		b.VY *= math.Exp(-3 * dt)
		b.Y += b.VY * dt
		b.Y = math.Min(b.Y, wl-0.35)
		b.Y = math.Max(b.Y, ground+0.1)
		b.OnGround = false
		b.Underwater = b.Eye() < wl
		return
	}
	b.Underwater = wet && b.Eye() < wl
	if b.OnGround && jump {
		b.VY = JumpSpeed
		b.OnGround = false
	}
	b.VY -= Gravity * dt
	b.Y += b.VY * dt
	if b.Y <= ground {
		if b.VY < -1 || !b.OnGround {
			b.VY = 0
		}
		// smooth snapping so stairs and bumps do not jolt the camera
		if b.OnGround {
			b.Y += (ground - b.Y) * math.Min(1, dt*18)
			if math.Abs(ground-b.Y) < 0.01 {
				b.Y = ground
			}
		} else {
			b.Y = ground
		}
		b.VY = 0
		b.OnGround = true
	} else if b.Y > ground+0.08 {
		b.OnGround = false
	}
}

func (b *Body) tryMove(nx, nz float64, w World) bool {
	if nx == b.X && nz == b.Z {
		return true
	}
	cur := w.GroundAt(b.X, b.Z, b.Y, StepUp)
	for i := 0; i < 3; i++ {
		dx, dz := w.Push(nx, nz, b.Radius, b.Y)
		if dx == 0 && dz == 0 {
			break
		}
		nx, nz = nx+dx, nz+dz
	}
	next := w.GroundAt(nx, nz, b.Y, StepUp)
	run := math.Hypot(nx-b.X, nz-b.Z)
	if run > 1e-9 && next-cur > MaxSlope*run+0.05 && next-b.Y > StepUp*0.9 {
		return false
	}
	if run > 1e-9 && b.OnGround && cur-next > 3.5 && run < 1 {
		// do not walk off tall ledges accidentally unless already airborne
		if cur-next > 6 {
			return false
		}
	}
	b.X, b.Z = nx, nz
	return true
}
