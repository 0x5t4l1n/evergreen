// Package svg renders Evergreen as a standalone animated SVG suitable for a
// GitHub README. Everything is driven by SMIL and CSS animation: no scripts,
// no external references, so it plays inside an <img> tag.
package svg

import (
	"fmt"
	"math"
	"strings"

	"evergreen/internal/procedural"
)

// Options configure the generator.
type Options struct {
	Width, Height int
	Seed          uint64
	Theme         string  // day|dawn|dusk|night|cycle
	Density       float64 // 0.2..2 vegetation density
	Intensity     float64 // 0..2 animation speed multiplier (0 = still)
}

// Themes lists the supported themes.
var Themes = []string{"day", "dawn", "dusk", "night", "cycle"}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{Width: 1200, Height: 520, Seed: 42, Theme: "day", Density: 1, Intensity: 1}
}

// Validate checks and normalises options.
func (o *Options) Validate() error {
	if o.Width < 320 || o.Width > 4000 || o.Height < 160 || o.Height > 2400 {
		return fmt.Errorf("size %dx%d out of range (320..4000 x 160..2400)", o.Width, o.Height)
	}
	ok := false
	for _, t := range Themes {
		if o.Theme == t {
			ok = true
		}
	}
	if !ok {
		return fmt.Errorf("unknown theme %q (want one of %s)", o.Theme, strings.Join(Themes, ", "))
	}
	if o.Density < 0.2 || o.Density > 2 {
		return fmt.Errorf("density %.2f out of range (0.2..2)", o.Density)
	}
	if o.Intensity < 0 || o.Intensity > 2 {
		return fmt.Errorf("intensity %.2f out of range (0..2)", o.Intensity)
	}
	return nil
}

type palette struct {
	skyTop, skyMid, skyLow        string
	sun, mountainFar, mountainMid string
	fog                           string
	leafA, leafB, leafC           string
	trunk                         string
	water, waterHi                string
	stars                         bool
	moon                          bool
	glow                          float64
	light                         float64 // overall brightness 0..1
}

func paletteFor(theme string) palette {
	switch theme {
	case "night":
		return palette{"#030914", "#0a1830", "#16304a", "#dfe9ff", "#0e2236", "#0b1c2c", "#6fa0b8", "#0f4a3a", "#0b3a30", "#14604a", "#2a1c14", "#0b2a45", "#4fc3d9", true, true, 1, 0.25}
	case "dusk":
		return palette{"#1a1740", "#7a3f6b", "#f08a4b", "#ffd29a", "#4a3a68", "#2f3a58", "#e7a58a", "#1b5a38", "#164a32", "#2a7a46", "#33200f", "#2a4a6a", "#ffb27a", true, false, 0.5, 0.55}
	case "dawn":
		return palette{"#27406e", "#e6a7a0", "#ffe3b3", "#fff1c9", "#7a8fb0", "#51709a", "#ffe1cc", "#2f8a4c", "#237a45", "#47a35a", "#3a2a18", "#3b7eaa", "#fff0cf", false, false, 0.2, 0.75}
	}
	return palette{"#2f78c4", "#7fb9e8", "#cfe9f2", "#fff6c9", "#7fa3b8", "#5c8aa0", "#e3f2ee", "#1f8a45", "#16703a", "#34b05a", "#4a3220", "#2e86b8", "#bfefff", false, false, 0, 1}
}

type gen struct {
	o   Options
	p   palette
	r   *procedural.RNG
	sb  strings.Builder
	w   float64
	h   float64
	nid int
}

func (g *gen) f(format string, a ...any) { fmt.Fprintf(&g.sb, format, a...) }

// dur scales an animation duration by the intensity (slower when calmer).
func (g *gen) dur(sec float64) string {
	if g.o.Intensity <= 0 {
		sec = 1e6
	} else {
		sec /= g.o.Intensity
	}
	return fmt.Sprintf("%.2fs", sec)
}

