package wildlife

import (
	"math"
	"testing"
)

type testEnv struct{ lakeX float64 }

func (testEnv) Ground(x, z float64) float64 { return 2 }
func (e testEnv) Water(x, z float64) (float64, float64, bool) {
	if x > e.lakeX {
		return 3, 2, true
	}
	return 0, 0, false
}

func TestMeshesAreValid(t *testing.T) {
	for s := Species(0); s < NumSpecies; s++ {
		m := Mesh(s)
		if m.VertexCount() == 0 || len(m.I)%3 != 0 || len(m.I) == 0 {
			t.Fatalf("%s: bad mesh", s)
		}
		for _, ix := range m.I {
			if int(ix) >= m.VertexCount() {
				t.Fatalf("%s: index out of range", s)
			}
		}
		for _, f := range m.V {
			if math.IsNaN(float64(f)) {
				t.Fatalf("%s: NaN", s)
			}
		}
		if Mesh(s) != m {
			t.Fatalf("%s: mesh not cached", s)
		}
	}
	lo, hi := Mesh(Deer).Bounds()
	if hi.Y-lo.Y < 1.8 || hi.X-lo.X < 1.2 {
		t.Fatal("deer should be deer-sized")
	}
	lo, hi = Mesh(Bird).Bounds()
	if hi.Z-lo.Z < 0.7 {
		t.Fatal("bird wingspan")
	}
}

func TestAnimatedVerticesCarryAnimationWeights(t *testing.T) {
	has := func(s Species, lo, hi float32) bool {
		m := Mesh(s)
		for i := 9; i < len(m.V); i += 13 {
			if m.V[i] >= lo && m.V[i] < hi {
				return true
			}
		}
		return false
	}
	if !has(Bird, -1.01, -0.1) || !has(Butterfly, -1.01, -0.1) {
		t.Fatal("wings must flap")
	}
	if !has(Deer, 2, 3) {
		t.Fatal("deer legs must swing")
	}
	if !has(Fish, 3, 4) {
		t.Fatal("fish tails must wag")
	}
}

func TestFormationFlying(t *testing.T) {
	f := &Flock{Center: Vec3{X: 0, Y: 40, Z: 0}, Heading: 0, Speed: 9, Height: 40}
	var birds []*Entity
	for i := 0; i < 7; i++ {
		b, s := formationSlot(i)
		e := &Entity{}
		e.Init(Bird, Vec3{X: b, Y: 40, Z: s}, uint64(i))
		e.Flock, e.Slot = f, i
		birds = append(birds, e)
	}
	env := testEnv{lakeX: 1e9}
	for i := 0; i < 600; i++ {
		f.Update(0.05, Vec3{X: 300, Z: 0}, float64(i)*0.05, 0)
		for _, e := range birds {
			e.Update(0.05, env, Conditions{Player: Vec3{X: 300}}, nil)
		}
	}
	if f.Center.X < 100 {
		t.Fatalf("flock should travel, centre x=%v", f.Center.X)
	}
	var maxSpread float64
	for _, e := range birds {
		maxSpread = math.Max(maxSpread, math.Hypot(e.Pos.X-f.Center.X, e.Pos.Z-f.Center.Z))
	}
	if maxSpread > 14 {
		t.Fatalf("birds should hold formation near the leader, spread=%v", maxSpread)
	}
	if birds[0].Anim < 5 {
		t.Fatal("flying birds must flap")
	}
}

func TestDeerFleeFromPlayer(t *testing.T) {
	d := &Entity{}
	d.Init(Deer, Vec3{X: 0, Y: 2, Z: 0}, 1)
	env := testEnv{lakeX: 1e9}
	cond := Conditions{Player: Vec3{X: -6, Z: 0}}
	for i := 0; i < 80; i++ {
		d.Update(0.05, env, cond, nil)
	}
	if d.State != Flee || d.Pos.X < 8 {
		t.Fatalf("deer should run away: state=%v x=%v", d.State, d.Pos.X)
	}
	calm := &Entity{}
	calm.Init(Deer, Vec3{X: 0, Z: 0}, 1)
	for i := 0; i < 40; i++ {
		calm.Update(0.05, env, Conditions{Player: Vec3{X: -80}}, nil)
	}
	if calm.State == Flee {
		t.Fatal("distant visitor must not scare deer")
	}
}

func TestDeerRestAtNightAndStorm(t *testing.T) {
	d := &Entity{}
	d.Init(Deer, Vec3{}, 2)
	d.State, d.Timer = Graze, 0.01
	env := testEnv{lakeX: 1e9}
	for i := 0; i < 20; i++ {
		d.Update(0.1, env, Conditions{Player: Vec3{X: 500}, Night: 1}, nil)
	}
	if d.State != Rest && d.State != Graze {
		t.Fatalf("deer should settle at night, state=%v", d.State)
	}
	d.Update(0.1, env, Conditions{Player: Vec3{X: 500}, Thunder: true}, nil)
	if d.State != Flee {
		t.Fatal("thunder should startle deer")
	}
}

