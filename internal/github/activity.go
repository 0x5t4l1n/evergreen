// Package github maps public GitHub activity onto the forest. It only parses
// and validates a static JSON file produced by the scheduled workflow; the
// browser never talks to the GitHub API and never sees a token.
package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"evergreen/internal/engine"
	"evergreen/internal/procedural"
	"evergreen/internal/vegetation"
)

// SchemaVersion is the supported file format version.
const SchemaVersion = 1

const (
	maxRepos    = 200
	maxNameLen  = 100
	maxCount    = 10_000_000
	maxFileSize = 4 << 20
)

var repoName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Repo is one repository.
type Repo struct {
	Name     string `json:"name"`
	Stars    int    `json:"stars"`
	Language string `json:"language"`
	Releases int    `json:"releases"`
	Size     int    `json:"size"`
}

// User is the activity summary for one account.
type User struct {
	Login         string `json:"login"`
	Repos         []Repo `json:"repos"`
	Commits       int    `json:"commits"`       // recent public commits
	PullRequests  int    `json:"pullRequests"`  // authored pull requests
	Stars         int    `json:"stars"`         // stars received
	Releases      int    `json:"releases"`      // releases published
	Contributions int    `json:"contributions"` // recent public events
}

// File is the top-level JSON document.
type File struct {
	Version     int             `json:"version"`
	GeneratedAt time.Time       `json:"generatedAt"`
	Users       map[string]User `json:"users"`
}

// Parse decodes and validates untrusted JSON. Invalid users are dropped and
// reported; a structurally invalid file is rejected.
func Parse(data []byte) (*File, []error) {
	if len(data) > maxFileSize {
		return nil, []error{errors.New("activity file too large")}
	}
	var f File
	dec := json.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&f); err != nil {
		return nil, []error{fmt.Errorf("activity json: %w", err)}
	}
	if f.Version != SchemaVersion {
		return nil, []error{fmt.Errorf("unsupported activity version %d", f.Version)}
	}
	var errs []error
	clean := map[string]User{}
	for key, u := range f.Users {
		v, e := validateUser(u)
		if e != nil {
			errs = append(errs, fmt.Errorf("user %q: %w", key, e))
			continue
		}
		clean[strings.ToLower(v.Login)] = v
	}
	f.Users = clean
	return &f, errs
}

func clampInt(v int) int {
	if v < 0 {
		return 0
	}
	if v > maxCount {
		return maxCount
	}
	return v
}

func validateUser(u User) (User, error) {
	if !engine.ValidGithubUser(u.Login) {
		return User{}, fmt.Errorf("invalid login")
	}
	out := User{Login: u.Login, Commits: clampInt(u.Commits), PullRequests: clampInt(u.PullRequests),
		Stars: clampInt(u.Stars), Releases: clampInt(u.Releases), Contributions: clampInt(u.Contributions)}
	for _, r := range u.Repos {
		if len(out.Repos) >= maxRepos {
			break
		}
		if len(r.Name) == 0 || len(r.Name) > maxNameLen || !repoName.MatchString(r.Name) {
			continue
		}
		lang := r.Language
		if len(lang) > 40 {
			lang = lang[:40]
		}
		out.Repos = append(out.Repos, Repo{Name: r.Name, Stars: clampInt(r.Stars), Language: sanitize(lang), Releases: clampInt(r.Releases), Size: clampInt(r.Size)})
	}
	return out, nil
}

func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == '<' || r == '>' || r == '"' || r == '&' {
			return -1
		}
		return r
	}, s)
}

// Lookup finds a user case-insensitively.
func (f *File) Lookup(login string) (User, bool) {
	u, ok := f.Users[strings.ToLower(strings.TrimPrefix(login, "@"))]
	return u, ok
}

// RepoTree is a tree grown for one repository.
type RepoTree struct {
	Name  string
	Stars int
	Kind  vegetation.Kind
	Scale float64
	DX    float64 // offset from the forest entrance (metres)
	DZ    float64
}

// Influence is the environmental effect of an account's activity.
type Influence struct {
	Trees        []RepoTree
	PlantBoost   float64 // 0..1 extra plants (commits)
	FlowerBoost  float64 // 0..1 extra flowers (pull requests)
	FireflyBoost float64 // 0..1 extra fireflies (stars)
	TreeGrowth   float64 // 0..1 World Tree growth (releases)
	Density      float64 // 1.0..1.4 forest density multiplier (contributions)
}

// None is the neutral influence used without GitHub data.
func None() Influence { return Influence{Density: 1, TreeGrowth: 0.5} }

func logScale(n int, full float64) float64 {
	return math.Min(1, math.Log1p(float64(n))/math.Log1p(full))
}

// Influence derives environmental changes from a user's activity.
func (u User) Influence() Influence {
	repos := append([]Repo(nil), u.Repos...)
	sort.SliceStable(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	inf := Influence{
		PlantBoost:   logScale(u.Commits, 1500),
		FlowerBoost:  logScale(u.PullRequests, 300),
		FireflyBoost: logScale(u.Stars, 1000),
		TreeGrowth:   0.5 + 0.5*logScale(u.Releases, 80),
		Density:      1 + 0.4*logScale(u.Contributions, 2000),
	}
	kinds := []vegetation.Kind{vegetation.TreeOak, vegetation.TreePine, vegetation.TreeBirch, vegetation.TreeRain}
	for i, r := range repos {
		h := procedural.NewRNG(procedural.SeedFromString(u.Login + "/" + r.Name))
		ang := float64(i)*2.399963 + h.Range(-0.2, 0.2) // golden angle spiral
		rad := 24 + math.Sqrt(float64(i))*11 + h.Range(-2, 2)
		kind := kinds[int(procedural.SeedFromString(r.Language)%uint64(len(kinds)))]
		if r.Language == "" {
			kind = kinds[h.Intn(len(kinds))]
		}
		inf.Trees = append(inf.Trees, RepoTree{
			Name: r.Name, Stars: r.Stars, Kind: kind,
			Scale: 1 + 0.18*math.Log2(1+float64(r.Stars)) + 0.1*logScale(r.Size, 100000),
			DX:    math.Cos(ang) * rad, DZ: math.Sin(ang) * rad,
		})
	}
	return inf
}
