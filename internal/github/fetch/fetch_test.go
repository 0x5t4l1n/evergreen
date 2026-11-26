package fetch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"evergreen/internal/github"
)

func server(t *testing.T, gotAuth *string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/users/octo/repos", func(w http.ResponseWriter, r *http.Request) {
		*gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`[{"name":"a","stargazers_count":5,"language":"Go","size":10},{"name":"fork","stargazers_count":99,"fork":true},{"name":"b","stargazers_count":2,"language":"C"}]`))
	})
	mux.HandleFunc("/repos/octo/a/releases", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[{"id":1},{"id":2}]`)) })
	mux.HandleFunc("/repos/octo/b/releases", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	mux.HandleFunc("/users/octo/events/public", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"type":"PushEvent","payload":{"size":3}},{"type":"WatchEvent","payload":{}},{"type":"PushEvent","payload":{"size":2}}]`))
	})
	mux.HandleFunc("/search/issues", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Query().Get("q"), "author:octo") {
			http.Error(w, "bad query", 400)
			return
		}
		_, _ = w.Write([]byte(`{"total_count":7}`))
	})
	mux.HandleFunc("/users/missing/repos", func(w http.ResponseWriter, r *http.Request) { http.NotFound(w, r) })
	return httptest.NewServer(mux)
}

func TestUserAggregation(t *testing.T) {
	var auth string
	srv := server(t, &auth)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "secret-token", HTTP: srv.Client()}
	u, err := c.User(context.Background(), "octo")
	if err != nil {
		t.Fatal(err)
	}
	if len(u.Repos) != 2 || u.Stars != 7 || u.Releases != 2 || u.Commits != 5 || u.Contributions != 3 || u.PullRequests != 7 {
		t.Fatalf("aggregation wrong: %+v", u)
	}
	if auth != "Bearer secret-token" {
		t.Fatalf("token should be sent as bearer header, got %q", auth)
	}
}

func TestBuildOutputNeverContainsToken(t *testing.T) {
	var auth string
	srv := server(t, &auth)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "secret-token", HTTP: srv.Client()}
	f, errs := c.Build(context.Background(), []string{"octo", "missing", "bad user"}, time.Unix(0, 0))
	if len(errs) != 2 {
		t.Fatalf("missing and invalid users should be reported: %v", errs)
	}
	data, _ := json.Marshal(f)
	if strings.Contains(string(data), "secret-token") {
		t.Fatal("token leaked into output")
	}
	parsed, perrs := github.Parse(data)
	if parsed == nil || len(perrs) != 0 {
		t.Fatalf("output must round-trip through the validator: %v", perrs)
	}
	if _, ok := parsed.Lookup("octo"); !ok {
		t.Fatal("octo missing from output")
	}
}

func TestInvalidLoginRejectedBeforeRequest(t *testing.T) {
	c := &Client{BaseURL: "http://127.0.0.1:1", HTTP: http.DefaultClient}
	if _, err := c.User(context.Background(), "../etc/passwd"); err == nil {
		t.Fatal("path traversal login must be rejected")
	}
}
