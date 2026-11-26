package shaders

import (
	"strings"
	"testing"
)

func TestSourcesAreEmbeddedAndAssembled(t *testing.T) {
	for name, src := range map[string]string{
		"common": Common, "mesh.vert": MeshVert, "mesh.frag": MeshFrag, "fullscreen.vert": FullscreenVert,
		"sky.frag": SkyFrag, "particles.vert": ParticlesVert, "particles.frag": ParticlesFrag,
		"cpuparticles.vert": CPUParticlesVert, "cpuparticles.frag": CPUParticlesFrag,
		"bright.frag": BrightFrag, "blur.frag": BlurFrag, "post.frag": PostFrag,
	} {
		if len(strings.TrimSpace(src)) < 50 {
			t.Errorf("%s looks empty", name)
		}
		if strings.Contains(src, "#version") {
			t.Errorf("%s must not declare #version; Source() does that", name)
		}
	}
	s := Source(MeshVert, true, "SHADOW")
	if !strings.HasPrefix(s, "#version 300 es\n") || !strings.Contains(s, "#define SHADOW") || !strings.Contains(s, "uniform Frame") {
		t.Fatal("assembled source is missing required parts")
	}
	if strings.Contains(Source(BlurFrag, false), "uniform Frame") {
		t.Fatal("common block must be optional")
	}
}

// Braces must balance so a stray edit is caught without a GPU.
func TestShaderBracesBalance(t *testing.T) {
	for name, src := range map[string]string{"mesh.frag": MeshFrag, "mesh.vert": MeshVert, "sky.frag": SkyFrag, "post.frag": PostFrag, "particles.vert": ParticlesVert} {
		if strings.Count(src, "{") != strings.Count(src, "}") || strings.Count(src, "(") != strings.Count(src, ")") {
			t.Errorf("%s: unbalanced braces/parentheses", name)
		}
	}
}
