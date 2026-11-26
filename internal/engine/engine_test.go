package engine

import (
	"math"
	"testing"
)

func TestConfigValidateRepairs(t *testing.T) {
	c := Config{Seed: "", Preset: "ULTRA", TimeMode: "bogus", TimeOfDay: 99, Volume: -3, Github: "bad--name", Weather: "storm", Season: "autumn", CameraMode: "explore", TimeSpeed: 1}
	errs := c.Validate()
	if len(errs) < 4 {
		t.Fatalf("expected several repairs, got %v", errs)
	}
	if c.Preset != "ultra" || c.TimeMode != "cycle" || c.TimeOfDay != 24 || c.Volume != 0 || c.Github != "" || c.Seed == "" {
		t.Fatalf("unexpected repaired config %+v", c)
	}
}

func TestParseConfig(t *testing.T) {
	c, errs := ParseConfig([]byte(`{"seed":"abc","preset":"high","volume":0.2}`))
	if len(errs) != 0 || c.Seed != "abc" || c.Preset != "high" || c.Volume != 0.2 || c.Weather != "auto" {
		t.Fatalf("parse: %+v %v", c, errs)
	}
	if _, errs = ParseConfig([]byte(`{not json`)); len(errs) == 0 {
		t.Fatal("malformed JSON must report an error")
	}
	c2, _ := ParseConfig(c.JSON())
	if c2 != c {
		t.Fatal("JSON round trip mismatch")
	}
}

func TestGithubUser(t *testing.T) {
	for _, ok := range []string{"octocat", "a", "a-b-c", "User123"} {
		if !ValidGithubUser(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "-a", "a-", "a--b", "a b", "a/b", "../x", "this-is-a-very-long-username-that-exceeds-the-limit"} {
		if ValidGithubUser(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}

func TestMatrixAndFrustum(t *testing.T) {
	proj := Perspective(math.Pi/3, 1.5, 0.1, 500)
	view := LookAt(Vec3{0, 5, 0}, Vec3{0, 5, -10}, Vec3{0, 1, 0})
	f := NewFrustum(proj.Mul(view))
	if !f.IntersectsAABB(Vec3{-1, 4, -21}, Vec3{1, 6, -19}) {
		t.Fatal("box in front must be visible")
	}
	if f.IntersectsAABB(Vec3{-1, 4, 19}, Vec3{1, 6, 21}) {
		t.Fatal("box behind camera must be culled")
	}
	if f.IntersectsAABB(Vec3{900, 4, -21}, Vec3{902, 6, -19}) {
		t.Fatal("box far to the side must be culled")
	}
}

func TestAdaptiveQuality(t *testing.T) {
	a := NewAdaptive()
	for i := 0; i < 400; i++ {
		a.Observe(45)
	}
	if a.Scale >= 1 && a.Tier == 0 {
		t.Fatal("slow frames must reduce quality")
	}
	if a.Effective(QualityFor("ultra")).RenderScale >= 1 {
		t.Fatal("effective scale should be reduced")
	}
	for i := 0; i < 20000; i++ {
		a.Observe(8)
	}
	if a.Scale != 1 || a.Tier != 0 {
		t.Fatalf("fast frames must restore quality, got scale=%v tier=%v", a.Scale, a.Tier)
	}
	a.Enabled = false
	if a.Observe(100) {
		t.Fatal("disabled controller must not change")
	}
}

func TestPoolRecycles(t *testing.T) {
	p := NewPool(func() *[4]int { return new([4]int) }, func(v *[4]int) { *v = [4]int{} })
	a := p.Get()
	a[0] = 5
	p.Put(a)
	if p.Live() != 0 {
		t.Fatalf("live=%d", p.Live())
	}
	b := p.Get()
	if b != a || b[0] != 0 {
		t.Fatal("pool should return the reset recycled value")
	}
}
