// Package tests holds cross-package integration tests that need no browser.
package tests

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
	"time"

	"evergreen/internal/camera"
	"evergreen/internal/ecosystem"
	"evergreen/internal/engine"
	"evergreen/internal/github"
	"evergreen/internal/procedural"
	"evergreen/internal/weather"
	"evergreen/internal/wildlife"
	"evergreen/internal/world"
	"evergreen/internal/zones"
)

func worldHash(seed uint64, chunks [][2]int) string {
	w := world.New(seed, world.DefaultConfig())
	h := sha256.New()
	for _, c := range chunks {
		ch := &world.Chunk{CX: c[0], CZ: c[1]}
		w.Chunks[world.Key{X: c[0], Z: c[1]}] = ch
		// generate through the public streaming API
		for i := 0; i < 400 && !ch.Ready; i++ {
			w.Update((float64(c[0])+0.5)*64, (float64(c[1])+0.5)*64, 0, 50*time.Millisecond)
		}
		for l := world.Layer(0); l < world.NumLayers; l++ {
			for _, it := range w.Chunks[world.Key{X: c[0], Z: c[1]}].Layers[l] {
				var b [8]byte
				binary.LittleEndian.PutUint32(b[:4], math.Float32bits(it.X))
				binary.LittleEndian.PutUint32(b[4:], math.Float32bits(it.Z))
				h.Write(b[:])
			}
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil)[:8])
}

func TestSeedStringsReproduceTheSameWorld(t *testing.T) {
	seed := procedural.SeedFromString("evergreen")
	chunks := [][2]int{{0, 4}, {1, 4}, {-2, 7}}
	if worldHash(seed, chunks) != worldHash(seed, chunks) {
		t.Fatal("same seed must produce the same forest")
	}
	if worldHash(seed, chunks) == worldHash(procedural.SeedFromString("other"), chunks) {
		t.Fatal("different seeds must differ")
	}
}

func TestExplorationLoopStaysBoundedAndGrounded(t *testing.T) {
	w := world.New(5, world.Config{ViewChunks: 3, Density: 0.4, Influence: github.None()})
	sx, sz, yaw := w.Atlas().Spawn()
	w.Preload(sx, sz, 2, time.Minute)
	cam := camera.New(w, sx, sz, yaw)
	cam.SetMode(camera.Explore, w)
	eco := ecosystem.New(5)
	wx := weather.New(5)
	maxChunks := 0
	for i := 0; i < 900; i++ {
		cam.Update(1.0/30, camera.Input{Move: 1, Sprint: true}, w)
		w.Update(cam.Pos.X, cam.Pos.Z, float64(i)/30, 20*time.Millisecond)
		w.TakeReleased()
		wx.Update(1.0 / 30)
		eco.Update(1.0/30, w, ecosystem.Settings{Cap: 40, Day: 1, Cond: wildlife.Conditions{Player: cam.Pos, Rain: wx.Current.Rain}})
		maxChunks = max(maxChunks, len(w.Chunks))
		if cam.Pos.Y < w.TerrainHeight(cam.Pos.X, cam.Pos.Z)-0.5 && cam.Body.Depth < 1.5 {
			t.Fatalf("camera fell through the world at step %d", i)
		}
	}
	if maxChunks > 70 {
		t.Fatalf("resident chunks grew to %d while exploring", maxChunks)
	}
	if math.Hypot(cam.Pos.X-sx, cam.Pos.Z-sz) < 50 {
		t.Fatal("sprinting for 30 seconds should cover real distance")
	}
	eco.Clear()
	if eco.Live() != 0 {
		t.Fatal("ecosystem pool leaked")
	}
}

func TestEveryZoneIsReachableByWalkingSurfaces(t *testing.T) {
	w := world.New(9, world.DefaultConfig())
	for _, z := range w.Atlas().Realm(0, 0) {
		lm := zones.Build(z)
		if len(lm.Parts) == 0 {
			t.Fatalf("%s has no geometry", z.Key)
		}
		if h := w.Gen.Height(z.X+z.Radius*0.9, z.Z); math.IsNaN(h) {
			t.Fatalf("%s: terrain NaN near the edge", z.Key)
		}
	}
	if engine.QualityFor("ultra").ViewChunks <= engine.QualityFor("low").ViewChunks {
		t.Fatal("presets must scale draw distance")
	}
}
