# GitHub integration

```
ecosystem.yml (every 6 h) → cmd/evergreen-activity → web/data/activity.json → deploy.yml → data/activity.json on the site
browser: fetch("data/activity.json") → github.Parse (validate) → User.Influence() → world density/plants/trees
```

| Activity | Effect |
|---|---|
| Repository | a tree in the grove around the Forest Entrance (kind from language, size from stars and repo size) |
| Commits (recent public) | more grass, shrubs and ferns |
| Pull requests | more flowers |
| Stars received | more fireflies at night |
| Releases | the World Tree grows (scale 0.88-1.0 and the grow-in animation) |
| Contributions (recent events) | forest density up to +40 % |

Set the account list with the repository variable `EVERGREEN_GITHUB_USERS` (comma separated; defaults to the repository owner). Enter a username in the settings panel to link it. If the file or user is missing the forest simply stays procedural.

Security: the token is used only inside the workflow job as an `Authorization` header, never written to the JSON (tested), and the WASM build has no HTTP client. The browser only reads a static file and validates it.
