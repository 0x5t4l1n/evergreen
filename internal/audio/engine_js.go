//go:build js && wasm

package audio

import (
	"math"
	"syscall/js"
	"unsafe"

	"evergreen/internal/procedural"
)

// Listener is the camera position and orientation for spatial audio.
type Listener struct {
	X, Y, Z    float64
	FX, FY, FZ float64
}

type layer struct{ gain js.Value }

// Engine synthesizes the soundscape with the Web Audio API. All sound is
// generated procedurally from noise and oscillators, so no audio assets are
// shipped. It only starts after an explicit user gesture.
type Engine struct {
	ctx        js.Value
	master     js.Value
	muffle     js.Value
	reverbSend js.Value
	layers     map[string]*layer
	waterfall  js.Value // panner for the waterfall layer
	station    js.Value
	white      js.Value
	pink       js.Value
	started    bool
	muted      bool
	vol        float64
	rng        *procedural.RNG
	reverbGain js.Value
	convolver  js.Value
	timers     map[string]float64
}

// NewEngine creates an idle engine (no AudioContext until Start).
func NewEngine(seed uint64) *Engine {
	return &Engine{layers: map[string]*layer{}, vol: 0.6, rng: procedural.NewRNG(seed ^ 0xA0D10), timers: map[string]float64{}}
}

// Running reports whether the audio graph exists.
func (e *Engine) Running() bool { return e.started }

// Muted reports the mute state.
func (e *Engine) Muted() bool { return e.muted }

func (e *Engine) now() float64 { return e.ctx.Get("currentTime").Float() }

func (e *Engine) node(name string, args ...any) js.Value { return e.ctx.Call(name, args...) }

func connect(a js.Value, bs ...js.Value) js.Value {
	for _, b := range bs {
		a.Call("connect", b)
	}
	return a
}

func (e *Engine) gainNode(v float64) js.Value {
	g := e.node("createGain")
	g.Get("gain").Set("value", v)
	return g
}

func (e *Engine) filter(kind string, freq, q float64) js.Value {
	f := e.node("createBiquadFilter")
	f.Set("type", kind)
	f.Get("frequency").Set("value", freq)
	f.Get("Q").Set("value", q)
	return f
}

func (e *Engine) osc(kind string, freq float64) js.Value {
	o := e.node("createOscillator")
	o.Set("type", kind)
	o.Get("frequency").Set("value", freq)
	return o
}

func (e *Engine) noiseBuffer(seconds float64, pink bool) js.Value {
	rate := e.ctx.Get("sampleRate").Float()
	n := int(seconds * rate)
	buf := e.node("createBuffer", 1, n, rate)
	data := make([]float32, n)
	r := procedural.NewRNG(7)
	var b0, b1, b2 float64
	for i := range data {
		w := r.Float()*2 - 1
		if pink {
			b0 = 0.99765*b0 + w*0.0990460
			b1 = 0.96300*b1 + w*0.2965164
			b2 = 0.57000*b2 + w*1.0526913
			data[i] = float32((b0 + b1 + b2 + w*0.1848) * 0.2)
		} else {
			data[i] = float32(w * 0.5)
		}
	}
	ch := buf.Call("getChannelData", 0)
	u8 := js.Global().Get("Uint8Array").New(ch.Get("buffer"), ch.Get("byteOffset"), n*4)
	js.CopyBytesToJS(u8, unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), n*4))
	return buf
}

func (e *Engine) noiseSource(pink bool) js.Value {
	s := e.node("createBufferSource")
	if pink {
		s.Set("buffer", e.pink)
	} else {
		s.Set("buffer", e.white)
	}
	s.Set("loop", true)
	s.Call("start", 0, e.rng.Float()*3)
	return s
}

func (e *Engine) lfo(freq, depth float64, target js.Value) {
	o := e.osc("sine", freq)
	g := e.gainNode(depth)
	connect(o, g)
	connect(g, target)
	o.Call("start")
}

func (e *Engine) addLayer(name string, build func(out js.Value)) {
	g := e.gainNode(0)
	connect(g, e.muffle)
	build(g)
	e.layers[name] = &layer{gain: g}
}

