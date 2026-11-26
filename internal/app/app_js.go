//go:build js && wasm

package app

import (
	"fmt"
	"math"
	"strings"
	"syscall/js"
	"time"

	"evergreen/internal/atmosphere"
	"evergreen/internal/audio"
	"evergreen/internal/camera"
	"evergreen/internal/ecosystem"
	"evergreen/internal/engine"
	"evergreen/internal/github"
	"evergreen/internal/particles"
	"evergreen/internal/procedural"
	"evergreen/internal/renderer"
	"evergreen/internal/water"
	"evergreen/internal/weather"
	"evergreen/internal/wildlife"
	"evergreen/internal/world"
	"evergreen/internal/zones"
)

const (
	rippleN    = 96
	rippleCell = 0.5
)

// App is the whole running experience.
type App struct {
	win, canvas js.Value
	cfg         engine.Config
	saved       Saved
	input       *inputState
	funcs       []js.Func

	r     *renderer.Renderer
	w     *world.World
	cam   *camera.Camera
	clock atmosphere.Clock
	wx    *weather.Weather
	eco   *ecosystem.Manager
	rip   *water.Ripples
	cpu   *particles.System
	aud   *audio.Engine
	adapt *engine.Adaptive
	q     engine.Quality

	season     atmosphere.Season
	look       atmosphere.SeasonLook
	lookTarget atmosphere.SeasonLook
	atmos      atmosphere.State
	activity   *github.File
	influence  github.Influence

	lastT        float64
	started      bool
	needResize   bool
	paused       bool
	frameMs      float64
	fps          float64
	fpsFrames    int
	fpsTimer     float64
	hudTimer     float64
	saveTimer    float64
	waterTimer   float64
	waterPlane   float64
	waterDist    float64
	stargaze     float64
	stargazeOn   bool
	zoneID       zones.ID
	zoneBlend    float64
	prompt       string
	interactions *zones.Landmark
	cpuBuf       []float32
	hud          HUD
	lastX, lastZ float64
	boltPending  bool
	zoneSeen     map[zones.ID]float64
	lightExtra   []renderer.Light
	loadingDone  bool
}

// Run builds the application and starts the render loop. It never returns
// while the page is open.
func Run() {
	a := &App{win: js.Global(), input: newInput(), zoneSeen: map[zones.ID]float64{}, waterPlane: math.NaN()}
	if err := a.init(); err != nil {
		uiCall("fatal", err.Error())
		return
	}
	a.exposeAPI()
	a.requestFrame()
	select {}
}

func (a *App) init() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("startup failed: %v", r)
		}
	}()
	doc := a.win.Get("document")
	a.canvas = doc.Call("getElementById", "evergreen-canvas")
	if !a.canvas.Truthy() {
		return fmt.Errorf("canvas element #evergreen-canvas not found")
	}
	saved, errs := ParseSaved([]byte(storageGet()))
	if storageGet() == "" && prefersReducedMotion() {
		saved.Config.ReducedMotion = true
	}
	a.saved = saved
	a.cfg = saved.Config
	for _, e := range errs {
		uiCall("log", "settings repaired: "+e.Error())
	}
	a.adapt = engine.NewAdaptive()
	a.adapt.Enabled = a.cfg.AutoQuality
	a.q = a.adapt.Effective(a.qualityBase())

	r, err := renderer.New(a.canvas)
	if err != nil {
		return err
	}
	a.r = r
	if err := a.resize(); err != nil {
		return err
	}
	a.buildWorld(true)
	a.aud = audio.NewEngine(procedural.SeedFromString(a.cfg.Seed))
	a.aud.SetVolume(a.cfg.Volume)
	a.aud.SetMuted(a.cfg.Muted)
	a.rip = water.NewRipples(rippleN, rippleCell, 7)
	a.cpu = particles.New(6000, 3)
	a.applyTime()
	a.applySeason(true)
	a.bindInput()
	if a.cfg.Github != "" {
		a.loadActivity(false)
	}
	return nil
}

func (a *App) qualityBase() engine.Quality {
	q := engine.QualityFor(a.cfg.Preset)
	if a.cfg.DrawDistance >= 2 {
		q.ViewChunks = int(a.cfg.DrawDistance)
	}
	if a.cfg.ReducedMotion {
		q.ParticleScale *= 0.6
	}
	return q
}

