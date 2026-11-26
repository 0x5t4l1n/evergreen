// Package camera implements Evergreen's four exploration modes (ambient,
// explore, scroll, cinematic), smooth transitions between them, and the view
// matrices derived from the camera pose.
package camera

import (
	"math"

	"evergreen/internal/engine"
	"evergreen/internal/physics"
	"evergreen/internal/procedural"
	"evergreen/internal/zones"
)

// Mode selects how the camera is driven.
type Mode int

// Camera modes.
const (
	Ambient Mode = iota
	Explore
	Scroll
	Cinematic
)

var modeNames = [...]string{"ambient", "explore", "scroll", "cinematic"}

func (m Mode) String() string { return modeNames[m] }

// ParseMode converts a config string to a Mode (Ambient when unknown).
func ParseMode(s string) Mode {
	for i, n := range modeNames {
		if n == s {
			return Mode(i)
		}
	}
	return Ambient
}

// Env is what the camera needs from the world.
type Env interface {
	physics.World
	Atlas() *zones.Atlas
	TerrainHeight(x, z float64) float64
}

// Input is one frame of user input.
type Input struct {
	Move, Strafe, Vert float64 // -1..1
	LookX, LookY       float64 // radians this frame
	Zoom               float64 // wheel delta in pixels
	Scroll             float64 // scroll-mode delta in pixels
	Sprint, Jump       bool
	Active             bool // any user input this frame
}

// Camera holds the pose and mode state.
type Camera struct {
	Mode          Mode
	Pos           engine.Vec3 // eye position
	Yaw, Pitch    float64
	FOV           float64 // radians
	Focus         float64 // focus distance for depth of field (metres)
	Body          *physics.Body
	ReducedMotion bool

	targetFOV float64
	t         float64
	// transition
	from     pose
	blend    float64
	blending bool
	// autopilot
	wpIndex int
	wpRealm [2]int
	orbit   float64
	orbitT  float64
	flying  bool
	// scroll
	scrollPos, scrollTarget float64
	scrollVel               float64
	smoothedEyeY            float64
}

type pose struct {
	pos             engine.Vec3
	yaw, pitch, fov float64
}

// New creates a camera standing at (x,z) looking along yaw.
func New(env Env, x, z, yaw float64) *Camera {
	c := &Camera{Mode: Ambient, Yaw: yaw, FOV: 70 * math.Pi / 180, targetFOV: 70 * math.Pi / 180, Focus: 40}
	c.Body = physics.NewBody(env, x, z)
	c.Pos = engine.Vec3{X: x, Y: c.Body.Eye(), Z: z}
	c.smoothedEyeY = c.Pos.Y
	c.wpRealm = [2]int{0, 0}
	return c
}

// Reset teleports the camera.
func (c *Camera) Reset(env Env, x, z, yaw float64) {
	c.Body = physics.NewBody(env, x, z)
	c.Yaw, c.Pitch = yaw, 0
	c.Pos = engine.Vec3{X: x, Y: c.Body.Eye(), Z: z}
	c.smoothedEyeY = c.Pos.Y
	c.blending = false
	c.flying = false
	c.scrollTarget, c.scrollPos = 0, 0
	c.targetFOV = 70 * math.Pi / 180
}

// SetMode switches mode with a smooth blend from the current pose.
func (c *Camera) SetMode(m Mode, env Env) {
	if m == c.Mode {
		return
	}
	c.from = pose{c.Pos, c.Yaw, c.Pitch, c.FOV}
	c.blend, c.blending = 0, true
	c.Mode = m
	c.flying = false
	if m == Explore || m == Ambient {
		c.Body.X, c.Body.Z = c.Pos.X, c.Pos.Z
		c.Body.Y = env.GroundAt(c.Pos.X, c.Pos.Z, 1e9, 1e9)
		c.Body.VY = 0
		c.Body.OnGround = true
	}
	if m == Scroll {
		c.scrollPos = c.nearestScroll(env)
		c.scrollTarget = c.scrollPos
	}
}

