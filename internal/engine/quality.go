package engine

import "math"

// Quality describes every performance-relevant knob derived from a preset.
type Quality struct {
	Name          string
	ViewChunks    int     // terrain/tree radius in chunks
	GrassChunks   float64 // grass radius in chunks
	ShadowRes     int     // 0 disables shadows
	ShadowDist    float64 // metres covered by the shadow map
	ParticleScale float64
	RenderScale   float64 // internal resolution multiplier
	Bloom         bool
	GodRays       bool
	DOF           bool
	Reflection    int // 0 analytic sky only, 1 half-res planar, 2 higher-res planar
	Density       float64
	WildlifeCap   int
	GenBudgetMs   float64
}

var qualityTable = map[string]Quality{
	"low":    {"low", 4, 0.9, 0, 0, 0.35, 0.7, false, false, false, 0, 0.45, 24, 3},
	"medium": {"medium", 6, 1.4, 1024, 70, 0.6, 0.85, true, false, false, 0, 0.7, 48, 4},
	"high":   {"high", 8, 1.9, 2048, 110, 1.0, 1.0, true, true, true, 1, 1.0, 90, 5},
	"ultra":  {"ultra", 10, 2.4, 4096, 150, 1.6, 1.0, true, true, true, 2, 1.25, 150, 6},
}

// QualityFor returns the preset table entry (medium for unknown names).
func QualityFor(name string) Quality {
	if q, ok := qualityTable[name]; ok {
		return q
	}
	return qualityTable["medium"]
}

// Adaptive tunes render scale and tier from observed frame times. It is
// deliberately slow to raise quality and quick to lower it.
type Adaptive struct {
	Scale   float64 // 0.5..1, multiplies Quality.RenderScale
	Tier    int     // 0 none, -1/-2 progressively cheaper
	ema     float64
	slow    int
	fast    int
	Enabled bool
}

// NewAdaptive returns a controller at full quality.
func NewAdaptive() *Adaptive { return &Adaptive{Scale: 1, Enabled: true, ema: 16} }

// Observe feeds one frame time (ms) and returns true when the effective
// quality changed and render targets should be rebuilt.
func (a *Adaptive) Observe(ms float64) bool {
	if !a.Enabled || ms <= 0 || ms > 500 {
		return false
	}
	a.ema += (ms - a.ema) * 0.08
	changed := false
	switch {
	case a.ema > 26:
		a.slow++
		a.fast = 0
	case a.ema < 14.5:
		a.fast++
		a.slow = 0
	default:
		a.slow, a.fast = 0, 0
	}
	if a.slow > 45 {
		a.slow = 0
		if a.Scale > 0.55 {
			a.Scale = math.Max(0.5, a.Scale-0.125)
			changed = true
		} else if a.Tier > -2 {
			a.Tier--
			changed = true
		}
	}
	if a.fast > 600 {
		a.fast = 0
		if a.Tier < 0 {
			a.Tier++
			changed = true
		} else if a.Scale < 1 {
			a.Scale = math.Min(1, a.Scale+0.125)
			changed = true
		}
	}
	return changed
}

// Effective applies the adaptive state to a base quality.
func (a *Adaptive) Effective(q Quality) Quality {
	q.RenderScale *= a.Scale
	if a.Tier < 0 {
		q.ShadowRes /= 2
		if a.Tier <= -2 {
			q.ShadowRes = 0
			q.Reflection = 0
			q.GodRays = false
			q.DOF = false
			q.ViewChunks = max(3, q.ViewChunks-2)
		}
		q.ParticleScale *= 0.6
		q.GrassChunks *= 0.8
	}
	return q
}