func (g *gen) id(prefix string) string {
	g.nid++
	return fmt.Sprintf("%s%d", prefix, g.nid)
}

// Generate renders the SVG document.
func Generate(o Options) (string, error) {
	if err := o.Validate(); err != nil {
		return "", err
	}
	g := &gen{o: o, p: paletteFor(o.Theme), r: procedural.NewRNG(o.Seed ^ 0x5EED), w: float64(o.Width), h: float64(o.Height)}
	g.document()
	return g.sb.String(), nil
}

func (g *gen) document() {
	w, h := g.w, g.h
	g.f(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img" aria-labelledby="t d">`+"\n", g.o.Width, g.o.Height, g.o.Width, g.o.Height)
	g.f("<title id=\"t\">Evergreen: an infinite living world</title>\n<desc id=\"d\">An animated forest with a glowing World Tree, a waterfall, a flowing river, drifting fog, falling leaves, birds and fireflies.</desc>\n")
	g.defs()
	g.f(`<rect width="%d" height="%d" fill="url(#sky)"/>`+"\n", g.o.Width, g.o.Height)
	g.skyBodies()
	g.mountains(0.46, g.p.mountainFar, 70, 0.55, 11)
	g.fogBand(0.50, 0.32, 38)
	g.mountains(0.56, g.p.mountainMid, 55, 0.8, 23)
	g.fogBand(0.60, 0.28, 52)
	g.treeLayer(0.60, 0.32, 0.55, 0.85*g.o.Density, 0.55)
	g.worldTree(w*0.68, h*0.74)
	g.waterfall(w*0.2, h*0.5)
	g.treeLayer(0.72, 0.5, 0.8, 0.9*g.o.Density, 0.8)
	g.river()
	g.fogBand(0.78, 0.22, 44)
	g.treeLayer(0.9, 0.85, 1.1, 0.6*g.o.Density, 1.0)
	g.foreground()
	g.leaves()
	g.birds()
	g.fireflies()
	g.f(`<rect width="%d" height="%d" fill="url(#vig)"/>`+"\n", g.o.Width, g.o.Height)
	g.f("</svg>\n")
	_ = h
}

func (g *gen) defs() {
	p := g.p
	g.f("<defs>\n")
	if g.o.Theme == "cycle" {
		d := g.dur(48)
		g.f(`<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1">`)
		g.f(`<stop offset="0" stop-color="#2f78c4"><animate attributeName="stop-color" dur="%s" repeatCount="indefinite" values="#2f78c4;#27406e;#1a1740;#030914;#030914;#27406e;#2f78c4"/></stop>`, d)
		g.f(`<stop offset="0.55" stop-color="#7fb9e8"><animate attributeName="stop-color" dur="%s" repeatCount="indefinite" values="#7fb9e8;#e6a7a0;#7a3f6b;#0a1830;#0a1830;#e6a7a0;#7fb9e8"/></stop>`, d)
		g.f(`<stop offset="1" stop-color="#cfe9f2"><animate attributeName="stop-color" dur="%s" repeatCount="indefinite" values="#cfe9f2;#ffe3b3;#f08a4b;#16304a;#16304a;#ffe3b3;#cfe9f2"/></stop>`, d)
		g.f("</linearGradient>\n")
	} else {
		g.f(`<linearGradient id="sky" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s"/><stop offset="0.55" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient>`+"\n", p.skyTop, p.skyMid, p.skyLow)
	}
	g.f(`<radialGradient id="sun"><stop offset="0" stop-color="%s" stop-opacity="1"/><stop offset="0.35" stop-color="%s" stop-opacity="0.5"/><stop offset="1" stop-color="%s" stop-opacity="0"/></radialGradient>`+"\n", p.sun, p.sun, p.sun)
	g.f(`<radialGradient id="vig" cx="0.5" cy="0.5" r="0.75"><stop offset="0.6" stop-color="#000" stop-opacity="0"/><stop offset="1" stop-color="#000" stop-opacity="%.2f"/></radialGradient>`+"\n", 0.35+0.25*(1-p.light))
	g.f(`<linearGradient id="fog" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s" stop-opacity="0"/><stop offset="0.5" stop-color="%s" stop-opacity="0.55"/><stop offset="1" stop-color="%s" stop-opacity="0"/></linearGradient>`+"\n", p.fog, p.fog, p.fog)
	g.f(`<linearGradient id="river" x1="0" y1="0" x2="1" y2="0"><stop offset="0" stop-color="%s"/><stop offset="0.5" stop-color="%s"/><stop offset="1" stop-color="%s"/></linearGradient>`+"\n", p.water, p.waterHi, p.water)
	g.f(`<linearGradient id="fall" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stop-color="%s" stop-opacity="0.9"/><stop offset="1" stop-color="#ffffff" stop-opacity="0.85"/></linearGradient>`+"\n", p.waterHi)
	g.f(`<radialGradient id="glow"><stop offset="0" stop-color="#9dffd8" stop-opacity="0.95"/><stop offset="1" stop-color="#9dffd8" stop-opacity="0"/></radialGradient>` + "\n")
	g.f(`<radialGradient id="ff"><stop offset="0" stop-color="#fff7a8" stop-opacity="1"/><stop offset="1" stop-color="#ffe45c" stop-opacity="0"/></radialGradient>` + "\n")
	g.f(`<path id="leaf" d="M0 0 C 4 -5 9 -4 11 0 C 9 4 4 5 0 0 Z"/>` + "\n")
	g.f("</defs>\n")
}

func (g *gen) skyBodies() {
	p := g.p
	w, h := g.w, g.h
	if p.stars {
		g.f(`<g fill="#fff">`)
		for i := 0; i < int(90*math.Sqrt(w*h)/700); i++ {
			x, y := g.r.Range(0, w), g.r.Range(0, h*0.5)
			s := g.r.Range(0.5, 1.6)
			g.f(`<circle cx="%.1f" cy="%.1f" r="%.2f" opacity="%.2f"><animate attributeName="opacity" values="%.2f;%.2f;%.2f" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></circle>`,
				x, y, s, g.r.Range(0.3, 0.9), 0.25, 1.0, 0.25, g.dur(g.r.Range(2, 6)), g.r.Range(0, 5))
		}
		g.f("</g>\n")
		// shooting star
		g.f(`<line x1="0" y1="0" x2="-60" y2="-22" stroke="#fff" stroke-width="1.6" stroke-linecap="round" opacity="0"><animateMotion dur="%s" repeatCount="indefinite" path="M %.0f %.0f L %.0f %.0f" begin="2s"/><animate attributeName="opacity" values="0;0;1;0;0" keyTimes="0;0.7;0.75;0.82;1" dur="%s" repeatCount="indefinite" begin="2s"/></line>`+"\n",
			g.dur(9), w*0.75, h*0.08, w*0.35, h*0.3, g.dur(9))
	}
	if g.o.Theme == "cycle" {
		// sun and moon travel opposite arcs across the 48 second day
		d := g.dur(48)
		arc := fmt.Sprintf("M %.0f %.0f Q %.0f %.0f %.0f %.0f", w*0.05, h*0.5, w*0.5, -h*0.25, w*0.95, h*0.5)
		g.f(`<g><circle r="%.0f" fill="url(#sun)"/><circle r="%.0f" fill="#fff6c9"/><animateMotion dur="%s" repeatCount="indefinite" path="%s"/></g>`+"\n", h*0.16, h*0.045, d, arc)
		g.f(`<g><circle r="%.0f" fill="#dfe9ff"/><circle cx="%.1f" r="%.0f" fill="#0a1830"/><animateMotion dur="%s" begin="-24s" repeatCount="indefinite" path="%s"/></g>`+"\n", h*0.04, h*0.014, h*0.036, d, arc)
		return
	}
	if p.moon {
		g.f(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="url(#sun)" opacity="0.6"/><circle cx="%.0f" cy="%.0f" r="%.0f" fill="#e8efff"/><circle cx="%.0f" cy="%.0f" r="%.0f" fill="#0a1830" opacity="0.85"/>`+"\n",
			w*0.2, h*0.17, h*0.17, w*0.2, h*0.17, h*0.05, w*0.2+h*0.017, h*0.17-h*0.006, h*0.043)
		return
	}
	sy := h * 0.3
	if g.o.Theme != "day" {
		sy = h * 0.44
	}
	g.f(`<circle cx="%.0f" cy="%.0f" r="%.0f" fill="url(#sun)"/><circle cx="%.0f" cy="%.0f" r="%.0f" fill="%s"/>`+"\n", w*0.24, sy, h*0.24, w*0.24, sy, h*0.05, p.sun)
}

func (g *gen) mountains(base float64, col string, amp, op float64, salt uint64) {
	r := procedural.NewRNG(g.o.Seed*131 + salt)
	w, h := g.w, g.h
	var sb strings.Builder
	fmt.Fprintf(&sb, "M0 %.1f ", h*base)
	steps := 14
	for i := 0; i <= steps; i++ {
		x := w * float64(i) / float64(steps)
		y := h*base - amp*(0.35+0.65*math.Abs(procedural.Noise2(g.o.Seed+salt, float64(i)*0.7, 3)))*(0.6+0.8*r.Float())
		fmt.Fprintf(&sb, "L%.1f %.1f ", x, y)
	}
	fmt.Fprintf(&sb, "L%.0f %.0f L0 %.0f Z", w, h, h)
	g.f(`<path d="%s" fill="%s" opacity="%.2f"/>`+"\n", sb.String(), col, op)
}

func (g *gen) fogBand(y, op, h0 float64) {
	h := g.h
	d := g.dur(g.r.Range(26, 44))
	g.f(`<g opacity="%.2f"><rect x="%.0f" y="%.1f" width="%.0f" height="%.0f" fill="url(#fog)"><animateTransform attributeName="transform" type="translate" values="%.0f 0;%.0f 0;%.0f 0" dur="%s" repeatCount="indefinite"/></rect></g>`+"\n",
		op, -g.w*0.4, h*y-h0, g.w*1.8, h0*2, 0.0, g.w*0.2, 0.0, d)
}

func (g *gen) shade(c string, k float64) string {
	var r, gg, b int
	fmt.Sscanf(c, "#%02x%02x%02x", &r, &gg, &b)
	f := func(v int) int { return int(math.Max(0, math.Min(255, float64(v)*k))) }
	return fmt.Sprintf("#%02x%02x%02x", f(r), f(gg), f(b))
}

// treeLayer draws a row of conifers and round trees along a ground line.
func (g *gen) treeLayer(ground, size, depthShade, density, sway float64) {
	w, h := g.w, g.h
	n := int(w / 38 * density * (0.6 + size))
	k := 0.55 + 0.45*depthShade
	if g.p.light < 0.5 {
		k = 0.9
	}
	for i := 0; i < n; i++ {
		x := g.r.Range(-20, w+20)
		gy := h*ground + g.r.Range(-6, 8)
		sz := (28 + g.r.Range(0, 38)) * (0.55 + size)
		col := []string{g.p.leafA, g.p.leafB, g.p.leafC}[g.r.Intn(3)]
		col = g.shade(col, k)
		trunk := g.shade(g.p.trunk, k)
		dur := g.dur(g.r.Range(4, 8) / sway)
		g.f(`<g transform="translate(%.1f %.1f)"><rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`, x, gy, -sz*0.04, -sz*0.3, sz*0.08, sz*0.32, trunk)
		g.f(`<g><animateTransform attributeName="transform" type="rotate" values="-1.2 0 0;1.2 0 0;-1.2 0 0" dur="%s" begin="-%.1fs" repeatCount="indefinite"/>`, dur, g.r.Range(0, 6))
		if g.r.Chance(0.55) {
			for t := 0; t < 4; t++ {
				ty := -sz*0.25 - float64(t)*sz*0.26
				tw := sz * (0.46 - float64(t)*0.09)
				g.f(`<path d="M%.1f %.1f L0 %.1f L%.1f %.1f Z" fill="%s"/>`, -tw, ty, ty-sz*0.42, tw, ty, g.shade(col, 0.88+0.08*float64(t)))
			}
		} else {
			for t := 0; t < 4; t++ {
				cx, cy := g.r.Range(-sz*0.3, sz*0.3), -sz*(0.62+g.r.Range(0, 0.55))
				g.f(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, cx, cy, sz*g.r.Range(0.26, 0.4), g.shade(col, 0.85+0.1*float64(t)))
			}
			g.f(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s" opacity="0.5"/>`, -sz*0.1, -sz*0.95, sz*0.16, g.shade(col, 1.25))
		}
		g.f("</g></g>\n")
	}
}

