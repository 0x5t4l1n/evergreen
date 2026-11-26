package weather

import (
	"math"
	"testing"
)

func run(w *Weather, seconds float64) {
	for t := 0.0; t < seconds; t += 0.1 {
		w.Update(0.1)
	}
}

func TestForcedWeatherConverges(t *testing.T) {
	w := New(1)
	w.Force(HeavyRain, true)
	run(w, 40)
	want := target(HeavyRain)
	if math.Abs(w.Current.Rain-want.Rain) > 0.02 || math.Abs(w.Current.Cloud-want.Cloud) > 0.02 {
		t.Fatalf("did not converge: %+v want %+v", w.Current, want)
	}
	if w.Current.Wetness < 0.8 {
		t.Fatalf("ground should be wet after heavy rain: %v", w.Current.Wetness)
	}
}

func TestTransitionsAreSmooth(t *testing.T) {
	w := New(2)
	w.Force(Storm, true)
	prev := w.Current
	for i := 0; i < 600; i++ {
		w.Update(0.1)
		if math.Abs(w.Current.Rain-prev.Rain) > 0.05 || math.Abs(w.Current.Fog-prev.Fog) > 0.05 {
			t.Fatal("weather values jumped within one 0.1s step")
		}
		prev = w.Current
	}
}

func TestStormProducesLightningAndThunder(t *testing.T) {
	w := New(3)
	w.Force(Storm, true)
	bolts, flashed := 0, false
	for i := 0; i < 3000; i++ {
		w.Update(0.1)
		if w.Bolt {
			bolts++
			if w.BoltDist <= 0 || w.BoltDist > 1 {
				t.Fatal("bolt distance out of range")
			}
		}
		if w.Flash() > 0.2 {
			flashed = true
		}
	}
	if bolts < 10 || !flashed {
		t.Fatalf("storm should strike repeatedly: bolts=%d flashed=%v", bolts, flashed)
	}
	c := New(3)
	c.Force(Clear, true)
	run(c, 300)
	if c.Bolt || c.Flash() != 0 {
		t.Fatal("clear weather has no lightning")
	}
}

func TestAutoWeatherChangesDeterministically(t *testing.T) {
	seq := func(seed uint64) []Kind {
		w := New(seed)
		var out []Kind
		last := w.Kind
		for i := 0; i < 36000; i++ {
			w.Update(0.1)
			if w.Kind != last {
				out = append(out, w.Kind)
				last = w.Kind
			}
		}
		return out
	}
	a, b := seq(5), seq(5)
	if len(a) < 5 {
		t.Fatalf("weather should evolve, only %d changes in an hour", len(a))
	}
	if len(a) != len(b) {
		t.Fatal("non-deterministic")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("non-deterministic sequence")
		}
	}
}

func TestSeasonBiasShiftsRain(t *testing.T) {
	count := func(bias RainBias) int {
		n := 0
		for seed := uint64(0); seed < 200; seed++ {
			w := New(seed)
			w.Season = bias
			if k := w.pick(); k == LightRain || k == HeavyRain || k == Storm {
				n++
			}
		}
		return n
	}
	if count(2) <= count(0.3) {
		t.Fatal("higher rain bias must yield more rainy picks")
	}
}

func TestParseAndVisibility(t *testing.T) {
	if k, ok := Parse("densefog"); !ok || k != DenseFog {
		t.Fatal("parse")
	}
	if _, ok := Parse("auto"); ok {
		t.Fatal("auto is not a kind")
	}
	w := New(1)
	w.Force(DenseFog, true)
	run(w, 40)
	if w.Visibility() < 4 {
		t.Fatalf("dense fog must cut visibility: %v", w.Visibility())
	}
}

func TestWindWithinBounds(t *testing.T) {
	w := New(1)
	w.Force(Storm, true)
	for i := 0; i < 1000; i++ {
		w.Update(0.1)
		if s := w.WindSpeed(); s < 0 || s > 1.4 || math.IsNaN(s) {
			t.Fatalf("wind out of range %v", s)
		}
	}
}