// Start builds the audio graph. It must be called from a user gesture.
func (e *Engine) Start() error {
	if e.started {
		_ = e.ctx.Call("resume")
		return nil
	}
	ac := js.Global().Get("AudioContext")
	if ac.IsUndefined() {
		ac = js.Global().Get("webkitAudioContext")
	}
	if ac.IsUndefined() {
		return errNoAudio
	}
	e.ctx = ac.New()
	e.white, e.pink = e.noiseBuffer(4, false), e.noiseBuffer(6, true)
	comp := e.node("createDynamicsCompressor")
	e.master = e.gainNode(0)
	e.muffle = e.filter("lowpass", 20000, 0.7)
	connect(e.muffle, e.master)
	connect(e.master, comp)
	connect(comp, e.ctx.Get("destination"))
	e.master.Get("gain").Call("setTargetAtTime", e.effectiveVolume(), e.now(), 0.3)

	// reverb: a decaying stereo noise impulse
	rate := e.ctx.Get("sampleRate").Float()
	irLen := int(rate * 2.8)
	ir := e.node("createBuffer", 2, irLen, rate)
	rr := procedural.NewRNG(99)
	for c := 0; c < 2; c++ {
		d := make([]float32, irLen)
		for i := range d {
			t := float64(i) / float64(irLen)
			d[i] = float32((rr.Float()*2 - 1) * math.Pow(1-t, 2.6))
		}
		ch := ir.Call("getChannelData", c)
		u8 := js.Global().Get("Uint8Array").New(ch.Get("buffer"), ch.Get("byteOffset"), irLen*4)
		js.CopyBytesToJS(u8, unsafe.Slice((*byte)(unsafe.Pointer(&d[0])), irLen*4))
	}
	e.convolver = e.node("createConvolver")
	e.convolver.Set("buffer", ir)
	e.reverbSend = e.gainNode(0)
	e.reverbGain = e.gainNode(0.9)
	connect(e.reverbSend, e.convolver)
	connect(e.convolver, e.reverbGain)
	connect(e.reverbGain, e.muffle)

	e.addLayer("wind", func(out js.Value) {
		n := e.noiseSource(true)
		bp := e.filter("bandpass", 420, 0.6)
		g := e.gainNode(0.9)
		connect(n, bp, g)
		connect(g, out)
		e.lfo(0.11, 260, bp.Get("frequency"))
		e.lfo(0.07, 0.3, g.Get("gain"))
	})
	e.addLayer("rain", func(out js.Value) {
		n := e.noiseSource(false)
		hp := e.filter("highpass", 1100, 0.5)
		lp := e.filter("lowpass", 9500, 0.5)
		connect(n, hp)
		connect(hp, lp)
		connect(lp, out)
		n2 := e.noiseSource(true)
		bp := e.filter("bandpass", 380, 0.5)
		g := e.gainNode(0.5)
		connect(n2, bp, g)
		connect(g, out)
	})
	e.addLayer("water", func(out js.Value) {
		n := e.noiseSource(true)
		bp := e.filter("bandpass", 900, 0.45)
		g := e.gainNode(1)
		connect(n, bp, g)
		connect(g, out)
		e.lfo(0.31, 0.25, g.Get("gain"))
		n2 := e.noiseSource(false)
		bp2 := e.filter("bandpass", 2400, 1.2)
		g2 := e.gainNode(0.25)
		connect(n2, bp2, g2)
		connect(g2, out)
		e.lfo(0.9, 0.12, g2.Get("gain"))
	})
	e.addLayer("waterfall", func(out js.Value) {
		n := e.noiseSource(false)
		lp := e.filter("lowpass", 3800, 0.4)
		hp := e.filter("highpass", 110, 0.4)
		connect(n, lp)
		connect(lp, hp)
		e.waterfall = e.node("createPanner")
		e.waterfall.Set("panningModel", "HRTF")
		e.waterfall.Set("distanceModel", "inverse")
		e.waterfall.Set("refDistance", 20)
		e.waterfall.Set("rolloffFactor", 0.8)
		connect(hp, e.waterfall)
		connect(e.waterfall, out)
	})
	e.addLayer("forest", func(out js.Value) {
		n := e.noiseSource(true)
		lp := e.filter("lowpass", 650, 0.5)
		g := e.gainNode(0.8)
		connect(n, lp, g)
		connect(g, out)
		r := e.noiseSource(false)
		hp := e.filter("highpass", 4200, 0.4)
		rg := e.gainNode(0.0)
		connect(r, hp, rg)
		connect(rg, out)
		e.lfo(0.13, 0.04, rg.Get("gain"))
		e.lfo(0.37, 0.03, rg.Get("gain"))
	})
	e.addLayer("insects", func(out js.Value) {
		for i, f := range []float64{4300, 5200, 3700} {
			o := e.osc("sine", f)
			g := e.gainNode(0)
			am := e.osc("square", 5.5+float64(i)*1.7)
			amg := e.gainNode(0.02)
			connect(am, amg)
			connect(amg, g.Get("gain"))
			g.Get("gain").Set("value", 0.02)
			connect(o, g)
			connect(g, out)
			o.Call("start")
			am.Call("start")
		}
	})
	e.addLayer("cave", func(out js.Value) {
		for _, f := range []float64{55, 82.4, 110.3} {
			o := e.osc("sine", f)
			g := e.gainNode(0.25)
			connect(o, g)
			connect(g, out)
			e.lfo(0.05+f*0.0003, 0.08, g.Get("gain"))
			o.Call("start")
		}
		n := e.noiseSource(true)
		lp := e.filter("lowpass", 240, 0.7)
		g := e.gainNode(0.5)
		connect(n, lp, g)
		connect(g, out)
	})
	e.addLayer("station", func(out js.Value) {
		for _, f := range []float64{100, 150, 301} {
			o := e.osc("sine", f)
			g := e.gainNode(0.12)
			connect(o, g)
			e.station = e.node("createPanner")
			connect(g, out)
			o.Call("start")
		}
		n := e.noiseSource(true)
		hp := e.filter("bandpass", 1800, 2)
		g := e.gainNode(0.05)
		connect(n, hp, g)
		connect(g, out)
		whine := e.osc("sine", 7800)
		wg := e.gainNode(0.004)
		connect(whine, wg)
		connect(wg, out)
		whine.Call("start")
	})
	e.addLayer("tree", func(out js.Value) {
		for i, f := range []float64{220, 277.18, 329.63, 440, 554.37} {
			o := e.osc("sine", f)
			g := e.gainNode(0.07)
			connect(o, g)
			connect(g, out)
			e.lfo(0.04+float64(i)*0.013, 0.05, g.Get("gain"))
			o.Call("start")
		}
	})
	e.addLayer("observatory", func(out js.Value) {
		n := e.noiseSource(true)
		bp := e.filter("bandpass", 1300, 4)
		g := e.gainNode(0.5)
		connect(n, bp, g)
		connect(g, out)
		e.lfo(0.08, 500, bp.Get("frequency"))
		for _, f := range []float64{880, 1318.5} {
			o := e.osc("sine", f)
			og := e.gainNode(0.015)
			connect(o, og)
			connect(og, out)
			e.lfo(0.1, 0.012, og.Get("gain"))
			o.Call("start")
		}
	})
	e.addLayer("underwater", func(out js.Value) {
		n := e.noiseSource(true)
		lp := e.filter("lowpass", 260, 0.8)
		g := e.gainNode(0.9)
		connect(n, lp, g)
		connect(g, out)
	})
	e.started = true
	return nil
}

