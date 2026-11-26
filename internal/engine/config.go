package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Config is the complete user-facing configuration. It is persisted as JSON.
type Config struct {
	Seed          string  `json:"seed"`
	Preset        string  `json:"preset"`      // low|medium|high|ultra
	AutoQuality   bool    `json:"autoQuality"` // adapt resolution to frame time
	TimeMode      string  `json:"timeMode"`    // cycle|manual|local
	TimeOfDay     float64 `json:"timeOfDay"`   // hours 0..24
	TimeSpeed     float64 `json:"timeSpeed"`   // in-game hours per real minute
	Weather       string  `json:"weather"`     // auto|clear|lightrain|heavyrain|storm|lightfog|densefog
	Season        string  `json:"season"`      // auto|spring|summer|autumn|winter
	CameraMode    string  `json:"cameraMode"`  // ambient|explore|scroll|cinematic
	Volume        float64 `json:"volume"`      // 0..1
	Muted         bool    `json:"muted"`
	ShowFPS       bool    `json:"showFps"`
	DrawDistance  float64 `json:"drawDistance"` // chunks, 0 = preset default
	Github        string  `json:"github"`
	ReducedMotion bool    `json:"reducedMotion"`
}

var (
	presets     = []string{"low", "medium", "high", "ultra"}
	timeModes   = []string{"cycle", "manual", "local"}
	weathers    = []string{"auto", "clear", "lightrain", "heavyrain", "storm", "lightfog", "densefog"}
	seasons     = []string{"auto", "spring", "summer", "autumn", "winter"}
	cameraModes = []string{"ambient", "explore", "scroll", "cinematic"}
)

// DefaultConfig returns the out-of-the-box configuration.
func DefaultConfig() Config {
	return Config{
		Seed: "evergreen", Preset: "medium", AutoQuality: true, TimeMode: "cycle", TimeOfDay: 9.5,
		TimeSpeed: 1, Weather: "auto", Season: "summer", CameraMode: "ambient", Volume: 0.6,
	}
}

func oneOf(v string, set []string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// Validate normalises the configuration in place and returns every problem it
// repaired, so callers can report them without failing to start.
func (c *Config) Validate() []error {
	var errs []error
	fix := func(field *string, set []string, def, name string) {
		*field = strings.ToLower(strings.TrimSpace(*field))
		if !oneOf(*field, set) {
			errs = append(errs, fmt.Errorf("invalid %s %q, using %q", name, *field, def))
			*field = def
		}
	}
	d := DefaultConfig()
	fix(&c.Preset, presets, d.Preset, "preset")
	fix(&c.TimeMode, timeModes, d.TimeMode, "timeMode")
	fix(&c.Weather, weathers, d.Weather, "weather")
	fix(&c.Season, seasons, d.Season, "season")
	fix(&c.CameraMode, cameraModes, d.CameraMode, "cameraMode")
	c.Seed = strings.TrimSpace(c.Seed)
	if len(c.Seed) > 64 {
		c.Seed = c.Seed[:64]
		errs = append(errs, fmt.Errorf("seed truncated to 64 characters"))
	}
	if c.Seed == "" {
		c.Seed = d.Seed
	}
	num := func(v *float64, lo, hi, def float64, name string) {
		if math.IsNaN(*v) || math.IsInf(*v, 0) {
			errs = append(errs, fmt.Errorf("%s is not a number", name))
			*v = def
		}
		if *v < lo || *v > hi {
			errs = append(errs, fmt.Errorf("%s %.2f out of range [%.1f,%.1f]", name, *v, lo, hi))
			*v = math.Min(hi, math.Max(lo, *v))
		}
	}
	num(&c.TimeOfDay, 0, 24, d.TimeOfDay, "timeOfDay")
	num(&c.TimeSpeed, 0, 600, d.TimeSpeed, "timeSpeed")
	num(&c.Volume, 0, 1, d.Volume, "volume")
	if c.DrawDistance != 0 {
		num(&c.DrawDistance, 2, 14, 0, "drawDistance")
	}
	c.Github = strings.TrimSpace(strings.TrimPrefix(c.Github, "@"))
	if c.Github != "" && !ValidGithubUser(c.Github) {
		errs = append(errs, fmt.Errorf("invalid GitHub username %q", c.Github))
		c.Github = ""
	}
	return errs
}

// ValidGithubUser reports whether s is a syntactically valid GitHub login.
func ValidGithubUser(s string) bool {
	if len(s) == 0 || len(s) > 39 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return !strings.Contains(s, "--")
}

// ParseConfig decodes JSON on top of the defaults and validates it.
func ParseConfig(data []byte) (Config, []error) {
	c := DefaultConfig()
	if len(data) > 0 {
		if err := json.Unmarshal(data, &c); err != nil {
			return DefaultConfig(), []error{fmt.Errorf("config parse: %w", err)}
		}
	}
	return c, c.Validate()
}

// JSON serialises the configuration.
func (c Config) JSON() []byte {
	b, _ := json.Marshal(c)
	return b
}
