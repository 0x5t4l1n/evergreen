//go:build !(js && wasm)

// Command evergreen only runs in the browser; build it with
// GOOS=js GOARCH=wasm (see `make wasm`).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "evergreen runs in the browser: build with `make wasm` and serve with `make serve`")
	os.Exit(2)
}