func (a *App) numericSeed() uint64 { return procedural.SeedFromString(a.cfg.Seed) }

func (a *App) worldConfig() world.Config {
	return world.Config{ViewChunks: a.q.ViewChunks, GrassChunks: a.q.GrassChunks, Density: a.q.Density, Influence: a.influence}
}

// buildWorld (re)creates the world for the current seed.
func (a *App) buildWorld(initial bool) {
	if a.w != nil {
		for _, c := range a.w.Chunks {
			a.r.ReleaseChunk(c)
		}
		for _, lm := range a.w.Landmarks {
			a.r.ReleaseLandmark(lm)
		}
		a.eco.Clear()
	}
	seed := a.numericSeed()
	a.influence = github.None()
	a.w = world.New(seed, a.worldConfig())
	a.wx = weather.New(seed)
	a.eco = ecosystem.New(seed)
	sx, sz, yaw := a.w.Atlas().Spawn()
	if initial && !(a.saved.X == 0 && a.saved.Z == 0) {
		sx, sz, yaw = a.saved.X, a.saved.Z, a.saved.Yaw
	}
	a.w.Preload(sx, sz, 2, 1200*time.Millisecond)
	if a.cam == nil {
		a.cam = camera.New(a.w, sx, sz, yaw)
	} else {
		a.cam.Reset(a.w, sx, sz, yaw)
	}
	a.cam.ReducedMotion = a.cfg.ReducedMotion
	a.cam.SetMode(camera.ParseMode(a.cfg.CameraMode), a.w)
	a.applyWeather()
}

func (a *App) resize() error {
	dpr := a.win.Get("devicePixelRatio").Float()
	if dpr <= 0 || math.IsNaN(dpr) {
		dpr = 1
	}
	dpr = math.Min(dpr, 2)
	if a.q.RenderScale < 0.8 {
		dpr = math.Min(dpr, 1.25)
	}
	w, h := a.win.Get("innerWidth").Int(), a.win.Get("innerHeight").Int()
	a.needResize = false
	return a.r.Resize(w, h, dpr, math.Max(0.4, math.Min(1.25, a.q.RenderScale)))
}

func (a *App) applyTime() {
	a.clock.Speed = a.cfg.TimeSpeed
	a.clock.SyncLocal = a.cfg.TimeMode == "local"
	switch a.cfg.TimeMode {
	case "manual":
		a.clock.Hours = a.cfg.TimeOfDay
		a.clock.Paused = true
	default:
		if a.cfg.TimeMode == "cycle" && a.clock.Paused {
			a.clock.Paused = false
		}
		if a.clock.Hours == 0 && !a.clock.SyncLocal && a.started == false {
			a.clock.Hours = a.cfg.TimeOfDay
		}
	}
	if a.paused {
		a.clock.Paused = true
	}
}

func (a *App) applyWeather() {
	if k, ok := weather.Parse(a.cfg.Weather); ok {
		a.wx.Force(k, true)
	} else {
		a.wx.Force(a.wx.Kind, false)
	}
}

func (a *App) applySeason(snap bool) {
	y, mo, d, _, _, _ := nowLocal()
	_ = y
	_ = d
	now := time.Date(2026, time.Month(mo), 15, 12, 0, 0, 0, time.UTC)
	a.season = atmosphere.ParseSeason(a.cfg.Season, now)
	a.lookTarget = atmosphere.LookFor(a.season)
	if snap || a.look.Grade == (atmosphere.Vec{}) {
		a.look = a.lookTarget
	}
}

