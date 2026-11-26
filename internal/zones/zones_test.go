package zones

import (
	"math"
	"testing"
)

func TestLayoutDeterministic(t *testing.T) {
	a, b := NewAtlas(42, nil), NewAtlas(42, nil)
	za, zb := a.Realm(0, 0), b.Realm(0, 0)
	if len(za) != NumZones {
		t.Fatalf("want %d zones, got %d", NumZones, len(za))
	}
	for i := range za {
		if za[i].X != zb[i].X || za[i].Z != zb[i].Z || za[i].Seed != zb[i].Seed {
			t.Fatalf("zone %d differs between identical seeds", i)
		}
	}
	c := NewAtlas(43, nil).Realm(0, 0)
	same := true
	for i := range za {
		if za[i].X != c[i].X {
			same = false
		}
	}
	if same {
		t.Fatal("different seeds should move zones")
	}
}

func TestZonesDoNotOverlap(t *testing.T) {
	for seed := uint64(0); seed < 20; seed++ {
		zs := NewAtlas(seed, nil).Realm(0, 0)
		for i := range zs {
			for j := i + 1; j < len(zs); j++ {
				d := math.Hypot(zs[i].X-zs[j].X, zs[i].Z-zs[j].Z)
				if d < (zs[i].Radius+zs[j].Radius)*0.55 {
					t.Fatalf("seed %d: zones %s and %s too close (%.0f)", seed, zs[i].Key, zs[j].Key, d)
				}
			}
		}
	}
}

func TestRealmLookupAndSpawn(t *testing.T) {
	a := NewAtlas(7, nil)
	rx, rz := RealmOf(RealmSize+10, -RealmSize)
	if rx != 1 || rz != -1 {
		t.Fatalf("realm of = %d,%d", rx, rz)
	}
	x, z, _ := a.Spawn()
	e := a.Find(Entrance, 0, 0)
	if math.Hypot(x-e.X, z-e.Z) > 20 {
		t.Fatal("spawn must be at the forest entrance")
	}
	if zn, ok := a.At(e.X, e.Z); !ok || zn.ID != Entrance {
		t.Fatal("At() should find the entrance at its centre")
	}
	if len(a.Nearby(e.X, e.Z, 0)) == 0 {
		t.Fatal("Nearby should include the entrance")
	}
}

func TestShapesAreBoundedAndContinuous(t *testing.T) {
	a := NewAtlas(5, nil)
	for _, zn := range a.Realm(0, 0) {
		prevW, prevRel := -1.0, 0.0
		for i := -400; i <= 400; i++ {
			x := zn.X + float64(i)
			w, rel := zn.Shape(x, zn.Z)
			if w < -1e-9 || w > 1+1e-9 || math.IsNaN(rel) || math.Abs(rel) > 120 {
				t.Fatalf("%s: shape out of range w=%v rel=%v", zn.Key, w, rel)
			}
			if prevW >= 0 {
				if math.Abs(w-prevW) > 0.08 {
					t.Fatalf("%s: weight jumps %v->%v at %d", zn.Key, prevW, w, i)
				}
				if zn.ID != Falls && math.Abs(rel-prevRel) > 4 {
					t.Fatalf("%s: height jumps %v->%v at %d", zn.Key, prevRel, rel, i)
				}
			}
			prevW, prevRel = w, rel
		}
		if w, _ := zn.Shape(zn.X+zn.Radius*1.2, zn.Z); w != 0 {
			t.Fatalf("%s: zone must not influence terrain outside its radius", zn.Key)
		}
	}
}

func TestWaterfallDropsAndHasBothLevels(t *testing.T) {
	z := NewAtlas(1, nil).Find(Falls, 0, 0)
	if FallsProfile(-30)-FallsProfile(10) < 20 {
		t.Fatal("falls must drop at least 20m")
	}
	if rel, _, ok := z.WaterLevel(z.X-30, z.Z); !ok || rel != FallsUpperWater {
		t.Fatal("upper river water level missing")
	}
	if rel, _, ok := z.WaterLevel(z.X+40, z.Z); !ok || rel != FallsLowerWater {
		t.Fatal("pool water level missing")
	}
	if _, _, ok := z.WaterLevel(z.X+400, z.Z); ok {
		t.Fatal("no water outside the zone")
	}
}

func TestCaveHoleAndFloor(t *testing.T) {
	z := NewAtlas(1, nil).Find(Cave, 0, 0)
	if !z.Hole(z.X, z.Z) || z.Hole(z.X+CaveHoleRadius+1, z.Z) {
		t.Fatal("cave hole radius wrong")
	}
	if CaveFloor(CaveLakeU, CaveLakeV) >= CaveLakeLevel {
		t.Fatal("lake bed must be below the lake surface")
	}
	if CaveFloor(30, 0) < -0.01 {
		t.Fatal("entrance floor should be level")
	}
}

