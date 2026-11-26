// Package zones defines Evergreen's nine connected environmental zones: where
// they sit in the infinite world, how they reshape terrain, and the landmark
// geometry, lights, colliders and interactions that make each one distinctive.
package zones

import (
	"math"
	"sync"

	"evergreen/internal/procedural"
	"evergreen/internal/vegetation"
)

// ID identifies a zone type.
type ID int

// The nine zones.
const (
	Entrance ID = iota + 1
	Wilderness
	Falls
	Cave
	Treehouse
	Ruins
	Observatory
	Station
	WorldTree
	NumZones = 9
)

// RealmSize is the spacing in metres of repeating zone layouts. Every realm
// contains one instance of each zone, so the world is infinite yet landmarks
// stay reachable.
const RealmSize = 2800.0

// Info is static descriptive data about a zone type.
type Info struct {
	ID      ID
	Key     string
	Name    string
	Tagline string
	Radius  float64
	Biome   vegetation.Biome
	Ambient string // audio bed key
}

var infos = [...]Info{
	{Entrance, "entrance", "Forest Entrance", "Warm light between giants", 130, vegetation.Meadow, "forest"},
	{Wilderness, "wilderness", "Deep Wilderness", "The canopy closes overhead", 150, vegetation.Rainforest, "deep"},
	{Falls, "falls", "Emerald Falls", "Water falls from the sky", 150, vegetation.Wetland, "falls"},
	{Cave, "cave", "Bioluminescent Cave", "Light grows where the sun cannot reach", 130, vegetation.Rainforest, "cave"},
	{Treehouse, "treehouse", "Treehouse Sanctuary", "Homes strung between giants", 130, vegetation.Forest, "treehouse"},
	{Ruins, "ruins", "Forgotten Civilization", "Stone remembers what the forest forgot", 120, vegetation.Forest, "ruins"},
	{Observatory, "observatory", "Celestial Observatory", "Above the weather, below the stars", 150, vegetation.Alpine, "observatory"},
	{Station, "station", "Cyber Forest Station", "A quiet signal inside the rain", 110, vegetation.Rainforest, "station"},
	{WorldTree, "worldtree", "The World Tree", "Where every root begins", 170, vegetation.Glade, "worldtree"},
}

// InfoFor returns static data for a zone ID.
func InfoFor(id ID) Info {
	if id < 1 || int(id) > len(infos) {
		return Info{}
	}
	return infos[id-1]
}

// All returns the static zone table.
func All() []Info { return append([]Info(nil), infos[:]...) }

// Zone is one placed instance of a zone type.
type Zone struct {
	Info
	X, Z  float64 // centre (world metres)
	BaseY float64 // natural terrain height at the centre; landmark origin
	Realm [2]int
	Seed  uint64
}

var layout = [...]struct{ angle, radius float64 }{
	Entrance: {90, 300}, Wilderness: {45, 520}, Falls: {0, 640}, Cave: {-45, 620},
	Treehouse: {-90, 560}, Ruins: {-135, 650}, Observatory: {180, 700}, Station: {135, 600}, WorldTree: {0, 0},
}

// Atlas places zones deterministically for a world seed.
type Atlas struct {
	Seed  uint64
	baseY func(x, z float64) float64
	mu    sync.Mutex
	cache map[[2]int][]Zone
	paths map[[2]int][]PathSeg
}

// NewAtlas creates an atlas. baseY supplies natural terrain height at the zone
// centres (nil means flat zero, which is useful in tests).
func NewAtlas(seed uint64, baseY func(x, z float64) float64) *Atlas {
	return &Atlas{Seed: seed, baseY: baseY, cache: map[[2]int][]Zone{}, paths: map[[2]int][]PathSeg{}}
}

// RealmOf returns the realm indices containing a world position.
func RealmOf(x, z float64) (int, int) {
	return int(math.Floor((x + RealmSize/2) / RealmSize)), int(math.Floor((z + RealmSize/2) / RealmSize))
}

// Realm returns the nine zones of a realm.
func (a *Atlas) Realm(rx, rz int) []Zone {
	key := [2]int{rx, rz}
	a.mu.Lock()
	defer a.mu.Unlock()
	if zs, ok := a.cache[key]; ok {
		return zs
	}
	zs := make([]Zone, 0, NumZones)
	cx, cz := float64(rx)*RealmSize, float64(rz)*RealmSize
	for _, inf := range infos {
		h := procedural.NewRNG(procedural.Hash3(a.Seed, int32(rx), int32(rz), int32(inf.ID)))
		l := layout[inf.ID]
		ang := (l.angle + h.Range(-9, 9)) * math.Pi / 180
		rad := l.radius
		if inf.ID != WorldTree {
			rad *= h.Range(0.93, 1.07)
		}
		z := Zone{Info: inf, X: cx + math.Cos(ang)*rad, Z: cz + math.Sin(ang)*rad, Realm: key, Seed: h.Next()}
		if a.baseY != nil {
			z.BaseY = a.baseY(z.X, z.Z)
		}
		zs = append(zs, z)
	}
	a.cache[key] = zs
	return zs
}

