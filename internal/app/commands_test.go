package app

import (
	"encoding/json"
	"math"
	"testing"

	"evergreen/internal/engine"
	"evergreen/internal/zones"
)

func TestApplyCommandUpdatesConfig(t *testing.T) {
	cfg := engine.DefaultConfig()
	cases := []struct {
		name, value string
		check       func() bool
		eff         Effect
	}{
		{"weather", "storm", func() bool { return cfg.Weather == "storm" }, EffWeather},
		{"season", "winter", func() bool { return cfg.Season == "winter" }, EffSeason | EffWorld},
		{"cameraMode", "cinematic", func() bool { return cfg.CameraMode == "cinematic" }, EffCamera},
		{"preset", "ultra", func() bool { return cfg.Preset == "ultra" }, EffQuality | EffWorld},
		{"volume", "0.25", func() bool { return cfg.Volume == 0.25 }, EffAudio},
		{"muted", "true", func() bool { return cfg.Muted }, EffAudio},
		{"showFps", "true", func() bool { return cfg.ShowFPS }, EffHUD},
		{"seed", "my forest", func() bool { return cfg.Seed == "my forest" }, EffWorld},
		{"github", "octocat", func() bool { return cfg.Github == "octocat" }, EffGithub | EffWorld},
		{"timeSpeed", "30", func() bool { return cfg.TimeSpeed == 30 }, EffTime},
	}
	for _, c := range cases {
		eff, err := ApplyCommand(&cfg, c.name, c.value)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !c.check() || eff&c.eff != c.eff {
			t.Fatalf("%s=%s: cfg=%+v eff=%v", c.name, c.value, cfg, eff)
		}
	}
}

func TestTimeOfDayPinsManualMode(t *testing.T) {
	cfg := engine.DefaultConfig()
	if _, err := ApplyCommand(&cfg, "timeOfDay", "21.5"); err != nil || cfg.TimeMode != "manual" || cfg.TimeOfDay != 21.5 {
		t.Fatalf("%+v %v", cfg, err)
	}
}

func TestApplyCommandRejectsBadInputWithoutChange(t *testing.T) {
	cfg := engine.DefaultConfig()
	before := cfg
	for _, c := range [][2]string{
		{"weather", "hurricane"}, {"season", "monsoon"}, {"cameraMode", "fly"}, {"volume", "loud"}, {"volume", "5"},
		{"timeOfDay", "99"}, {"github", "no spaces allowed"}, {"nonsense", "1"}, {"preset", "extreme"}, {"drawDistance", "100"},
	} {
		if _, err := ApplyCommand(&cfg, c[0], c[1]); err == nil {
			t.Errorf("%s=%s should fail", c[0], c[1])
		}
		if cfg != before {
			t.Fatalf("%s=%s mutated the config", c[0], c[1])
		}
	}
}

func TestSavedRoundTripAndRepair(t *testing.T) {
	s := Saved{Config: engine.DefaultConfig(), X: 123.5, Z: -40, Yaw: 1.2, Discovered: []int{1, 3}, Distance: 900}
	s.Config.Seed = "abc"
	got, errs := ParseSaved(s.Marshal())
	if len(errs) != 0 || got.X != 123.5 || got.Config.Seed != "abc" || len(got.Discovered) != 2 || got.Distance != 900 {
		t.Fatalf("round trip: %+v %v", got, errs)
	}
	bad, errs := ParseSaved([]byte(`{"config":{"preset":"nope","volume":9},"x":1e300,"discovered":[1,1,42,-3,9],"distance":-5}`))
	if len(errs) == 0 {
		t.Fatal("repairs must be reported")
	}
	if bad.Config.Preset != "medium" || bad.Config.Volume != 1 || bad.X != 0 || bad.Distance != 0 {
		t.Fatalf("repair failed: %+v", bad)
	}
	if len(bad.Discovered) != 2 || bad.Discovered[0] != 1 || bad.Discovered[1] != 9 {
		t.Fatalf("discovered not sanitised: %v", bad.Discovered)
	}
	if def, errs := ParseSaved([]byte(`{{{`)); len(errs) == 0 || def.Config.Seed == "" {
		t.Fatal("malformed JSON falls back to defaults")
	}
	if def, errs := ParseSaved(nil); errs != nil || def.Config.Seed == "" {
		t.Fatal("empty input is fine")
	}
}

func TestDiscover(t *testing.T) {
	var s Saved
	if !s.Discover(4) || s.Discover(4) || len(s.Discovered) != 1 {
		t.Fatal("discover should be idempotent")
	}
}

func TestHUDJSONAndClock(t *testing.T) {
	var out map[string]any
	if err := json.Unmarshal([]byte(HUD{FPS: 60, Zone: "Emerald Falls", Prompt: "Press E"}.JSON()), &out); err != nil {
		t.Fatal(err)
	}
	if out["fps"].(float64) != 60 || out["zone"] != "Emerald Falls" || out["prompt"] != "Press E" {
		t.Fatalf("%v", out)
	}
	for in, want := range map[float64]string{0: "00:00", 9.5: "09:30", 23.99: "23:59", 24.5: "00:30", -1: "23:00"} {
		if got := ClockString(in); got != want {
			t.Errorf("ClockString(%v)=%s want %s", in, got, want)
		}
	}
}

func TestArrivalPointsAreInsideZones(t *testing.T) {
	a := zones.NewAtlas(5, func(x, z float64) float64 { return 0 })
	for _, z := range a.Realm(0, 0) {
		x, zz, yaw := ArrivalPoint(z)
		if d := math.Hypot(x-z.X, zz-z.Z); d > z.Radius {
			t.Errorf("%s: arrival %.0fm from centre, outside radius %.0f", z.Key, d, z.Radius)
		}
		if math.IsNaN(yaw) {
			t.Errorf("%s: yaw NaN", z.Key)
		}
	}
}