// Teleport moves the camera to a world position (used by zone navigation).
func (c *Camera) Teleport(env Env, x, z, yaw float64) {
	c.from = pose{c.Pos, c.Yaw, c.Pitch, c.FOV}
	c.blend, c.blending = 0, true
	c.Body.X, c.Body.Z = x, z
	c.Body.Y = env.GroundAt(x, z, 1e9, 1e9)
	c.Body.VY, c.Body.OnGround = 0, true
	c.Yaw = yaw
	c.Pos = engine.Vec3{X: x, Y: c.Body.Eye(), Z: z}
	c.smoothedEyeY = c.Pos.Y
	c.flying = false
	c.from.pos = c.Pos.Add(c.Pos.Sub(c.from.pos).Scale(0)) // jump cut: blend only orientation
	c.blending = false
}

// Dir is the unit view direction.
func (c *Camera) Dir() engine.Vec3 {
	cp := math.Cos(c.Pitch)
	return engine.Vec3{X: cp * math.Cos(c.Yaw), Y: math.Sin(c.Pitch), Z: cp * math.Sin(c.Yaw)}
}

// View returns the view matrix.
func (c *Camera) View() engine.Mat4 {
	return engine.LookAt(c.Pos, c.Pos.Add(c.Dir()), engine.Vec3{Y: 1})
}

// Proj returns the projection matrix.
func (c *Camera) Proj(aspect float64) engine.Mat4 {
	return engine.Perspective(c.FOV, aspect, 0.15, 3200)
}

func angleLerp(a, b, t float64) float64 { return a + math.Remainder(b-a, 2*math.Pi)*t }

// Update advances the camera by dt seconds.
func (c *Camera) Update(dt float64, in Input, env Env) {
	dt = math.Min(dt, 0.1)
	c.t += dt
	switch c.Mode {
	case Explore:
		c.explore(dt, in, env)
	case Ambient:
		c.ambient(dt, in, env)
	case Scroll:
		c.scroll(dt, in, env)
	case Cinematic:
		c.cinematic(dt, env)
	}
	c.FOV += (c.targetFOV - c.FOV) * math.Min(1, dt*6)
	if c.blending {
		c.blend += dt / 1.8
		k := procedural.SmoothStep(0, 1, c.blend)
		c.Pos = c.from.pos.Lerp(c.Pos, k)
		c.Yaw = angleLerp(c.from.yaw, c.Yaw, k)
		c.Pitch = procedural.Mix(c.from.pitch, c.Pitch, k)
		if c.blend >= 1 {
			c.blending = false
		}
	}
}

func (c *Camera) look(in Input) {
	c.Yaw += in.LookX
	c.Pitch = procedural.Clamp(c.Pitch+in.LookY, -1.45, 1.45)
}

func (c *Camera) zoom(in Input) {
	if in.Zoom != 0 {
		c.targetFOV = procedural.Clamp(c.targetFOV*math.Exp(in.Zoom*0.0012), 28*math.Pi/180, 95*math.Pi/180)
	}
}

func (c *Camera) explore(dt float64, in Input, env Env) {
	c.look(in)
	c.zoom(in)
	fx, fz := math.Cos(c.Yaw), math.Sin(c.Yaw)
	speed := 5.6
	if in.Sprint {
		speed = 12
	}
	vx := (fx*in.Move - fz*in.Strafe) * speed
	vz := (fz*in.Move + fx*in.Strafe) * speed
	if l := math.Hypot(in.Move, in.Strafe); l > 1 {
		vx, vz = vx/l, vz/l
	}
	c.Body.Step(dt, vx, vz, in.Vert, in.Jump, env)
	eyeY := c.Body.Eye()
	c.smoothedEyeY += (eyeY - c.smoothedEyeY) * math.Min(1, dt*16)
	if math.Abs(c.smoothedEyeY-eyeY) > 4 {
		c.smoothedEyeY = eyeY
	}
	c.Pos = engine.Vec3{X: c.Body.X, Y: c.smoothedEyeY, Z: c.Body.Z}
	c.Focus = 12
}

