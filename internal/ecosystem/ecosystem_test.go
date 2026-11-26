package ecosystem

import (
	"testing"

	"evergreen/internal/engine"
	"evergreen/internal/vegetation"
	"evergreen/internal/wildlife"
)

type env struct{ lakeX float64 }

func (env) Ground(x, z float64) float64 { return 1 }
func (e env) Water(x, z float64) (float64, float64, bool) {
	if x > e.lakeX {
		return 2, 2.5, true
	}
	return 0, 0, false
}
func (env) Biome(x, z float64) vegetation.Biome { return vegetation.Meadow }

func settings(day, rain float64) Settings {
	return Settings{Cap: 90, Day: day, Cond: wildlife.Conditions{Player: engine.Vec3{X: 0, Y: 2, Z: 0}, Rain: rain, Night: 1 - day}}
}

func simulate(m *Manager, e Env, s Settings, secs float64) {
	for t := 0.0; t < secs; t += 0.1 {
		m.Update(0.1, e, s)
	}
}

func TestPopulatesInDaylight(t *testing.T) {
	m := New(1)
	simulate(m, env{lakeX: 20}, settings(1, 0), 60)
	for _, sp := range []wildlife.Species{wildlife.Bird, wildlife.Butterfly, wildlife.Deer, wildlife.Rabbit, wildlife.Fish, wildlife.Dragonfly} {
		if m.Count(sp) == 0 {
			t.Errorf("no %s spawned by day", sp)
		}
	}
	if m.Total() > 90+16 {
		t.Fatalf("population exceeds cap: %d", m.Total())
	}
}

func TestNightAndRainChangePopulation(t *testing.T) {
	m := New(2)
	simulate(m, env{lakeX: 20}, settings(1, 0), 60)
	if m.Count(wildlife.Butterfly) == 0 {
		t.Fatal("butterflies expected by day")
	}
	simulate(m, env{lakeX: 20}, settings(0, 0), 60)
	if m.Count(wildlife.Butterfly) != 0 || m.Count(wildlife.Bird) != 0 {
		t.Fatalf("day animals should leave at night: butterflies=%d birds=%d", m.Count(wildlife.Butterfly), m.Count(wildlife.Bird))
	}
	if m.Count(wildlife.Frog) == 0 {
		t.Fatal("frogs should be out at night")
	}
	m2 := New(2)
	simulate(m2, env{lakeX: 20}, settings(1, 1), 60)
	if m2.Count(wildlife.Butterfly)+m2.Count(wildlife.Dragonfly) != 0 || m2.Count(wildlife.Bird) != 0 {
		t.Fatal("insects and birds shelter in heavy rain")
	}
}

func TestNoWaterNoFish(t *testing.T) {
	m := New(3)
	simulate(m, env{lakeX: 1e9}, settings(1, 0), 40)
	if m.Count(wildlife.Fish)+m.Count(wildlife.Dragonfly)+m.Count(wildlife.Frog) != 0 {
		t.Fatal("aquatic species need water")
	}
}

func TestDeterministicPopulation(t *testing.T) {
	a, b := New(7), New(7)
	simulate(a, env{lakeX: 20}, settings(1, 0), 30)
	simulate(b, env{lakeX: 20}, settings(1, 0), 30)
	for sp := wildlife.Species(0); sp < wildlife.NumSpecies; sp++ {
		ia, ib := a.Instances(sp), b.Instances(sp)
		if len(ia) != len(ib) {
			t.Fatalf("%s: counts differ", sp)
		}
		for i := range ia {
			if ia[i] != ib[i] {
				t.Fatalf("%s: instance %d differs", sp, i)
			}
		}
	}
}

func TestPoolDoesNotLeak(t *testing.T) {
	m := New(4)
	e := env{lakeX: 20}
	simulate(m, e, settings(1, 0), 60)
	simulate(m, e, settings(0, 1), 120)
	// everything alive must equal what is checked out of the pool
	if m.Live() != m.Total() {
		t.Fatalf("pool leak: live=%d total=%d", m.Live(), m.Total())
	}
	m.Clear()
	if m.Live() != 0 || m.Total() != 0 {
		t.Fatalf("clear should release everything: live=%d", m.Live())
	}
}

func TestDistantAnimalsDespawn(t *testing.T) {
	m := New(5)
	e := env{lakeX: 1e9}
	simulate(m, e, settings(1, 0), 30)
	before := m.Total()
	if before == 0 {
		t.Fatal("expected animals")
	}
	far := settings(1, 0)
	far.Cond.Player = engine.Vec3{X: 5000, Z: 5000}
	m.Update(0.1, e, far)
	simulate(m, e, far, 5)
	// original animals are all > despawn radius from the new player position, so
	// the population around the new position must consist of fresh spawns only
	for sp := wildlife.Species(0); sp < wildlife.NumSpecies; sp++ {
		for i, v := range m.Instances(sp) {
			if i%8 == 0 && v < 4000 {
				t.Fatalf("%s: stale animal left behind at x=%v", sp, v)
			}
		}
	}
}

func TestInstancesPackEightFloats(t *testing.T) {
	m := New(6)
	simulate(m, env{lakeX: 20}, settings(1, 0), 30)
	total := 0
	for sp := wildlife.Species(0); sp < wildlife.NumSpecies; sp++ {
		if len(m.Instances(sp))%8 != 0 {
			t.Fatalf("%s: instance data not multiple of 8", sp)
		}
		total += len(m.Instances(sp)) / 8
	}
	if total != m.Total() {
		t.Fatalf("instances %d != entities %d", total, m.Total())
	}
}