// exec applies one UI command.
func (a *App) exec(name, value string) string {
	switch name {
	case "pause":
		a.paused = value == "true" || (value == "toggle" && !a.paused)
		a.clock.Paused = a.paused || a.cfg.TimeMode == "manual"
		return ""
	case "resetCamera":
		sx, sz, yaw := a.w.Atlas().Spawn()
		a.cam.Reset(a.w, sx, sz, yaw)
		a.cam.SetMode(camera.ParseMode(a.cfg.CameraMode), a.w)
		return ""
	case "zone":
		var id int
		fmt.Sscanf(value, "%d", &id)
		return a.gotoZone(zones.ID(id))
	case "screenshot":
		a.r.Capture = true
		a.r.CaptureCB = func(url string) { download(url, "evergreen-"+time.Now().Format("20060102-150405")+".png") }
		return ""
	case "audioStart":
		a.userGesture()
		return ""
	case "stargaze":
		a.stargazeOn = value == "true"
		return ""
	}
	prevPreset, prevSeed, prevGithub := a.cfg.Preset, a.cfg.Seed, a.cfg.Github
	eff, err := ApplyCommand(&a.cfg, name, value)
	if err != nil {
		return err.Error()
	}
	if eff&EffQuality != 0 {
		a.adapt.Enabled = a.cfg.AutoQuality
		a.q = a.adapt.Effective(a.qualityBase())
		a.cam.ReducedMotion = a.cfg.ReducedMotion
		_ = a.resize()
		if a.cfg.Preset != prevPreset {
			a.w.SetConfig(a.worldConfig())
		}
	}
	if eff&EffTime != 0 {
		a.applyTime()
	}
	if eff&EffWeather != 0 {
		a.applyWeather()
	}
	if eff&EffSeason != 0 {
		a.applySeason(false)
	}
	if eff&EffCamera != 0 {
		a.cam.ReducedMotion = a.cfg.ReducedMotion
		a.cam.SetMode(camera.ParseMode(a.cfg.CameraMode), a.w)
		a.input.dragging = false
		if a.cfg.CameraMode != "explore" {
			if doc := a.win.Get("document"); doc.Get("pointerLockElement").Truthy() {
				doc.Call("exitPointerLock")
			}
		}
	}
	if eff&EffAudio != 0 {
		a.aud.SetVolume(a.cfg.Volume)
		a.aud.SetMuted(a.cfg.Muted)
	}
	if name == "seed" && a.cfg.Seed != prevSeed {
		a.saved.X, a.saved.Z = 0, 0
		a.buildWorld(false)
		uiCall("toast", "New world: seed "+a.cfg.Seed)
	}
	if name == "github" && a.cfg.Github != prevGithub {
		a.influence = github.None()
		a.w.SetConfig(a.worldConfig())
		if a.cfg.Github != "" {
			a.loadActivity(true)
		}
	}
	a.save()
	return ""
}

func (a *App) gotoZone(id zones.ID) string {
	if id < 1 || id > zones.NumZones {
		return "unknown zone"
	}
	cx, cz := a.cam.Pos.X, a.cam.Pos.Z
	z := a.w.Atlas().Find(id, cx, cz)
	x, zz, yaw := ArrivalPoint(z)
	a.w.Preload(x, zz, 2, 700*time.Millisecond)
	for i := 0; i < 4; i++ {
		a.w.Update(x, zz, a.lastT, 30*time.Millisecond)
	}
	if a.cfg.CameraMode == "ambient" || a.cfg.CameraMode == "cinematic" || a.cfg.CameraMode == "scroll" {
		a.cfg.CameraMode = "explore"
		a.cam.SetMode(camera.Explore, a.w)
		uiCall("mode", "explore")
	}
	a.cam.Teleport(a.w, x, zz, yaw)
	a.cam.Pitch = -0.05
	return ""
}

func (a *App) loadActivity(announce bool) {
	user := a.cfg.Github
	if user == "" {
		return
	}
	fetchText("data/activity.json", func(body string, err error) {
		if err != nil {
			if announce {
				uiCall("toast", "No GitHub activity file found; the forest stays procedural")
			}
			return
		}
		f, errs := github.Parse([]byte(body))
		if f == nil {
			if announce {
				uiCall("toast", "GitHub activity file is invalid")
			}
			uiCall("log", fmt.Sprint(errs))
			return
		}
		a.activity = f
		u, ok := f.Lookup(user)
		if !ok {
			if announce {
				uiCall("toast", "No activity recorded for "+user+" yet; add them to the workflow")
			}
			return
		}
		a.influence = u.Influence()
		a.w.SetConfig(a.worldConfig())
		uiCall("toast", fmt.Sprintf("GitHub: %d repositories grew into trees", len(u.Repos)))
	})
}

func (a *App) userGesture() {
	if a.aud == nil || a.aud.Running() {
		return
	}
	if err := a.aud.Start(); err != nil {
		uiCall("toast", "Audio unavailable: "+err.Error())
		return
	}
	a.aud.SetVolume(a.cfg.Volume)
	a.aud.SetMuted(a.cfg.Muted)
	uiCall("audio", true)
}

