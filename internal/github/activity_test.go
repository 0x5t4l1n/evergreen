package github

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

const sample = `{"version":1,"generatedAt":"2026-01-02T03:04:05Z","users":{
 "Octocat":{"login":"Octocat","commits":120,"pullRequests":14,"stars":55,"releases":9,"contributions":300,
   "repos":[{"name":"hello-world","stars":40,"language":"Go","releases":3,"size":120},
            {"name":"spoon-knife","stars":10,"language":"HTML","size":10},
            {"name":"bad name!","stars":1},
            {"name":"","stars":1}]},
 "bad user":{"login":"bad user"}
}}`

func TestParseValidatesAndFilters(t *testing.T) {
	f, errs := Parse([]byte(sample))
	if f == nil {
		t.Fatalf("parse failed: %v", errs)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "bad user") {
		t.Fatalf("expected exactly the invalid user to be reported: %v", errs)
	}
	u, ok := f.Lookup("@octocat")
	if !ok || u.Login != "Octocat" {
		t.Fatal("case-insensitive lookup with @ prefix")
	}
	if len(u.Repos) != 2 {
		t.Fatalf("invalid repo names must be dropped, got %d", len(u.Repos))
	}
	if _, ok := f.Lookup("nobody"); ok {
		t.Fatal("unknown user")
	}
}

func TestParseRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"garbage":       `{nope`,
		"wrong version": `{"version":99,"users":{}}`,
		"empty":         ``,
	}
	for name, in := range cases {
		if f, errs := Parse([]byte(in)); f != nil || len(errs) == 0 {
			t.Errorf("%s: should be rejected", name)
		}
	}
	if f, errs := Parse([]byte(strings.Repeat(" ", maxFileSize+1))); f != nil || len(errs) == 0 {
		t.Error("oversized file should be rejected")
	}
}

func TestNumbersAreClamped(t *testing.T) {
	f, _ := Parse([]byte(`{"version":1,"users":{"a":{"login":"a","commits":-5,"stars":99999999999,"repos":[{"name":"r","stars":-1,"language":"<script>"}]}}}`))
	u, _ := f.Lookup("a")
	if u.Commits != 0 || u.Stars != maxCount || u.Repos[0].Stars != 0 {
		t.Fatalf("not clamped: %+v", u)
	}
	if strings.ContainsAny(u.Repos[0].Language, "<>") {
		t.Fatal("language must be sanitised")
	}
}

func TestRepoLimit(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"version":1,"users":{"a":{"login":"a","repos":[`)
	for i := 0; i < 500; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"name":"r%d"}`, i)
	}
	sb.WriteString(`]}}}`)
	f, _ := Parse([]byte(sb.String()))
	u, _ := f.Lookup("a")
	if len(u.Repos) != maxRepos {
		t.Fatalf("repo cap: %d", len(u.Repos))
	}
}

func TestInfluenceMapping(t *testing.T) {
	f, _ := Parse([]byte(sample))
	u, _ := f.Lookup("octocat")
	inf := u.Influence()
	if len(inf.Trees) != 2 {
		t.Fatalf("each repository grows a tree: %d", len(inf.Trees))
	}
	if inf.PlantBoost <= 0 || inf.FlowerBoost <= 0 || inf.FireflyBoost <= 0 || inf.TreeGrowth <= 0.5 || inf.Density <= 1 {
		t.Fatalf("all activity channels should register: %+v", inf)
	}
	var big, small RepoTree
	for _, tr := range inf.Trees {
		if tr.Name == "hello-world" {
			big = tr
		} else {
			small = tr
		}
	}
	if big.Scale <= small.Scale {
		t.Fatal("starred repos grow bigger trees")
	}
	again := u.Influence()
	for i := range inf.Trees {
		if inf.Trees[i] != again.Trees[i] {
			t.Fatal("influence must be deterministic")
		}
	}
}

func TestMoreActivityMoreForest(t *testing.T) {
	low := User{Login: "a", Commits: 5, Releases: 1}.Influence()
	high := User{Login: "a", Commits: 900, Releases: 60, Stars: 800, PullRequests: 200, Contributions: 1800}.Influence()
	if high.PlantBoost <= low.PlantBoost || high.TreeGrowth <= low.TreeGrowth || high.Density <= low.Density || high.FireflyBoost <= low.FireflyBoost {
		t.Fatalf("low=%+v high=%+v", low, high)
	}
	if high.Density > 1.4+1e-9 || high.TreeGrowth > 1+1e-9 {
		t.Fatal("influence must saturate")
	}
}

func TestNoneIsNeutral(t *testing.T) {
	n := None()
	if n.Density != 1 || len(n.Trees) != 0 || n.PlantBoost != 0 {
		t.Fatalf("%+v", n)
	}
}

func TestGroveWithinReach(t *testing.T) {
	var repos []Repo
	for i := 0; i < 60; i++ {
		repos = append(repos, Repo{Name: fmt.Sprintf("r%d", i)})
	}
	inf := User{Login: "a", Repos: repos}.Influence()
	for _, tr := range inf.Trees {
		if d := math.Hypot(tr.DX, tr.DZ); d < 18 || d > 120 {
			t.Fatalf("repo tree out of the entrance grove: %v", d)
		}
	}
}
