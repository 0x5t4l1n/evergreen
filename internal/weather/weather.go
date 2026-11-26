// Package weather simulates smoothly changing weather: clear skies, rain,
// thunderstorms, fog and wind, with lightning and season-aware probabilities.
package weather

import (
	"math"

	"evergreen/internal/procedural"
)

// Kind is a discrete weather state.
type Kind int

// Weather kinds.
const (
	Clear Kind = iota
	LightRain
	HeavyRain
	Storm
	LightFog
	DenseFog
	numKinds
)

var names = [...]string{"clear", "lightrain", "heavyrain", "storm", "lightfog", "densefog"}

func (k Kind) String() string { return names[k] }

// Parse returns the Kind for a config name; ok is false for "auto"/unknown.
func Parse(s string) (Kind, bool) {
	for i, n := range names {
		if n == s {
			return Kind(i), true
		}
	}
	return Clear, false
}

// Params are the continuous values a Kind targets.
type Params struct {
	Rain    float64 // 0..1
	Fog     float64 // 0..1
	Cloud   float64 // 0..1 cover
	Wind    float64 // 0..1
	Dark    float64 // light reduction 0..1
	Thunder float64 // 0..1 lightning activity
	Wetness float64 // 0..1 ground wetness (lags rain)
}

func target(k Kind) Params {
	switch k {
	case LightRain:
		return Params{Rain: 0.35, Fog: 0.2, Cloud: 0.7, Wind: 0.35, Dark: 0.25}
	case HeavyRain:
		return Params{Rain: 0.85, Fog: 0.35, Cloud: 0.92, Wind: 0.6, Dark: 0.5}
	case Storm:
		return Params{Rain: 1, Fog: 0.35, Cloud: 1, Wind: 0.95, Dark: 0.68, Thunder: 1}
	case LightFog:
		return Params{Rain: 0, Fog: 0.55, Cloud: 0.45, Wind: 0.12, Dark: 0.1}
	case DenseFog:
		return Params{Rain: 0, Fog: 1, Cloud: 0.7, Wind: 0.06, Dark: 0.25}
	}
	return Params{Cloud: 0.18, Wind: 0.18}
}

// Weather is the evolving weather state.
type Weather struct {
	Current   Params
	Kind      Kind // the weather being transitioned towards
	Forced    bool // true when the user pinned a weather
	Season    RainBias
	seed      uint64
	timer     float64
	changes   int
	lightning float64 // remaining flash intensity
	nextBolt  float64
	Bolt      bool // true on the frame a bolt strikes (for thunder audio)
	BoltDist  float64
	gust      float64
	t         float64
}

// RainBias scales how often rain occurs for the season (1 = neutral).
type RainBias float64

// New creates weather starting clear.
func New(seed uint64) *Weather {
	w := &Weather{seed: seed, Season: 1}
	w.Current = target(Clear)
	w.timer = 90
	return w
}

// Force pins the weather to k (or releases it when auto is true).
func (w *Weather) Force(k Kind, forced bool) {
	w.Forced = forced
	if forced {
		w.Kind = k
	}
}

// pick chooses the next weather with season-dependent odds.
func (w *Weather) pick() Kind {
	r := procedural.NewRNG(w.seed + uint64(w.changes)*7919)
	w.changes++
	b := float64(w.Season)
	weights := [numKinds]float64{Clear: 5, LightRain: 2 * b, HeavyRain: 1 * b, Storm: 0.6 * b, LightFog: 1.4, DenseFog: 0.5}
	if w.Kind == Storm {
		weights[Storm] = 0
	}
	var total float64
	for _, x := range weights {
		total += x
	}
	u := r.Float() * total
	for k, x := range weights {
		if u < x {
			return Kind(k)
		}
		u -= x
	}
	return Clear
}

// Update advances the simulation by dt seconds.
func (w *Weather) Update(dt float64) {
	w.t += dt
	w.Bolt = false
	if !w.Forced {
		w.timer -= dt
		if w.timer <= 0 {
			w.Kind = w.pick()
			r := procedural.NewRNG(w.seed ^ uint64(w.changes)*104729)
			w.timer = r.Range(70, 220)
		}
	}
	tg := target(w.Kind)
	k := 1 - math.Exp(-dt*0.35) // roughly a 3 second time constant
	step := func(c *float64, t float64) { *c += (t - *c) * k }
	step(&w.Current.Rain, tg.Rain)
	step(&w.Current.Fog, tg.Fog)
	step(&w.Current.Cloud, tg.Cloud)
	step(&w.Current.Wind, tg.Wind)
	step(&w.Current.Dark, tg.Dark)
	step(&w.Current.Thunder, tg.Thunder)
	wet := 1 - math.Exp(-dt*0.05)
	if w.Current.Rain > w.Current.Wetness {
		wet = 1 - math.Exp(-dt*0.25)
	}
	w.Current.Wetness += (math.Min(1, w.Current.Rain*1.4) - w.Current.Wetness) * wet

	// gusts: slow noise on top of the base wind
	w.gust = 0.5 + 0.5*procedural.Noise2(w.seed, w.t*0.15, 3.1)

	// lightning
	if w.lightning > 0 {
		w.lightning = math.Max(0, w.lightning-dt*3.5)
	}
	if w.Current.Thunder > 0.5 {
		w.nextBolt -= dt
		if w.nextBolt <= 0 {
			r := procedural.NewRNG(w.seed ^ uint64(w.t*1000))
			w.lightning = 1
			w.Bolt = true
			w.BoltDist = r.Range(0.1, 1)
			w.nextBolt = r.Range(4, 14)
		}
	}
}

// Flash returns the current lightning flash brightness (0..1), with the
// characteristic double flicker.
func (w *Weather) Flash() float64 {
	if w.lightning <= 0 {
		return 0
	}
	return w.lightning * (0.6 + 0.4*math.Abs(math.Sin(w.lightning*22)))
}

// WindSpeed is the effective wind including gusts (0..~1.3).
func (w *Weather) WindSpeed() float64 { return w.Current.Wind * (0.7 + 0.6*w.gust) }

// Gust is the current gust factor 0..1.
func (w *Weather) Gust() float64 { return w.gust }

// Visibility is the fog density multiplier applied to the base atmosphere.
func (w *Weather) Visibility() float64 { return 1 + w.Current.Fog*5.5 + w.Current.Rain*1.2 }