// tourOrder is the sequence the autopilots visit.
var tourOrder = []zones.ID{zones.Entrance, zones.Wilderness, zones.Falls, zones.Cave, zones.Treehouse, zones.Ruins, zones.Observatory, zones.Station, zones.WorldTree}

func (c *Camera) waypoint(env Env) zones.Zone {
	zs := env.Atlas().Realm(c.wpRealm[0], c.wpRealm[1])
	id := tourOrder[c.wpIndex%len(tourOrder)]
	for _, z := range zs {
		if z.ID == id {
			return z
		}
	}
	return zs[0]
}

func (c *Camera) nextWaypoint() {
	c.wpIndex++
	if c.wpIndex%len(tourOrder) == 0 {
		c.wpRealm[0]++
	}
}

// ambient wanders slowly toward successive landmarks at head height, looking around gently.
func (c *Camera) ambient(dt float64, in Input, env Env) {
	if in.Active && (in.LookX != 0 || in.LookY != 0) {
		c.look(in)
	}
	wp := c.waypoint(env)
	tx, tz := wp.X-math.Cos(float64(c.wpIndex)*1.7)*wp.Radius*0.35, wp.Z-math.Sin(float64(c.wpIndex)*1.7)*wp.Radius*0.35
	if math.Hypot(tx-c.Body.X, tz-c.Body.Z) < 38 {
		c.nextWaypoint()
	}
	want := math.Atan2(tz-c.Body.Z, tx-c.Body.X)
	wander := 0.55 * math.Sin(c.t*0.11) * math.Sin(c.t*0.043+1)
	heading := c.moveHeading(want+wander, env)
	speed := 4.2
	if c.ReducedMotion {
		speed = 2.0
	}
	nx, nz := c.Body.X+math.Cos(heading)*speed*dt, c.Body.Z+math.Sin(heading)*speed*dt
	for i := 0; i < 3; i++ {
		dx, dz := env.Push(nx, nz, 1.2, env.TerrainHeight(nx, nz))
		nx, nz = nx+dx, nz+dz
	}
	c.Body.X, c.Body.Z = nx, nz
	ground := env.GroundAt(nx, nz, 1e9, 1e9)
	if wl, ok := env.WaterLevel(nx, nz); ok && wl > ground {
		ground = wl
	}
	target := ground + 3.6 + 0.9*math.Sin(c.t*0.35)
	c.smoothedEyeY += (target - c.smoothedEyeY) * math.Min(1, dt*1.6)
	c.Body.Y = ground
	c.Pos = engine.Vec3{X: nx, Y: c.smoothedEyeY, Z: nz}
	c.Yaw = angleLerp(c.Yaw, heading+0.35*math.Sin(c.t*0.17), math.Min(1, dt*0.8))
	c.Pitch += (-0.06 + 0.05*math.Sin(c.t*0.23) - c.Pitch) * math.Min(1, dt*0.6)
	c.targetFOV = 68 * math.Pi / 180
	c.Focus = 30
}

// moveHeading steers away from steep slopes by testing a few candidate headings.
func (c *Camera) moveHeading(want float64, env Env) float64 {
	best, bestScore := want, math.Inf(1)
	for _, off := range []float64{0, 0.5, -0.5, 1.0, -1.0} {
		h := want + off
		ax, az := c.Body.X+math.Cos(h)*14, c.Body.Z+math.Sin(h)*14
		hh := env.TerrainHeight(ax, az)
		score := math.Abs(hh-c.Body.Y)*0.5 + math.Abs(off)*2
		if _, deep := env.WaterLevel(ax, az); deep {
			score += 1.5
		}
		if score < bestScore {
			best, bestScore = h, score
		}
	}
	return best
}

