package particles

import (
	"testing"

	"evergreen/internal/engine"
)

func TestPlanRainAndSnow(t *testing.T) {
	q := engine.QualityFor("high")
	rainy := PlanFor(q, PlanInput{Rain: 1, Day: 1, Season: 1})
	if rainy.Rain == 0 || rainy.Snow != 0 {
		t.Fatalf("summer rain: %+v", rainy)
	}
	snowy := PlanFor(q, PlanInput{Rain: 1, Day: 1, Season: 3})
	if snowy.Snow == 0 || snowy.Rain != 0 {
		t.Fatalf("winter precipitation falls as snow: %+v", snowy)
	}
	clear := PlanFor(q, PlanInput{Day: 1, Season: 1})
	if clear.Rain != 0 || clear.Pollen == 0 || clear.Dust == 0 {
		t.Fatalf("clear day: %+v", clear)
	}
}

func TestPlanFirefliesAtNightAndStars(t *testing.T) {
	q := engine.QualityFor("high")
	night := PlanFor(q, PlanInput{Day: 0, Season: 1})
	day := PlanFor(q, PlanInput{Day: 1, Season: 1})
	if night.Fireflies <= day.Fireflies {
		t.Fatal("fireflies come out at night")
	}
	boosted := PlanFor(q, PlanInput{Day: 0, Season: 1, StarsBoost: 1})
	if boosted.Fireflies <= night.Fireflies {
		t.Fatal("GitHub stars must add fireflies")
	}
}

func TestPlanScalesWithQualityAndReducedMotion(t *testing.T) {
	in := PlanInput{Rain: 0.8, Day: 0.5, Season: 2, Wind: 0.5, LeafFall: 1}
	low := PlanFor(engine.QualityFor("low"), in).Total()
	ultra := PlanFor(engine.QualityFor("ultra"), in).Total()
	if low >= ultra {
		t.Fatalf("low=%d ultra=%d", low, ultra)
	}
	in.ReducedMotion = true
	rm := PlanFor(engine.QualityFor("ultra"), in)
	if rm.Leaves != 0 || rm.Total() >= ultra {
		t.Fatalf("reduced motion must calm particles: %+v", rm)
	}
}

func TestEmitUpdateAndExpiry(t *testing.T) {
	s := New(500, 1)
	src := []Source{{Kind: Mist, X: 0, Y: 0, Z: 0, Spread: 3, Rate: 100}}
	for i := 0; i < 60; i++ {
		s.Update(1.0/30, 0.5, 0, src, 1)
	}
	if s.Count() < 100 {
		t.Fatalf("mist should accumulate, got %d", s.Count())
	}
	for i := 0; i < 600; i++ {
		s.Update(1.0/30, 0, 0, nil, 1)
	}
	if s.Count() != 0 {
		t.Fatalf("particles must expire, %d left", s.Count())
	}
}

func TestCapacityRespected(t *testing.T) {
	s := New(50, 2)
	for i := 0; i < 1000; i++ {
		s.Emit(Glow, 0, 0, 0, 1)
	}
	if s.Count() != 50 {
		t.Fatalf("count %d exceeds capacity 50", s.Count())
	}
	buf := make([]float32, 8*30)
	if n := s.Fill(buf); n != 30 {
		t.Fatalf("fill should clamp to dst: %d", n)
	}
	s.Clear()
	if s.Count() != 0 {
		t.Fatal("clear")
	}
}

func TestSplashFallsUnderGravity(t *testing.T) {
	s := New(10, 3)
	s.Emit(Splash, 0, 0, 0, 0.1)
	vy0 := s.p[0].VY
	s.Update(0.2, 0, 0, nil, 1)
	if s.p[0].VY >= vy0 {
		t.Fatal("splash should decelerate")
	}
}

func TestFillOutputInRange(t *testing.T) {
	s := New(200, 4)
	for _, k := range []Kind{Mist, Splash, Glow, Steam, Pollen} {
		for i := 0; i < 20; i++ {
			s.Emit(k, 1, 2, 3, 1)
		}
	}
	s.Update(0.5, 0, 0, nil, 1)
	buf := make([]float32, 8*200)
	n := s.Fill(buf)
	if n != 100 {
		t.Fatalf("n=%d", n)
	}
	for i := 0; i < n; i++ {
		a := buf[i*8+7]
		if a < 0 || a > 1 || buf[i*8+3] <= 0 {
			t.Fatalf("bad particle %d: alpha=%v size=%v", i, a, buf[i*8+3])
		}
	}
}

func TestKindFromName(t *testing.T) {
	for _, n := range []string{"mist", "splash", "glow", "spark", "steam", "pollen"} {
		if _, ok := KindFromName(n); !ok {
			t.Errorf("%s", n)
		}
	}
	if _, ok := KindFromName("nope"); ok {
		t.Fatal("unknown")
	}
}