type audioError string

func (a audioError) Error() string { return string(a) }

const errNoAudio = audioError("Web Audio is not supported in this browser")

func (e *Engine) effectiveVolume() float64 {
	if e.muted {
		return 0
	}
	return e.vol * 0.9
}

// SetVolume sets the master volume 0..1.
func (e *Engine) SetVolume(v float64) {
	e.vol = procedural.Clamp(v, 0, 1)
	if e.started {
		e.master.Get("gain").Call("setTargetAtTime", e.effectiveVolume(), e.now(), 0.15)
	}
}

// SetMuted mutes or unmutes the output.
func (e *Engine) SetMuted(m bool) {
	e.muted = m
	if e.started {
		e.master.Get("gain").Call("setTargetAtTime", e.effectiveVolume(), e.now(), 0.15)
	}
}

func (e *Engine) set(name string, v float64) {
	if l, ok := e.layers[name]; ok {
		l.gain.Get("gain").Call("setTargetAtTime", v, e.now(), 0.7)
	}
}

// Update applies layer levels and listener pose. fallsPos places the waterfall in 3D space.
func (e *Engine) Update(l Levels, ls Listener, fallsX, fallsY, fallsZ float64) {
	if !e.started || e.ctx.Get("state").String() == "suspended" {
		return
	}
	set := e.set
	set("wind", l.Wind*0.55)
	set("rain", l.Rain*0.5)
	set("water", l.Water*0.35)
	set("waterfall", l.Waterfall*1.2)
	set("forest", l.Forest*0.3)
	set("insects", l.Insects*0.35)
	set("cave", l.Cave*0.5)
	set("station", l.Station*0.4)
	set("tree", l.Tree*0.5)
	set("observatory", l.Observatory*0.5)
	set("underwater", l.Underwater*0.45)
	e.reverbSend.Get("gain").Call("setTargetAtTime", l.Reverb, e.now(), 0.5)
	cut := 20000 - l.Muffle*19700
	e.muffle.Get("frequency").Call("setTargetAtTime", cut, e.now(), 0.25)
	lst := e.ctx.Get("listener")
	if lst.Get("positionX").Truthy() {
		lst.Get("positionX").Set("value", ls.X)
		lst.Get("positionY").Set("value", ls.Y)
		lst.Get("positionZ").Set("value", ls.Z)
		lst.Get("forwardX").Set("value", ls.FX)
		lst.Get("forwardY").Set("value", ls.FY)
		lst.Get("forwardZ").Set("value", ls.FZ)
		lst.Get("upX").Set("value", 0)
		lst.Get("upY").Set("value", 1)
		lst.Get("upZ").Set("value", 0)
	}
	if e.waterfall.Truthy() {
		e.waterfall.Get("positionX").Set("value", fallsX)
		e.waterfall.Get("positionY").Set("value", fallsY)
		e.waterfall.Get("positionZ").Set("value", fallsZ)
	}
}

