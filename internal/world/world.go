package world

import (
	"math"
	"sort"
	"time"

	"evergreen/internal/github"
	"evergreen/internal/terrain"
	"evergreen/internal/vegetation"
	"evergreen/internal/zones"
)

// Config controls streaming and density.
type Config struct {
	ViewChunks  int     // load radius in chunks
	GrassChunks float64 // grass draw radius in chunks (renderer)
	Density     float64 // vegetation density multiplier
	Influence   github.Influence
}

// DefaultConfig is a medium-quality configuration.
func DefaultConfig() Config {
	return Config{ViewChunks: 6, GrassChunks: 1.4, Density: 0.7, Influence: github.None()}
}

// World is the streaming world state.
type World struct {
	Gen        *terrain.Generator
	Cfg        Config
	Chunks     map[Key]*Chunk
	Landmarks  map[zones.ID]*zones.Landmark
	released   []*Chunk
	relLM      []*zones.Landmark
	pending    []Key
	lastCenter Key
	dirty      bool
	now        float64
	seed       uint64
	Stats      Stats
}

// Stats are exposed for the HUD and tests.
type Stats struct {
	Loaded, Pending int
	Unloaded        uint64
	Generated       uint64
}

// New creates a world for a numeric seed.
func New(seed uint64, cfg Config) *World {
	return &World{Gen: terrain.New(seed), Cfg: cfg, Chunks: map[Key]*Chunk{}, Landmarks: map[zones.ID]*zones.Landmark{}, seed: seed, dirty: true, lastCenter: Key{1 << 30, 1 << 30}}
}

// SetConfig changes streaming parameters. A density or influence change
// regenerates chunks so the new rules apply.
func (w *World) SetConfig(c Config) {
	regen := c.Density != w.Cfg.Density || len(c.Influence.Trees) != len(w.Cfg.Influence.Trees) ||
		c.Influence.PlantBoost != w.Cfg.Influence.PlantBoost || c.Influence.FlowerBoost != w.Cfg.Influence.FlowerBoost || c.Influence.Density != w.Cfg.Influence.Density
	w.Cfg = c
	w.dirty = true
	if regen {
		w.Reset()
	}
	for _, lm := range w.Landmarks {
		lm.Growth = c.Influence.TreeGrowth
	}
}

// Reset unloads every chunk (they stream back in).
func (w *World) Reset() {
	for k, c := range w.Chunks {
		w.released = append(w.released, c)
		delete(w.Chunks, k)
		w.Stats.Unloaded++
	}
	w.pending = nil
	w.lastCenter = Key{1 << 30, 1 << 30}
	w.dirty = true
}

// TakeReleased returns chunks and landmarks that were unloaded since the last
// call so the renderer can free their GPU resources.
func (w *World) TakeReleased() ([]*Chunk, []*zones.Landmark) {
	c, l := w.released, w.relLM
	w.released, w.relLM = nil, nil
	return c, l
}

func chunkOf(x, z float64) Key {
	return Key{int(math.Floor(x / terrain.ChunkSize)), int(math.Floor(z / terrain.ChunkSize))}
}

func (w *World) lodFor(d float64) int {
	switch {
	case d < 2.6:
		return 0
	case d < 5.2:
		return 1
	}
	return 2
}

// Update streams chunks around (x,z). budget limits generation time per call.
func (w *World) Update(x, z, now float64, budget time.Duration) {
	w.now = now
	ck := chunkOf(x, z)
	if w.dirty || ck != w.lastCenter {
		w.lastCenter = ck
		w.dirty = false
		w.plan(ck)
	}
	w.unload(x, z)
	w.landmarks(x, z)
	deadline := time.Now().Add(budget)
	for len(w.pending) > 0 {
		k := w.pending[0]
		c := w.Chunks[k]
		if c == nil {
			c = &Chunk{CX: k.X, CZ: k.Z}
			w.Chunks[k] = c
		}
		if w.step(c, ck, deadline) {
			w.pending = w.pending[1:]
		}
		if time.Now().After(deadline) {
			break
		}
	}
	w.relod(x, z, deadline)
	w.Stats.Loaded = len(w.Chunks)
	w.Stats.Pending = len(w.pending)
}

// plan builds the nearest-first list of chunks that must exist.
func (w *World) plan(center Key) {
	r := w.Cfg.ViewChunks
	w.pending = w.pending[:0]
	type cand struct {
		k Key
		d float64
	}
	var cs []cand
	for dz := -r; dz <= r; dz++ {
		for dx := -r; dx <= r; dx++ {
			d := math.Hypot(float64(dx), float64(dz))
			if d > float64(r)+0.5 {
				continue
			}
			k := Key{center.X + dx, center.Z + dz}
			if c, ok := w.Chunks[k]; ok && c.Ready {
				continue
			}
			cs = append(cs, cand{k, d})
		}
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].d < cs[j].d })
	for _, c := range cs {
		w.pending = append(w.pending, c.k)
	}
}

