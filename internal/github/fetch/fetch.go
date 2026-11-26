// Package fetch talks to the GitHub REST API. It is used only by the
// scheduled workflow (cmd/evergreen-activity), never by the browser build.
package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"

	"evergreen/internal/engine"
	"evergreen/internal/github"
)

// Client fetches public activity.
type Client struct {
	BaseURL string
	Token   string // optional; raises rate limits. Never written to output.
	HTTP    *http.Client
}

// NewClient returns a client for api.github.com.
func NewClient(token string) *Client {
	return &Client{BaseURL: "https://api.github.com", Token: token, HTTP: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "evergreen-activity")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	return json.Unmarshal(body, out)
}

type apiRepo struct {
	Name     string `json:"name"`
	Stars    int    `json:"stargazers_count"`
	Language string `json:"language"`
	Size     int    `json:"size"`
	Fork     bool   `json:"fork"`
	Archived bool   `json:"archived"`
}

type apiEvent struct {
	Type    string `json:"type"`
	Payload struct {
		Size int `json:"size"`
	} `json:"payload"`
}

// User collects the activity summary for one login.
func (c *Client) User(ctx context.Context, login string) (github.User, error) {
	if !engine.ValidGithubUser(login) {
		return github.User{}, fmt.Errorf("invalid login %q", login)
	}
	u := github.User{Login: login}
	var repos []apiRepo
	if err := c.get(ctx, "/users/"+url.PathEscape(login)+"/repos", url.Values{"per_page": {"100"}, "sort": {"pushed"}, "type": {"owner"}}, &repos); err != nil {
		return u, err
	}
	sort.SliceStable(repos, func(i, j int) bool { return repos[i].Stars > repos[j].Stars })
	for i, r := range repos {
		if r.Fork {
			continue
		}
		repo := github.Repo{Name: r.Name, Stars: r.Stars, Language: r.Language, Size: r.Size}
		u.Stars += r.Stars
		if i < 10 {
			var rel []struct {
				ID int `json:"id"`
			}
			if err := c.get(ctx, "/repos/"+url.PathEscape(login)+"/"+url.PathEscape(r.Name)+"/releases", url.Values{"per_page": {"100"}}, &rel); err == nil {
				repo.Releases = len(rel)
				u.Releases += len(rel)
			}
		}
		u.Repos = append(u.Repos, repo)
	}
	var events []apiEvent
	if err := c.get(ctx, "/users/"+url.PathEscape(login)+"/events/public", url.Values{"per_page": {"100"}}, &events); err == nil {
		for _, e := range events {
			u.Contributions++
			if e.Type == "PushEvent" {
				u.Commits += e.Payload.Size
			}
		}
	}
	var search struct {
		Total int `json:"total_count"`
	}
	if err := c.get(ctx, "/search/issues", url.Values{"q": {"author:" + login + " type:pr"}, "per_page": {"1"}}, &search); err == nil {
		u.PullRequests = search.Total
	}
	return u, nil
}

// Build fetches every login and returns a file ready to marshal. A login that
// fails is skipped and reported so one bad account cannot break the others.
func (c *Client) Build(ctx context.Context, logins []string, now time.Time) (*github.File, []error) {
	f := &github.File{Version: github.SchemaVersion, GeneratedAt: now.UTC(), Users: map[string]github.User{}}
	var errs []error
	for _, l := range logins {
		u, err := c.User(ctx, l)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", l, err))
			continue
		}
		f.Users[l] = u
	}
	return f, errs
}
