package vegetation

import (
	"math"
	"testing"

	"evergreen/internal/mesh"
)

func TestAllKindsBuildValidMeshes(t *testing.T) {
	for k := Kind(0); k < numKinds; k++ {
		for v := 0; v < Variants(k); v++ {
			for lod := 0; lod <= 1; lod++ {
				m := Get(k, v, lod)
				if m.VertexCount() == 0 || len(m.I) == 0 || len(m.I)%3 != 0 {
					t.Fatalf("kind %d v%d lod%d empty/malformed", k, v, lod)
				}
				for _, ix := range m.I {
					if int(ix) >= m.VertexCount() {
						t.Fatalf("kind %d v%d lod%d bad index", k, v, lod)
					}
				}
				for _, f := range m.V {
					if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
						t.Fatalf("kind %d v%d lod%d NaN", k, v, lod)
					}
				}
			}
		}
	}
}

func TestLODReducesGeometry(t *testing.T) {
	for _, k := range []Kind{TreeOak, TreePine, TreeRain, TreeAncient, TreeBirch} {
		if Get(k, 0, 1).VertexCount() >= Get(k, 0, 0).VertexCount() {
			t.Errorf("kind %d: LOD1 must have fewer vertices", k)
		}
	}
}

func TestCacheReturnsSameMesh(t *testing.T) {
	if Get(TreeOak, 1, 0) != Get(TreeOak, 1, 0) || Get(TreeOak, 1, 0) != Get(TreeOak, 4, 0) {
		t.Fatal("meshes must be cached and variants must wrap")
	}
}

func TestTreesHaveExpectedScale(t *testing.T) {
	_, hi := Get(TreeAncient, 0, 0).Bounds()
	if hi.Y < 30 {
		t.Fatalf("ancient tree too short: %v", hi.Y)
	}
	_, hi = Get(TreeOak, 0, 0).Bounds()
	if hi.Y < 6 || hi.Y > 25 {
		t.Fatalf("oak height implausible: %v", hi.Y)
	}
}

func TestGrassUsesGrassMaterial(t *testing.T) {
	m := Get(Grass, 0, 0)
	if m.V[12] != mesh.MatGrass || !m.TwoSided {
		t.Fatal("grass must use grass material and be two-sided")
	}
}

func TestPickTreeCoversProfile(t *testing.T) {
	p := ProfileFor(Forest)
	seen := map[Kind]bool{}
	for i := 0; i < 100; i++ {
		seen[p.PickTree(float64(i)/100)] = true
	}
	for _, kw := range p.Trees {
		if !seen[kw.Kind] {
			t.Errorf("kind %d never picked", kw.Kind)
		}
	}
	if ProfileFor(Biome(99)).TreeSpacing != ProfileFor(Meadow).TreeSpacing {
		t.Fatal("unknown biome should fall back to meadow")
	}
}