// Nearby returns zones whose influence (plus margin) reaches (x,z).
func (a *Atlas) Nearby(x, z, margin float64) []Zone {
	rx, rz := RealmOf(x, z)
	var out []Zone
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for _, zn := range a.Realm(rx+dx, rz+dz) {
				if math.Hypot(zn.X-x, zn.Z-z) < zn.Radius+margin {
					out = append(out, zn)
				}
			}
		}
	}
	return out
}

// At returns the zone whose radius contains (x,z), preferring the closest centre.
func (a *Atlas) At(x, z float64) (Zone, bool) {
	best, bd, ok := Zone{}, math.MaxFloat64, false
	for _, zn := range a.Nearby(x, z, 0) {
		if d := math.Hypot(zn.X-x, zn.Z-z); d < bd {
			best, bd, ok = zn, d, true
		}
	}
	return best, ok
}

// Find returns the zone of a given type nearest to (x,z).
func (a *Atlas) Find(id ID, x, z float64) Zone {
	rx, rz := RealmOf(x, z)
	best, bd := Zone{}, math.MaxFloat64
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			for _, zn := range a.Realm(rx+dx, rz+dz) {
				if zn.ID == id {
					if d := math.Hypot(zn.X-x, zn.Z-z); d < bd {
						best, bd = zn, d
					}
				}
			}
		}
	}
	return best
}

// Spawn returns the initial camera position and look direction: the Forest
// Entrance of realm (0,0), facing the World Tree.
func (a *Atlas) Spawn() (x, z, yaw float64) {
	e := a.Find(Entrance, 0, 0)
	t := a.Find(WorldTree, 0, 0)
	dx, dz := t.X-e.X, t.Z-e.Z
	l := math.Hypot(dx, dz)
	x, z = e.X-dx/l*10, e.Z-dz/l*10
	return x, z, math.Atan2(dz, dx)
}

// PathSeg is a straight piece of the forest path network.
type PathSeg struct{ X0, Z0, X1, Z1 float64 }

// PathsNear returns path segments intersecting the box expanded by margin.
func (a *Atlas) PathsNear(minX, minZ, maxX, maxZ, margin float64) []PathSeg {
	rx, rz := RealmOf((minX+maxX)/2, (minZ+maxZ)/2)
	segs := a.realmPaths(rx, rz)
	var out []PathSeg
	for _, s := range segs {
		if math.Max(s.X0, s.X1) < minX-margin || math.Min(s.X0, s.X1) > maxX+margin ||
			math.Max(s.Z0, s.Z1) < minZ-margin || math.Min(s.Z0, s.Z1) > maxZ+margin {
			continue
		}
		out = append(out, s)
	}
	return out
}

func (a *Atlas) realmPaths(rx, rz int) []PathSeg {
	key := [2]int{rx, rz}
	zs := a.Realm(rx, rz)
	a.mu.Lock()
	if p, ok := a.paths[key]; ok {
		a.mu.Unlock()
		return p
	}
	a.mu.Unlock()
	var tree Zone
	for _, z := range zs {
		if z.ID == WorldTree {
			tree = z
		}
	}
	var segs []PathSeg
	curve := func(ax, az, bx, bz float64, wob float64, trim0, trim1 float64) {
		mx, mz := (ax+bx)/2, (az+bz)/2
		dx, dz := bx-ax, bz-az
		l := math.Hypot(dx, dz)
		cx, cz := mx-dz/l*wob, mz+dx/l*wob
		const n = 14
		px, pz := ax, az
		for i := 1; i <= n; i++ {
			t := float64(i) / n
			x := (1-t)*(1-t)*ax + 2*(1-t)*t*cx + t*t*bx
			zz := (1-t)*(1-t)*az + 2*(1-t)*t*cz + t*t*bz
			if t > trim0/l && t < 1-trim1/l {
				segs = append(segs, PathSeg{px, pz, x, zz})
			}
			px, pz = x, zz
		}
	}
	var ring []Zone
	for _, z := range zs {
		if z.ID == WorldTree {
			continue
		}
		h := procedural.NewRNG(z.Seed ^ 0xA7)
		curve(tree.X, tree.Z, z.X, z.Z, h.Range(-60, 60), 28, 16)
		ring = append(ring, z)
	}
	for i, z := range ring {
		n := ring[(i+1)%len(ring)]
		h := procedural.NewRNG(z.Seed ^ 0xB3)
		curve(z.X, z.Z, n.X, n.Z, h.Range(-50, 50), 20, 20)
	}
	a.mu.Lock()
	a.paths[key] = segs
	a.mu.Unlock()
	return segs
}

// PathDistance returns the distance from (x,z) to the nearest path segment in segs.
func PathDistance(segs []PathSeg, x, z float64) float64 {
	best := math.MaxFloat64
	for _, s := range segs {
		dx, dz := s.X1-s.X0, s.Z1-s.Z0
		l2 := dx*dx + dz*dz
		t := 0.0
		if l2 > 0 {
			t = procedural.Clamp(((x-s.X0)*dx+(z-s.Z0)*dz)/l2, 0, 1)
		}
		if d := math.Hypot(x-(s.X0+dx*t), z-(s.Z0+dz*t)); d < best {
			best = d
		}
	}
	return best
}
