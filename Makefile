GO      ?= go
GOROOT  := $(shell $(GO) env GOROOT)
WASMEXEC := $(firstword $(wildcard $(GOROOT)/lib/wasm/wasm_exec.js $(GOROOT)/misc/wasm/wasm_exec.js))
DIST    ?= dist
BASE    ?=
PORT    ?= 8080
SEED    ?= 42

.PHONY: setup dev build wasm svg test lint clean serve smoke fmt activity

setup: ## install smoke-test dependencies and sync wasm_exec.js with the toolchain
	cp "$(WASMEXEC)" web/wasm_exec.js
	cd tests/smoke && npm install --no-audit --no-fund
	$(GO) mod download

wasm: ## compile the WebAssembly module into dist/
	mkdir -p $(DIST)
	GOOS=js GOARCH=wasm $(GO) build -trimpath -ldflags="-s -w" -o $(DIST)/evergreen.wasm ./cmd/evergreen

svg: ## regenerate the README SVG
	$(GO) run ./cmd/evergreen-svg --seed $(SEED) --theme day --output assets/evergreen.svg
	$(GO) run ./cmd/evergreen-svg --seed $(SEED) --theme night --output assets/evergreen-night.svg

build: wasm svg ## full static site in dist/
	cp web/index.html web/style.css web/loader.js web/ui.js web/icon.svg dist/
	cp "$(WASMEXEC)" dist/wasm_exec.js
	mkdir -p dist/assets dist/data
	cp assets/evergreen.svg assets/evergreen-night.svg assets/logo.svg dist/assets/
	if [ -f web/data/activity.json ]; then cp web/data/activity.json dist/data/activity.json; else echo '{"version":1,"generatedAt":"1970-01-01T00:00:00Z","users":{}}' > dist/data/activity.json; fi
	touch dist/.nojekyll

serve: build ## build and serve at http://127.0.0.1:$(PORT)/
	$(GO) run ./cmd/serve -dir $(DIST) -addr 127.0.0.1:$(PORT) -prefix "$(BASE)"

dev: serve ## alias for serve

fmt:
	gofmt -w cmd internal shaders

test: ## unit tests (native) and a wasm compile check
	$(GO) test ./...
	GOOS=js GOARCH=wasm $(GO) vet -composites=false ./internal/renderer ./internal/app ./internal/audio ./cmd/evergreen

lint:
	@test -z "$$(gofmt -l cmd internal shaders)" || (echo "gofmt needed:"; gofmt -l cmd internal shaders; exit 1)
	$(GO) vet -composites=false ./...
	GOOS=js GOARCH=wasm $(GO) vet -composites=false ./internal/renderer ./internal/app ./internal/audio ./cmd/evergreen

smoke: build ## browser smoke test (needs Chrome/Chromium)
	cd tests/smoke && node smoke.mjs ../../$(DIST)

activity: ## refresh web/data/activity.json (set EVERGREEN_GITHUB_USERS, GITHUB_TOKEN)
	$(GO) run ./cmd/evergreen-activity --output web/data/activity.json

clean:
	rm -rf $(DIST) tests/smoke/node_modules tests/smoke/artifacts
