package procedural

import (
	"math"
	"testing"
)

func TestNoiseDeterministicAndBounded(t *testing.T) {
	for i := 0; i < 2000; i++ {
		x, y := float64(i)*0.37-300, float64(i)*0.91+17
		a, b := Noise2(7, x, y), Noise2(7, x, y)
		if a != b {
			t.Fatalf("noise not deterministic at %v,%v", x, y)
		}
		if a < -1.2 || a > 1.2 {
			t.Fatalf("noise out of range: %v", a)
		}
	}
	if Noise2(1, 3.3, 4.4) == Noise2(2, 3.3, 4.4) {
		t.Fatal("different seeds should differ")
	}
}

func TestNoiseContinuity(t *testing.T) {
	for i := 0; i < 500; i++ {
		x := float64(i) * 0.173
		if d := math.Abs(Noise2(3, x, 5.5) - Noise2(3, x+0.001, 5.5)); d > 0.05 {
			t.Fatalf("discontinuity %v at %v", d, x)
		}
	}
}

func TestRidgedRange(t *testing.T) {
	for i := 0; i < 500; i++ {
		v := Ridged(9, float64(i)*0.31, float64(i)*0.17, 4)
		if v < 0 || v > 1.0001 {
			t.Fatalf("ridged out of range %v", v)
		}
	}
}

func TestRNGDeterminism(t *testing.T) {
	a, b := NewRNG(42), NewRNG(42)
	for i := 0; i < 100; i++ {
		if a.Next() != b.Next() {
			t.Fatal("rng diverged")
		}
	}
	r := NewRNG(1)
	for i := 0; i < 1000; i++ {
		if f := r.Float(); f < 0 || f >= 1 {
			t.Fatalf("float out of range %v", f)
		}
		if n := r.Intn(5); n < 0 || n > 4 {
			t.Fatalf("intn out of range %d", n)
		}
	}
}

func TestSeedFromString(t *testing.T) {
	if SeedFromString("42") != 42 {
		t.Fatal("numeric seed should map to itself")
	}
	if SeedFromString("evergreen") != SeedFromString("evergreen") || SeedFromString("evergreen") == SeedFromString("evergreeN") {
		t.Fatal("text seed hashing broken")
	}
	if SeedFromString("  ") != 0 {
		t.Fatal("blank seed should be zero")
	}
}

func TestSmoothStep(t *testing.T) {
	if SmoothStep(0, 1, -1) != 0 || SmoothStep(0, 1, 2) != 1 || math.Abs(SmoothStep(0, 1, 0.5)-0.5) > 1e-9 {
		t.Fatal("smoothstep wrong")
	}
}