// ---- cinematic ---------------------------------------------------------------

func (c *Camera) cinematic(dt float64, env Env) {
	wp := c.waypoint(env)
	focusY := wp.BaseY + 10
	orbitR, height := wp.Radius*0.7, 34.0
	switch wp.ID {
	case zones.WorldTree:
		orbitR, height, focusY = 175, 120, wp.BaseY+90
	case zones.Observatory:
		orbitR, height, focusY = 80, 30, wp.BaseY+72
	case zones.Falls:
		orbitR, height, focusY = 70, 22, wp.BaseY-2
	case zones.Cave:
		orbitR, height, focusY = 105, 40, wp.BaseY+8
	case zones.Treehouse:
		orbitR, height, focusY = 70, 30, wp.BaseY+22
	case zones.Station:
		orbitR, height, focusY = 42, 12, wp.BaseY+3
	case zones.Ruins:
		orbitR, height, focusY = 62, 24, wp.BaseY+6
	}
	speed := 0.045
	if c.ReducedMotion {
		speed = 0.015
	}
	c.orbit += dt * speed
	c.orbitT += dt
	ang := c.orbit + float64(c.wpIndex)*0.9
	goal := engine.Vec3{X: wp.X + math.Cos(ang)*orbitR, Z: wp.Z + math.Sin(ang)*orbitR}
	ground := env.TerrainHeight(goal.X, goal.Z)
	goal.Y = math.Max(ground+10, wp.BaseY+height)
	if !c.flying {
		c.flying = true
		c.orbitT = 0
	}
	// ease toward the orbit position; this also acts as the fly-between-landmarks motion
	k := math.Min(1, dt*0.35)
	c.Pos = c.Pos.Lerp(goal, k)
	if c.Pos.Y < env.TerrainHeight(c.Pos.X, c.Pos.Z)+6 {
		c.Pos.Y = env.TerrainHeight(c.Pos.X, c.Pos.Z) + 6
	}
	look := engine.Vec3{X: wp.X, Y: focusY, Z: wp.Z}.Sub(c.Pos)
	c.Yaw = angleLerp(c.Yaw, math.Atan2(look.Z, look.X), math.Min(1, dt*0.8))
	c.Pitch += (math.Atan2(look.Y, math.Hypot(look.X, look.Z)) - c.Pitch) * math.Min(1, dt*0.8)
	c.targetFOV = 58 * math.Pi / 180
	c.Focus = math.Max(20, look.Len())
	if c.orbitT > 55 && c.Pos.Sub(goal).Len() < orbitR*0.4 || c.orbitT > 90 {
		c.nextWaypoint()
		c.orbitT = 0
	}
	c.Body.X, c.Body.Z = c.Pos.X, c.Pos.Z
}

// ---- scroll ----------------------------------------------------------------------

// scrollPath returns the tour polyline starting at realm (0,0).
func scrollPath(env Env, upto int) []engine.Vec3 {
	var pts []engine.Vec3
	for r := 0; r*len(tourOrder) < upto+len(tourOrder); r++ {
		zs := env.Atlas().Realm(r, 0)
		for _, id := range tourOrder {
			for _, z := range zs {
				if z.ID != id {
					continue
				}
				p := engine.Vec3{X: z.X, Z: z.Z}
				if id == tourOrder[0] && len(pts) > 0 {
					// bridge the gap between realms with evenly spaced points so the spline stays tame
					last := pts[len(pts)-1]
					const gaps = 6
					for g := 1; g < gaps; g++ {
						pts = append(pts, last.Lerp(p, float64(g)/gaps))
					}
				}
				pts = append(pts, p)
			}
		}
	}
	return pts
}