func TestAllLandmarksBuild(t *testing.T) {
	a := NewAtlas(99, func(x, z float64) float64 { return 10 })
	for _, zn := range a.Realm(0, 0) {
		lm := Build(zn)
		if len(lm.Parts) == 0 || lm.Radius <= 0 {
			t.Fatalf("%s: empty landmark", zn.Key)
		}
		for _, p := range lm.Parts {
			m := p.Mesh
			if m.VertexCount() == 0 || len(m.I) == 0 {
				t.Fatalf("%s: part %s empty", zn.Key, m.Name)
			}
			for _, ix := range m.I {
				if int(ix) >= m.VertexCount() {
					t.Fatalf("%s: part %s has bad index", zn.Key, m.Name)
				}
			}
			for _, f := range m.V {
				if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
					t.Fatalf("%s: part %s has NaN", zn.Key, m.Name)
				}
			}
		}
		for i := range lm.Parts {
			x, y, z, _, s := lm.PartPose(i, 1)
			if math.IsNaN(x + y + z + s) {
				t.Fatalf("%s: bad pose", zn.Key)
			}
		}
	}
}

func TestStationHasInteractionAndLights(t *testing.T) {
	z := NewAtlas(1, nil).Find(Station, 0, 0)
	lm := Build(z)
	if len(lm.Lights) < 4 || len(lm.Interact) == 0 {
		t.Fatal("station needs lights and an interactive control panel")
	}
	it, ok := lm.NearestInteract(z.X+9.2, z.BaseY+1.5, z.Z+1.0)
	if !ok || it.Action != "station.console" {
		t.Fatal("console must be discoverable next to the panel")
	}
	if _, ok := lm.NearestInteract(z.X-100, z.BaseY, z.Z); ok {
		t.Fatal("nothing should be interactive far away")
	}
	before := lm.Mode
	if msg := lm.Activate("station.console"); msg == "" || lm.Mode == before {
		t.Fatal("console activation must change mode")
	}
}

func TestRuinsDoorOpensAndUnblocks(t *testing.T) {
	z := NewAtlas(1, nil).Find(Ruins, 0, 0)
	lm := Build(z)
	px, pz := z.X, z.Z-32+5.0+0.5 // inside the door footprint
	dx, _ := lm.Push(px, pz-0.1, 0.4, z.BaseY+4.3)
	_ = dx
	closed := math.Hypot(lm.Push(px, pz, 0.4, z.BaseY+4.3))
	if closed == 0 {
		t.Fatal("closed door must push the player back")
	}
	lm.Activate("ruins.glyph")
	for i := 0; i < 400; i++ {
		lm.Update(0.05)
	}
	if lm.DoorOpen(1) < 0.9 {
		t.Fatalf("door did not open: %v", lm.DoorOpen(1))
	}
	if open := math.Hypot(lm.Push(px, pz, 0.4, z.BaseY+4.3)); open >= closed {
		t.Fatal("open door must not block")
	}
}

func TestWalkableSurfaces(t *testing.T) {
	w := Walkable{X0: 0, Z0: 0, X1: 10, Z1: 0, R: 1, Y0: 0, Y1: 5}
	if y, ok := w.HeightAt(5, 0.5); !ok || math.Abs(y-2.5) > 1e-9 {
		t.Fatalf("segment height wrong: %v %v", y, ok)
	}
	if _, ok := w.HeightAt(5, 2); ok {
		t.Fatal("outside half width must miss")
	}
	if _, ok := w.HeightAt(-1, 0); ok {
		t.Fatal("beyond end must miss")
	}
	d := Walkable{Disc: true, X0: 3, Z0: 3, R: 2, Y0: 7}
	if y, ok := d.HeightAt(3.5, 3); !ok || y != 7 {
		t.Fatal("disc height wrong")
	}
}

func TestTreehouseIsClimbable(t *testing.T) {
	z := NewAtlas(1, func(x, z float64) float64 { return 0 }).Find(Treehouse, 0, 0)
	lm := Build(z)
	// walk the whole walk list: some surface must reach platform height
	maxY := 0.0
	for _, w := range lm.Walk {
		maxY = math.Max(maxY, math.Max(w.Y0, w.Y1))
	}
	if maxY < 15 {
		t.Fatalf("treehouse platforms too low: %v", maxY)
	}
	// ramp start must be reachable from the ground
	a := 0.4
	x, zz := z.X+math.Cos(a)*12.5, z.Z+math.Sin(a)*12.5
	if _, ok := lm.GroundAt(x, zz, z.BaseY, 0.6); !ok {
		t.Fatal("ramp base must be walkable from ground level")
	}
}

func TestPathNetworkAndDistance(t *testing.T) {
	a := NewAtlas(3, nil)
	e := a.Find(Entrance, 0, 0)
	segs := a.PathsNear(e.X-100, e.Z-100, e.X+100, e.Z+100, 10)
	if len(segs) == 0 {
		t.Fatal("paths should pass near the entrance")
	}
	if d := PathDistance(segs, segs[0].X0, segs[0].Z0); d > 1e-6 {
		t.Fatalf("distance to own endpoint = %v", d)
	}
	if PathDistance(nil, 0, 0) < 1e9 {
		t.Fatal("no segments means infinite distance")
	}
}

func TestLoreForAllZones(t *testing.T) {
	for id := ID(1); id <= NumZones; id++ {
		if len(LoreFor(id)) == 0 || InfoFor(id).Name == "" {
			t.Fatalf("zone %d missing lore or name", id)
		}
	}
}
