// Package vegetation generates procedural plant, rock and fungus geometry and
// holds the biome rules that decide what grows where.
package vegetation

// Biome classifies a point of the world for vegetation and ground texturing.
type Biome int

// Biomes, ordered roughly from open and dry to dense and wet.
const (
	Meadow Biome = iota
	Forest
	Rainforest
	Highland
	Wetland
	Alpine
	Glade // bioluminescent grove around the World Tree
)

var biomeNames = [...]string{"meadow", "forest", "rainforest", "highland", "wetland", "alpine", "glade"}

func (b Biome) String() string {
	if int(b) < len(biomeNames) {
		return biomeNames[b]
	}
	return "unknown"
}

// KindWeight pairs a vegetation kind with a relative spawn weight.
type KindWeight struct {
	Kind   Kind
	Weight float64
}

// Profile summarises the vegetation rules for one biome.
type Profile struct {
	TreeSpacing    float64 // metres between jittered tree cells
	TreeChance     float64 // probability a cell holds a tree
	AncientChance  float64 // per 30m cell
	Trees          []KindWeight
	GrassDensity   float64 // clumps per m2 at Density==1
	FlowerChance   float64
	ShrubChance    float64
	FernChance     float64
	RockChance     float64
	MushroomChance float64
	GlowChance     float64
	VineChance     float64
}

var profiles = [...]Profile{
	Meadow: {TreeSpacing: 9, TreeChance: 0.28, AncientChance: 0.05, Trees: []KindWeight{{TreeOak, 5}, {TreeBirch, 3}, {TreePine, 1}},
		GrassDensity: 0.55, FlowerChance: 0.35, ShrubChance: 0.08, FernChance: 0.02, RockChance: 0.025, MushroomChance: 0.01},
	Forest: {TreeSpacing: 6, TreeChance: 0.62, AncientChance: 0.12, Trees: []KindWeight{{TreeOak, 4}, {TreePine, 4}, {TreeBirch, 2}},
		GrassDensity: 0.32, FlowerChance: 0.06, ShrubChance: 0.14, FernChance: 0.14, RockChance: 0.04, MushroomChance: 0.04},
	Rainforest: {TreeSpacing: 5, TreeChance: 0.85, AncientChance: 0.22, Trees: []KindWeight{{TreeRain, 6}, {TreeOak, 1}},
		GrassDensity: 0.18, FlowerChance: 0.04, ShrubChance: 0.22, FernChance: 0.38, RockChance: 0.06, MushroomChance: 0.07, VineChance: 0.5},
	Highland: {TreeSpacing: 8, TreeChance: 0.38, AncientChance: 0.03, Trees: []KindWeight{{TreePine, 8}, {TreeBirch, 1}},
		GrassDensity: 0.22, FlowerChance: 0.05, ShrubChance: 0.07, FernChance: 0.03, RockChance: 0.1, MushroomChance: 0.01},
	Wetland: {TreeSpacing: 8, TreeChance: 0.3, AncientChance: 0.04, Trees: []KindWeight{{TreeOak, 3}, {TreeRain, 2}},
		GrassDensity: 0.4, FlowerChance: 0.12, ShrubChance: 0.18, FernChance: 0.2, RockChance: 0.03, MushroomChance: 0.05},
	Alpine: {TreeSpacing: 12, TreeChance: 0.1, AncientChance: 0, Trees: []KindWeight{{TreePine, 1}},
		GrassDensity: 0.05, FlowerChance: 0.01, ShrubChance: 0.02, RockChance: 0.16},
	Glade: {TreeSpacing: 7, TreeChance: 0.4, AncientChance: 0.1, Trees: []KindWeight{{TreeRain, 3}, {TreeAncient, 1}, {TreeBirch, 2}},
		GrassDensity: 0.3, FlowerChance: 0.25, ShrubChance: 0.1, FernChance: 0.3, RockChance: 0.03, MushroomChance: 0.12, GlowChance: 0.5},
}

// ProfileFor returns the vegetation rules for a biome.
func ProfileFor(b Biome) Profile {
	if int(b) < 0 || int(b) >= len(profiles) {
		return profiles[Meadow]
	}
	return profiles[b]
}

// PickTree chooses a tree kind from a biome profile using u in [0,1).
func (p Profile) PickTree(u float64) Kind {
	var total float64
	for _, k := range p.Trees {
		total += k.Weight
	}
	u *= total
	for _, k := range p.Trees {
		if u < k.Weight {
			return k.Kind
		}
		u -= k.Weight
	}
	return TreeOak
}
