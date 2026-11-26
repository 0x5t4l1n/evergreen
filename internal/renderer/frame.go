// Package renderer turns the simulation state into WebGL2 draw calls. This
// file is platform independent: it builds the per-pass uniform block, the
// shadow and reflection matrices and the light list, so all of it is unit
// testable without a GPU.
package renderer

import (
	"math"
	"sort"

	"evergreen/internal/atmosphere"
	"evergreen/internal/engine"
	"evergreen/internal/mesh"
	"evergreen/internal/particles"
	"evergreen/internal/world"
	"evergreen/internal/zones"
)

// Uniform block layout (vec4 slots). Must match shaders/common.glsl.
const (
	slotVP      = 0
	slotLightVP = 4
	slotCam     = 8
	slotSunDir  = 9
	slotSunCol  = 10
	slotSkyTop  = 11
	slotSkyHor  = 12
	slotFogCol  = 13
	slotAmb     = 14
	slotFogP    = 15
	slotWind    = 16
	slotSeason  = 17
	slotParams  = 18
	slotMoon    = 19
	slotRes     = 20
	slotWater   = 21
	slotZone    = 22
	slotCamR    = 23
	slotCamU    = 24
	slotCamF    = 25
	slotGrade   = 26
	slotMisc    = 27
	slotLights  = 28
	slotLightC  = 36
	frameSlots  = 44
	// FrameFloats is the number of float32 values in the uniform block.
	FrameFloats = frameSlots * 4
)

// MaxLights is the number of dynamic point lights the shaders support.
const MaxLights = 8

// FrameData is the CPU copy of the uniform block.
type FrameData [FrameFloats]float32

func (f *FrameData) vec(slot int, x, y, z, w float64) {
	i := slot * 4
	f[i], f[i+1], f[i+2], f[i+3] = float32(x), float32(y), float32(z), float32(w)
}

func (f *FrameData) mat(slot int, m engine.Mat4) { copy(f[slot*4:slot*4+16], m[:]) }

// Light is a world-space point light.
type Light struct {
	Pos     engine.Vec3
	Color   [3]float32
	Radius  float64
	Flicker float64
}

// Scene is everything the renderer needs for one frame.
type Scene struct {
	Time        float64
	Cam         CameraState
	Atmos       atmosphere.State
	Look        atmosphere.SeasonLook
	Rain        float64
	Wind        [2]float64 // wind vector (x,z), already scaled
	Gust        float64
	Cloud       float64
	Flash       float64
	Wetness     float64
	FogDensity  float64
	Lights      []Light
	Underwater  bool
	WaterPlane  float64 // NaN when no planar reflection is wanted
	RippleOX    float64
	RippleOZ    float64
	RippleSpan  float64
	ZoneID      float64
	ZoneBlend   float64
	StationMode float64
	Stargaze    float64
	GroundRef   float64
	OpenAir     float64
	Focus       float64
	Quality     engine.Quality

	Chunks    []*world.Chunk
	Landmarks []*zones.Landmark
	Animals   []AnimalBatch
	Plan      particles.Plan
	CPUParts  []float32 // 8 floats per particle
	Ripples   []uint16  // half-float height field, row major, RippleN*RippleN
	RippleN   int
}

// AnimalBatch is one species worth of instances.
type AnimalBatch struct {
	Mesh *mesh.Mesh
	Data []float32 // 8 floats per instance
}

// CameraState is the view of the camera needed for rendering.
type CameraState struct {
	Pos            engine.Vec3
	Dir, Right, Up engine.Vec3
	FOV, Aspect    float64
	View, Proj     engine.Mat4
}

// NewCameraState derives matrices and basis vectors from a pose.
func NewCameraState(pos engine.Vec3, yaw, pitch, fov, aspect float64) CameraState {
	cp := math.Cos(pitch)
	dir := engine.Vec3{X: cp * math.Cos(yaw), Y: math.Sin(pitch), Z: cp * math.Sin(yaw)}
	right := dir.Cross(engine.Vec3{Y: 1}).Norm()
	up := right.Cross(dir).Norm()
	return CameraState{
		Pos: pos, Dir: dir, Right: right, Up: up, FOV: fov, Aspect: aspect,
		View: engine.LookAt(pos, pos.Add(dir), engine.Vec3{Y: 1}),
		Proj: engine.Perspective(fov, aspect, 0.15, 3200),
	}
}

// Pass selects which view a uniform block describes.
type Pass int

// Render passes.
const (
	PassMain Pass = iota
	PassShadow
	PassReflect
)