func (w *World) unload(x, z float64) {
	limit := float64(w.Cfg.ViewChunks) + 1.6
	cx, cz := x/terrain.ChunkSize-0.5, z/terrain.ChunkSize-0.5
	for k, c := range w.Chunks {
		if math.Hypot(float64(k.X)-cx, float64(k.Z)-cz) > limit {
			w.released = append(w.released, c)
			delete(w.Chunks, k)
			w.Stats.Unloaded++
		}
	}
}

func (w *World) landmarks(x, z float64) {
	reach := float64(w.Cfg.ViewChunks)*terrain.ChunkSize + 120
	want := map[zones.ID]bool{}
	for _, zn := range w.Gen.Atlas.Nearby(x, z, reach) {
		want[zn.ID] = true
		if _, ok := w.Landmarks[zn.ID]; !ok {
			lm := zones.Build(zn)
			lm.Growth = w.Cfg.Influence.TreeGrowth
			w.Landmarks[zn.ID] = lm
			break // one heavy build per frame
		} else if w.Landmarks[zn.ID].Zone.X != zn.X || w.Landmarks[zn.ID].Zone.Z != zn.Z {
			// the same zone type in a different realm replaced the old one
			w.relLM = append(w.relLM, w.Landmarks[zn.ID])
			delete(w.Landmarks, zn.ID)
		}
	}
	for id, lm := range w.Landmarks {
		if !want[id] {
			w.relLM = append(w.relLM, lm)
			delete(w.Landmarks, id)
		}
	}
}

// relod rebuilds terrain meshes whose level of detail changed.
func (w *World) relod(x, z float64, deadline time.Time) {
	if len(w.pending) > 0 {
		return
	}
	cx, cz := x/terrain.ChunkSize-0.5, z/terrain.ChunkSize-0.5
	for k, c := range w.Chunks {
		if !c.Ready || c.job != nil {
			continue
		}
		d := math.Hypot(float64(k.X)-cx, float64(k.Z)-cz)
		want := w.lodFor(d)
		// hysteresis: only change when clearly past the threshold
		if want != c.LOD && (math.Abs(d-2.6) > 0.25 && math.Abs(d-5.2) > 0.25) {
			c.job = &job{stage: stageGrid, relod: true, lod: want}
		}
	}
	for _, c := range w.Chunks {
		if c.job != nil && c.job.relod {
			w.step(c, w.lastCenter, deadline)
			if time.Now().After(deadline) {
				return
			}
		}
	}
}

// ---- generation state machine ---------------------------------------------------------------

type stage int

const (
	stageGrid stage = iota
	stageMesh
	stagePlace
	stageDone
)

type job struct {
	stage stage
	gr    *terrain.Grid
	relod bool
	lod   int
	step  int
}

// step advances chunk generation; it returns true when the chunk is finished.
func (w *World) step(c *Chunk, center Key, deadline time.Time) bool {
	if c.Ready && c.job == nil {
		return true
	}
	if c.job == nil {
		d := math.Hypot(float64(c.CX-center.X), float64(c.CZ-center.Z))
		c.job = &job{stage: stageGrid, lod: w.lodFor(d)}
	}
	j := c.job
	for {
		switch j.stage {
		case stageGrid:
			if j.gr == nil {
				j.gr = w.Gen.NewGrid(c.CX, c.CZ)
			}
			if !w.Gen.FillRows(j.gr, 5) {
				if time.Now().After(deadline) {
					return false
				}
				continue
			}
			j.stage = stageMesh
		case stageMesh:
			stride := 1 << j.lod
			c.Terrain = terrain.BuildTerrain(j.gr, stride)
			c.Water = terrain.BuildWater(j.gr, max(1, min(stride, 2)))
			c.LOD = j.lod
			c.Version++
			if !j.relod {
				n := terrain.ChunkCells + 3
				c.Heights = make([]float32, n*n)
				c.MinY, c.MaxY = float32(math.Inf(1)), float32(math.Inf(-1))
				for i := range j.gr.Samples {
					h := float32(j.gr.Samples[i].Height)
					c.Heights[i] = h
					c.MinY, c.MaxY = min(c.MinY, h), max(c.MaxY, h)
				}
			}
			if j.relod {
				j.stage = stageDone
			} else {
				j.stage = stagePlace
			}
			if time.Now().After(deadline) {
				return false
			}
		case stagePlace:
			p := &placer{c: c, gr: j.gr, g: w.Gen, seed: w.seed, cfg: w.Cfg}
			switch j.step {
			case 0:
				p.trees()
			case 1:
				p.understory()
			case 2:
				p.flowers()
			case 3:
				p.grass()
			}
			j.step++
			if j.step >= 4 {
				j.stage = stageDone
			}
			if time.Now().After(deadline) && j.stage != stageDone {
				return false
			}
		case stageDone:
			if !j.relod {
				c.Ready = true
				c.LoadedAt = w.now
				w.Stats.Generated++
			}
			c.job = nil
			return true
		}
	}
}

