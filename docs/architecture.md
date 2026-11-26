# Architecture

## Layers

1. **Foundation** (`procedural`, `engine`, `mesh`). Deterministic hashing/noise/RNG; vectors, matrices, frustum, config, quality presets, pools; one interleaved vertex format (13 floats: position, normal, colour, sway, uv, material) shared by Go and GLSL.
2. **World content** (`vegetation`, `zones`, `terrain`, `world`). `terrain.Generator` turns a seed into heights, water and biomes and blends in zone shapes. `zones` places nine landmarks per 2.8 km realm and builds their meshes, lights, colliders, walkable surfaces, emitters and interactions. `world` streams 64 m chunks, places vegetation and exposes the physics/wildlife environments.
3. **Simulation** (`atmosphere`, `weather`, `water`, `particles`, `wildlife`, `ecosystem`, `camera`, `physics`, `audio` mix model, `github`). All pure Go, all unit tested natively.
4. **Presentation** (`renderer`, audio engine, `app`, `web/`). WebGL2 passes, Web Audio synthesis, the render loop, input and the DOM shell.

## Frame

`app.frame` → input → clock/weather/season → camera + physics → `world.Update` (time-sliced) → ecosystem, ripples, particles, audio → compose a `renderer.Scene` → `Renderer.Render`:

1. Shadow map (sun-aligned ortho, texel-snapped) for Medium and up
2. Planar reflection (mirrored view, clip plane) for High and Ultra, only when water is nearby
3. Main pass into a scaled FBO: sky, terrain, trees (LOD0/LOD1), small plants, grass, landmarks, animals; then blended water and glass; then GPU and CPU particles
4. Post: bright pass, two blur passes, composite (bloom, god rays, depth of field, grade, vignette)

One uniform block (`Frame`, 44 vec4s) carries all per-frame state, which keeps the JavaScript call count per frame low. Chunk and prototype geometry lives in shared buffers with one VAO per draw.

## Streaming

`world.Update` plans a nearest-first list of missing chunks, then advances a per-chunk state machine (sample grid rows → build terrain and water meshes → place trees → understory → flowers → grass) until a time budget (3-6 ms by preset) is spent. Chunks beyond `view radius + 1.6` unload and are handed back to the renderer to free their GPU buffers. Terrain LOD (stride 1/2/4 with skirts) is chosen by distance and refined as you approach. Landmarks load when you are within view distance + 120 m and are built one per frame.

## Why one mesh pipeline?

Terrain, water, plants, buildings and animals all use the same vertex layout and one fragment shader that branches on a material id. That keeps the renderer small (one program plus a depth-only variant) at the cost of a long fragment shader; branches are coherent per primitive, so they are cheap in practice.