// Tick drives periodic one-shot sounds. dt is seconds.
func (e *Engine) Tick(dt float64, l Levels, ls Listener) {
	if !e.started || e.muted {
		return
	}
	due := func(name string, mean float64) bool {
		e.timers[name] -= dt
		if e.timers[name] <= 0 {
			e.timers[name] = mean * (0.5 + e.rng.Float())
			return true
		}
		return false
	}
	around := func(r float64) (float64, float64, float64) {
		a := e.rng.Range(0, 2*math.Pi)
		return ls.X + math.Cos(a)*r, ls.Y + e.rng.Range(1, 10), ls.Z + math.Sin(a)*r
	}
	if l.Birds > 0.05 && due("bird", 3.2/(0.3+l.Birds)) {
		x, y, z := around(e.rng.Range(8, 40))
		e.Chirp(x, y, z, l.Birds)
	}
	if l.Cave > 0.2 && due("drip", 2.4) {
		x, y, z := around(e.rng.Range(3, 20))
		e.Ping(x, y-5, z, 900+e.rng.Range(0, 900), 0.12)
	}
	if l.Station > 0.5 && due("beep", 6) {
		e.Beep(ls.X, ls.Y, ls.Z)
	}
	if l.Insects > 0.2 && due("owl", 22) {
		x, y, z := around(e.rng.Range(25, 70))
		e.Hoot(x, y+6, z)
	}
}

func (e *Engine) panner(x, y, z float64) js.Value {
	p := e.node("createPanner")
	p.Set("panningModel", "HRTF")
	p.Set("distanceModel", "inverse")
	p.Set("refDistance", 6)
	p.Set("rolloffFactor", 1.1)
	if p.Get("positionX").Truthy() {
		p.Get("positionX").Set("value", x)
		p.Get("positionY").Set("value", y)
		p.Get("positionZ").Set("value", z)
	}
	return p
}

func (e *Engine) envelope(g js.Value, t0, attack, hold, release, peak float64) {
	gp := g.Get("gain")
	gp.Call("setValueAtTime", 0.0001, t0)
	gp.Call("linearRampToValueAtTime", peak, t0+attack)
	gp.Call("setValueAtTime", peak, t0+attack+hold)
	gp.Call("exponentialRampToValueAtTime", 0.0001, t0+attack+hold+release)
}

// Chirp plays a short bird phrase at a world position.
func (e *Engine) Chirp(x, y, z, level float64) {
	if !e.started || e.muted {
		return
	}
	t := e.now()
	p := e.panner(x, y, z)
	out := e.gainNode(1)
	connect(p, out)
	connect(out, e.muffle)
	send := e.gainNode(0.15)
	connect(out, send)
	connect(send, e.reverbSend)
	base := e.rng.Range(2200, 4300)
	n := 2 + e.rng.Intn(4)
	for i := 0; i < n; i++ {
		o := e.osc("sine", base)
		g := e.gainNode(0)
		st := t + float64(i)*e.rng.Range(0.11, 0.2)
		d := e.rng.Range(0.05, 0.11)
		o.Get("frequency").Call("setValueAtTime", base*e.rng.Range(0.85, 1.1), st)
		o.Get("frequency").Call("exponentialRampToValueAtTime", base*e.rng.Range(0.7, 1.5), st+d)
		e.envelope(g, st, 0.008, d*0.5, d*0.8, 0.08*(0.4+level))
		connect(o, g)
		connect(g, p)
		o.Call("start", st)
		o.Call("stop", st+d*2+0.05)
	}
}

