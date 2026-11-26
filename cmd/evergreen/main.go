//go:build js && wasm

// Command evergreen is the WebAssembly entry point of the Evergreen world.
package main

import "evergreen/internal/app"

func main() { app.Run() }
