package audio

import (
	"testing"

	"evergreen/internal/zones"
)

func TestDayVsNightLayers(t *testing.T) {
	day := Mix(Inputs{Day: 1, Zone: zones.Entrance, ZoneBlend: 1})
	night := Mix(Inputs{Day: 0, Zone: zones.Entrance, ZoneBlend: 1})
	if day.Birds <= night.Birds || night.Insects <= day.Insects {
		t.Fatalf("birds by day, insects by night: day=%+v night=%+v", day, night)
	}
}

func TestRainAndStormLayers(t *testing.T) {
	dry := Mix(Inputs{Day: 1})
	rain := Mix(Inputs{Day: 1, Rain: 1, Wind: 0.9})
	if rain.Rain <= dry.Rain || rain.Birds >= dry.Birds || rain.Wind <= dry.Wind {
		t.Fatalf("rain should add rain+wind and silence birds: %+v vs %+v", rain, dry)
	}
	if rain.Muffle <= dry.Muffle {
		t.Fatal("rain should slightly muffle")
	}
}

func TestCaveIsEnclosed(t *testing.T) {
	c := Mix(Inputs{Day: 1, Wind: 1, Rain: 1, InCave: true, Zone: zones.Cave, ZoneBlend: 1})
	if c.Cave < 0.5 || c.Reverb < 0.5 {
		t.Fatalf("cave drone and reverb expected: %+v", c)
	}
	if c.Wind > 0.3 || c.Birds > 0.05 || c.Rain > 0.5 {
		t.Fatalf("outdoor sounds should be muted in the cave: %+v", c)
	}
}

func TestStationElectronicAmbience(t *testing.T) {
	in := Mix(Inputs{Day: 1, InStation: true, Zone: zones.Station, ZoneBlend: 1})
	out := Mix(Inputs{Day: 1, Zone: zones.Station, ZoneBlend: 1})
	if in.Station < 0.6 || out.Station > in.Station || out.Station <= 0 {
		t.Fatalf("station hum louder inside, faintly audible outside: in=%v out=%v", in.Station, out.Station)
	}
}

func TestWaterLayers(t *testing.T) {
	far := Mix(Inputs{Day: 1})
	near := Mix(Inputs{Day: 1, NearWater: 1, FallsProx: 1})
	if near.Water <= far.Water || near.Waterfall <= 0 || far.Waterfall != 0 {
		t.Fatalf("%+v %+v", near, far)
	}
}

func TestUnderwaterMuffles(t *testing.T) {
	u := Mix(Inputs{Day: 1, Underwater: true, Wind: 1, NearWater: 1})
	if u.Muffle < 0.9 || u.Underwater != 1 || u.Wind != 0 || u.Birds != 0 {
		t.Fatalf("%+v", u)
	}
}

func TestLevelsStayNormalised(t *testing.T) {
	for _, in := range []Inputs{
		{Day: 1, Rain: 1, Wind: 1, NearWater: 1, FallsProx: 1, Zone: zones.WorldTree, ZoneBlend: 1},
		{Day: 0, Zone: zones.Observatory, ZoneBlend: 1, Wind: 1},
		{Day: 0.5, InCave: true, InStation: true, Zone: zones.Cave, ZoneBlend: 2},
	} {
		l := Mix(in)
		for _, v := range []float64{l.Forest, l.Birds, l.Wind, l.Rain, l.Water, l.Waterfall, l.Insects, l.Cave, l.Station, l.Tree, l.Frogs, l.Observatory, l.Underwater, l.Muffle, l.Reverb} {
			if v < 0 || v > 1.5 {
				t.Fatalf("level out of range %v for %+v", v, in)
			}
		}
	}
}

func TestThunderTiming(t *testing.T) {
	if ThunderDelay(0.9) <= ThunderDelay(0.1) || ThunderGain(0.9) >= ThunderGain(0.1) {
		t.Fatal("distant thunder is later and quieter")
	}
	if ThunderDelay(-1) < 0.2 || ThunderGain(5) < 0.2 {
		t.Fatal("bounds")
	}
}