func (a *App) save() {
	if a.cam == nil {
		return
	}
	a.saved.Config = a.cfg
	a.saved.X, a.saved.Z, a.saved.Yaw = a.cam.Pos.X, a.cam.Pos.Z, a.cam.Yaw
	storageSet(string(a.saved.Marshal()))
}

func (a *App) requestFrame() {
	var cb js.Func
	cb = js.FuncOf(func(this js.Value, args []js.Value) any {
		a.frame(args[0].Float() / 1000)
		a.win.Call("requestAnimationFrame", cb)
		return nil
	})
	a.funcs = append(a.funcs, cb)
	a.win.Call("requestAnimationFrame", cb)
}

// ---- per-frame ---------------------------------------------------------------------

func (a *App) frame(t float64) {
	defer func() {
		if r := recover(); r != nil {
			uiCall("log", fmt.Sprintf("frame error: %v", r))
		}
	}()
	dt := 0.016
	if a.lastT > 0 {
		dt = math.Max(0, math.Min(0.1, t-a.lastT))
	}
	a.lastT = t
	a.frameMs = a.frameMs*0.9 + dt*1000*0.1
	if a.r.Lost() {
		return
	}
	if a.r.NeedsReinit() {
		a.r.DropAllChunks(a.w.Chunks)
		uiCall("toast", "Graphics context restored")
		a.needResize = true
		a.r.Resize(1, 1, 1, 1)
	}
	if a.needResize {
		_ = a.resize()
	}
	if a.adapt.Observe(dt * 1000) {
		a.q = a.adapt.Effective(a.qualityBase())
		a.w.SetConfig(a.worldConfig())
		_ = a.resize()
	}
	if !a.paused {
		a.update(dt, t)
	}
	a.input.endFrame()
	a.render(t)
	if !a.loadingDone && a.w.ReadyChunks() >= 6 {
		a.loadingDone = true
		a.started = true
		uiCall("ready")
	}
	a.fpsFrames++
	a.fpsTimer += dt
	if a.fpsTimer >= 0.5 {
		a.fps = float64(a.fpsFrames) / a.fpsTimer
		a.fpsFrames, a.fpsTimer = 0, 0
	}
	a.hudTimer += dt
	if a.hudTimer > 0.25 {
		a.hudTimer = 0
		a.pushHUD()
	}
	a.saveTimer += dt
	if a.saveTimer > 6 {
		a.saveTimer = 0
		a.save()
	}
}

func (a *App) update(dt, t float64) {
	in := a.input.consume(float64(a.r.Stats.SceneW), float64(a.r.Stats.SceneH))
	// walking while in an automatic mode takes control
	if (a.cfg.CameraMode == "ambient" || a.cfg.CameraMode == "cinematic") && (in.Move != 0 || in.Strafe != 0 || in.Jump) {
		a.cfg.CameraMode = "explore"
		a.cam.SetMode(camera.Explore, a.w)
		uiCall("mode", "explore")
	}
	a.handleInteraction()
	a.updateClock(dt)
	a.wx.Season = weather.RainBias(a.look.RainBias)
	a.wx.Update(dt)
	k := math.Min(1, dt*0.5)
	a.look = atmosphere.BlendLook(a.look, a.lookTarget, k)
	if a.stargazeOn && a.cfg.CameraMode == "explore" {
		a.stargaze = math.Min(1, a.stargaze+dt*0.6)
	} else {
		a.stargaze = math.Max(0, a.stargaze-dt*0.8)
	}
	hours := a.clock.Hours
	if a.stargaze > 0 {
		hours += math.Remainder(23.3-hours, 24) * procedural.SmoothStep(0, 1, a.stargaze)
	}
	a.atmos = atmosphere.Compute(hours, a.clock.Days, a.season)

	if a.stargaze > 0.05 {
		md := a.atmos.MoonDir
		a.cam.Yaw += math.Remainder(math.Atan2(md[2], md[0])-a.cam.Yaw, 2*math.Pi) * math.Min(1, dt*1.5)
		a.cam.Pitch += (math.Asin(md[1]) - a.cam.Pitch) * math.Min(1, dt*1.5)
		in.LookX, in.LookY = 0, 0
		in.Move, in.Strafe = 0, 0
		in.Zoom = -200 * a.stargaze * dt * 10
	}
	a.cam.Update(dt, in, a.w)
	px, pz := a.cam.Pos.X, a.cam.Pos.Z
	a.saved.Distance += math.Hypot(px-a.lastX, pz-a.lastZ) * boolf(a.lastX != 0 || a.lastZ != 0)
	a.lastX, a.lastZ = px, pz

	a.w.Update(px, pz, t, time.Duration(a.q.GenBudgetMs*float64(time.Millisecond)))
	chunks, lms := a.w.TakeReleased()
	for _, c := range chunks {
		a.r.ReleaseChunk(c)
	}
	for _, lm := range lms {
		a.r.ReleaseLandmark(lm)
	}
	for _, lm := range a.w.Landmarks {
		lm.Update(dt)
	}

	a.updateZone(dt)
	a.updateWater(dt, px, pz)
	cond := wildlife.Conditions{
		Player: engine.Vec3{X: px, Y: a.cam.Pos.Y, Z: pz}, Rain: a.wx.Current.Rain, Night: a.atmos.Night,
		Wind: a.wx.WindSpeed(), Thunder: a.wx.Bolt, Cold: a.look.Snow,
	}
	a.eco.Update(dt, a.w, ecosystem.Settings{Cap: a.q.WildlifeCap, Season: int(a.season), Cond: cond, Day: a.atmos.Day})
	a.updateAudio(dt)
}