// LightMatrix builds the sun's orthographic view-projection, snapped to shadow
// texels so shadows do not shimmer when the camera moves.
func LightMatrix(sunDir, center engine.Vec3, radius float64, res int) engine.Mat4 {
	l := sunDir.Norm()
	up := engine.Vec3{Y: 1}
	if math.Abs(l.Y) > 0.98 {
		up = engine.Vec3{X: 1}
	}
	right := l.Scale(-1).Cross(up).Norm()
	u := right.Cross(l.Scale(-1)).Norm()
	if res > 0 {
		texel := 2 * radius / float64(res)
		cx, cy := center.Dot(right), center.Dot(u)
		sx, sy := math.Floor(cx/texel)*texel, math.Floor(cy/texel)*texel
		center = center.Sub(right.Scale(cx - sx)).Sub(u.Scale(cy - sy))
	}
	eye := center.Add(l.Scale(radius * 2.2))
	view := engine.LookAt(eye, center, up)
	proj := engine.Ortho(-radius, radius, -radius, radius, 0.5, radius*4.4)
	return proj.Mul(view)
}

// ReflectionMatrix mirrors the world about the horizontal plane y=plane.
func ReflectionMatrix(plane float64) engine.Mat4 {
	m := engine.Identity()
	m[5] = -1
	m[13] = float32(2 * plane)
	return m
}

// ShadowCenter is the point the shadow volume is centred on: pushed forward
// along the view so more of the shadow map covers what is on screen.
func ShadowCenter(cam CameraState, radius float64) engine.Vec3 {
	return cam.Pos.Add(engine.Vec3{X: cam.Dir.X, Z: cam.Dir.Z}.Norm().Scale(radius * 0.35))
}

// SelectLights picks the nearest point lights to the camera.
func SelectLights(lms []*zones.Landmark, cam engine.Vec3, extra []Light) []Light {
	var all []Light
	for _, lm := range lms {
		if math.Hypot(cam.X-lm.Zone.X, cam.Z-lm.Zone.Z) > lm.Radius+150 {
			continue
		}
		for _, li := range lm.Lights {
			x, y, z := lm.World(li.U, li.Y, li.V)
			all = append(all, Light{Pos: engine.Vec3{X: x, Y: y, Z: z}, Color: [3]float32{li.R, li.G, li.B}, Radius: li.Radius, Flicker: li.Flicker})
		}
	}
	all = append(all, extra...)
	// prefer lights whose radius actually reaches the camera surroundings
	score := func(l Light) float64 {
		d := l.Pos.Sub(cam).Len()
		return d - l.Radius*0.6
	}
	sort.SliceStable(all, func(i, j int) bool { return score(all[i]) < score(all[j]) })
	if len(all) > MaxLights {
		all = all[:MaxLights]
	}
	return all
}