func (g *gen) worldTree(cx, gy float64) {
	h := g.h
	size := h * 0.46
	trunk := g.shade(g.p.trunk, 0.9)
	canopy := g.shade(g.p.leafC, 0.9)
	bright := g.p.light
	g.f(`<g transform="translate(%.1f %.1f)">`+"\n", cx, gy)
	g.f(`<ellipse cx="0" cy="%.1f" rx="%.1f" ry="%.1f" fill="url(#glow)" opacity="%.2f"><animate attributeName="opacity" values="%.2f;%.2f;%.2f" dur="%s" repeatCount="indefinite"/></ellipse>`+"\n",
		-size*0.9, size*0.8, size*0.55, 0.15+0.5*g.p.glow, 0.1+0.35*g.p.glow, 0.25+0.6*g.p.glow, 0.1+0.35*g.p.glow, g.dur(7))
	// roots
	for i := -4; i <= 4; i++ {
		sx := float64(i) * size * 0.09
		g.f(`<path d="M%.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f L%.1f %.1f Z" fill="%s"/>`, sx*0.25, -size*0.18, sx*0.7, -size*0.06, sx*1.5, size*0.0, sx*2.1, size*0.02, sx*2.2, size*0.06, trunk)
	}
	// trunk
	g.f(`<path d="M%.1f 0 C %.1f %.1f %.1f %.1f %.1f %.1f L%.1f %.1f C %.1f %.1f %.1f %.1f %.1f 0 Z" fill="%s"/>`+"\n",
		-size*0.14, -size*0.1, -size*0.5, -size*0.1, -size*1.0, -size*0.06, -size*1.15, size*0.06, -size*1.15, size*0.1, -size*0.5, size*0.14, -size*0.1, size*0.14, trunk)
	// glowing veins
	for i := 0; i < 5; i++ {
		x := g.r.Range(-size*0.08, size*0.08)
		g.f(`<path d="M%.1f 0 q %.1f %.1f %.1f %.1f" stroke="#7dffd0" stroke-width="1.6" fill="none" stroke-linecap="round" opacity="0.7" stroke-dasharray="10 14"><animate attributeName="stroke-dashoffset" from="0" to="-48" dur="%s" repeatCount="indefinite"/></path>`+"\n",
			x, g.r.Range(-6, 6), -size*0.4, g.r.Range(-5, 5), -size*0.9, g.dur(g.r.Range(4, 8)))
	}
	// canopy
	g.f(`<g><animateTransform attributeName="transform" type="rotate" values="-0.6 0 0;0.6 0 0;-0.6 0 0" dur="%s" repeatCount="indefinite"/>`, g.dur(9))
	for i := 0; i < 26; i++ {
		a := g.r.Range(0, 2*math.Pi)
		rr := g.r.Range(0, size*0.55)
		x, y := math.Cos(a)*rr*1.3, -size*1.1-math.Abs(math.Sin(a))*rr*0.6
		c := g.shade(canopy, 0.75+0.5*g.r.Float()*bright+0.1)
		g.f(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, x, y, size*g.r.Range(0.1, 0.2), c)
	}
	g.f("</g>\n")
	// glowing motes drifting upward through the canopy
	for i := 0; i < int(18*g.o.Density+4); i++ {
		x := g.r.Range(-size*0.6, size*0.6)
		y0 := -size * g.r.Range(0.4, 1.2)
		g.f(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#a8ffe0" opacity="0"><animate attributeName="cy" values="%.1f;%.1f" dur="%s" begin="-%.1fs" repeatCount="indefinite"/><animate attributeName="opacity" values="0;%.2f;0" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></circle>`+"\n",
			x, y0, g.r.Range(1, 2.6), y0, y0-size*0.5, g.dur(g.r.Range(5, 10)), g.r.Range(0, 8), 0.4+0.6*g.p.glow, g.dur(g.r.Range(5, 10)), g.r.Range(0, 8))
	}
	g.f("</g>\n")
}