func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func (a *App) updateClock(dt float64) {
	y, mo, d, h, mi, s := nowLocal()
	_, _ = y, d
	a.clock.Advance(dt, time.Date(2026, time.Month(mo), 1, h, mi, s, 0, time.UTC))
}

func (a *App) handleInteraction() {
	a.prompt = ""
	a.interactions = nil
	for _, lm := range a.w.Landmarks {
		if it, ok := lm.NearestInteract(a.cam.Pos.X, a.cam.Body.Y, a.cam.Pos.Z); ok && it.Prompt != "" {
			a.prompt = it.Prompt
			a.interactions = lm
			if a.stargazeOn && it.Action == "observatory.telescope" {
				a.prompt = "Press E to stop stargazing"
			}
			if a.input.wasPressed("KeyE") {
				msg := lm.Activate(it.Action)
				if msg == "stargaze" {
					a.stargazeOn = !a.stargazeOn
					if a.stargazeOn {
						uiCall("toast", "Through the telescope: the moon, close and sharp")
					}
				} else if msg != "" {
					uiCall("toast", msg)
				}
			}
			return
		}
	}
	if a.stargazeOn {
		a.prompt = "Press E to stop stargazing"
		if a.input.wasPressed("KeyE") {
			a.stargazeOn = false
		}
	}
}

func (a *App) updateZone(dt float64) {
	z, blend, ok := a.w.ZoneAt(a.cam.Pos.X, a.cam.Pos.Z)
	if !ok {
		a.zoneID, a.zoneBlend = 0, math.Max(0, a.zoneBlend-dt)
		return
	}
	a.zoneBlend = math.Max(0, math.Min(1, blend*2.2))
	if z.ID != a.zoneID && blend > 0.18 {
		a.zoneID = z.ID
		info := zones.InfoFor(z.ID)
		if a.saved.Discover(int(z.ID)) {
			uiCall("zone", info.Name, info.Tagline, true)
		} else if a.t()-a.zoneSeen[z.ID] > 40 {
			uiCall("zone", info.Name, info.Tagline, false)
		}
		a.zoneSeen[z.ID] = a.t()
		if lore := zones.LoreFor(z.ID); len(lore) > 0 {
			uiCall("lore", lore[int(a.saved.Distance)%len(lore)])
		}
	}
}

func (a *App) t() float64 { return a.lastT }

func (a *App) updateWater(dt, px, pz float64) {
	a.waterTimer -= dt
	if a.waterTimer <= 0 {
		a.waterTimer = 0.4
		a.waterPlane, a.waterDist = math.NaN(), math.Inf(1)
		for _, r := range []float64{0, 8, 16, 28, 44, 70} {
			steps := 8
			if r == 0 {
				steps = 1
			}
			for k := 0; k < steps; k++ {
				ang := float64(k) * 2 * math.Pi / float64(steps)
				x, z := px+math.Cos(ang)*r, pz+math.Sin(ang)*r
				if lvl, ok := a.w.WaterLevel(x, z); ok {
					if r < a.waterDist {
						a.waterDist = r
						a.waterPlane = lvl
					}
				}
			}
			if !math.IsInf(a.waterDist, 1) {
				break
			}
		}
		if lvl := a.waterPlane; !math.IsNaN(lvl) && a.cam.Pos.Y < lvl+0.4 {
			a.waterPlane = math.NaN() // camera is at or under the surface: analytic water only
		}
	}
	a.rip.Recenter(px, pz)
	if a.cam.Body != nil && a.cam.Body.Depth > 0.1 && a.cfg.CameraMode == "explore" {
		sp := math.Hypot(a.cam.Body.X-a.lastX, a.cam.Body.Z-a.lastZ)
		if sp > 0.01 || a.cam.Body.Swimming {
			a.rip.Drop(a.cam.Body.X, a.cam.Body.Z, math.Min(0.45, 0.1+sp*3), 0.9)
		}
	}
	a.rip.Step(dt, a.wx.Current.Rain)
}