func TestFishStayInWater(t *testing.T) {
	env := testEnv{lakeX: 0}
	f := &Entity{}
	f.Init(Fish, Vec3{X: 10, Y: 2, Z: 0}, 3)
	for i := 0; i < 4000; i++ {
		f.Update(0.05, env, Conditions{Player: Vec3{X: -500}}, nil)
		if f.Pos.X < -0.5 {
			t.Fatalf("fish left the water at step %d: x=%v", i, f.Pos.X)
		}
		if f.Pos.Y > 3.01 || f.Pos.Y < 1.0 {
			t.Fatalf("fish outside water column: y=%v", f.Pos.Y)
		}
	}
}

func TestRabbitHopsAndFlees(t *testing.T) {
	r := &Entity{}
	r.Init(Rabbit, Vec3{X: 0, Y: 2, Z: 0}, 4)
	env := testEnv{lakeX: 1e9}
	hopped := false
	for i := 0; i < 100; i++ {
		r.Update(0.05, env, Conditions{Player: Vec3{X: -5}}, nil)
		if r.Pos.Y > 2.05 {
			hopped = true
		}
	}
	if !hopped || r.Pos.X < 4 {
		t.Fatalf("rabbit should hop away: x=%v hopped=%v", r.Pos.X, hopped)
	}
}

func TestFrogCroaksAtNight(t *testing.T) {
	f := &Entity{}
	f.Init(Frog, Vec3{X: -3, Z: 0}, 5)
	var ev []Event
	for i := 0; i < 4000; i++ {
		f.Update(0.1, testEnv{lakeX: 1e9}, Conditions{Player: Vec3{X: 500}, Night: 1}, &ev)
	}
	if len(ev) == 0 {
		t.Fatal("frogs should croak at night")
	}
	var day []Event
	g := &Entity{}
	g.Init(Frog, Vec3{}, 5)
	for i := 0; i < 4000; i++ {
		g.Update(0.1, testEnv{lakeX: 1e9}, Conditions{Player: Vec3{X: 500}}, &day)
	}
	if len(day) != 0 {
		t.Fatal("frogs stay quiet on dry days")
	}
}

func TestAnimalsRespectWater(t *testing.T) {
	env := testEnv{lakeX: 0}
	d := &Entity{}
	d.Init(Deer, Vec3{X: -5, Y: 2, Z: 0}, 6)
	d.Yaw = 0
	for i := 0; i < 2000; i++ {
		d.Update(0.05, env, Conditions{Player: Vec3{X: -500}}, nil)
		if d.Pos.X > 0.5 {
			t.Fatalf("deer waded into deep water at step %d", i)
		}
	}
}

func TestInstanceFadeAndSize(t *testing.T) {
	e := &Entity{}
	e.Init(Butterfly, Vec3{X: 1, Y: 2, Z: 3}, 1)
	e.Fade = 0.5
	inst := e.Instance(nil)
	if len(inst) != 8 || inst[0] != 1 || inst[3] != float32(e.Scale)*0.5 {
		t.Fatalf("instance = %v", inst)
	}
	e.Leaving = true
	for i := 0; i < 100; i++ {
		e.Update(0.05, testEnv{lakeX: 1e9}, Conditions{Player: Vec3{X: 99}}, nil)
	}
	if e.Alive {
		t.Fatal("leaving entity should die after fading out")
	}
}

func TestInstanceEncodesAnimationForShader(t *testing.T) {
	e := &Entity{}
	e.Init(Bird, Vec3{}, 1)
	e.Anim, e.Gait = 12, 1.5
	inst := e.Instance(nil)
	if inst[6] > -1000 || inst[6] < -1000-2*3.1416 {
		t.Fatalf("animals must encode phase in birth < -500: %v", inst[6])
	}
	if got := -(float64(inst[6]) + 1000); got < 1.49 || got > 1.51 {
		t.Fatalf("phase round trip: %v", got)
	}
	if inst[7] < 0.99 || inst[7] > 1.01 {
		t.Fatalf("amplitude: %v", inst[7])
	}
	e.Update(0.5, testEnv{lakeX: 1e9}, Conditions{Player: Vec3{X: 1e5}}, nil)
	if e.Gait < 0 || e.Gait >= 2*3.15 {
		t.Fatalf("gait must stay wrapped: %v", e.Gait)
	}
}
