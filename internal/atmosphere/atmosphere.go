// Package atmosphere models time of day, seasons, the sun and moon, and
// turns them (together with weather) into the lighting values the renderer uses.
package atmosphere

import (
	"math"
	"time"

	"evergreen/internal/procedural"
)

// Season of the year.
type Season int

// Seasons.
const (
	Spring Season = iota
	Summer
	Autumn
	Winter
)

var seasonNames = [...]string{"spring", "summer", "autumn", "winter"}

func (s Season) String() string { return seasonNames[s&3] }

// ParseSeason converts a config string; "auto" derives it from the date.
func ParseSeason(name string, now time.Time) Season {
	for i, n := range seasonNames {
		if n == name {
			return Season(i)
		}
	}
	m := now.Month()
	switch {
	case m >= 3 && m <= 5:
		return Spring
	case m >= 6 && m <= 8:
		return Summer
	case m >= 9 && m <= 11:
		return Autumn
	}
	return Winter
}

// Clock tracks the in-world time of day.
type Clock struct {
	Hours     float64 // 0..24
	Speed     float64 // in-world hours per real minute
	SyncLocal bool
	Paused    bool
	Days      float64 // days elapsed (drives moon phase)
}

// Advance moves the clock forward by dt real seconds.
func (c *Clock) Advance(dt float64, now time.Time) {
	if c.SyncLocal {
		c.Hours = float64(now.Hour()) + float64(now.Minute())/60 + float64(now.Second())/3600
		return
	}
	if c.Paused {
		return
	}
	d := dt * c.Speed / 60
	c.Hours += d
	c.Days += d / 24
	for c.Hours >= 24 {
		c.Hours -= 24
	}
	for c.Hours < 0 {
		c.Hours += 24
	}
}

// Phase names the part of the day.
func Phase(h float64) string {
	switch {
	case h >= 5 && h < 7.5:
		return "sunrise"
	case h >= 7.5 && h < 12:
		return "morning"
	case h >= 12 && h < 17:
		return "afternoon"
	case h >= 17 && h < 19.5:
		return "sunset"
	case h >= 19.5 && h < 23:
		return "evening"
	}
	return "midnight"
}

// Vec is an RGB colour or direction.
type Vec [3]float64

func mixV(a, b Vec, t float64) Vec {
	return Vec{procedural.Mix(a[0], b[0], t), procedural.Mix(a[1], b[1], t), procedural.Mix(a[2], b[2], t)}
}

// State is the lighting environment for one frame.
type State struct {
	SunDir       Vec     // unit vector toward the sun
	MoonDir      Vec     // unit vector toward the moon
	SunColor     Vec     // sunlight colour (already scaled by intensity)
	SunIntensity float64 // 0..1.2
	Day          float64 // 0 night .. 1 full day
	Night        float64 // 1-Day, smoothed
	SkyTop       Vec
	SkyHorizon   Vec
	Fog          Vec
	Ambient      Vec
	MoonPhase    float64 // 0 new .. 0.5 full .. 1
	StarVis      float64
	Elev         float64 // sun elevation in radians
}