func (a *App) insideInfo() (cave, station bool, zoneBase float64) {
	for _, lm := range a.w.Landmarks {
		z := lm.Zone
		u, v := a.cam.Pos.X-z.X, a.cam.Pos.Z-z.Z
		switch z.ID {
		case zones.Cave:
			if math.Hypot(u, v) < zones.CaveInnerRadius+4 && a.cam.Pos.Y < z.BaseY+24 {
				cave = true
			}
			if u > 30 && u < 66 && math.Abs(v) < 8 && a.cam.Pos.Y < z.BaseY+10 {
				cave = true
			}
		case zones.Station:
			if math.Abs(u) < 12.3 && math.Abs(v) < 7.3 && a.cam.Pos.Y < z.BaseY+5.5 && a.cam.Pos.Y > z.BaseY {
				station = true
			}
		}
	}
	return
}

func (a *App) underwater() bool {
	if a.cam.Body != nil && a.cam.Body.Underwater && a.cfg.CameraMode == "explore" {
		return true
	}
	if lvl, ok := a.w.WaterLevel(a.cam.Pos.X, a.cam.Pos.Z); ok && a.cam.Pos.Y < lvl-0.05 {
		return true
	}
	return false
}

func (a *App) audioInputs() audio.Inputs {
	cave, station, _ := a.insideInfo()
	nearW := math.Max(0, 1-a.waterDist/46)
	if math.IsInf(a.waterDist, 1) {
		nearW = 0
	}
	fp := 0.0
	var fx, fy, fz float64
	if f := a.w.Landmark(zones.Falls); f != nil {
		fx, fy, fz = f.World(4, zones.FallsLowerWater+3, 0)
		d := math.Hypot(a.cam.Pos.X-fx, a.cam.Pos.Z-fz)
		fp = math.Pow(math.Max(0, 1-d/170), 1.4)
	}
	_ = fx
	_ = fy
	_ = fz
	return audio.Inputs{Zone: a.zoneID, ZoneBlend: a.zoneBlend, Rain: a.wx.Current.Rain, Wind: a.wx.WindSpeed(), Day: a.atmos.Day,
		NearWater: nearW, FallsProx: fp, Underwater: a.underwater(), InCave: cave, InStation: station, Winter: a.season == atmosphere.Winter}
}

func (a *App) listener() audio.Listener {
	d := a.cam.Dir()
	return audio.Listener{X: a.cam.Pos.X, Y: a.cam.Pos.Y, Z: a.cam.Pos.Z, FX: d.X, FY: d.Y, FZ: d.Z}
}

func (a *App) updateAudio(dt float64) {
	if !a.aud.Running() {
		return
	}
	in := a.audioInputs()
	lv := audio.Mix(in)
	var fx, fy, fz float64
	if f := a.w.Landmark(zones.Falls); f != nil {
		fx, fy, fz = f.World(4, zones.FallsLowerWater+3, 0)
	}
	ls := a.listener()
	a.aud.Update(lv, ls, fx, fy, fz)
	a.aud.Tick(dt, lv, ls)
	if a.wx.Bolt {
		a.aud.Thunder(a.wx.BoltDist)
	}
	for _, ev := range a.eco.Events {
		if ev.Kind == "croak" {
			a.aud.Croak(ev.Pos.X, ev.Pos.Y, ev.Pos.Z)
		}
	}
}

// ---- scene composition ------------------------------------------------------------------

