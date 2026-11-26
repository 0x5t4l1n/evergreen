package svg

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

func mustGen(t *testing.T, o Options) string {
	t.Helper()
	s, err := Generate(o)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestWellFormedXMLForAllThemes(t *testing.T) {
	for _, theme := range Themes {
		o := DefaultOptions()
		o.Theme = theme
		s := mustGen(t, o)
		dec := xml.NewDecoder(strings.NewReader(s))
		elems := 0
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("%s: invalid XML: %v", theme, err)
			}
			if _, ok := tok.(xml.StartElement); ok {
				elems++
			}
		}
		if elems < 200 {
			t.Fatalf("%s: suspiciously few elements: %d", theme, elems)
		}
	}
}

func TestSafeForImageContext(t *testing.T) {
	for _, theme := range Themes {
		o := DefaultOptions()
		o.Theme = theme
		s := mustGen(t, o)
		low := strings.ToLower(s)
		for _, bad := range []string{"<script", "onload=", "onclick=", "javascript:", "http://", "https://", "<foreignobject", "<image", "xlink:href"} {
			if bad == "http://" {
				// the XML namespace is the only permitted URL
				if strings.Count(low, "http://") != 1 {
					t.Fatalf("%s: unexpected external URL", theme)
				}
				continue
			}
			if strings.Contains(low, bad) {
				t.Fatalf("%s: contains forbidden %q", theme, bad)
			}
		}
	}
}

func TestContainsRequestedElements(t *testing.T) {
	s := mustGen(t, DefaultOptions())
	for _, want := range []string{"animateMotion", "animateTransform", "<animate ", "id=\"sky\"", "id=\"river\"", "id=\"fall\"", "World Tree"[:0] + "glow", "stroke-dashoffset", "viewBox=\"0 0 1200 520\"", "role=\"img\""} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q", want)
		}
	}
	night := DefaultOptions()
	night.Theme = "night"
	n := mustGen(t, night)
	if !strings.Contains(n, "#e8efff") || !strings.Contains(n, "url(#ff)") {
		t.Fatal("night should show a moon and fireflies")
	}
	if strings.Contains(s, "url(#ff)") {
		t.Fatal("daytime has no fireflies")
	}
	cycle := DefaultOptions()
	cycle.Theme = "cycle"
	if !strings.Contains(mustGen(t, cycle), `attributeName="stop-color"`) {
		t.Fatal("cycle theme animates the sky colours")
	}
}

func TestDeterministicAndSeedSensitive(t *testing.T) {
	o := DefaultOptions()
	if mustGen(t, o) != mustGen(t, o) {
		t.Fatal("same options must give identical output")
	}
	o2 := o
	o2.Seed = 43
	if mustGen(t, o) == mustGen(t, o2) {
		t.Fatal("seed must change the picture")
	}
}

func TestThemesDiffer(t *testing.T) {
	seen := map[string]bool{}
	for _, th := range Themes {
		o := DefaultOptions()
		o.Theme = th
		s := mustGen(t, o)
		if seen[s] {
			t.Fatalf("theme %s duplicates another", th)
		}
		seen[s] = true
	}
}

func TestDensityAndIntensity(t *testing.T) {
	lo, hi := DefaultOptions(), DefaultOptions()
	lo.Density, hi.Density = 0.3, 1.8
	if len(mustGen(t, hi)) <= len(mustGen(t, lo)) {
		t.Fatal("higher density should draw more")
	}
	still := DefaultOptions()
	still.Intensity = 0
	if !strings.Contains(mustGen(t, still), "1000000.00s") {
		t.Fatal("intensity 0 should effectively stop animation")
	}
	fast, slow := DefaultOptions(), DefaultOptions()
	fast.Intensity, slow.Intensity = 2, 0.5
	if mustGen(t, fast) == mustGen(t, slow) {
		t.Fatal("intensity should change durations")
	}
}

func TestSizeIsReasonable(t *testing.T) {
	s := mustGen(t, DefaultOptions())
	if len(s) > 400_000 {
		t.Fatalf("README SVG too large for GitHub: %d bytes", len(s))
	}
	big := DefaultOptions()
	big.Density = 2
	if n := len(mustGen(t, big)); n > 700_000 {
		t.Fatalf("dense SVG too large: %d bytes", n)
	}
}

func TestValidation(t *testing.T) {
	bad := []Options{
		{Width: 10, Height: 500, Theme: "day", Density: 1, Intensity: 1},
		{Width: 1200, Height: 5000, Theme: "day", Density: 1, Intensity: 1},
		{Width: 1200, Height: 500, Theme: "neon", Density: 1, Intensity: 1},
		{Width: 1200, Height: 500, Theme: "day", Density: 9, Intensity: 1},
		{Width: 1200, Height: 500, Theme: "day", Density: 1, Intensity: -1},
	}
	for i, o := range bad {
		if _, err := Generate(o); err == nil {
			t.Errorf("case %d should be rejected", i)
		}
	}
}

func TestCustomSizeApplied(t *testing.T) {
	o := DefaultOptions()
	o.Width, o.Height = 800, 400
	s := mustGen(t, o)
	if !bytes.Contains([]byte(s), []byte(`width="800" height="400"`)) {
		t.Fatal("size not applied")
	}
}
