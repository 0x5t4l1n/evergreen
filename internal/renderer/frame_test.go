package renderer

import (
	"math"
	"testing"

	"evergreen/internal/atmosphere"
	"evergreen/internal/engine"
	"evergreen/internal/zones"
)

func project(vp engine.Mat4, p engine.Vec3) (float64, float64, float64) {
	x := float64(vp[0])*p.X + float64(vp[4])*p.Y + float64(vp[8])*p.Z + float64(vp[12])
	y := float64(vp[1])*p.X + float64(vp[5])*p.Y + float64(vp[9])*p.Z + float64(vp[13])
	w := float64(vp[3])*p.X + float64(vp[7])*p.Y + float64(vp[11])*p.Z + float64(vp[15])
	return x / w, y / w, w
}

func testScene() *Scene {
	return &Scene{
		Time:  12.5,
		Cam:   NewCameraState(engine.Vec3{X: 10, Y: 7, Z: -3}, 0.6, -0.2, 1.1, 16.0/9),
		Atmos: atmosphere.Compute(10, 0, atmosphere.Summer), Look: atmosphere.LookFor(atmosphere.Summer),
		Quality: engine.QualityFor("high"), WaterPlane: math.NaN(), OpenAir: 1,
	}
}

func TestFrameLayoutSize(t *testing.T) {
	if FrameFloats != 176 || frameSlots*4 != len(FrameData{}) {
		t.Fatalf("uniform block size %d", FrameFloats)
	}
	if slotLightC+MaxLights != frameSlots {
		t.Fatal("slot table must tile the block exactly")
	}
}

func TestBuildMainPassValues(t *testing.T) {
	s := testScene()
	s.Rain, s.Cloud = 0.7, 0.5
	f := s.Build(PassMain, 1280, 720)
	if f[slotCam*4+3] != 12.5 || f[slotCam*4] != 10 {
		t.Fatal("camera/time slot")
	}
	if f[slotRes*4] != 1280 || f[slotRes*4+1] != 720 || math.Abs(float64(f[slotRes*4+2])-1.0/1280) > 1e-9 {
		t.Fatal("resolution slot")
	}
	if f[slotWind*4+3] != 0.7 || f[slotParams*4+2] != 0.5 {
		t.Fatal("rain/cloud slots")
	}
	if f[slotMisc*4+1] != 0 {
		t.Fatal("reflection must be off without a water plane")
	}
	s.WaterPlane = 3
	if s.Build(PassMain, 1, 1)[slotMisc*4+1] != 1 {
		t.Fatal("reflection should be on with a water plane at high quality")
	}
	s.Quality = engine.QualityFor("low")
	g := s.Build(PassMain, 1, 1)
	if g[slotMisc*4+1] != 0 || g[slotSunCol*4+3] != 0 {
		t.Fatal("low quality: no planar reflection, no shadows")
	}
}

func TestStormDarkensSun(t *testing.T) {
	clear, storm := testScene(), testScene()
	storm.Cloud = 1
	a := clear.Build(PassMain, 1, 1)
	b := storm.Build(PassMain, 1, 1)
	if b[slotSunDir*4+3] >= a[slotSunDir*4+3] || b[slotSunCol*4] >= a[slotSunCol*4] {
		t.Fatal("overcast sky must dim sunlight")
	}
}

func TestUnderwaterOnlyInMainPass(t *testing.T) {
	s := testScene()
	s.Underwater = true
	if s.Build(PassMain, 1, 1)[slotWater*4] != 1 || s.Build(PassShadow, 1, 1)[slotWater*4] != 0 {
		t.Fatal("underwater flag")
	}
}

func TestReflectionPassKeepsPointsOnPlaneFixed(t *testing.T) {
	s := testScene()
	s.WaterPlane = 3
	main := s.Build(PassMain, 1, 1)
	refl := s.Build(PassReflect, 1, 1)
	var vpM, vpR engine.Mat4
	copy(vpM[:], main[:16])
	copy(vpR[:], refl[:16])
	for _, p := range []engine.Vec3{{X: 20, Y: 3, Z: 4}, {X: 15, Y: 3, Z: 12}, {X: 30, Y: 3, Z: -8}} {
		x1, y1, _ := project(vpM, p)
		x2, y2, _ := project(vpR, p)
		if math.Abs(x1-x2) > 1e-5 || math.Abs(y1-y2) > 1e-5 {
			t.Fatalf("a point on the water plane must project identically in the mirrored view: (%v,%v) vs (%v,%v)", x1, y1, x2, y2)
		}
	}
	// a point above the plane maps to where its mirror image below would appear
	above := engine.Vec3{X: 20, Y: 13, Z: 4}
	below := engine.Vec3{X: 20, Y: -7, Z: 4}
	xa, ya, _ := project(vpR, above)
	xb, yb, _ := project(vpM, below)
	if math.Abs(xa-xb) > 1e-5 || math.Abs(ya-yb) > 1e-5 {
		t.Fatal("mirrored view must show the reflected world")
	}
	if refl[slotMisc*4+2] > 3 || refl[slotMisc*4+2] < 2.5 {
		t.Fatalf("clip plane should sit at the water surface: %v", refl[slotMisc*4+2])
	}
	if main[slotMisc*4+2] > -1e8 {
		t.Fatal("main pass must not clip")
	}
	if refl[slotCam*4+1] != float32(2*3-7) {
		t.Fatalf("mirrored camera height: %v", refl[slotCam*4+1])
	}
}

