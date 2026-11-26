package water

import (
	"math"
	"testing"
)

func TestRipplePropagatesAndDecays(t *testing.T) {
	r := NewRipples(64, 0.5, 1)
	r.Recenter(0, 0)
	r.Drop(0, 0, 0.8, 1)
	start := r.Energy()
	if start == 0 {
		t.Fatal("drop must add energy")
	}
	var peakFar float64
	for i := 0; i < 240; i++ {
		r.Step(1.0/60, 0)
		peakFar = math.Max(peakFar, math.Abs(r.Height(6, 0)))
	}
	if peakFar < 1e-4 {
		t.Fatal("ripple should reach distant cells")
	}
	for i := 0; i < 3000; i++ {
		r.Step(1.0/60, 0)
	}
	if r.Energy() > start*0.2 {
		t.Fatalf("ripples should decay: %v -> %v", start, r.Energy())
	}
}

func TestRainAddsEnergy(t *testing.T) {
	r := NewRipples(64, 0.5, 2)
	r.Recenter(0, 0)
	for i := 0; i < 120; i++ {
		r.Step(1.0/60, 1)
	}
	if r.Energy() < 1 {
		t.Fatalf("rain should disturb the surface: %v", r.Energy())
	}
	dry := NewRipples(64, 0.5, 2)
	dry.Recenter(0, 0)
	for i := 0; i < 120; i++ {
		dry.Step(1.0/60, 0)
	}
	if dry.Energy() != 0 {
		t.Fatal("no rain, no ripples")
	}
}

func TestRecenterPreservesWorldPositions(t *testing.T) {
	r := NewRipples(64, 1, 3)
	r.Recenter(0, 0)
	r.Drop(5, 5, 0.5, 1)
	for i := 0; i < 10; i++ {
		r.Step(1.0/60, 0)
	}
	before := r.Height(5, 5)
	r.Recenter(8, 0)
	if math.Abs(r.Height(5, 5)-before) > 1e-6 {
		t.Fatalf("recentre moved ripples: %v vs %v", r.Height(5, 5), before)
	}
	r.Recenter(5000, 5000)
	if r.Energy() != 0 {
		t.Fatal("teleport should clear the field")
	}
}

func TestHalfFloatRoundTrip(t *testing.T) {
	for _, f := range []float32{0, 0.5, -0.5, 1, -1.25, 0.1, 0.375, 100, -0.0625} {
		got := HalfToFloat32(Float32ToHalf(f))
		if math.Abs(float64(got-f)) > math.Abs(float64(f))*0.002+1e-6 {
			t.Errorf("half(%v) = %v", f, got)
		}
	}
	if Float32ToHalf(1e9) != 0x7c00 {
		t.Fatal("overflow should map to +inf")
	}
	if len(NewRipples(8, 1, 1).Half()) != 64 {
		t.Fatal("half buffer size")
	}
}

func TestStableUnderStress(t *testing.T) {
	r := NewRipples(48, 0.5, 4)
	r.Recenter(0, 0)
	for i := 0; i < 600; i++ {
		r.Drop(float64(i%20)-10, float64(i%13)-6, 1, 1.5)
		r.Step(1.0/60, 1)
	}
	for _, h := range r.h {
		if math.IsNaN(float64(h)) || math.Abs(float64(h)) > 1.6 {
			t.Fatal("simulation became unstable")
		}
	}
}
