package terrain

import (
	"math"
	"testing"

	"evergreen/internal/zones"
)

func TestDeterministicHeights(t *testing.T) {
	a, b := New(1234), New(1234)
	for i := 0; i < 400; i++ {
		x, z := float64(i)*17.3-2000, float64(i)*-9.1+300
		if a.Height(x, z) != b.Height(x, z) {
			t.Fatalf("same seed differs at %v,%v", x, z)
		}
	}
	c := New(1235)
	diff := 0
	for i := 0; i < 100; i++ {
		x, z := float64(i)*31.1, float64(i)*7.7
		if a.Height(x, z) != c.Height(x, z) {
			diff++
		}
	}
	if diff < 90 {
		t.Fatalf("different seeds should produce different worlds (%d/100)", diff)
	}
}

func TestHeightsPlausibleAndContinuous(t *testing.T) {
	g := New(7)
	minH, maxH := math.Inf(1), math.Inf(-1)
	for i := 0; i < 3000; i++ {
		x := float64(i)*3.3 - 5000
		h := g.Height(x, 120)
		if math.IsNaN(h) {
			t.Fatal("NaN height")
		}
		minH, maxH = math.Min(minH, h), math.Max(maxH, h)
		if i > 0 {
			if d := math.Abs(h - g.Height(x-3.3, 120)); d > 25 {
				t.Fatalf("terrain cliff of %.1fm over 3.3m at x=%v", d, x)
			}
		}
	}
	if maxH-minH < 20 {
		t.Fatalf("terrain too flat: %v..%v", minH, maxH)
	}
	if minH < -60 || maxH > 400 {
		t.Fatalf("terrain range implausible: %v..%v", minH, maxH)
	}
}

func TestMountainsAppearFarFromZones(t *testing.T) {
	g := New(11)
	high := 0.0
	for x := -6000.0; x < 6000; x += 60 {
		for z := -6000.0; z < 6000; z += 60 {
			high = math.Max(high, g.Height(x, z))
		}
	}
	if high < 90 {
		t.Fatalf("expected mountain ranges, tallest peak %.0fm", high)
	}
}

func TestRiversExistAndHoldWater(t *testing.T) {
	g := New(3)
	wet := 0
	for x := -4000.0; x < 4000; x += 25 {
		for z := -4000.0; z < 4000; z += 25 {
			if s := g.Sample(x, z); s.Wet() {
				wet++
				if s.WaterY <= s.Height {
					t.Fatal("wet sample must be below its water surface")
				}
			}
		}
	}
	if wet < 300 {
		t.Fatalf("expected rivers and lakes, found %d wet samples", wet)
	}
}

func TestZonesReshapeTerrain(t *testing.T) {
	g := New(5)
	obs := g.Atlas.Find(zones.Observatory, 0, 0)
	if h := g.Height(obs.X, obs.Z); h < obs.BaseY+60 {
		t.Fatalf("observatory summit should be raised: %.1f vs base %.1f", h, obs.BaseY)
	}
	falls := g.Atlas.Find(zones.Falls, 0, 0)
	up, down := g.Height(falls.X-30, falls.Z), g.Height(falls.X+25, falls.Z)
	if up-down < 15 {
		t.Fatalf("falls cliff missing: up %.1f down %.1f", up, down)
	}
	if s := g.Sample(falls.X+46, falls.Z); !s.Wet() || math.Abs(s.WaterY-(falls.BaseY+zones.FallsLowerWater)) > 1e-6 {
		t.Fatalf("plunge pool should be wet: %+v", s)
	}
	if s := g.Sample(falls.X-30, falls.Z); !s.Wet() {
		t.Fatal("upper river should be wet")
	}
	cave := g.Atlas.Find(zones.Cave, 0, 0)
	if !g.Sample(cave.X, cave.Z).Hole {
		t.Fatal("cave footprint should be a terrain hole")
	}
	if h := g.Height(cave.X+20, cave.Z); math.Abs(h-cave.BaseY) > 3.1 {
		t.Fatalf("cave floor should be near base: %v vs %v", h, cave.BaseY)
	}
}

func TestBiomeOverridesInZones(t *testing.T) {
	g := New(9)
	e := g.Atlas.Find(zones.Entrance, 0, 0)
	if b := g.Sample(e.X, e.Z).Biome; b != zones.InfoFor(zones.Entrance).Biome {
		t.Fatalf("entrance biome = %v", b)
	}
	w := g.Atlas.Find(zones.WorldTree, 0, 0)
	if g.Sample(w.X+40, w.Z).Biome != zones.InfoFor(zones.WorldTree).Biome {
		t.Fatal("world tree grove biome wrong")
	}
}

func TestChunkMeshesAreValid(t *testing.T) {
	g := New(21)
	for _, c := range [][2]int{{0, 0}, {-3, 5}, {10, -8}} {
		gr := g.NewGrid(c[0], c[1])
		g.Fill(gr)
		for _, stride := range []int{1, 2, 4} {
			m := BuildTerrain(gr, stride)
			if m.VertexCount() == 0 || len(m.I) == 0 {
				t.Fatalf("empty terrain %v stride %d", c, stride)
			}
			for _, ix := range m.I {
				if int(ix) >= m.VertexCount() {
					t.Fatalf("bad index %v", c)
				}
			}
		}
		if BuildTerrain(gr, 1).VertexCount() <= BuildTerrain(gr, 4).VertexCount() {
			t.Fatal("LOD0 must have more vertices than LOD2")
		}
	}
}

func TestChunkEdgesMatchAcrossNeighbours(t *testing.T) {
	g := New(2)
	a, b := g.NewGrid(0, 0), g.NewGrid(1, 0)
	g.Fill(a)
	g.Fill(b)
	for j := 0; j <= ChunkCells; j++ {
		if a.At(ChunkCells, j).Height != b.At(0, j).Height {
			t.Fatalf("seam mismatch at row %d", j)
		}
	}
}

func TestWaterMesh(t *testing.T) {
	g := New(1)
	f := g.Atlas.Find(zones.Falls, 0, 0)
	cx, cz := int(math.Floor((f.X+46)/ChunkSize)), int(math.Floor(f.Z/ChunkSize))
	gr := g.NewGrid(cx, cz)
	g.Fill(gr)
	w := BuildWater(gr, 1)
	if w == nil || !w.Transparent || w.VertexCount() == 0 {
		t.Fatal("falls pool chunk must produce a transparent water mesh")
	}
}

func TestIncrementalFillMatchesFull(t *testing.T) {
	g := New(8)
	a, b := g.NewGrid(2, 2), g.NewGrid(2, 2)
	g.Fill(a)
	steps := 0
	for !g.FillRows(b, 4) {
		steps++
	}
	if steps < 5 {
		t.Fatal("incremental fill should take several steps")
	}
	for i := range a.Samples {
		if a.Samples[i] != b.Samples[i] {
			t.Fatal("incremental fill differs from full fill")
		}
	}
}

func TestBilinearMatchesGridPoints(t *testing.T) {
	g := New(4)
	gr := g.NewGrid(1, 1)
	g.Fill(gr)
	x, z := gr.X0+5*gr.Step, gr.Z0+7*gr.Step
	if math.Abs(gr.HeightBilinear(x, z)-gr.At(5, 7).Height) > 1e-6 {
		t.Fatal("bilinear should reproduce sample heights")
	}
}
