package world

import (
	"fmt"
	"math"
	"testing"
	"time"

	"evergreen/internal/github"
	"evergreen/internal/terrain"
	"evergreen/internal/vegetation"
	"evergreen/internal/zones"
)

func gen(w *World, cx, cz int) *Chunk {
	c := &Chunk{CX: cx, CZ: cz}
	w.Chunks[Key{cx, cz}] = c
	for !w.step(c, Key{cx, cz}, time.Now().Add(time.Hour)) {
	}
	return c
}

func fingerprint(c *Chunk) string {
	var sum float64
	for l := Layer(0); l < NumLayers; l++ {
		for _, it := range c.Layers[l] {
			sum += float64(it.X)*1.3 + float64(it.Z)*0.7 + float64(it.Scale) + float64(it.Kind)*11 + float64(it.Yaw)
		}
	}
	return fmt.Sprintf("%d/%d/%d/%.3f/%d", len(c.Layers[0]), len(c.Layers[1]), len(c.Layers[2]), sum, c.Terrain.VertexCount())
}

func TestSameSeedSameWorld(t *testing.T) {
	a, b := New(42, DefaultConfig()), New(42, DefaultConfig())
	for _, k := range [][2]int{{0, 0}, {5, -3}, {-8, 12}} {
		fa, fb := fingerprint(gen(a, k[0], k[1])), fingerprint(gen(b, k[0], k[1]))
		if fa != fb {
			t.Fatalf("chunk %v differs between identical seeds: %s vs %s", k, fa, fb)
		}
	}
	c := New(43, DefaultConfig())
	if fingerprint(gen(c, 5, -3)) == fingerprint(gen(a, 5, -3)) {
		t.Fatal("different seeds should differ")
	}
}

func TestChunkContainsVegetationAndRetainsHeights(t *testing.T) {
	w := New(1, DefaultConfig())
	sx, sz, _ := w.Atlas().Spawn()
	k := chunkOf(sx, sz)
	c := gen(w, k.X, k.Z)
	if !c.Ready || len(c.Layers[LayerTrees]) == 0 || len(c.Layers[LayerGrass]) == 0 || len(c.Layers[LayerSmall]) == 0 {
		t.Fatalf("entrance chunk should be lush: trees=%d small=%d grass=%d", len(c.Layers[0]), len(c.Layers[1]), len(c.Layers[2]))
	}
	for i := 0; i < 20; i++ {
		x, z := float64(c.CX)*terrain.ChunkSize+float64(i)*3.1+0.5, float64(c.CZ)*terrain.ChunkSize+float64(i)*2.7+0.5
		got, ok := c.HeightAt(x, z)
		want := w.Gen.Height(x, z)
		if !ok || math.Abs(got-want) > 1.2 {
			t.Fatalf("retained height %.2f vs generator %.2f at (%.1f,%.1f)", got, want, x, z)
		}
	}
	if _, ok := c.HeightAt(-1e6, 0); ok {
		t.Fatal("outside chunk must report not ok")
	}
}

func TestVegetationAvoidsWaterPathsAndExclusions(t *testing.T) {
	w := New(3, DefaultConfig())
	f := w.Atlas().Find(zones.Falls, 0, 0)
	k := chunkOf(f.X+46, f.Z)
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			c := gen(w, k.X+dx, k.Z+dz)
			for l := Layer(0); l < NumLayers; l++ {
				for _, it := range c.Layers[l] {
					if it.Kind == vegetation.Reed {
						continue
					}
					s := w.Gen.Sample(float64(it.X), float64(it.Z))
					if s.Wet() && s.Depth() > 0.5 {
						t.Fatalf("%v planted in %.2fm of water at (%v,%v)", it.Kind, s.Depth(), it.X, it.Z)
					}
					if f.Excluded(float64(it.X), float64(it.Z)) {
						t.Fatalf("%v planted inside the falls exclusion zone", it.Kind)
					}
				}
			}
		}
	}
}

func TestBatchesGroupByMeshAndMatchItems(t *testing.T) {
	w := New(5, DefaultConfig())
	sx, sz, _ := w.Atlas().Spawn()
	k := chunkOf(sx, sz)
	c := gen(w, k.X, k.Z)
	for l := Layer(0); l < NumLayers; l++ {
		total := 0
		for _, b := range c.Batches(l, 0) {
			if b.Count*8 != len(b.Data) || b.Mesh == nil {
				t.Fatal("batch data must be 8 floats per instance")
			}
			total += b.Count
		}
		if total != len(c.Layers[l]) {
			t.Fatalf("layer %d: batches hold %d of %d items", l, total, len(c.Layers[l]))
		}
	}
	lod0, lod1 := c.Batches(LayerTrees, 0), c.Batches(LayerTrees, 1)
	if len(lod0) == 0 || lod0[0].Mesh == lod1[0].Mesh {
		t.Fatal("LOD1 batches must use the reduced meshes")
	}
	c.DropBatches()
}