func TestShadowMatrixCoversAndIsStable(t *testing.T) {
	sun := engine.Vec3{X: 0.3, Y: 0.8, Z: -0.2}.Norm()
	center := engine.Vec3{X: 100, Y: 5, Z: 40}
	vp := LightMatrix(sun, center, 80, 2048)
	x, y, _ := project(vp, center)
	if math.Abs(x) > 0.02 || math.Abs(y) > 0.02 {
		t.Fatalf("centre should map to the middle of the shadow map: %v,%v", x, y)
	}
	for _, p := range []engine.Vec3{{X: 150, Y: 0, Z: 40}, {X: 100, Y: 40, Z: 90}, {X: 60, Y: 20, Z: 10}} {
		px, py, _ := project(vp, p)
		if math.Abs(px) > 1 || math.Abs(py) > 1 {
			t.Fatalf("point within the radius falls outside the shadow map: %v,%v", px, py)
		}
	}
	// sub-texel camera motion must not move the shadow map (no shimmering)
	texel := 160.0 / 2048
	a := LightMatrix(sun, center, 80, 2048)
	b := LightMatrix(sun, center.Add(engine.Vec3{X: texel * 0.2}), 80, 2048)
	same := true
	for i := range a {
		if math.Abs(float64(a[i]-b[i])) > 1e-4 {
			same = false
		}
	}
	if !same {
		t.Fatal("shadow matrix must snap to texels")
	}
	c := LightMatrix(sun, center.Add(engine.Vec3{X: texel * 3}), 80, 2048)
	if a == c {
		t.Fatal("moving several texels must move the map")
	}
}

func TestSelectLightsNearestFirstAndCapped(t *testing.T) {
	z := zones.NewAtlas(1, func(x, z float64) float64 { return 0 }).Find(zones.Station, 0, 0)
	lm := zones.Build(z)
	if len(lm.Lights) <= MaxLights {
		t.Skip("station has few lights")
	}
	cam := engine.Vec3{X: z.X, Y: z.BaseY + 2, Z: z.Z}
	got := SelectLights([]*zones.Landmark{lm}, cam, nil)
	if len(got) != MaxLights {
		t.Fatalf("expected %d lights, got %d", MaxLights, len(got))
	}
	far := SelectLights([]*zones.Landmark{lm}, engine.Vec3{X: z.X + 5000}, nil)
	if len(far) != 0 {
		t.Fatal("distant landmarks contribute no lights")
	}
	extra := SelectLights(nil, cam, []Light{{Pos: cam, Radius: 5}})
	if len(extra) != 1 {
		t.Fatal("extra lights are included")
	}
}

func TestSunScreen(t *testing.T) {
	s := testScene()
	s.Atmos = atmosphere.Compute(12, 0, atmosphere.Summer)
	// look straight at the sun
	sd := engine.Vec3{X: s.Atmos.SunDir[0], Y: s.Atmos.SunDir[1], Z: s.Atmos.SunDir[2]}
	yaw := math.Atan2(sd.Z, sd.X)
	pitch := math.Asin(sd.Y)
	s.Cam = NewCameraState(engine.Vec3{}, yaw, pitch, 1.1, 1.6)
	u, v, vis := s.SunScreen(1.6)
	if math.Abs(u-0.5) > 0.01 || math.Abs(v-0.5) > 0.01 || vis < 0.5 {
		t.Fatalf("sun should be centred: %v %v %v", u, v, vis)
	}
	s.Cam = NewCameraState(engine.Vec3{}, yaw+math.Pi, -pitch, 1.1, 1.6)
	if _, _, vis := s.SunScreen(1.6); vis != 0 {
		t.Fatal("sun behind camera must be invisible")
	}
	s.Atmos = atmosphere.Compute(0, 0, atmosphere.Summer)
	if _, _, vis := s.SunScreen(1.6); vis != 0 {
		t.Fatal("no sun at midnight")
	}
}
