package terrain

import (
	"math"

	"evergreen/internal/mesh"
	"evergreen/internal/procedural"
	"evergreen/internal/vegetation"
	"evergreen/internal/zones"
)

// Grid is a (cells+3)^2 block of samples covering one chunk plus a one-sample
// margin, used for normals, vegetation placement and mesh building.
type Grid struct {
	CX, CZ  int
	X0, Z0  float64 // world position of sample (0,0) (the chunk corner)
	Cells   int
	Step    float64
	Samples []Sample
	Path    []float32 // path mask per sample (0..1)
	Near    []zones.Zone
	filled  int
}

// NewGrid prepares an empty grid for chunk (cx,cz).
func (g *Generator) NewGrid(cx, cz int) *Grid {
	gr := &Grid{CX: cx, CZ: cz, X0: float64(cx) * ChunkSize, Z0: float64(cz) * ChunkSize, Cells: ChunkCells, Step: CellSize}
	n := gr.Stride()
	gr.Samples = make([]Sample, n*n)
	gr.Path = make([]float32, n*n)
	gr.Near = g.Atlas.Nearby(gr.X0+ChunkSize/2, gr.Z0+ChunkSize/2, ChunkSize)
	return gr
}

// Stride is the number of samples per grid row.
func (gr *Grid) Stride() int { return gr.Cells + 3 }

// At returns the sample at grid coordinates i,j in [-1,Cells+1].
func (gr *Grid) At(i, j int) *Sample { return &gr.Samples[(j+1)*gr.Stride()+i+1] }

// PathAt returns the path mask at grid coordinates.
func (gr *Grid) PathAt(i, j int) float64 { return float64(gr.Path[(j+1)*gr.Stride()+i+1]) }

// FillRows samples up to maxRows more rows of the grid and reports whether it is complete.
func (g *Generator) FillRows(gr *Grid, maxRows int) bool {
	n := gr.Stride()
	segs := g.Atlas.PathsNear(gr.X0-2, gr.Z0-2, gr.X0+ChunkSize+2, gr.Z0+ChunkSize+2, 6)
	for ; maxRows > 0 && gr.filled < n; maxRows-- {
		j := gr.filled - 1
		for i := -1; i <= gr.Cells+1; i++ {
			x, z := gr.X0+float64(i)*gr.Step, gr.Z0+float64(j)*gr.Step
			s := g.SampleWith(gr.Near, x, z)
			gr.Samples[(j+1)*n+i+1] = s
			if len(segs) > 0 {
				d := zones.PathDistance(segs, x, z)
				gr.Path[(j+1)*n+i+1] = float32(1 - procedural.SmoothStep(1.2, 3.2, d))
			}
		}
		gr.filled++
	}
	return gr.filled >= n
}

// Fill computes the whole grid at once (tests and tools).
func (g *Generator) Fill(gr *Grid) { g.FillRows(gr, gr.Stride()) }

// HeightBilinear interpolates the grid height at world (x,z).
func (gr *Grid) HeightBilinear(x, z float64) float64 {
	fx, fz := (x-gr.X0)/gr.Step, (z-gr.Z0)/gr.Step
	fx = math.Max(-1, math.Min(float64(gr.Cells+1)-1e-6, fx))
	fz = math.Max(-1, math.Min(float64(gr.Cells+1)-1e-6, fz))
	i, j := int(math.Floor(fx)), int(math.Floor(fz))
	tx, tz := fx-float64(i), fz-float64(j)
	h00, h10 := gr.At(i, j).Height, gr.At(i+1, j).Height
	h01, h11 := gr.At(i, j+1).Height, gr.At(i+1, j+1).Height
	return procedural.Mix(procedural.Mix(h00, h10, tx), procedural.Mix(h01, h11, tx), tz)
}

// Normal returns the unit surface normal at grid sample i,j.
func (gr *Grid) Normal(i, j int) mesh.Vec3 {
	dx := gr.At(i+1, j).Height - gr.At(i-1, j).Height
	dz := gr.At(i, j+1).Height - gr.At(i, j-1).Height
	return mesh.Vec3{X: -dx / (2 * gr.Step), Y: 1, Z: -dz / (2 * gr.Step)}.Norm()
}

// Slope returns 0 (flat) .. 1 (vertical) at a sample.
func (gr *Grid) Slope(i, j int) float64 { return 1 - gr.Normal(i, j).Y }

