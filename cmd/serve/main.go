// Command serve is a small static file server for local development. It
// serves WebAssembly with the correct MIME type, disables caching so rebuilds
// show up immediately, and can mount the site under a sub-path to mimic
// GitHub Pages project URLs.
package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"strings"
)

func handler(dir, prefix string) http.Handler {
	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
	_ = mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	fs := http.FileServer(http.Dir(dir))
	if prefix != "" {
		fs = http.StripPrefix(prefix, fs)
	}
	mux := http.NewServeMux()
	root := "/"
	if prefix != "" {
		root = prefix + "/"
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" {
				http.Redirect(w, r, root, http.StatusFound)
				return
			}
			http.NotFound(w, r)
		})
	}
	mux.Handle(root, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fs.ServeHTTP(w, r)
	}))
	return mux
}

func main() {
	dir := flag.String("dir", "dist", "directory to serve")
	addr := flag.String("addr", "127.0.0.1:8080", "listen address")
	prefix := flag.String("prefix", "", "optional URL prefix, e.g. /evergreen")
	flag.Parse()
	if st, err := os.Stat(*dir); err != nil || !st.IsDir() {
		log.Fatalf("directory %q not found; run `make build` first", *dir)
	}
	p := strings.TrimRight(*prefix, "/")
	if p != "" && !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	fmt.Printf("Evergreen dev server: http://%s%s/\n", *addr, p)
	log.Fatal(http.ListenAndServe(*addr, handler(*dir, p)))
}
