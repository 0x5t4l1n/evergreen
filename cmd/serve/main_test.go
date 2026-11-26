package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"index.html": "<h1>hi</h1>", "evergreen.wasm": "\x00asm\x01\x00\x00\x00", "a.svg": "<svg/>"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestWasmMimeAndNoCache(t *testing.T) {
	srv := httptest.NewServer(handler(setup(t), ""))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/evergreen.wasm")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "application/wasm" {
		t.Fatalf("wasm content type = %q", ct)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("dev server must not cache")
	}
	r2, err2 := http.Get(srv.URL + "/a.svg")
	if err2 != nil {
		t.Fatal(err2)
	}
	defer r2.Body.Close()
	if ct := r2.Header.Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("svg content type = %q", ct)
	}
}

func TestSubPathMountMimicsGitHubPages(t *testing.T) {
	srv := httptest.NewServer(handler(setup(t), "/evergreen"))
	defer srv.Close()
	for path, want := range map[string]int{"/evergreen/": 200, "/evergreen/evergreen.wasm": 200, "/index.html": 404, "/evergreen/missing": 404} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("%s: got %d want %d", path, resp.StatusCode, want)
		}
	}
}