func (a *App) render(t float64) {
	cam := a.cam
	aspect := float64(a.r.Stats.SceneW) / math.Max(1, float64(a.r.Stats.SceneH))
	if a.r.Stats.SceneH == 0 {
		aspect = float64(a.win.Get("innerWidth").Int()) / math.Max(1, float64(a.win.Get("innerHeight").Int()))
	}
	cs := renderer.NewCameraState(cam.Pos, cam.Yaw, cam.Pitch, cam.FOV, aspect)
	wind := a.wx.WindSpeed()
	wdir := 0.7 + 0.3*math.Sin(t*0.02)
	cave, station, _ := a.insideInfo()
	open := 1.0
	if cave || station {
		open = 0
	}
	ground := a.w.TerrainHeight(cam.Pos.X, cam.Pos.Z)
	viewDist := float64(a.q.ViewChunks) * 64
	fog := 2.1 / math.Max(120, viewDist) * a.wx.Visibility()
	if a.zoneID == zones.Wilderness {
		fog *= 1 + 0.5*a.zoneBlend
	}
	if a.zoneID == zones.WorldTree {
		fog *= 0.8
	}
	under := a.underwater()
	var extra []renderer.Light
	if a.atmos.Night > 0.3 && a.cfg.CameraMode == "explore" && open > 0 {
		extra = append(extra, renderer.Light{Pos: engine.Vec3{X: cam.Pos.X, Y: cam.Pos.Y + 0.4, Z: cam.Pos.Z}, Color: [3]float32{0.75, 0.55, 0.35}, Radius: 13, Flicker: 0.12})
	}
	lms := make([]*zones.Landmark, 0, len(a.w.Landmarks))
	for _, lm := range a.w.Landmarks {
		lms = append(lms, lm)
	}
	chunks := make([]*world.Chunk, 0, len(a.w.Chunks))
	for _, c := range a.w.Chunks {
		chunks = append(chunks, c)
	}
	stationMode := 0.0
	if l := a.w.Landmark(zones.Station); l != nil {
		stationMode = l.ModeValue()
	}
	glow := 0.0
	switch {
	case cave:
		glow = 1
	case a.zoneID == zones.WorldTree:
		glow = 0.9 * a.zoneBlend
	case a.zoneID == zones.Wilderness:
		glow = 0.25 * a.atmos.Night * a.zoneBlend
	}
	plan := particles.PlanFor(a.q, particles.PlanInput{
		Rain: a.wx.Current.Rain * open, Wind: wind, Day: a.atmos.Day, Season: int(a.season), LeafFall: a.look.LeafFall,
		Fog: a.wx.Current.Fog, StarsBoost: a.influence.FireflyBoost, Glow: glow, ReducedMotion: a.cfg.ReducedMotion,
	})
	if cave || station || under {
		plan.Pollen, plan.Dust, plan.Leaves = 0, 0, 0
	}
	a.updateCPUParticles(1.0/60, wind)
	var animals []renderer.AnimalBatch
	for sp := wildlife.Species(0); sp < wildlife.NumSpecies; sp++ {
		if d := a.eco.Instances(sp); len(d) > 0 {
			animals = append(animals, renderer.AnimalBatch{Mesh: wildlife.Mesh(sp), Data: d})
		}
	}
	focus := 0.0
	if a.q.DOF && (a.cfg.CameraMode == "cinematic" || a.stargaze > 0.1) {
		focus = cam.Focus
	}
	sc := &renderer.Scene{
		Time: t, Cam: cs, Atmos: a.atmos, Look: a.look, Rain: a.wx.Current.Rain * open,
		Wind: [2]float64{math.Cos(wdir) * wind, math.Sin(wdir) * wind}, Gust: a.wx.Gust(), Cloud: a.wx.Current.Cloud, Flash: a.wx.Flash() * (0.4 + 0.6*open),
		Wetness: a.wx.Current.Wetness, FogDensity: fog, Lights: renderer.SelectLights(lms, cam.Pos, extra), Underwater: under,
		WaterPlane: a.waterPlane, RippleOX: a.rip.OriginX, RippleOZ: a.rip.OriginZ, RippleSpan: a.rip.Span(),
		ZoneID: float64(a.zoneID), ZoneBlend: a.zoneBlend, StationMode: stationMode, Stargaze: a.stargaze,
		GroundRef: ground, OpenAir: open, Focus: focus, Quality: a.q,
		Chunks: chunks, Landmarks: lms, Animals: animals, Plan: plan, CPUParts: a.cpuBuf, Ripples: a.rip.Half(), RippleN: rippleN,
	}
	if a.stargaze > 0.1 {
		sc.Quality.DOF = false
	}
	if err := a.r.Render(sc); err != nil {
		uiCall("log", err.Error())
	}
}