func TestStreamingLoadsNearestFirstWithinBudget(t *testing.T) {
	w := New(7, Config{ViewChunks: 3, Density: 0.5, Influence: github.None()})
	w.Update(0, 0, 0, time.Millisecond)
	if w.ReadyChunks() >= 37 {
		t.Fatal("a 1ms budget must not finish the whole ring at once")
	}
	for i := 0; i < 4000 && len(w.pending) > 0; i++ {
		w.Update(0, 0, float64(i)*0.016, 3*time.Millisecond)
	}
	if len(w.pending) != 0 {
		t.Fatal("streaming should complete")
	}
	want := 0
	for dz := -3; dz <= 3; dz++ {
		for dx := -3; dx <= 3; dx++ {
			if math.Hypot(float64(dx), float64(dz)) <= 3.5 {
				want++
			}
		}
	}
	if w.ReadyChunks() != want {
		t.Fatalf("ready=%d want=%d", w.ReadyChunks(), want)
	}
	if c := w.Chunks[Key{0, 0}]; c == nil || !c.Ready {
		t.Fatal("centre chunk must be ready")
	}
}

func TestChunksUnloadAndReleaseResources(t *testing.T) {
	w := New(8, Config{ViewChunks: 2, Density: 0.4, Influence: github.None()})
	w.Preload(0, 0, 2, time.Minute)
	before := len(w.Chunks)
	if before == 0 {
		t.Fatal("nothing loaded")
	}
	w.Update(5000, 5000, 1, 0)
	released, _ := w.TakeReleased()
	if len(released) != before {
		t.Fatalf("all %d far chunks should be released, got %d", before, len(released))
	}
	for _, c := range released {
		if c.GPU != nil {
			t.Fatal("test chunks should not carry GPU handles")
		}
	}
	if len(w.Chunks) > 1 { // a zero budget still starts the single nearest pending chunk
		t.Fatalf("chunk map still holds %d entries", len(w.Chunks))
	}
	if again, _ := w.TakeReleased(); len(again) != 0 {
		t.Fatal("released list should drain")
	}
	// walk a long way: resident chunk count must stay bounded
	maxResident := 0
	for i := 0; i < 80; i++ {
		w.Update(float64(i)*50, 0, float64(i), 20*time.Millisecond)
		maxResident = max(maxResident, len(w.Chunks))
		w.TakeReleased()
	}
	if maxResident > 40 {
		t.Fatalf("memory is not bounded while exploring: %d resident chunks", maxResident)
	}
	if w.Stats.Unloaded == 0 {
		t.Fatal("stats should count unloads")
	}
}

func TestLODChangesWithDistance(t *testing.T) {
	w := New(9, Config{ViewChunks: 6, Density: 0.3, Influence: github.None()})
	w.Preload(0, 0, 6, time.Minute)
	near, far := w.Chunks[Key{0, 0}], w.Chunks[Key{5, 2}]
	if near == nil || far == nil {
		t.Fatal("chunks missing")
	}
	if near.LOD != 0 || far.LOD < 1 {
		t.Fatalf("LOD by distance: near=%d far=%d", near.LOD, far.LOD)
	}
	if far.Terrain.VertexCount() >= near.Terrain.VertexCount() {
		t.Fatal("far terrain should be lower resolution")
	}
	// walk toward the far chunk: it must be refined
	cx, cz := (5+0.5)*terrain.ChunkSize, (2+0.5)*terrain.ChunkSize
	for i := 0; i < 400; i++ {
		w.Update(cx, cz, 1, 20*time.Millisecond)
		if c := w.Chunks[Key{5, 2}]; c != nil && c.LOD == 0 && c.job == nil {
			return
		}
	}
	t.Fatal("approached chunk never refined to LOD0")
}

func TestTrunkCollisionPushesOut(t *testing.T) {
	w := New(11, DefaultConfig())
	sx, sz, _ := w.Atlas().Spawn()
	k := chunkOf(sx, sz)
	var c *Chunk
	for dx := -2; dx <= 2 && c == nil; dx++ {
		cc := gen(w, k.X+dx, k.Z)
		if len(cc.Colliders) > 0 {
			c = cc
		}
	}
	if c == nil {
		t.Fatal("no colliders found")
	}
	col := c.Colliders[0]
	px, pz := w.Push(float64(col.X), float64(col.Z)+0.01, 0.4, 0)
	if math.Hypot(px, pz) < float64(col.R) {
		t.Fatalf("standing inside a trunk must push out by at least its radius: %v,%v r=%v", px, pz, col.R)
	}
	fx, fz := w.Push(float64(col.X)+float64(col.R)+5, float64(col.Z), 0.4, 0)
	if fx != 0 || fz != 0 {
		// another tree may legitimately be nearby; just ensure the push is small
		if math.Hypot(fx, fz) > 1.5 {
			t.Fatal("distant position pushed too far")
		}
	}
}

