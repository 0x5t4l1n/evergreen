// Package shaders embeds Evergreen's GLSL ES 3.00 sources.
package shaders

import (
	_ "embed"
	"strings"
)

//go:embed common.glsl
var Common string

//go:embed mesh.vert
var MeshVert string

//go:embed mesh.frag
var MeshFrag string

//go:embed fullscreen.vert
var FullscreenVert string

//go:embed sky.frag
var SkyFrag string

//go:embed particles.vert
var ParticlesVert string

//go:embed particles.frag
var ParticlesFrag string

//go:embed cpuparticles.vert
var CPUParticlesVert string

//go:embed cpuparticles.frag
var CPUParticlesFrag string

//go:embed bright.frag
var BrightFrag string

//go:embed blur.frag
var BlurFrag string

//go:embed post.frag
var PostFrag string

// Source assembles a complete shader: version line, default precision,
// optional #defines, the common block (when withCommon is set) and the body.
func Source(body string, withCommon bool, defines ...string) string {
	var sb strings.Builder
	sb.WriteString("#version 300 es\nprecision highp float;\nprecision highp int;\nprecision highp sampler2D;\nprecision highp sampler2DShadow;\n")
	for _, d := range defines {
		sb.WriteString("#define " + d + "\n")
	}
	if withCommon {
		sb.WriteString(Common)
		sb.WriteString("\n")
	}
	sb.WriteString(body)
	return sb.String()
}
