package physics

import (
	"math"
	"testing"
)

type flat struct {
	groundY float64
	water   float64
	hasWat  bool
	wallX   float64 // wall at x >= wallX when >0
	ramp    bool
}

func (f flat) GroundAt(x, z, feetY, stepUp float64) float64 {
	if f.ramp && x > 0 && x < 20 {
		return f.groundY + x*0.5
	}
	return f.groundY
}
func (f flat) WaterLevel(x, z float64) (float64, bool) { return f.water, f.hasWat }
func (f flat) Push(x, z, r, y float64) (float64, float64) {
	if f.wallX > 0 && x+r > f.wallX {
		return f.wallX - (x + r), 0
	}
	return 0, 0
}

func settle(b *Body, w World, secs float64) {
	for t := 0.0; t < secs; t += 1.0 / 60 {
		b.Step(1.0/60, 0, 0, 0, false, w)
	}
}

func TestFallsAndLands(t *testing.T) {
	w := flat{groundY: 10}
	b := &Body{X: 0, Y: 30, Radius: 0.4, EyeHeight: 1.7}
	settle(b, w, 3)
	if math.Abs(b.Y-10) > 0.01 || !b.OnGround {
		t.Fatalf("should land on the ground: y=%v", b.Y)
	}
	if math.Abs(b.Eye()-11.7) > 0.01 {
		t.Fatal("eye height")
	}
}

func TestWalksAndStopsAtWall(t *testing.T) {
	w := flat{groundY: 0, wallX: 5}
	b := NewBody(w, 0, 0)
	for i := 0; i < 600; i++ {
		b.Step(1.0/60, 6, 0, 0, false, w)
	}
	if b.X > 5-b.Radius+1e-6 {
		t.Fatalf("walked through the wall: x=%v", b.X)
	}
	if b.X < 3 {
		t.Fatalf("should reach the wall, x=%v", b.X)
	}
}

func TestJump(t *testing.T) {
	w := flat{}
	b := NewBody(w, 0, 0)
	b.Step(1.0/60, 0, 0, 0, true, w)
	peak := 0.0
	for i := 0; i < 90; i++ {
		b.Step(1.0/60, 0, 0, 0, false, w)
		peak = math.Max(peak, b.Y)
	}
	if peak < 0.8 || peak > 1.5 {
		t.Fatalf("jump peak %v", peak)
	}
	if !b.OnGround {
		t.Fatal("should land again")
	}
}

func TestSwimming(t *testing.T) {
	w := flat{groundY: -5, water: 0, hasWat: true}
	b := &Body{X: 0, Y: 0, Radius: 0.4, EyeHeight: 1.7}
	settle(b, w, 2)
	if !b.Swimming {
		t.Fatal("deep water should cause swimming")
	}
	if b.Y > 0 || b.Y < -5 {
		t.Fatalf("swimmer out of bounds y=%v", b.Y)
	}
	for i := 0; i < 120; i++ {
		b.Step(1.0/60, 0, 0, -1, false, w)
	}
	if !b.Underwater || b.Y > -1 {
		t.Fatalf("should dive underwater: y=%v underwater=%v", b.Y, b.Underwater)
	}
	if b.Y < -4.95 {
		t.Fatalf("must not sink through the bed: %v", b.Y)
	}
}

func TestWadingSlowsDown(t *testing.T) {
	dry, wet := flat{}, flat{water: 0.7, hasWat: true}
	a, b := NewBody(dry, 0, 0), NewBody(wet, 0, 0)
	for i := 0; i < 120; i++ {
		a.Step(1.0/60, 5, 0, 0, false, dry)
		b.Step(1.0/60, 5, 0, 0, false, wet)
	}
	if b.X >= a.X {
		t.Fatalf("wading should be slower: %v vs %v", b.X, a.X)
	}
}

func TestClimbsGentleSlopes(t *testing.T) {
	w := flat{ramp: true}
	b := NewBody(w, -2, 0)
	for i := 0; i < 400; i++ {
		b.Step(1.0/60, 4, 0, 0, false, w)
	}
	if b.X < 10 || b.Y < 4 {
		t.Fatalf("should climb the ramp: x=%v y=%v", b.X, b.Y)
	}
}