// Compute evaluates the sky for an hour (0..24), the elapsed days and a season.
func Compute(hours, days float64, season Season) State {
	var s State
	// Sun path: east at 6h, zenith at 12h, west at 18h; slightly tilted by season.
	tilt := [...]float64{0.55, 0.75, 0.5, 0.32}[season&3]
	ang := (hours - 6) / 12 * math.Pi
	az := math.Cos(ang)
	s.SunDir = norm(Vec{az * 0.92, math.Sin(ang) * (0.35 + tilt*0.65), -0.38})
	s.Elev = math.Asin(s.SunDir[1])
	// Moon opposes the sun and drifts with the lunar day.
	s.MoonPhase = math.Mod(days/29.53+0.35, 1)
	ma := ang + math.Pi
	s.MoonDir = norm(Vec{math.Cos(ma) * 0.85, math.Sin(ma)*0.7 + 0.05, 0.45})

	up := s.SunDir[1]
	s.Day = procedural.SmoothStep(-0.12, 0.22, up)
	s.Night = 1 - procedural.SmoothStep(-0.22, 0.05, up)
	twilight := math.Exp(-math.Pow((up-0.02)/0.17, 2))

	dayTop, dayHor := Vec{0.18, 0.42, 0.82}, Vec{0.62, 0.78, 0.92}
	nightTop, nightHor := Vec{0.005, 0.012, 0.035}, Vec{0.02, 0.04, 0.09}
	duskHor := Vec{1.0, 0.48, 0.2}
	duskTop := Vec{0.22, 0.28, 0.5}
	s.SkyTop = mixV(nightTop, dayTop, s.Day)
	s.SkyHorizon = mixV(nightHor, dayHor, s.Day)
	s.SkyHorizon = mixV(s.SkyHorizon, duskHor, twilight*0.85)
	s.SkyTop = mixV(s.SkyTop, duskTop, twilight*0.45)

	warm := math.Exp(-math.Max(up, 0) * 3.2)
	sunCol := mixV(Vec{1, 0.96, 0.88}, Vec{1, 0.55, 0.25}, warm*0.9)
	s.SunIntensity = procedural.SmoothStep(-0.04, 0.2, up) * 1.1
	moonLight := 0.12 * (1 - math.Abs(s.MoonPhase-0.5)*1.6) * s.Night
	moonCol := Vec{0.55, 0.68, 1}
	sun := s.SunIntensity
	s.SunColor = Vec{sunCol[0]*sun + moonCol[0]*moonLight, sunCol[1]*sun + moonCol[1]*moonLight, sunCol[2]*sun + moonCol[2]*moonLight}
	if sun < 0.02 {
		// night: light comes from the moon
		s.SunDir = s.MoonDir
	}
	s.Ambient = mixV(Vec{0.045, 0.07, 0.125}, Vec{0.34, 0.42, 0.52}, s.Day)
	s.Ambient = mixV(s.Ambient, Vec{0.45, 0.3, 0.25}, twilight*0.35)
	s.Fog = mixV(s.SkyHorizon, Vec{0.5, 0.58, 0.55}, 0.35*s.Day)
	s.StarVis = s.Night * s.Night
	return s
}

func norm(v Vec) Vec {
	l := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if l == 0 {
		return v
	}
	return Vec{v[0] / l, v[1] / l, v[2] / l}
}

// SeasonLook is how a season changes foliage and grading.
type SeasonLook struct {
	Blend      float64 // continuous season index (0=spring..3=winter)
	LeafFall   float64 // 0..1 amount of leaves lost / fall activity
	Snow       float64 // 0..1 snow cover
	Autumn     float64 // 0..1 autumn colour mix
	Spring     float64 // 0..1 blossom/fresh-green mix
	Grade      Vec     // colour grade multiplier
	Saturation float64
	RainBias   float64 // multiplies rain probability
	SnowFall   bool
}

// LookFor returns the look of a discrete season.
func LookFor(s Season) SeasonLook {
	switch s {
	case Spring:
		return SeasonLook{Blend: 0, Spring: 1, LeafFall: 0.05, Grade: Vec{1.0, 1.04, 1.0}, Saturation: 1.12, RainBias: 1.4}
	case Autumn:
		return SeasonLook{Blend: 2, Autumn: 1, LeafFall: 1, Grade: Vec{1.08, 1.0, 0.9}, Saturation: 1.08, RainBias: 1.2}
	case Winter:
		return SeasonLook{Blend: 3, Snow: 1, LeafFall: 0.3, Grade: Vec{0.92, 0.98, 1.1}, Saturation: 0.7, RainBias: 0.6, SnowFall: true}
	}
	return SeasonLook{Blend: 1, LeafFall: 0.1, Grade: Vec{1.02, 1.0, 0.96}, Saturation: 1.05, RainBias: 0.7}
}

// BlendLook interpolates smoothly between seasons (used when the season changes).
func BlendLook(a, b SeasonLook, t float64) SeasonLook {
	m := func(x, y float64) float64 { return procedural.Mix(x, y, t) }
	return SeasonLook{
		Blend: m(a.Blend, b.Blend), LeafFall: m(a.LeafFall, b.LeafFall), Snow: m(a.Snow, b.Snow), Autumn: m(a.Autumn, b.Autumn),
		Spring: m(a.Spring, b.Spring), Grade: mixV(a.Grade, b.Grade, t), Saturation: m(a.Saturation, b.Saturation),
		RainBias: m(a.RainBias, b.RainBias), SnowFall: b.SnowFall,
	}
}
