package camera

import (
	"math"
	"testing"

	"evergreen/internal/engine"
	"evergreen/internal/zones"
)

type testEnv struct{ atlas *zones.Atlas }

func newEnv() *testEnv {
	return &testEnv{atlas: zones.NewAtlas(1, func(x, z float64) float64 { return 5 })}
}

func (e *testEnv) GroundAt(x, z, feetY, stepUp float64) float64 { return e.TerrainHeight(x, z) }
func (e *testEnv) WaterLevel(x, z float64) (float64, bool)      { return 0, false }
func (e *testEnv) Push(x, z, r, y float64) (float64, float64)   { return 0, 0 }
func (e *testEnv) Atlas() *zones.Atlas                          { return e.atlas }
func (e *testEnv) TerrainHeight(x, z float64) float64           { return 5 + 2*math.Sin(x*0.01) }

func run(c *Camera, env Env, secs float64, in Input) {
	for t := 0.0; t < secs; t += 1.0 / 60 {
		c.Update(1.0/60, in, env)
	}
}

func TestParseMode(t *testing.T) {
	for i, n := range modeNames {
		if ParseMode(n) != Mode(i) || Mode(i).String() != n {
			t.Fatalf("mode %s", n)
		}
	}
	if ParseMode("nope") != Ambient {
		t.Fatal("unknown mode defaults to ambient")
	}
}

func TestExploreWalksForwardAtEyeHeight(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.SetMode(Explore, env)
	run(c, env, 2, Input{})
	x0 := c.Pos.X
	run(c, env, 2, Input{Move: 1})
	if c.Pos.X-x0 < 8 {
		t.Fatalf("should move ~11m, moved %v", c.Pos.X-x0)
	}
	if h := c.Pos.Y - env.TerrainHeight(c.Pos.X, c.Pos.Z); h < 1.5 || h > 2.0 {
		t.Fatalf("eye height above ground = %v", h)
	}
	run(c, env, 1, Input{Move: 1, Sprint: true})
}

func TestMouseLookClampsPitch(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.SetMode(Explore, env)
	for i := 0; i < 100; i++ {
		c.Update(1.0/60, Input{LookY: 0.2}, env)
	}
	if c.Pitch > 1.46 {
		t.Fatalf("pitch %v exceeds clamp", c.Pitch)
	}
	d := c.Dir()
	if math.Abs(d.Len()-1) > 1e-9 {
		t.Fatal("direction not unit")
	}
}

func TestWheelZoomChangesFOVWithinLimits(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.SetMode(Explore, env)
	for i := 0; i < 200; i++ {
		c.Update(1.0/60, Input{Zoom: -300}, env)
	}
	if c.FOV < 27*math.Pi/180 || c.FOV > 40*math.Pi/180 {
		t.Fatalf("zoom-in fov %v", c.FOV*180/math.Pi)
	}
	for i := 0; i < 400; i++ {
		c.Update(1.0/60, Input{Zoom: 300}, env)
	}
	if c.FOV > 96*math.Pi/180 {
		t.Fatal("fov upper clamp")
	}
}

func TestModeTransitionIsContinuous(t *testing.T) {
	env := newEnv()
	c := New(env, 100, 100, 1)
	c.SetMode(Explore, env)
	run(c, env, 1, Input{Move: 1})
	prev := c.Pos
	c.SetMode(Cinematic, env)
	maxJump := 0.0
	for i := 0; i < 300; i++ {
		c.Update(1.0/60, Input{}, env)
		maxJump = math.Max(maxJump, c.Pos.Sub(prev).Len())
		prev = c.Pos
	}
	if maxJump > 8 {
		t.Fatalf("camera jumped %v m in one frame on mode switch", maxJump)
	}
}

