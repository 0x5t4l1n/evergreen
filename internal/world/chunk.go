// Package world streams the infinite world: it generates chunks around the
// visitor in small time slices, places vegetation, tracks collision data,
// loads zone landmarks and unloads everything that falls out of range.
package world

import (
	"math"
	"sort"

	"evergreen/internal/mesh"
	"evergreen/internal/terrain"
	"evergreen/internal/vegetation"
)

// Layer groups vegetation by draw-distance class.
type Layer int

// Layers.
const (
	LayerTrees Layer = iota
	LayerSmall
	LayerGrass
	NumLayers
)

// Item is one placed plant, rock or fungus.
type Item struct {
	X, Y, Z, Scale, Yaw, Tint float32
	Kind                      vegetation.Kind
	Variant                   uint8
}

// Circle is a trunk/rock collider.
type Circle struct{ X, Z, R float32 }

// Batch is a group of instances sharing one mesh. Data holds 8 floats per
// instance (x,y,z,scale, yaw,tint,birth,aux).
type Batch struct {
	Mesh  *mesh.Mesh
	Data  []float32
	Count int
}

// Chunk is one 64m x 64m square of the world.
type Chunk struct {
	CX, CZ     int
	Ready      bool
	Terrain    *mesh.Mesh
	Water      *mesh.Mesh
	LOD        int
	Version    uint32 // bumps when Terrain/Water change so the renderer re-uploads
	Heights    []float32
	Layers     [NumLayers][]Item
	Colliders  []Circle
	MinY, MaxY float32
	LoadedAt   float64
	GPU        any // renderer-owned handle

	batches [NumLayers][2][]*Batch
	job     *job
}

// Key identifies a chunk in maps.
type Key struct{ X, Z int }

// Center returns the world position of the chunk centre.
func (c *Chunk) Center() (float64, float64) {
	return (float64(c.CX) + 0.5) * terrain.ChunkSize, (float64(c.CZ) + 0.5) * terrain.ChunkSize
}

// Bounds returns the world-space AABB (including tall trees).
func (c *Chunk) Bounds() (lo, hi mesh.Vec3) {
	x0, z0 := float64(c.CX)*terrain.ChunkSize, float64(c.CZ)*terrain.ChunkSize
	top := float64(c.MaxY)
	if len(c.Layers[LayerTrees]) > 0 {
		top += 60
	} else {
		top += 8
	}
	return mesh.Vec3{X: x0, Y: float64(c.MinY) - 3, Z: z0}, mesh.Vec3{X: x0 + terrain.ChunkSize, Y: top, Z: z0 + terrain.ChunkSize}
}

// ItemCount returns the number of placed items.
func (c *Chunk) ItemCount() int {
	n := 0
	for _, l := range c.Layers {
		n += len(l)
	}
	return n
}

// Batches returns instance batches for a layer at a level of detail. The
// result is cached; birth is the time written into each instance so plants
// grow in smoothly when they appear.
func (c *Chunk) Batches(l Layer, lod int) []*Batch {
	if lod > 1 {
		lod = 1
	}
	if b := c.batches[l][lod]; b != nil {
		return b
	}
	type key struct {
		k vegetation.Kind
		v uint8
	}
	groups := map[key]*Batch{}
	var order []key
	for _, it := range c.Layers[l] {
		k := key{it.Kind, it.Variant}
		g := groups[k]
		if g == nil {
			g = &Batch{Mesh: vegetation.Get(it.Kind, int(it.Variant), lod)}
			groups[k] = g
			order = append(order, k)
		}
		g.Data = append(g.Data, it.X, it.Y, it.Z, it.Scale, it.Yaw, it.Tint, float32(c.LoadedAt), 0)
		g.Count++
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i].k != order[j].k {
			return order[i].k < order[j].k
		}
		return order[i].v < order[j].v
	})
	out := make([]*Batch, 0, len(order))
	for _, k := range order {
		out = append(out, groups[k])
	}
	c.batches[l][lod] = out
	if out == nil {
		c.batches[l][lod] = []*Batch{}
	}
	return c.batches[l][lod]
}

// DropBatches frees cached batch data (after GPU upload).
func (c *Chunk) DropBatches() { c.batches = [NumLayers][2][]*Batch{} }

// HeightAt returns the retained terrain height at world (x,z) by bilinear
// interpolation, which matches the rendered LOD0 surface.
func (c *Chunk) HeightAt(x, z float64) (float64, bool) {
	if c.Heights == nil {
		return 0, false
	}
	n := terrain.ChunkCells + 3
	fx := (x - float64(c.CX)*terrain.ChunkSize) / terrain.CellSize
	fz := (z - float64(c.CZ)*terrain.ChunkSize) / terrain.CellSize
	if fx < 0 || fz < 0 || fx > terrain.ChunkCells || fz > terrain.ChunkCells {
		return 0, false
	}
	fx = math.Min(fx, terrain.ChunkCells-1e-6)
	fz = math.Min(fz, terrain.ChunkCells-1e-6)
	i, j := int(fx), int(fz)
	tx, tz := fx-float64(i), fz-float64(j)
	h := func(a, b int) float64 { return float64(c.Heights[(b+1)*n+a+1]) }
	// match the diagonal split used by the mesh builder
	h00, h10, h01, h11 := h(i, j), h(i+1, j), h(i, j+1), h(i+1, j+1)
	if math.Abs(h00-h11) < math.Abs(h10-h01) {
		if tz > tx {
			return h00 + (h11-h01)*tx + (h01-h00)*tz, true
		}
		return h00 + (h10-h00)*tx + (h11-h10)*tz, true
	}
	if tx+tz < 1 {
		return h00 + (h10-h00)*tx + (h01-h00)*tz, true
	}
	return h11 + (h01-h11)*(1-tx) + (h10-h11)*(1-tz), true
}