func (g *gen) waterfall(x, y float64) {
	h := g.h
	cliffH := h * 0.3
	wd := g.w * 0.05
	col := g.shade(g.p.mountainMid, 0.75)
	// jagged cliff face with a lighter lit edge
	var sb strings.Builder
	fmt.Fprintf(&sb, "M%.1f %.1f ", x-wd*3.4, y+cliffH)
	const n = 10
	for i := 0; i <= n; i++ {
		t := float64(i) / n
		px := x - wd*3.2 + t*wd*6.6
		py := y - 14 - 26*math.Sin(t*math.Pi)*(0.5+0.5*g.r.Float()) + g.r.Range(-6, 6)
		fmt.Fprintf(&sb, "L%.1f %.1f ", px, py)
	}
	fmt.Fprintf(&sb, "L%.1f %.1f Z", x+wd*3.4, y+cliffH)
	g.f(`<path d="%s" fill="%s"/>`+"\n", sb.String(), col)
	g.f(`<path d="%s" fill="#000" opacity="0.18" transform="translate(%.1f 0) scale(0.6 1) translate(%.1f 0)"/>`+"\n", sb.String(), x*0.4, -x*0.4*0.0)
	// the fall: tapered sheet that widens as it drops
	g.f(`<path d="M%.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f L%.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f Z" fill="url(#fall)" opacity="0.92"/>`+"\n",
		x-wd*0.45, y-10, x-wd*0.5, y+cliffH*0.3, x-wd*0.7, y+cliffH*0.7, x-wd*0.85, y+cliffH,
		x+wd*0.85, y+cliffH, x+wd*0.7, y+cliffH*0.7, x+wd*0.5, y+cliffH*0.3, x+wd*0.45, y-10)
	for i := 0; i < 9; i++ {
		lx := x - wd*0.4 + g.r.Range(0, 0.8)*wd
		g.f(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#fff" stroke-width="1.4" stroke-dasharray="10 18" opacity="0.8"><animate attributeName="stroke-dashoffset" from="0" to="-56" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></line>`+"\n",
			lx, y-6, lx+g.r.Range(-3, 3), y+cliffH, g.dur(g.r.Range(0.9, 1.5)), g.r.Range(0, 1))
	}
	for i := 0; i < 8; i++ {
		mx := x + g.r.Range(-wd, wd)
		g.f(`<circle cx="%.1f" cy="%.1f" r="4" fill="#fff" opacity="0"><animate attributeName="r" values="5;24" dur="%s" begin="-%.1fs" repeatCount="indefinite"/><animate attributeName="opacity" values="0;0.38;0" dur="%s" begin="-%.1fs" repeatCount="indefinite"/><animate attributeName="cy" values="%.1f;%.1f" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></circle>`+"\n",
			mx, y+cliffH, g.dur(4), g.r.Range(0, 4), g.dur(4), g.r.Range(0, 4), y+cliffH, y+cliffH-36, g.dur(4), g.r.Range(0, 4))
	}
}

