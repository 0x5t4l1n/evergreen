# Contributing to Evergreen

Thanks for helping the forest grow.

## Setup

```bash
make setup      # installs smoke-test deps, syncs wasm_exec.js with your Go toolchain
make test       # unit + integration tests, and a wasm vet pass
make lint       # gofmt check + go vet (native and js/wasm)
make serve      # builds dist/ and serves http://127.0.0.1:8080/
make smoke      # browser smoke test (needs Chrome or Chromium)
```

Go 1.24 or newer is required. The only non-Go dependency is `playwright-core`, used by the smoke test.

## Ground rules

- Keep simulation logic in pure Go packages that compile and test natively. Only `renderer`, `audio` (engine), and `app` (loop, DOM, input) may use `syscall/js`, behind `//go:build js && wasm`.
- Everything procedural must be deterministic: derive randomness from `procedural.RNG`/hashes seeded by the world seed and coordinates. Never use `math/rand` or wall-clock time inside generation.
- Add tests with new behaviour. Tests run without a GPU; shader changes are checked in a browser (`make smoke`) and by reading the console.
- Do not add dependencies without a strong reason. No frontend frameworks.
- GLSL: do not use `smoothstep` with reversed edges (undefined); use `sstep`. Do not compare interpolated varyings with `==`.
- Use only original or properly licensed assets. All audio is synthesized; all geometry is procedural.

## Pull requests

Describe what changed and how you verified it. Include a screenshot for visual changes. CI must pass.
