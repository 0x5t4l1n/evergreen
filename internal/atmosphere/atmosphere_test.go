package atmosphere

import (
	"math"
	"testing"
	"time"
)

func TestClockAdvanceWraps(t *testing.T) {
	c := Clock{Hours: 23.5, Speed: 60}
	c.Advance(60, time.Time{}) // one real minute at 60 h/min = 60 hours
	if c.Hours < 0 || c.Hours >= 24 {
		t.Fatalf("hours out of range: %v", c.Hours)
	}
	c = Clock{Hours: 10, Speed: 1}
	c.Advance(60, time.Time{})
	if math.Abs(c.Hours-11) > 1e-9 {
		t.Fatalf("one minute at 1h/min should add one hour, got %v", c.Hours)
	}
	c.Paused = true
	c.Advance(600, time.Time{})
	if math.Abs(c.Hours-11) > 1e-9 {
		t.Fatal("paused clock must not move")
	}
	c = Clock{SyncLocal: true}
	c.Advance(1, time.Date(2026, 1, 1, 14, 30, 0, 0, time.UTC))
	if math.Abs(c.Hours-14.5) > 1e-6 {
		t.Fatalf("local sync wrong: %v", c.Hours)
	}
}

func TestDayNightTransition(t *testing.T) {
	noon := Compute(12, 0, Summer)
	night := Compute(0, 0, Summer)
	if noon.Day < 0.99 || noon.SunDir[1] < 0.5 || noon.SunIntensity < 0.9 {
		t.Fatalf("noon should be bright: %+v", noon)
	}
	if night.Day > 0.01 || night.StarVis < 0.9 {
		t.Fatalf("midnight should be dark with stars: %+v", night)
	}
	if noon.SkyTop[2] <= night.SkyTop[2] {
		t.Fatal("daytime sky must be brighter")
	}
}

func TestTransitionIsSmooth(t *testing.T) {
	prev := Compute(0, 0, Summer)
	for h := 0.0; h <= 24; h += 0.05 {
		s := Compute(h, 0, Summer)
		for i := 0; i < 3; i++ {
			if math.Abs(s.SkyTop[i]-prev.SkyTop[i]) > 0.06 || math.Abs(s.SkyHorizon[i]-prev.SkyHorizon[i]) > 0.06 {
				t.Fatalf("sky colour jumps at %.2fh", h)
			}
		}
		if math.Abs(s.Day-prev.Day) > 0.08 {
			t.Fatalf("day factor jumps at %.2fh", h)
		}
		prev = s
	}
}

func TestSunriseAndSunsetHorizonColour(t *testing.T) {
	sr := Compute(6.2, 0, Summer)
	noon := Compute(12, 0, Summer)
	if sr.SkyHorizon[0]-sr.SkyHorizon[2] < noon.SkyHorizon[0]-noon.SkyHorizon[2] {
		t.Fatal("sunrise horizon should be warmer than noon")
	}
}

func TestSunDirIsUnit(t *testing.T) {
	for h := 0.0; h < 24; h += 0.5 {
		s := Compute(h, 3, Autumn)
		l := math.Sqrt(s.SunDir[0]*s.SunDir[0] + s.SunDir[1]*s.SunDir[1] + s.SunDir[2]*s.SunDir[2])
		if math.Abs(l-1) > 1e-6 {
			t.Fatalf("sun dir not unit at %v: %v", h, l)
		}
	}
}

func TestPhases(t *testing.T) {
	want := map[float64]string{6: "sunrise", 9: "morning", 14: "afternoon", 18: "sunset", 21: "evening", 2: "midnight"}
	for h, w := range want {
		if Phase(h) != w {
			t.Errorf("Phase(%v) = %s want %s", h, Phase(h), w)
		}
	}
}

func TestSeasons(t *testing.T) {
	if ParseSeason("autumn", time.Time{}) != Autumn {
		t.Fatal("explicit season")
	}
	if ParseSeason("auto", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) != Summer || ParseSeason("auto", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) != Winter {
		t.Fatal("auto season from date")
	}
	if LookFor(Winter).Snow != 1 || LookFor(Autumn).Autumn != 1 || LookFor(Summer).Snow != 0 {
		t.Fatal("season looks wrong")
	}
	mid := BlendLook(LookFor(Summer), LookFor(Winter), 0.5)
	if math.Abs(mid.Snow-0.5) > 1e-9 {
		t.Fatal("blend")
	}
}

func TestMoonPhaseCycles(t *testing.T) {
	a := Compute(0, 0, Summer).MoonPhase
	b := Compute(0, 29.53, Summer).MoonPhase
	if math.Abs(a-b) > 0.01 {
		t.Fatalf("moon should repeat every 29.53 days: %v vs %v", a, b)
	}
	if Compute(0, 7, Summer).MoonPhase == a {
		t.Fatal("moon phase should change")
	}
}
