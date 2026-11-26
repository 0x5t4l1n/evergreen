# Security policy

Evergreen is a static site: a Go WebAssembly module, WebGL2 shaders and a little JavaScript. There is no backend and no database.

## Reporting a vulnerability

Please open a private security advisory on the repository (Security → Advisories → Report a vulnerability). Do not file public issues for undisclosed vulnerabilities. Expect an acknowledgement within a few days.

## Design rules that keep it safe

- **No secrets in the browser.** GitHub activity is fetched by a scheduled workflow using the built-in `GITHUB_TOKEN` and written to `data/activity.json`. The token is only sent as a request header by `cmd/evergreen-activity`, is never written to the output (covered by a test), and the WASM build does not import `net/http`.
- **All external data is validated.** `internal/github.Parse` bounds file size, repository count, names (`^[A-Za-z0-9._-]+$`), numbers and strips markup characters from free text. Saved settings from `localStorage` and every UI command go through `engine.Config.Validate`.
- **No `innerHTML` with external data.** UI text is assigned with `textContent`.
- **The generated SVG is inert:** no scripts, no external references, no event handlers (covered by tests).
- **Workflows use minimal permissions**, per job, and third-party actions are limited to GitHub's own `actions/*`.