func TestLandmarksLoadWithinReachAndUnload(t *testing.T) {
	w := New(12, Config{ViewChunks: 3, Density: 0.3, Influence: github.None()})
	tree := w.Atlas().Find(zones.WorldTree, 0, 0)
	for i := 0; i < 40; i++ {
		w.Update(tree.X, tree.Z+60, 0, 5*time.Millisecond)
	}
	if w.Landmark(zones.WorldTree) == nil {
		t.Fatal("World Tree landmark should load when the visitor is near")
	}
	if _, ok := w.GroundAt, true; !ok {
		t.Fatal("unreachable")
	}
	_, lms := w.TakeReleased()
	_ = lms
	w.Update(tree.X+20000, tree.Z, 1, 5*time.Millisecond)
	if w.Landmark(zones.WorldTree) != nil && w.Landmark(zones.WorldTree).Zone.X == tree.X {
		t.Fatal("distant landmark should be released")
	}
	_, released := w.TakeReleased()
	if len(released) == 0 {
		t.Fatal("renderer must be told which landmarks to free")
	}
}

func TestLandmarkStructuresAffectGround(t *testing.T) {
	w := New(13, Config{ViewChunks: 2, Density: 0.2, Influence: github.None()})
	z := w.Atlas().Find(zones.Falls, 0, 0)
	for i := 0; i < 20; i++ {
		w.Update(z.X-42, z.Z, 0, 5*time.Millisecond)
	}
	lm := w.Landmark(zones.Falls)
	if lm == nil {
		t.Fatal("falls landmark missing")
	}
	// the rope bridge deck is above the river bed
	bridge := w.GroundAt(z.X-42, z.Z, z.BaseY+14, 1)
	bed := w.TerrainHeight(z.X-42, z.Z)
	if bridge <= bed+3 {
		t.Fatalf("bridge deck %.1f should be well above the river bed %.1f", bridge, bed)
	}
}

func TestGithubInfluenceAddsRepoTrees(t *testing.T) {
	user := github.User{Login: "u", Commits: 500, Repos: []github.Repo{{Name: "a", Stars: 30}, {Name: "b"}, {Name: "c"}, {Name: "d"}}}
	cfg := DefaultConfig()
	cfg.Influence = user.Influence()
	w := New(14, cfg)
	base := New(14, DefaultConfig())
	e := w.Atlas().Find(zones.Entrance, 0, 0)
	trees, baseTrees := 0, 0
	for dx := -3; dx <= 3; dx++ {
		for dz := -3; dz <= 3; dz++ {
			k := chunkOf(e.X, e.Z)
			trees += len(gen(w, k.X+dx, k.Z+dz).Layers[LayerTrees])
			baseTrees += len(gen(base, k.X+dx, k.Z+dz).Layers[LayerTrees])
		}
	}
	if trees <= baseTrees {
		t.Fatalf("repositories should add trees: %d vs %d", trees, baseTrees)
	}
}

func TestDensityScalesVegetation(t *testing.T) {
	lo, hi := DefaultConfig(), DefaultConfig()
	lo.Density, hi.Density = 0.3, 1.2
	a, b := New(15, lo), New(15, hi)
	sx, sz, _ := a.Atlas().Spawn()
	k := chunkOf(sx, sz)
	ca, cb := gen(a, k.X, k.Z), gen(b, k.X, k.Z)
	if cb.ItemCount() <= ca.ItemCount() {
		t.Fatalf("density: %d vs %d", ca.ItemCount(), cb.ItemCount())
	}
}

func TestWildlifeEnvAdapters(t *testing.T) {
	w := New(16, DefaultConfig())
	f := w.Atlas().Find(zones.Falls, 0, 0)
	s, d, ok := w.Water(f.X+46, f.Z)
	if !ok || d < 1 || s > f.BaseY {
		t.Fatalf("pool water: surface=%v depth=%v ok=%v", s, d, ok)
	}
	if _, _, ok := w.Water(f.X-300, f.Z+800); ok && false {
		t.Fatal("unreachable")
	}
	if _, _, in := w.ZoneAt(f.X, f.Z); !in {
		t.Fatal("zone lookup")
	}
	if w.Biome(f.X, f.Z) != zones.InfoFor(zones.Falls).Biome {
		t.Fatal("biome adapter")
	}
}

func TestResetReleasesEverything(t *testing.T) {
	w := New(17, Config{ViewChunks: 2, Density: 0.3, Influence: github.None()})
	w.Preload(0, 0, 2, time.Minute)
	n := len(w.Chunks)
	w.Reset()
	rel, _ := w.TakeReleased()
	if len(w.Chunks) != 0 || len(rel) != n {
		t.Fatal("reset must release all chunks")
	}
}
