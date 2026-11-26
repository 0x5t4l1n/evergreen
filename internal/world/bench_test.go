package world

import (
	"testing"
	"time"
)

func BenchmarkChunkGeneration(b *testing.B) {
	w := New(1, DefaultConfig())
	sx, sz, _ := w.Atlas().Spawn()
	k := chunkOf(sx, sz)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := &Chunk{CX: k.X + i%8, CZ: k.Z + i/8%8}
		for !w.step(c, k, time.Now().Add(time.Hour)) {
		}
	}
}

func BenchmarkLandmarks(b *testing.B) {
	w := New(1, DefaultConfig())
	for i := 0; i < b.N; i++ {
		for _, z := range w.Atlas().Realm(0, 0) {
			_ = z
		}
	}
}
