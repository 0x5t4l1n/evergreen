// Command evergreen-svg renders an animated Evergreen forest as an SVG that
// can be embedded in a GitHub README.
//
//	go run ./cmd/evergreen-svg --seed 42 --theme night --output assets/evergreen.svg
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"evergreen/internal/procedural"
	"evergreen/internal/svg"
)

func main() {
	def := svg.DefaultOptions()
	width := flag.Int("width", def.Width, "image width in pixels")
	height := flag.Int("height", def.Height, "image height in pixels")
	seed := flag.String("seed", "42", "world seed (number or text)")
	theme := flag.String("theme", def.Theme, "theme: "+strings.Join(svg.Themes, ", "))
	density := flag.Float64("density", def.Density, "vegetation density 0.2..2")
	intensity := flag.Float64("intensity", def.Intensity, "animation intensity 0..2 (0 = still)")
	output := flag.String("output", "assets/evergreen.svg", "output file, or - for stdout")
	flag.Parse()

	opts := svg.Options{Width: *width, Height: *height, Seed: procedural.SeedFromString(*seed), Theme: *theme, Density: *density, Intensity: *intensity}
	doc, err := svg.Generate(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
	if *output == "-" {
		fmt.Print(doc)
		return
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*output, []byte(doc), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d bytes, theme=%s, seed=%s)\n", *output, len(doc), *theme, *seed)
}