// ---- environment adapters --------------------------------------------------------------------------

// Atlas returns the zone atlas.
func (w *World) Atlas() *zones.Atlas { return w.Gen.Atlas }

// TerrainHeight returns ground height, preferring retained chunk data.
func (w *World) TerrainHeight(x, z float64) float64 {
	if c, ok := w.Chunks[chunkOf(x, z)]; ok && c.Ready {
		if h, ok := c.HeightAt(x, z); ok {
			return h
		}
	}
	return w.Gen.Height(x, z)
}

// GroundAt returns the highest standable surface up to stepUp above feetY.
func (w *World) GroundAt(x, z, feetY, stepUp float64) float64 {
	g := w.TerrainHeight(x, z)
	for _, lm := range w.Landmarks {
		if math.Hypot(x-lm.Zone.X, z-lm.Zone.Z) > lm.Radius+20 {
			continue
		}
		if y, ok := lm.GroundAt(x, z, feetY, stepUp); ok && y > g {
			g = y
		}
	}
	return g
}

// WaterLevel returns the water surface height at (x,z) when water covers the ground.
func (w *World) WaterLevel(x, z float64) (float64, bool) { return w.Gen.WaterAt(x, z) }

// Push resolves collisions against trunks, rocks and landmark structures.
func (w *World) Push(x, z, r, y float64) (float64, float64) {
	px, pz := 0.0, 0.0
	for _, lm := range w.Landmarks {
		if math.Hypot(x-lm.Zone.X, z-lm.Zone.Z) > lm.Radius+10 {
			continue
		}
		dx, dz := lm.Push(x+px, z+pz, r, y)
		px, pz = px+dx, pz+dz
	}
	k := chunkOf(x, z)
	for dz := -1; dz <= 1; dz++ {
		for dx := -1; dx <= 1; dx++ {
			c, ok := w.Chunks[Key{k.X + dx, k.Z + dz}]
			if !ok || !c.Ready {
				continue
			}
			for _, col := range c.Colliders {
				ox, oz := x+px-float64(col.X), z+pz-float64(col.Z)
				d2 := ox*ox + oz*oz
				lim := float64(col.R) + r
				if d2 < lim*lim && d2 > 1e-10 {
					d := math.Sqrt(d2)
					k := (lim - d) / d
					px, pz = px+ox*k, pz+oz*k
				}
			}
		}
	}
	return px, pz
}

// Ground implements wildlife.Env.
func (w *World) Ground(x, z float64) float64 { return w.TerrainHeight(x, z) }

// Water implements wildlife.Env: surface height, depth and presence.
func (w *World) Water(x, z float64) (float64, float64, bool) {
	s := w.Gen.Sample(x, z)
	if s.Wet() {
		return s.WaterY, s.Depth(), true
	}
	return 0, 0, false
}

// Biome implements ecosystem.Env.
func (w *World) Biome(x, z float64) vegetation.Biome { return w.Gen.Sample(x, z).Biome }

// ZoneAt returns the zone containing (x,z) and how deep inside it the point is (0..1).
func (w *World) ZoneAt(x, z float64) (zones.Zone, float64, bool) {
	zn, ok := w.Gen.Atlas.At(x, z)
	if !ok {
		return zones.Zone{}, 0, false
	}
	return zn, 1 - math.Hypot(zn.X-x, zn.Z-z)/zn.Radius, true
}

// Landmark returns the loaded landmark for a zone ID.
func (w *World) Landmark(id zones.ID) *zones.Landmark { return w.Landmarks[id] }

// ReadyChunks returns the number of fully generated chunks.
func (w *World) ReadyChunks() int {
	n := 0
	for _, c := range w.Chunks {
		if c.Ready {
			n++
		}
	}
	return n
}

// Preload generates chunks around (x,z) synchronously (loading screen and tests).
func (w *World) Preload(x, z float64, radius int, limit time.Duration) {
	old := w.Cfg.ViewChunks
	w.Cfg.ViewChunks = radius
	w.dirty = true
	end := time.Now().Add(limit)
	for time.Now().Before(end) {
		w.Update(x, z, w.now, 50*time.Millisecond)
		if len(w.pending) == 0 {
			break
		}
	}
	w.Cfg.ViewChunks = old
	w.dirty = true
}
