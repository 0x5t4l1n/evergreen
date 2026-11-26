# Procedural generation

Everything derives from a 64-bit seed (`procedural.SeedFromString`: numbers map to themselves, text is FNV-1a hashed).

## Terrain (`internal/terrain`)

`height = 12 + hills + mountains`, then rivers and lakes carve it, then zones reshape it.

- **Hills:** four octaves of Perlin noise plus two detail octaves.
- **Mountains:** a low-frequency mask times ridged multifractal noise, only far from realm centres so landmarks stay approachable.
- **Rivers:** `|fbm(warped x,z)|` near zero defines a thin winding band; the bed is cut to `surface - depth` where `surface` follows only the large-scale terrain, so water is level across a cross-section.
- **Lakes:** a second noise field carves basins to a fixed lake level.
- **Zones:** each zone supplies `Shape(x,z) → (weight, relativeHeight)`; the final height is `lerp(natural, base+relative, weight)`. Zones can also suppress rivers, define their own water level, punch a terrain hole (the cave) and exclude vegetation.
- **Biomes:** zone override, then elevation (alpine/highland), then moisture noise (meadow, forest, rainforest) and shoreline (wetland).

## Chunks and meshes

Each chunk samples a 35×35 grid (one margin sample for normals). Vertices carry splat weights (rock, litter, shore, path, zone strength, zone id); the fragment shader turns them into grass, rock strata, mud, packed paths, ruin paving, glowing loam and snow. Water quads are emitted only where a vertex is under its water level and not deeper than the source's `maxDepth`, which keeps the upstream river from hanging over the waterfall cliff.

## Vegetation (`internal/vegetation`, `world/place.go`)

Prototype meshes are built from tubes, cones and noise-displaced ellipsoids with a per-kind seed and two LODs: oak, pine, rainforest (buttress roots, vines), ancient giants, birch, shrubs, ferns, rocks, flowers, mushrooms, crystals, logs and reeds. Placement uses jittered grids at several scales (4 m trees, 30 m ancients, 3 m understory, 2.5 m flower clusters, 1.6 m grass). Each biome's `Profile` sets spacing, probabilities and kinds. Items avoid water, paths, steep slopes and zone exclusions. Foliage cut-out, season colours, moss and snow are done in the shader, so one mesh serves every season.

## Zones and landmarks (`internal/zones`)

Zone positions come from a layout table plus a seeded jitter per realm. Each landmark builder (about 100-200 lines each) assembles meshes with the shared builder and also emits lights, colliders, walkable segments/discs (bridges, ramps, stairs, platforms), particle emitters and interactables. The cave is a polar-grid mound with an inner chamber shell, a tunnel cut by an elliptical volume test, stalactites, crystals and an underground lake.

## Wildlife and the world around you

Animals are simulated near the visitor only and spawn/despawn with biome, water, weather, light and season (`ecosystem.targets`). Their meshes carry animation weights in the `sway` attribute (wing flap, leg swing, tail wag) that the vertex shader drives from an accumulated phase.
