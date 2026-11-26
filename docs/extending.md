# Extending Evergreen

## Add a biome

1. Add a constant to `vegetation.Biome` and a `Profile` in `vegetation/biome.go` (spacing, chances, tree kinds).
2. Teach `terrain.Generator.biome` when it applies (elevation, moisture, or a zone's `Info.Biome`).
3. If it needs ground colours, branch on the biome in `terrainColor` (the terrain vertex already carries rock/litter/shore weights; add a case in `terrain.BuildTerrain` for new weights).
4. Add a test next to `vegetation_test.go`.

## Add an animal

1. Add a `Species` constant and name in `wildlife/behavior.go`; give it a mesh in `meshes.go` (face +X, use sway weights for animation).
2. Implement `updateX` and call it from `Entity.Update`; set `Anim` (rad/s) when it should animate.
3. Add population rules to `ecosystem.targets` and spawn rules to `trySpawn`.
4. Extend `wildlife_test.go` and `ecosystem_test.go`.

## Add a zone

1. Add an `ID`, an `Info` row and a layout entry in `zones/zones.go`; shape terrain in `shape.go` (`Shape`, `WaterLevel`, `Hole`, `Excluded`).
2. Write `buildX(z Zone) *Landmark` using `newLMB`: add geometry, `light`, `circle`/`box` colliders, `walkSeg`/`walkDisc` surfaces, `Emitters`, `Interact`.
3. Add it to `Build`, `LoreFor`, the audio mix (`audio/mix.go`) and `app.ArrivalPoint`.
4. `zones_test.go` already iterates every zone, so shape bounds, landmark validity and non-overlap are checked automatically.