func (a *App) updateCPUParticles(dt, wind float64) {
	var src []particles.Source
	cam := a.cam.Pos
	for _, lm := range a.w.Landmarks {
		for _, e := range lm.Emitters {
			x, y, z := lm.World(e.U, e.Y, e.V)
			if math.Hypot(cam.X-x, cam.Z-z) > 110+e.Spread || math.Abs(cam.Y-y) > 160 {
				continue
			}
			k, ok := particles.KindFromName(e.Kind)
			if !ok {
				continue
			}
			rate := e.Rate
			if k == particles.Glow || k == particles.Pollen {
				rate *= 0.4 + 0.6*a.atmos.Night
				if k == particles.Pollen {
					rate = e.Rate * a.atmos.Day
				}
			}
			src = append(src, particles.Source{Kind: k, X: x, Y: y, Z: z, Spread: e.Spread, Rate: rate})
		}
	}
	a.cpu.Update(1.0/math.Max(20, a.fps+1e-3), math.Cos(0.7)*wind, math.Sin(0.7)*wind, src, a.q.ParticleScale)
	if cap(a.cpuBuf) < a.cpu.Capacity()*8 {
		a.cpuBuf = make([]float32, a.cpu.Capacity()*8)
	}
	a.cpuBuf = a.cpuBuf[:a.cpu.Capacity()*8]
	n := a.cpu.Fill(a.cpuBuf)
	a.cpuBuf = a.cpuBuf[:n*8]
}

func (a *App) pushHUD() {
	z := ""
	if a.zoneID != 0 {
		z = zones.InfoFor(a.zoneID).Name
	}
	st := a.r.Stats
	h := HUD{
		FPS: int(a.fps + 0.5), Zone: z, ZoneID: int(a.zoneID), Phase: atmosphere.Phase(a.clock.Hours), Clock: ClockString(a.clock.Hours),
		Weather: a.wx.Kind.String(), Season: a.season.String(), Mode: a.cfg.CameraMode, Preset: a.cfg.Preset, Chunks: st.Chunks, Pending: a.w.Stats.Pending,
		Draws: st.DrawCalls, Tris: st.Triangles, Scale: math.Round(a.q.RenderScale*100) / 100, Animals: a.eco.Total(), Prompt: a.prompt, Paused: a.paused,
		Audio: a.aud.Running() && !a.aud.Muted(), Underwater: a.underwater(), Discovered: a.saved.Discovered, GithubOK: a.influence.FireflyBoost > 0 || len(a.influence.Trees) > 0,
		X: int(a.cam.Pos.X), Z: int(a.cam.Pos.Z),
	}
	a.hud = h
	uiCall("state", h.JSON())
}

// ---- JS API -----------------------------------------------------------------------------

func (a *App) exposeAPI() {
	api := js.Global().Get("Object").New()
	api.Set("command", js.FuncOf(func(this js.Value, args []js.Value) any {
		defer func() {
			if r := recover(); r != nil {
				uiCall("log", fmt.Sprintf("command error: %v", r))
			}
		}()
		if len(args) < 1 {
			return "missing command"
		}
		v := ""
		if len(args) > 1 {
			v = args[1].String()
			if args[1].Type() == js.TypeBoolean {
				v = fmt.Sprint(args[1].Bool())
			}
		}
		return a.exec(args[0].String(), v)
	}))
	api.Set("config", js.FuncOf(func(this js.Value, args []js.Value) any { return string(a.cfg.JSON()) }))
	api.Set("hud", js.FuncOf(func(this js.Value, args []js.Value) any { return a.hud.JSON() }))
	api.Set("zones", js.FuncOf(func(this js.Value, args []js.Value) any {
		var sb strings.Builder
		sb.WriteString("[")
		for i, z := range zones.All() {
			if i > 0 {
				sb.WriteString(",")
			}
			fmt.Fprintf(&sb, `{"id":%d,"key":%q,"name":%q,"tagline":%q}`, z.ID, z.Key, z.Name, z.Tagline)
		}
		sb.WriteString("]")
		return sb.String()
	}))
	js.Global().Set("evergreen", api)
	uiCall("api")
}
