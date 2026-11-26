package zones

import "testing"

func BenchmarkBuildAllLandmarks(b *testing.B) {
	a := NewAtlas(1, func(x, z float64) float64 { return 10 })
	zs := a.Realm(0, 0)
	for i := 0; i < b.N; i++ {
		for _, z := range zs {
			Build(z)
		}
	}
}
