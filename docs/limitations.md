# Limitations and verification status

This is an honest account of what was built, how it was checked, and what remains approximate.

## Verified

- `go test ./...` (173 unit and integration tests) pass natively. `go vet` passes natively and for `GOOS=js GOARCH=wasm`.
- The WebAssembly build loads in Chrome, renders, accepts commands, streams chunks, and passes `make smoke` (WASM loads, canvas fills the viewport, scene renders, WASD navigation, zone teleport, weather/time/season/preset/seed controls, panel, fullscreen, no critical console errors), served under a sub-path.
- Visual inspection of screenshots: Forest Entrance, Emerald Falls, World Tree (day/night), Cave entrance, Treehouse, Cyber Station and Observatory at night, Deep Wilderness in a storm, Forgotten Civilization exterior. The deployed GitHub Pages site was also loaded and checked (HTTP/2, `application/wasm`).

## Not verified

- **Real-GPU performance.** All browser runs used SwiftShader (software WebGL), which renders at roughly 10 fps. Frame-rate claims for integrated GPUs are design targets, not measurements.
- **Audio** compiles and is wired, but was not listened to (no audio device in the test environment). Levels are covered by model tests only.
- **Not visually inspected:** Forgotten Civilization interior (hidden chamber), winter looks, underwater view, planar reflections (the matrix maths is unit tested), cinematic depth of field, touch controls, the live GitHub fetch (tested against a mock server).
- **Browsers:** only Chrome was run. Firefox, Safari and mobile browsers are expected to work (standard WebGL2/WASM/Web Audio) but are untested. WebGL context-loss recovery is implemented but was not exercised.
- `golangci-lint` was not available; "lint" means gofmt and `go vet`.

## Known approximations

- **Dynamic range:** the pipeline is LDR (RGBA8) with a soft tone curve, not HDR.
- **Caves:** a heightfield cannot hold caves, so the cave is a hollow mound with one main tunnel and two alcoves, not a branching network.
- **Wildlife:** animals are simulated only near the visitor and do not climb trees; squirrels and deer move on the ground.
- **Treehouse and bridges** are walkable via explicit surfaces; trunks, houses and ruins use circle/box colliders, so collision is coarse.
- **GPU instances** have no per-instance culling; chunk-level culling is used instead.
- **Fixed-size lights:** at most 8 point lights are active at once (nearest first).
- **Single thread:** Go WebAssembly runs on the main thread. Chunk generation is time-sliced, but large landmark builds can cause a short hitch.
- The binary is about 6.5 MB uncompressed (about 1.7 MB gzipped on Pages).