// Thunder rumbles after a delay that grows with distance (0..1).
func (e *Engine) Thunder(dist float64) {
	if !e.started || e.muted {
		return
	}
	t := e.now() + ThunderDelay(dist)
	gain := ThunderGain(dist)
	src := e.noiseSource(true)
	lp := e.filter("lowpass", 160+260*(1-dist), 0.8)
	g := e.gainNode(0)
	connect(src, lp, g)
	send := e.gainNode(0.5)
	connect(g, e.muffle)
	connect(g, send)
	connect(send, e.reverbSend)
	e.envelope(g, t, 0.06, 0.3, 3.5+dist*3, 0.9*gain)
	src.Call("stop", t+8)
	crack := e.noiseSource(false)
	hp := e.filter("highpass", 1500, 0.6)
	cg := e.gainNode(0)
	connect(crack, hp, cg)
	connect(cg, e.muffle)
	e.envelope(cg, t, 0.005, 0.02, 0.35, 0.35*gain*(1-dist))
	crack.Call("stop", t+1)
}

// Croak plays a frog call.
func (e *Engine) Croak(x, y, z float64) {
	if !e.started || e.muted {
		return
	}
	t := e.now()
	p := e.panner(x, y, z)
	connect(p, e.muffle)
	o := e.osc("sawtooth", e.rng.Range(110, 170))
	lp := e.filter("lowpass", 700, 2)
	g := e.gainNode(0)
	am := e.osc("square", e.rng.Range(14, 24))
	amg := e.gainNode(0.5)
	connect(am, amg)
	connect(amg, g.Get("gain"))
	connect(o, lp, g)
	connect(g, p)
	e.envelope(g, t, 0.01, 0.18, 0.12, 0.1)
	o.Call("start", t)
	am.Call("start", t)
	o.Call("stop", t+0.5)
	am.Call("stop", t+0.5)
}

// Ping is a short resonant drop (cave drips).
func (e *Engine) Ping(x, y, z, freq, level float64) {
	if !e.started || e.muted {
		return
	}
	t := e.now()
	p := e.panner(x, y, z)
	connect(p, e.muffle)
	send := e.gainNode(0.8)
	connect(p, send)
	connect(send, e.reverbSend)
	o := e.osc("sine", freq)
	o.Get("frequency").Call("exponentialRampToValueAtTime", freq*0.6, t+0.12)
	g := e.gainNode(0)
	e.envelope(g, t, 0.003, 0.01, 0.25, level)
	connect(o, g)
	connect(g, p)
	o.Call("start", t)
	o.Call("stop", t+0.4)
}

// Beep is a faint console tone inside the station.
func (e *Engine) Beep(x, y, z float64) {
	if !e.started || e.muted {
		return
	}
	t := e.now()
	for i := 0; i < 2; i++ {
		o := e.osc("square", 880+float64(i)*440)
		lp := e.filter("lowpass", 2400, 0.7)
		g := e.gainNode(0)
		st := t + float64(i)*0.11
		e.envelope(g, st, 0.004, 0.04, 0.05, 0.025)
		connect(o, lp, g)
		connect(g, e.muffle)
		o.Call("start", st)
		o.Call("stop", st+0.15)
	}
}

// Hoot is a distant night owl.
func (e *Engine) Hoot(x, y, z float64) {
	if !e.started || e.muted {
		return
	}
	t := e.now()
	p := e.panner(x, y, z)
	connect(p, e.muffle)
	for i, d := range []float64{0, 0.55, 1.0} {
		o := e.osc("sine", 360-float64(i%2)*25)
		g := e.gainNode(0)
		e.envelope(g, t+d, 0.06, 0.18, 0.25, 0.07)
		connect(o, g)
		connect(g, p)
		o.Call("start", t+d)
		o.Call("stop", t+d+0.6)
	}
}

// Close shuts the audio context down.
func (e *Engine) Close() {
	if e.started {
		e.ctx.Call("close")
		e.started = false
	}
}