// BuildTerrain creates the ground mesh. stride is 1, 2 or 4 (LOD0..2).
func BuildTerrain(gr *Grid, stride int) *mesh.Mesh {
	b := mesh.NewBuilder("terrain", uint64(gr.CX)*73856093^uint64(gr.CZ)*19349663)
	n := gr.Cells/stride + 1
	idx := make([]int32, n*n)
	for k := range idx {
		idx[k] = -1
	}
	vert := func(i, j int, drop float64) uint32 {
		s := gr.At(i, j)
		nrm := gr.Normal(i, j)
		slope := 1 - nrm.Y
		rock := procedural.SmoothStep(0.22, 0.45, slope) + procedural.SmoothStep(100, 150, s.Height)*0.7
		shore := 0.0
		if s.HasWater {
			shore = 1 - procedural.SmoothStep(0.2, 2.6, s.Height-s.WaterY)
			if s.Height < s.WaterY {
				shore = 1
			}
		}
		litter := 0.0
		switch s.Biome {
		case vegetation.Forest, vegetation.Rainforest, vegetation.Glade:
			litter = 0.8
		case vegetation.Wetland:
			litter = 0.4
		}
		path := gr.PathAt(i, j)
		zs, zid := 0.0, 0.0
		if s.ZoneID != 0 && s.ZoneW > 0.001 {
			zs, zid = s.ZoneW, float64(s.ZoneID)
		}
		x, z := gr.X0+float64(i)*gr.Step, gr.Z0+float64(j)*gr.Step
		p := mesh.Vec3{X: x, Y: s.Height - drop, Z: z}
		return b.Vert(p, nrm, [3]float32{float32(math.Min(1, rock)), float32(litter), float32(shore)}, float32(zid), float32(path), float32(zs), mesh.MatTerrain)
	}
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			idx[j*n+i] = int32(vert(i*stride, j*stride, 0))
		}
	}
	hole := func(i, j int) bool {
		for _, d := range [][2]int{{0, 0}, {1, 0}, {0, 1}, {1, 1}} {
			if !gr.At((i+d[0])*stride, (j+d[1])*stride).Hole {
				return false
			}
		}
		return true
	}
	for j := 0; j < n-1; j++ {
		for i := 0; i < n-1; i++ {
			if hole(i, j) {
				continue
			}
			a, c, d, e := uint32(idx[j*n+i]), uint32(idx[j*n+i+1]), uint32(idx[(j+1)*n+i+1]), uint32(idx[(j+1)*n+i])
			// split along the diagonal that best follows the surface
			h00, h11 := gr.At(i*stride, j*stride).Height, gr.At((i+1)*stride, (j+1)*stride).Height
			h10, h01 := gr.At((i+1)*stride, j*stride).Height, gr.At(i*stride, (j+1)*stride).Height
			if math.Abs(h00-h11) < math.Abs(h10-h01) {
				b.Tri(a, e, d)
				b.Tri(a, d, c)
			} else {
				b.Tri(a, e, c)
				b.Tri(c, e, d)
			}
		}
	}
	// skirts hide cracks between neighbouring chunks of different detail
	skirt := 2.5 * float64(stride)
	edge := func(coords func(k int) (int, int), flip bool) {
		for k := 0; k < n-1; k++ {
			i0, j0 := coords(k)
			i1, j1 := coords(k + 1)
			t0, t1 := uint32(idx[j0*n+i0]), uint32(idx[j1*n+i1])
			b0 := vert(i0*stride, j0*stride, skirt)
			b1 := vert(i1*stride, j1*stride, skirt)
			if flip {
				b.Quad(t0, b0, b1, t1)
			} else {
				b.Quad(t0, t1, b1, b0)
			}
		}
	}
	edge(func(k int) (int, int) { return k, 0 }, true)
	edge(func(k int) (int, int) { return k, n - 1 }, false)
	edge(func(k int) (int, int) { return 0, k }, false)
	edge(func(k int) (int, int) { return n - 1, k }, true)
	return b.Finish()
}

// BuildWater creates the water-surface mesh for a chunk, or nil if it has none.
func BuildWater(gr *Grid, stride int) *mesh.Mesh {
	b := mesh.NewBuilder("water", uint64(gr.CX)*31+uint64(gr.CZ))
	b.M.Transparent, b.M.NoShadow, b.M.TwoSided = true, true, true
	n := gr.Cells/stride + 1
	idx := make([]int32, n*n)
	for k := range idx {
		idx[k] = -1
	}
	get := func(i, j int) (uint32, bool) {
		s := gr.At(i*stride, j*stride)
		if !s.HasWater {
			return 0, false
		}
		if v := idx[j*n+i]; v >= 0 {
			return uint32(v), true
		}
		x, z := gr.X0+float64(i*stride)*gr.Step, gr.Z0+float64(j*stride)*gr.Step
		depth := s.WaterY - s.Height
		v := b.Vert(mesh.Vec3{X: x, Y: s.WaterY, Z: z}, mesh.Vec3{Y: 1}, [3]float32{0.1, 0.35, 0.3}, 0, float32(math.Max(-1, math.Min(depth, 8))), float32(s.River), mesh.MatWater)
		idx[j*n+i] = int32(v)
		return v, true
	}
	any := false
	for j := 0; j < n-1; j++ {
		for i := 0; i < n-1; i++ {
			var vs [4]uint32
			ok, wet := true, false
			lvl0 := gr.At(i*stride, j*stride).WaterY
			for k, d := range [][2]int{{0, 0}, {1, 0}, {1, 1}, {0, 1}} {
				s := gr.At((i+d[0])*stride, (j+d[1])*stride)
				if !s.HasWater || math.Abs(s.WaterY-lvl0) > 0.6 || s.WaterY-s.Height > s.MaxDepth {
					ok = false
					break
				}
				if s.Wet() {
					wet = true
				}
				vs[k], _ = get(i+d[0], j+d[1])
			}
			if ok && wet {
				b.Quad(vs[0], vs[3], vs[2], vs[1])
				any = true
			}
		}
	}
	if !any {
		return nil
	}
	return b.Finish()
}