func (g *gen) river() {
	w, h := g.w, g.h
	y := h * 0.86
	g.f(`<path d="M%.0f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f S %.1f %.1f %.1f %.1f L%.0f %.0f L%.0f %.0f Z" fill="url(#river)" opacity="0.92"/>`+"\n",
		-10.0, y+10, w*0.2, y-24, w*0.38, y+30, w*0.55, y+6, w*0.8, y-14, w+10, y+14, w+10, h+4, -10.0, h+4)
	for i := 0; i < 14; i++ {
		x := g.r.Range(0, w)
		yy := y + g.r.Range(10, h*0.12)
		g.f(`<path d="M%.1f %.1f q 12 -3 24 0" stroke="#fff" stroke-width="1.2" fill="none" opacity="0.5" stroke-linecap="round"><animateTransform attributeName="transform" type="translate" values="0 0;%.0f 0;0 0" dur="%s" begin="-%.1fs" repeatCount="indefinite"/><animate attributeName="opacity" values="0.1;0.6;0.1" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></path>`+"\n",
			x, yy, g.r.Range(30, 70), g.dur(g.r.Range(5, 9)), g.r.Range(0, 6), g.dur(g.r.Range(3, 6)), g.r.Range(0, 3))
	}
}

func (g *gen) foreground() {
	w, h := g.w, g.h
	base := g.shade(g.p.leafB, 0.55)
	g.f(`<path d="M0 %.0f`, h)
	for i := 0; i <= 40; i++ {
		x := w * float64(i) / 40
		g.f(` L%.1f %.1f`, x, h-12-g.r.Range(0, 34)*(0.4+math.Abs(math.Sin(float64(i)*0.7))))
	}
	g.f(` L%.0f %.0f Z" fill="%s"/>`+"\n", w, h, base)
	n := int(70 * g.o.Density)
	for i := 0; i < n; i++ {
		x := g.r.Range(0, w)
		y := h - g.r.Range(0, 30)
		sz := g.r.Range(14, 38)
		col := g.shade(g.p.leafC, 0.6+0.5*g.r.Float())
		g.f(`<path d="M%.1f %.1f q %.1f %.1f %.1f %.1f q %.1f %.1f %.1f %.1f Z" fill="%s"><animateTransform attributeName="transform" type="rotate" values="-3 %.1f %.1f;3 %.1f %.1f;-3 %.1f %.1f" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></path>`+"\n",
			x, y, -sz*0.1, -sz*0.7, sz*0.04, -sz, sz*0.1, sz*0.5, sz*0.06, sz, col,
			x, y, x, y, x, y, g.dur(g.r.Range(2.5, 5)), g.r.Range(0, 4))
	}
	// flowers
	for i := 0; i < int(26*g.o.Density); i++ {
		x, y := g.r.Range(0, w), h-g.r.Range(2, 26)
		col := []string{"#ffd5e8", "#fff2a8", "#ffffff", "#c9b5ff"}[g.r.Intn(4)]
		if g.p.light < 0.4 {
			col = g.shade(col, 0.6)
		}
		g.f(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="%s"/>`, x, y, g.r.Range(1.4, 2.6), col)
	}
	g.f("\n")
}

func (g *gen) leaves() {
	w, h := g.w, g.h
	n := int(16 * g.o.Density)
	cols := []string{"#e6b24a", "#d9822b", "#8fd45a", "#c85a2b", "#6fbf4a"}
	if g.p.light < 0.4 {
		cols = []string{"#4a7a8a", "#3a8a6a", "#5a9a7a"}
	}
	for i := 0; i < n; i++ {
		x0, y0 := g.r.Range(0, w), g.r.Range(-20, h*0.4)
		drift := g.r.Range(-120, 160)
		path := fmt.Sprintf("M%.0f %.0f C %.0f %.0f %.0f %.0f %.0f %.0f", x0, y0, x0+drift*0.2, y0+h*0.2, x0+drift*1.2, y0+h*0.4, x0+drift, y0+h*0.75)
		d := g.dur(g.r.Range(9, 16))
		g.f(`<use href="#leaf" fill="%s" opacity="0.9" transform="scale(%.2f)"><animateMotion dur="%s" begin="-%.1fs" repeatCount="indefinite" path="%s" rotate="auto"/></use>`+"\n", cols[g.r.Intn(len(cols))], g.r.Range(0.7, 1.4), d, g.r.Range(0, 12), path)
	}
}

func (g *gen) birds() {
	w, h := g.w, g.h
	if g.p.light < 0.3 && g.o.Theme != "cycle" {
		// bats at night
		for i := 0; i < 3; i++ {
			g.bird(g.r.Range(0, h*0.4)+h*0.1, 0.8, "#101a26", g.r.Range(0, 10), g.r.Range(10, 16))
		}
		return
	}
	for f := 0; f < 2; f++ {
		y := h * (0.12 + 0.12*float64(f))
		n := 5 + f*2
		start := g.r.Range(0, 12)
		for i := 0; i < n; i++ {
			rank := float64((i + 1) / 2)
			side := 1.0
			if i%2 == 0 {
				side = -1
			}
			if i == 0 {
				rank = 0
			}
			g.birdAt(-60-rank*18, y+side*rank*9, w+120, 1.0-0.1*float64(f), "#1c2a33", start+rank*0.15, 22+float64(f)*6)
		}
	}
}

func (g *gen) bird(y, scale float64, col string, delay, dur float64) {
	g.birdAt(-40, y, g.w+80, scale, col, delay, dur)
}

func (g *gen) birdAt(x0, y float64, span, scale float64, col string, delay, dur float64) {
	d := g.dur(dur)
	g.f(`<g transform="scale(%.2f)"><path d="M-7 0 Q -3 -5 0 0 Q 3 -5 7 0" stroke="%s" stroke-width="1.6" fill="none" stroke-linecap="round"><animate attributeName="d" dur="%s" repeatCount="indefinite" values="M-7 0 Q -3 -5 0 0 Q 3 -5 7 0;M-7 3 Q -3 1 0 0 Q 3 1 7 3;M-7 0 Q -3 -5 0 0 Q 3 -5 7 0"/><animateMotion dur="%s" begin="-%.1fs" repeatCount="indefinite" path="M %.0f %.1f q %.1f %.1f %.1f 8"/></path></g>`+"\n",
		scale, col, g.dur(0.7), d, delay, x0/scale, y/scale, span/(2*scale), -14/scale, span/scale)
}

func (g *gen) fireflies() {
	if g.p.light > 0.8 {
		return
	}
	w, h := g.w, g.h
	n := int(42 * g.o.Density * (1.1 - g.p.light))
	for i := 0; i < n; i++ {
		x, y := g.r.Range(0, w), g.r.Range(h*0.45, h*0.95)
		dx, dy := g.r.Range(-40, 40), g.r.Range(-25, 25)
		g.f(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="url(#ff)" opacity="0"><animate attributeName="opacity" values="0;1;0.2;1;0" dur="%s" begin="-%.1fs" repeatCount="indefinite"/><animateTransform attributeName="transform" type="translate" values="0 0;%.0f %.0f;%.0f %.0f;0 0" dur="%s" begin="-%.1fs" repeatCount="indefinite"/></circle>`+"\n",
			x, y, g.r.Range(3, 6), g.dur(g.r.Range(3, 7)), g.r.Range(0, 7), dx, dy, -dx*0.6, dy*0.4, g.dur(g.r.Range(7, 14)), g.r.Range(0, 10))
	}
}