func catmull(p0, p1, p2, p3 engine.Vec3, t float64) engine.Vec3 {
	t2, t3 := t*t, t*t*t
	f := func(a, b, c, d float64) float64 {
		return 0.5 * ((2 * b) + (-a+c)*t + (2*a-5*b+4*c-d)*t2 + (-a+3*b-3*c+d)*t3)
	}
	return engine.Vec3{X: f(p0.X, p1.X, p2.X, p3.X), Z: f(p0.Z, p1.Z, p2.Z, p3.Z)}
}

// ScrollPoint returns the path position (x,z) and unit tangent at distance s along the tour.
func ScrollPoint(env Env, s float64) (pos engine.Vec3, tangent engine.Vec3) {
	pts := scrollPath(env, int(s/300)+3)
	if s < 0 {
		s = 0
	}
	seg := 0
	rem := s
	for ; seg < len(pts)-1; seg++ {
		l := pts[seg+1].Sub(pts[seg]).LenXZ()
		if rem < l {
			break
		}
		rem -= l
	}
	if seg >= len(pts)-1 {
		seg = len(pts) - 2
	}
	get := func(i int) engine.Vec3 { return pts[max(0, min(len(pts)-1, i))] }
	l := pts[seg+1].Sub(pts[seg]).LenXZ()
	t := procedural.Clamp(rem/l, 0, 1)
	pos = catmull(get(seg-1), get(seg), get(seg+1), get(seg+2), t)
	ahead := catmull(get(seg-1), get(seg), get(seg+1), get(seg+2), math.Min(1, t+0.02))
	tangent = ahead.Sub(pos).Norm()
	if tangent.LenXZ() < 1e-6 {
		tangent = pts[seg+1].Sub(pts[seg]).Norm()
	}
	return
}

func (c *Camera) nearestScroll(env Env) float64 {
	best, bd := 0.0, math.Inf(1)
	for s := 0.0; s < 14000; s += 40 {
		p, _ := ScrollPoint(env, s)
		if d := math.Hypot(p.X-c.Pos.X, p.Z-c.Pos.Z); d < bd {
			best, bd = s, d
		}
	}
	return best
}

func (c *Camera) scroll(dt float64, in Input, env Env) {
	c.scrollTarget = math.Max(0, c.scrollTarget+in.Scroll*0.9+in.Move*dt*60)
	prev := c.scrollPos
	c.scrollPos += (c.scrollTarget - c.scrollPos) * math.Min(1, dt*3.2)
	c.scrollVel = (c.scrollPos - prev) / math.Max(dt, 1e-4)
	pos, tan := ScrollPoint(env, c.scrollPos)
	ground := env.TerrainHeight(pos.X, pos.Z)
	if wl, ok := env.WaterLevel(pos.X, pos.Z); ok && wl > ground {
		ground = wl
	}
	// parallax: gentle lateral drift and bob proportional to travel speed
	side := engine.Vec3{X: -tan.Z, Z: tan.X}
	sway := math.Sin(c.scrollPos*0.02) * 6
	if c.ReducedMotion {
		sway = 0
	}
	pos = pos.Add(side.Scale(sway))
	target := ground + 5.5 + math.Min(8, math.Abs(c.scrollVel)*0.02)
	c.smoothedEyeY += (target - c.smoothedEyeY) * math.Min(1, dt*3)
	c.Pos = engine.Vec3{X: pos.X, Y: c.smoothedEyeY, Z: pos.Z}
	yaw := math.Atan2(tan.Z, tan.X)
	c.Yaw = angleLerp(c.Yaw, yaw+0.25*math.Sin(c.scrollPos*0.011), math.Min(1, dt*4))
	c.Pitch += (-0.1 - c.Pitch) * math.Min(1, dt*2)
	c.targetFOV = (66 + math.Min(10, math.Abs(c.scrollVel)*0.03)) * math.Pi / 180
	c.Focus = 35
	c.Body.X, c.Body.Z = c.Pos.X, c.Pos.Z
	_ = in
}

// ScrollPosition reports the distance travelled along the scroll tour.
func (c *Camera) ScrollPosition() float64 { return c.scrollPos }