func TestAmbientDriftsToLandmarksAboveGround(t *testing.T) {
	env := newEnv()
	sx, sz, yaw := env.atlas.Spawn()
	c := New(env, sx, sz, yaw)
	start := c.Pos
	run(c, env, 60, Input{})
	if c.Pos.Sub(start).LenXZ() < 100 {
		t.Fatalf("ambient camera should travel, moved %v", c.Pos.Sub(start).LenXZ())
	}
	for i := 0; i < 600; i++ {
		c.Update(1.0/60, Input{}, env)
		if c.Pos.Y < env.TerrainHeight(c.Pos.X, c.Pos.Z)+1.5 {
			t.Fatal("ambient camera dipped into terrain")
		}
	}
	slow := New(env, sx, sz, yaw)
	slow.ReducedMotion = true
	run(slow, env, 60, Input{})
	if slow.Pos.Sub(start).LenXZ() >= c.Pos.Sub(start).LenXZ() {
		t.Fatal("reduced motion must slow the ambient camera")
	}
}

func TestCinematicOrbitsLandmark(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.SetMode(Cinematic, env)
	run(c, env, 40, Input{})
	wp := c.waypoint(env)
	d := math.Hypot(c.Pos.X-wp.X, c.Pos.Z-wp.Z)
	if d > 2500 {
		t.Fatalf("camera should be near its landmark, %v away", d)
	}
	if c.Pos.Y < env.TerrainHeight(c.Pos.X, c.Pos.Z)+5.9 {
		t.Fatal("cinematic camera must clear terrain")
	}
	if c.Focus <= 0 {
		t.Fatal("focus distance for depth of field")
	}
}

func TestScrollModeAdvancesAlongPath(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.SetMode(Scroll, env)
	start := c.ScrollPosition()
	last := start
	for i := 0; i < 300; i++ {
		c.Update(1.0/60, Input{Scroll: 12}, env)
		if c.ScrollPosition() < last-1e-9 {
			t.Fatal("scroll position went backwards")
		}
		last = c.ScrollPosition()
	}
	if last-start < 500 {
		t.Fatalf("scroll moved only %v", last-start)
	}
	before := c.ScrollPosition()
	for i := 0; i < 300; i++ {
		c.Update(1.0/60, Input{Scroll: -12}, env)
	}
	if c.ScrollPosition() >= before {
		t.Fatal("negative scroll should go back")
	}
}

func TestScrollPathContinuesAcrossRealms(t *testing.T) {
	env := newEnv()
	p0, _ := ScrollPoint(env, 0)
	pFar, _ := ScrollPoint(env, 12000)
	if pFar.X-p0.X < zones.RealmSize {
		t.Fatalf("scroll path should continue into following realms: %v", pFar.X-p0.X)
	}
	prev := p0
	for s := 0.0; s < 8000; s += 10 {
		p, tan := ScrollPoint(env, s)
		if p.Sub(prev).LenXZ() > 40 {
			t.Fatalf("path discontinuity at s=%v", s)
		}
		if math.Abs(tan.Len()-1) > 1e-6 {
			t.Fatalf("tangent not unit at s=%v: %v", s, tan.Len())
		}
		prev = p
	}
}

func TestViewMatrixLooksAtDirection(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.Yaw, c.Pitch = math.Pi/2, 0
	c.Pos = engine.Vec3{}
	f := engine.NewFrustum(c.Proj(1.6).Mul(c.View()))
	if !f.IntersectsAABB(engine.Vec3{X: -1, Y: -1, Z: 9}, engine.Vec3{X: 1, Y: 1, Z: 11}) {
		t.Fatal("object along +Z must be visible when yaw=90deg")
	}
	if f.IntersectsAABB(engine.Vec3{X: 9, Y: -1, Z: -1}, engine.Vec3{X: 11, Y: 1, Z: 1}) {
		t.Fatal("object along +X must not be visible")
	}
}

func TestResetAndTeleport(t *testing.T) {
	env := newEnv()
	c := New(env, 0, 0, 0)
	c.SetMode(Explore, env)
	c.Teleport(env, 500, 500, 1)
	if math.Hypot(c.Pos.X-500, c.Pos.Z-500) > 1 {
		t.Fatal("teleport")
	}
	c.Reset(env, 10, 10, 0.5)
	if c.Pos.X != 10 || c.Yaw != 0.5 {
		t.Fatal("reset")
	}
}
