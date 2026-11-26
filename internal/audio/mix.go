// Package audio models Evergreen's environmental soundscape. This file holds
// the platform-independent mix model (which layers play, and how loudly,
// for a given place, weather and time); the Web Audio synthesis lives in the
// js-only files of this package.
package audio

import (
	"evergreen/internal/procedural"
	"evergreen/internal/zones"
)

// Inputs describe the listener's surroundings.
type Inputs struct {
	Zone       zones.ID
	ZoneBlend  float64 // 0..1 how deep inside the zone
	Rain       float64
	Wind       float64
	Thunder    float64
	Day        float64
	NearWater  float64 // 0..1 proximity to flowing/standing water
	FallsProx  float64 // 0..1 proximity to the waterfall
	Underwater bool
	InCave     bool
	InStation  bool
	Winter     bool
}

// Levels are linear gains 0..1 for each synthesized layer.
type Levels struct {
	Forest, Birds, Wind, Rain, Water, Waterfall, Insects float64
	Cave, Station, Tree, Frogs, Observatory, Underwater  float64
	Muffle                                               float64 // 0 open .. 1 heavily low-passed
	Reverb                                               float64
}

// Mix computes layer levels. It is pure and cheap; the synthesizer smooths changes.
func Mix(in Inputs) Levels {
	var l Levels
	c := procedural.Clamp
	dry := 1 - c(in.Rain*1.8, 0, 1)
	night := 1 - in.Day
	inside := in.InCave || in.InStation
	open := 1.0
	if inside {
		open = 0.12
	}
	zb := c(in.ZoneBlend, 0, 1)
	z := func(id zones.ID) float64 {
		if in.Zone == id {
			return zb
		}
		return 0
	}
	winter := 1.0
	if in.Winter {
		winter = 0.35
	}
	l.Forest = open * (0.35 + 0.4*(z(zones.Wilderness)+z(zones.Entrance)+z(zones.Treehouse)+z(zones.WorldTree)))
	l.Birds = open * in.Day * dry * winter * (0.45 + 0.4*z(zones.Entrance) + 0.3*z(zones.Wilderness))
	l.Wind = open*(0.1+0.8*in.Wind) + 0.5*z(zones.Observatory)*open
	l.Rain = c(in.Rain, 0, 1) * (1 - 0.8*b2f(inside))
	l.Water = c(in.NearWater, 0, 1) * open * 0.7
	l.Waterfall = c(in.FallsProx, 0, 1) * open
	l.Insects = open * night * dry * winter * 0.7
	l.Frogs = open * (night*0.5 + in.Rain*0.5) * c(in.NearWater, 0, 1) * winter
	l.Cave = b2f(in.InCave) * 0.8
	l.Station = b2f(in.InStation)*0.7 + 0.25*z(zones.Station)*open
	l.Tree = z(zones.WorldTree) * 0.7
	l.Observatory = z(zones.Observatory) * open * 0.6
	l.Reverb = b2f(in.InCave)*0.9 + 0.15*z(zones.Ruins)
	if in.Underwater {
		l.Underwater = 1
		l.Muffle = 0.95
		l.Birds, l.Insects, l.Wind, l.Rain, l.Forest, l.Frogs = 0, 0, 0, l.Rain*0.15, 0, 0
		l.Water *= 0.4
	} else {
		l.Muffle = c(0.15*b2f(inside)+0.35*in.Rain, 0, 0.6)
	}
	return l
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// ThunderDelay converts a normalised strike distance to a delay in seconds
// (sound travels ~343 m/s; distances map to 0.3..5 km).
func ThunderDelay(dist float64) float64 { return procedural.Clamp(dist, 0, 1)*14 + 0.2 }

// ThunderGain falls off with distance.
func ThunderGain(dist float64) float64 { return 1 - 0.75*procedural.Clamp(dist, 0, 1) }
