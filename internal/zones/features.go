package zones

import (
	"math"

	"evergreen/internal/mesh"
)

// Light is a point light in zone-local coordinates (U,V = metres east/south of
// the zone centre, Y = metres above the zone base height).
type Light struct {
	U, Y, V float64
	R, G, B float32
	Radius  float64
	Flicker float64
}

// Collider blocks horizontal movement. A circle when HX==0, otherwise an
// axis-aligned box with half extents HX,HZ.
type Collider struct {
	CX, CZ, R, HX, HZ float64
	Y0, Y1            float64 // vertical extent (relative to base)
	Group             int     // non-zero groups can be disabled (doors)
}

// Walkable is a surface the player can stand on: a disc, or a segment with a
// half-width whose height varies linearly from Y0 to Y1 (bridges, stairs, ramps).
type Walkable struct {
	Disc           bool
	X0, Z0, X1, Z1 float64 // disc uses X0,Z0 as centre
	R              float64 // radius or half width
	Y0, Y1         float64
}

// HeightAt returns the surface height at local (u,v) if it covers that point.
func (w Walkable) HeightAt(u, v float64) (float64, bool) {
	if w.Disc {
		if math.Hypot(u-w.X0, v-w.Z0) <= w.R {
			return w.Y0, true
		}
		return 0, false
	}
	dx, dz := w.X1-w.X0, w.Z1-w.Z0
	l2 := dx*dx + dz*dz
	if l2 == 0 {
		return 0, false
	}
	t := ((u-w.X0)*dx + (v-w.Z0)*dz) / l2
	if t < 0 || t > 1 {
		return 0, false
	}
	if math.Hypot(u-(w.X0+dx*t), v-(w.Z0+dz*t)) > w.R {
		return 0, false
	}
	return w.Y0 + (w.Y1-w.Y0)*t, true
}

// Interactable is something the visitor can activate with the E key.
type Interactable struct {
	U, Y, V float64
	Radius  float64
	Prompt  string
	Action  string
}

// Emitter spawns CPU particles of a named kind around a point.
type Emitter struct {
	Kind    string // mist, spark, glow, steam
	U, Y, V float64
	Spread  float64
	Rate    float64 // particles per second at full quality
}

// Anim selects how a Part moves.
type Anim int

// Part animation kinds.
const (
	AnimNone Anim = iota
	AnimSpin
	AnimDoor
	AnimGrow
)

// Part is one drawable mesh of a landmark.
type Part struct {
	Mesh    *mesh.Mesh
	U, Y, V float64
	Anim    Anim
	Rate    float64 // spin rad/s
	Group   int     // door group for AnimDoor
	Slide   float64 // metres a door sinks when fully open
}

// Landmark is the complete built representation of a zone.
type Landmark struct {
	Zone       Zone
	Parts      []Part
	Lights     []Light
	Colliders  []Collider
	Walk       []Walkable
	Interact   []Interactable
	Emitters   []Emitter
	Radius     float64 // bounding radius for streaming/culling
	MinY, MaxY float64

	open   map[int]float64
	target map[int]float64
	Mode   int     // station console mode
	Growth float64 // World Tree growth 0..1 supplied by the ecosystem
	age    float64
}

func (l *Landmark) init() {
	l.open = map[int]float64{}
	l.target = map[int]float64{}
	l.Growth = 0.5
}

// Update eases animated state.
func (l *Landmark) Update(dt float64) {
	l.age += dt
	for g, t := range l.target {
		l.open[g] += (t - l.open[g]) * math.Min(1, dt*1.2)
	}
}

// DoorOpen reports how open a door group is (0..1).
func (l *Landmark) DoorOpen(g int) float64 { return l.open[g] }

// PartPose returns the world-space placement of part i: position, yaw, scale.
func (l *Landmark) PartPose(i int, t float64) (x, y, z, yaw, scale float64) {
	p := l.Parts[i]
	x, y, z = l.Zone.X+p.U, l.Zone.BaseY+p.Y, l.Zone.Z+p.V
	scale = 1
	switch p.Anim {
	case AnimSpin:
		yaw = p.Rate * t
	case AnimDoor:
		y -= p.Slide * l.open[p.Group]
	case AnimGrow:
		g := 0.88 + 0.12*l.Growth
		scale = g * (0.25 + 0.75*sm(0, 9, l.age))
	}
	return
}

// World converts local landmark coordinates to world coordinates.
func (l *Landmark) World(u, y, v float64) (float64, float64, float64) {
	return l.Zone.X + u, l.Zone.BaseY + y, l.Zone.Z + v
}