// Build fills the uniform block for a pass.
func (s *Scene) Build(pass Pass, sceneW, sceneH int) FrameData {
	var f FrameData
	c := s.Cam
	q := s.Quality
	a := s.Atmos
	shadowOn := q.ShadowRes > 0
	view := c.View
	proj := c.Proj
	right, up, fwd := c.Right, c.Up, c.Dir
	camPos := c.Pos
	clipY := -1e9
	lightVP := engine.Identity()
	if shadowOn {
		sd := engine.Vec3{X: a.SunDir[0], Y: a.SunDir[1], Z: a.SunDir[2]}
		lightVP = LightMatrix(sd, ShadowCenter(c, q.ShadowDist), q.ShadowDist, q.ShadowRes)
	}
	vp := proj.Mul(view)
	switch pass {
	case PassShadow:
		vp = lightVP
	case PassReflect:
		m := ReflectionMatrix(s.WaterPlane)
		vp = proj.Mul(view.Mul(m))
		right = engine.Vec3{X: right.X, Y: -right.Y, Z: right.Z}
		up = engine.Vec3{X: up.X, Y: -up.Y, Z: up.Z}
		fwd = engine.Vec3{X: fwd.X, Y: -fwd.Y, Z: fwd.Z}
		camPos = engine.Vec3{X: camPos.X, Y: 2*s.WaterPlane - camPos.Y, Z: camPos.Z}
		clipY = s.WaterPlane - 0.05
	}
	f.mat(slotVP, vp)
	f.mat(slotLightVP, lightVP)
	f.vec(slotCam, camPos.X, camPos.Y, camPos.Z, s.Time)
	f.vec(slotSunDir, a.SunDir[0], a.SunDir[1], a.SunDir[2], a.SunIntensity*(1-0.55*s.Cloud*s.Cloud))
	shadowStrength := 0.0
	if shadowOn {
		shadowStrength = 0.92 - 0.5*s.Cloud
	}
	darken := 1 - 0.45*s.Cloud
	f.vec(slotSunCol, a.SunColor[0]*darken, a.SunColor[1]*darken, a.SunColor[2]*darken, shadowStrength)
	f.vec(slotSkyTop, a.SkyTop[0]*darken, a.SkyTop[1]*darken, a.SkyTop[2]*darken, 0)
	gray := func(v [3]float64, k float64) [3]float64 {
		l := 0.3*v[0] + 0.59*v[1] + 0.11*v[2]
		return [3]float64{v[0] + (l*0.8-v[0])*k, v[1] + (l*0.8-v[1])*k, v[2] + (l*0.8-v[2])*k}
	}
	hor := gray(a.SkyHorizon, s.Cloud*0.6)
	f.vec(slotSkyHor, hor[0]*darken, hor[1]*darken, hor[2]*darken, 0)
	fog := gray(a.Fog, s.Cloud*0.5)
	f.vec(slotFogCol, fog[0]*darken, fog[1]*darken, fog[2]*darken, s.Wetness)
	f.vec(slotAmb, a.Ambient[0]*darken, a.Ambient[1]*darken, a.Ambient[2]*darken, s.OpenAir)
	f.vec(slotFogP, s.FogDensity, 0.045, s.GroundRef, 0.14)
	f.vec(slotWind, s.Wind[0], s.Wind[1], s.Gust, s.Rain)
	f.vec(slotSeason, s.Look.Blend, s.Look.Autumn, s.Look.Snow, s.Look.Spring)
	f.vec(slotParams, a.MoonPhase, a.StarVis, s.Cloud, s.Flash)
	f.vec(slotMoon, a.MoonDir[0], a.MoonDir[1], a.MoonDir[2], a.Night)
	f.vec(slotRes, float64(sceneW), float64(sceneH), 1/float64(sceneW), 1/float64(sceneH))
	under := 0.0
	if s.Underwater && pass == PassMain {
		under = 1
	}
	plane := s.WaterPlane
	if math.IsNaN(plane) {
		plane = -1e9
	}
	f.vec(slotWater, under, plane, s.RippleOX, s.RippleOZ)
	f.vec(slotZone, s.ZoneID, s.ZoneBlend, s.StationMode, s.Stargaze)
	tanY := math.Tan(c.FOV / 2)
	f.vec(slotCamR, right.X, right.Y, right.Z, tanY*c.Aspect)
	f.vec(slotCamU, up.X, up.Y, up.Z, tanY)
	f.vec(slotCamF, fwd.X, fwd.Y, fwd.Z, s.RippleSpan)
	f.vec(slotGrade, s.Look.Grade[0], s.Look.Grade[1], s.Look.Grade[2], s.Look.Saturation)
	reflect := 0.0
	if pass == PassMain && q.Reflection > 0 && !math.IsNaN(s.WaterPlane) {
		reflect = 1
	}
	f.vec(slotMisc, math.Max(30, q.GrassChunks*64*0.85), reflect, clipY, 7)
	for i := 0; i < MaxLights; i++ {
		if i < len(s.Lights) {
			l := s.Lights[i]
			f.vec(slotLights+i, l.Pos.X, l.Pos.Y, l.Pos.Z, l.Radius)
			f.vec(slotLightC+i, float64(l.Color[0]), float64(l.Color[1]), float64(l.Color[2]), l.Flicker)
		}
	}
	return f
}

// SunScreen projects the sun to normalised screen coordinates. visibility is
// 0 when the sun is behind the camera or below the horizon.
func (s *Scene) SunScreen(sceneAspect float64) (u, v, visibility float64) {
	a := s.Atmos
	sd := engine.Vec3{X: a.SunDir[0], Y: a.SunDir[1], Z: a.SunDir[2]}
	if a.SunIntensity < 0.05 || sd.Dot(s.Cam.Dir) <= 0.02 {
		return 0.5, 0.5, 0
	}
	p := s.Cam.Pos.Add(sd.Scale(1000))
	vp := s.Cam.Proj.Mul(s.Cam.View)
	x := float64(vp[0])*p.X + float64(vp[4])*p.Y + float64(vp[8])*p.Z + float64(vp[12])
	y := float64(vp[1])*p.X + float64(vp[5])*p.Y + float64(vp[9])*p.Z + float64(vp[13])
	w := float64(vp[3])*p.X + float64(vp[7])*p.Y + float64(vp[11])*p.Z + float64(vp[15])
	if w <= 0 {
		return 0.5, 0.5, 0
	}
	u, v = x/w*0.5+0.5, y/w*0.5+0.5
	edge := math.Max(math.Abs(u-0.5), math.Abs(v-0.5))
	visibility = (1 - math.Min(1, math.Max(0, (edge-0.5)/0.9))) * a.SunIntensity
	return
}
