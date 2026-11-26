# Shader development

Sources live in `shaders/*.glsl|vert|frag` and are embedded by `shaders/shaders.go`. `shaders.Source` prepends `#version 300 es`, precision, `#define`s and `common.glsl`, which declares the `Frame` uniform block and shared helpers (`hash*`, `vnoise*`, `fbm2`, `skyColor`, `applyFog`, `sstep`).

## Programs

| Program | Files | Notes |
|---|---|---|
| mesh | `mesh.vert`, `mesh.frag` | all geometry, material id branch; `SHADOW` define builds the depth-only variant |
| sky | `fullscreen.vert`, `sky.frag` | gradient, sun, moon with phase, stars, Milky Way, shooting stars, clouds, lightning |
| particles | `particles.*` | stateless: positions are a pure function of particle id and time, wrapped around the camera |
| cpuparticles | `cpuparticles.*` | mist, splashes, spores from CPU emitters |
| bright / blur / post | `bright.frag`, `blur.frag`, `post.frag` | bloom chain and composite |

## Materials

`vMat = id + variant/10` (see `internal/mesh`): 0 standard, 1 foliage, 2 emissive (variants: steady, pulse, blink, flicker), 3 screen (variants pick the dashboard), 4 glass, 5 waterfall/foam, 6 crystal, 7 bark, 8 stone, 9 terrain, 10 water, 11 metal, 12 glow, 13 wood, 14 flower, 15 grass, 16 cave rock, 17 interior. Add a material by choosing an id, a `Mat*` constant, and an `if (m == N.)` branch.

`aSway` animation channel: `(0,1]` wind sway weight, `(-1,0)` wing flap, `[2,3)` leg swing, `[3,4)` tail wag. Instances are `vec4 (pos, scale)` and `vec4 (yaw, tint, birth, amplitude)`; `birth < -500` marks an animal and encodes its animation phase.

## Rules learned the hard way

- Never call `smoothstep` with `edge0 >= edge1` (undefined); use `sstep`.
- Never compare interpolated varyings with `==`; use `abs(x - k) < .5`.
- Any code that creates an element buffer must unbind the current VAO first, or it rewires that VAO.
- Keep `FrameData` slot constants in `internal/renderer/frame.go` in sync with the block in `common.glsl` (a test checks the size).
- Shader compile errors surface in the page as the fallback message and in the console with the first lines of the source.