// GroundAt returns the highest walkable surface (world Y) at world (x,z) that
// is not more than stepUp above feetY.
func (l *Landmark) GroundAt(x, z, feetY, stepUp float64) (float64, bool) {
	u, v := x-l.Zone.X, z-l.Zone.Z
	best, ok := 0.0, false
	for _, w := range l.Walk {
		if y, hit := w.HeightAt(u, v); hit {
			wy := l.Zone.BaseY + y
			if wy <= feetY+stepUp && (!ok || wy > best) {
				best, ok = wy, true
			}
		}
	}
	return best, ok
}

// Push resolves overlap of a circle (world x,z,radius) at height y against the
// landmark colliders, returning the corrective offset.
func (l *Landmark) Push(x, z, radius, y float64) (dx, dz float64) {
	u, v := x-l.Zone.X, z-l.Zone.Z
	ry := y - l.Zone.BaseY
	for _, c := range l.Colliders {
		if c.Group != 0 && l.open[c.Group] > 0.6 {
			continue
		}
		if ry < c.Y0-1.7 || ry > c.Y1 {
			continue
		}
		if c.HX == 0 {
			ox, oz := u-c.CX, v-c.CZ
			d := math.Hypot(ox, oz)
			if d < c.R+radius && d > 1e-6 {
				k := (c.R + radius - d) / d
				u, v = u+ox*k, v+oz*k
			}
			continue
		}
		nx := math.Max(c.CX-c.HX, math.Min(u, c.CX+c.HX))
		nz := math.Max(c.CZ-c.HZ, math.Min(v, c.CZ+c.HZ))
		ox, oz := u-nx, v-nz
		d := math.Hypot(ox, oz)
		if d < radius {
			if d > 1e-6 {
				k := (radius - d) / d
				u, v = u+ox*k, v+oz*k
			} else {
				// centre inside the box: push out along the shallowest axis
				px, pz := c.HX-math.Abs(u-c.CX), c.HZ-math.Abs(v-c.CZ)
				if px < pz {
					u += math.Copysign(px+radius, u-c.CX)
				} else {
					v += math.Copysign(pz+radius, v-c.CZ)
				}
			}
		}
	}
	return l.Zone.X + u - x, l.Zone.Z + v - z
}

// NearestInteract returns the closest interactable within reach of world (x,y,z).
func (l *Landmark) NearestInteract(x, y, z float64) (Interactable, bool) {
	u, v := x-l.Zone.X, z-l.Zone.Z
	best, bd, ok := Interactable{}, math.MaxFloat64, false
	for _, it := range l.Interact {
		d := math.Hypot(u-it.U, v-it.V)
		if d < it.Radius && math.Abs(y-(l.Zone.BaseY+it.Y)) < 4 && d < bd {
			best, bd, ok = it, d, true
		}
	}
	return best, ok
}

// Activate runs an interaction and returns a message for the HUD.
func (l *Landmark) Activate(action string) string {
	switch action {
	case "ruins.glyph":
		if l.target[1] > 0.5 {
			l.target[1] = 0
			return "The glyphs dim. Stone grinds shut."
		}
		l.target[1] = 1
		return "The glyphs flare. Something deep inside the temple wakes."
	case "station.console":
		l.Mode = (l.Mode + 1) % 4
		return [...]string{"Console: passive monitoring", "Console: threat hunting", "Console: network sweep", "Console: lockdown drill"}[l.Mode]
	case "observatory.telescope":
		return "stargaze"
	}
	return ""
}

// ModeValue is the console mode as seen by the screen shader.
func (l *Landmark) ModeValue() float64 { return float64(l.Mode) }

// LoreFor returns the short lines shown when a zone is entered.
func LoreFor(id ID) []string {
	switch id {
	case Entrance:
		return []string{"The path begins where the light is warmest.", "Follow the trail inward. The giants are watching."}
	case Wilderness:
		return []string{"Rain hangs in the air long after it has fallen.", "Nothing here was planted. Everything is older than the trail."}
	case Falls:
		return []string{"Emerald water drops into a pool no one has found the bottom of."}
	case Cave:
		return []string{"The crystals hum when the lake is still.", "Light grows where the sun cannot reach."}
	case Treehouse:
		return []string{"Lanterns swing. Someone was here this morning.", "The bridges remember every footstep."}
	case Ruins:
		return []string{"The builders left no names, only symbols.", "One glyph on the stairs is warm to the touch."}
	case Observatory:
		return []string{"They mapped the sky from here for three hundred years.", "The telescope still points at the moon."}
	case Station:
		return []string{"Terminal logs: 'No intrusions detected. 214 days.'", "The rain tapping the glass is the only traffic left."}
	case WorldTree:
		return []string{"Every root in the forest leads here.", "It grows a little with each thing that is made in its name."}
	}
	return nil
}
