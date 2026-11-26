// Package app wires the engine together for the browser. The files without a
// build tag hold the platform-independent logic (commands, persistence, HUD
// state); app_js.go contains the DOM, input and render loop.
package app

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"evergreen/internal/engine"
)

// Effect is a bit set describing what must be refreshed after a command.
type Effect int

// Effects.
const (
	EffNone  Effect = 0
	EffWorld Effect = 1 << iota // seed or density changed: regenerate the world
	EffQuality
	EffWeather
	EffTime
	EffSeason
	EffCamera
	EffAudio
	EffGithub
	EffHUD
)

// ApplyCommand mutates cfg from a UI command. value is the raw string from
// JavaScript. On error cfg is left unchanged.
func ApplyCommand(cfg *engine.Config, name, value string) (Effect, error) {
	next := *cfg
	var eff Effect
	num := func() (float64, error) {
		f, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0, fmt.Errorf("%s: %q is not a number", name, value)
		}
		return f, nil
	}
	boolean := func() bool { return value == "true" || value == "1" || value == "on" }
	switch name {
	case "seed":
		next.Seed = value
		eff = EffWorld
	case "preset":
		next.Preset = value
		eff = EffQuality | EffWorld
	case "autoQuality":
		next.AutoQuality = boolean()
		eff = EffQuality
	case "timeMode":
		next.TimeMode = value
		eff = EffTime
	case "timeOfDay":
		f, err := num()
		if err != nil {
			return EffNone, err
		}
		next.TimeOfDay = f
		next.TimeMode = "manual"
		eff = EffTime
	case "timeSpeed":
		f, err := num()
		if err != nil {
			return EffNone, err
		}
		next.TimeSpeed = f
		eff = EffTime
	case "weather":
		next.Weather = value
		eff = EffWeather
	case "season":
		next.Season = value
		eff = EffSeason | EffWorld
	case "cameraMode":
		next.CameraMode = value
		eff = EffCamera
	case "volume":
		f, err := num()
		if err != nil {
			return EffNone, err
		}
		next.Volume = f
		eff = EffAudio
	case "muted":
		next.Muted = boolean()
		eff = EffAudio
	case "showFps":
		next.ShowFPS = boolean()
		eff = EffHUD
	case "drawDistance":
		f, err := num()
		if err != nil {
			return EffNone, err
		}
		next.DrawDistance = f
		eff = EffQuality
	case "github":
		next.Github = value
		eff = EffGithub | EffWorld
	case "reducedMotion":
		next.ReducedMotion = boolean()
		eff = EffCamera | EffQuality
	default:
		return EffNone, fmt.Errorf("unknown setting %q", name)
	}
	if errs := next.Validate(); len(errs) > 0 {
		// Validate repairs values; surface the first problem so the UI can tell the user.
		if name != "seed" || next.Seed == "" {
			return EffNone, errs[0]
		}
	}
	*cfg = next
	return eff, nil
}

// Saved is what persists between visits (localStorage).
type Saved struct {
	Config     engine.Config `json:"config"`
	X          float64       `json:"x"`
	Z          float64       `json:"z"`
	Yaw        float64       `json:"yaw"`
	Discovered []int         `json:"discovered"`
	Distance   float64       `json:"distance"` // metres travelled
	Version    int           `json:"v"`
}

// SavedVersion is the current persistence schema.
const SavedVersion = 1

// Marshal serialises the saved state.
func (s Saved) Marshal() []byte {
	s.Version = SavedVersion
	b, _ := json.Marshal(s)
	return b
}

// ParseSaved decodes persisted state, repairing or discarding anything invalid.
func ParseSaved(data []byte) (Saved, []error) {
	def := Saved{Config: engine.DefaultConfig(), Version: SavedVersion}
	if len(data) == 0 {
		return def, nil
	}
	s := def
	if err := json.Unmarshal(data, &s); err != nil {
		return def, []error{fmt.Errorf("saved state: %w", err)}
	}
	errs := s.Config.Validate()
	bad := func(v float64) bool { return v != v || v > 1e9 || v < -1e9 }
	if bad(s.X) || bad(s.Z) || bad(s.Yaw) || bad(s.Distance) {
		s.X, s.Z, s.Yaw, s.Distance = 0, 0, 0, 0
		errs = append(errs, fmt.Errorf("saved position out of range; reset"))
	}
	if s.Distance < 0 {
		s.Distance = 0
	}
	seen := map[int]bool{}
	clean := s.Discovered[:0]
	for _, id := range s.Discovered {
		if id >= 1 && id <= 9 && !seen[id] {
			seen[id] = true
			clean = append(clean, id)
		}
	}
	s.Discovered = clean
	return s, errs
}

// Discover records a zone as visited and reports whether it was new.
func (s *Saved) Discover(id int) bool {
	for _, d := range s.Discovered {
		if d == id {
			return false
		}
	}
	s.Discovered = append(s.Discovered, id)
	return true
}

// HUD is the state pushed to the DOM overlay a few times per second.
type HUD struct {
	FPS        int     `json:"fps"`
	Zone       string  `json:"zone"`
	ZoneID     int     `json:"zoneId"`
	Phase      string  `json:"phase"`
	Clock      string  `json:"clock"`
	Weather    string  `json:"weather"`
	Season     string  `json:"season"`
	Mode       string  `json:"mode"`
	Preset     string  `json:"preset"`
	Chunks     int     `json:"chunks"`
	Pending    int     `json:"pending"`
	Draws      int     `json:"draws"`
	Tris       int     `json:"tris"`
	Scale      float64 `json:"scale"`
	Animals    int     `json:"animals"`
	Prompt     string  `json:"prompt"`
	Paused     bool    `json:"paused"`
	Audio      bool    `json:"audio"`
	Underwater bool    `json:"underwater"`
	Discovered []int   `json:"discovered"`
	GithubOK   bool    `json:"githubOk"`
	X          int     `json:"x"`
	Z          int     `json:"z"`
}

// JSON encodes the HUD.
func (h HUD) JSON() string {
	b, _ := json.Marshal(h)
	return string(b)
}

// ClockString formats hours as HH:MM.
func ClockString(h float64) string {
	for h < 0 {
		h += 24
	}
	for h >= 24 {
		h -= 24
	}
	hh := int(h)
	mm := int((h - float64(hh)) * 60)
	return fmt.Sprintf("%02d:%02d", hh, mm)
}
