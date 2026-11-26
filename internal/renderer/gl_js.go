//go:build js && wasm

package renderer

import (
	"fmt"
	"strings"
	"syscall/js"
	"unsafe"

	"evergreen/internal/mesh"
	"evergreen/shaders"
)

// glc caches the WebGL constants used by the renderer.
type glc struct {
	TRIANGLES, POINTS, LINES, ARRAY_BUFFER, ELEMENT_ARRAY_BUFFER, UNIFORM_BUFFER, STATIC_DRAW, DYNAMIC_DRAW int
	FLOAT, UNSIGNED_INT, TEXTURE_2D, TEXTURE0, FRAMEBUFFER, DEPTH_TEST, CULL_FACE, BLEND                    int
	COLOR_BUFFER_BIT, DEPTH_BUFFER_BIT, LEQUAL, SRC_ALPHA, ONE_MINUS_SRC_ALPHA, ONE, CW, CCW, BACK          int
	LINEAR, NEAREST, CLAMP_TO_EDGE, RGBA, RGBA8, DEPTH_COMPONENT24, DEPTH_COMPONENT, UNSIGNED_BYTE          int
	COMPARE_REF_TO_TEXTURE, TEXTURE_COMPARE_MODE, TEXTURE_COMPARE_FUNC, TEXTURE_MIN_FILTER                  int
	TEXTURE_MAG_FILTER, TEXTURE_WRAP_S, TEXTURE_WRAP_T, DEPTH_ATTACHMENT, COLOR_ATTACHMENT0, R16F, RED      int
	HALF_FLOAT, FRAMEBUFFER_COMPLETE, VERTEX_SHADER, FRAGMENT_SHADER, COMPILE_STATUS, LINK_STATUS           int
	UNPACK_ALIGNMENT, RENDERBUFFER, SCISSOR_TEST, POLYGON_OFFSET_FILL, LESS                                 int
}

type program struct {
	p    js.Value
	locs map[string]js.Value
}

type meshObj struct {
	vbo, ibo, vao js.Value
	n             int // index count
}

type instDraw struct {
	vao      js.Value
	indices  int
	count    int
	twoSided bool
	noShadow bool
}

type instSet struct {
	buf   js.Value
	draws []instDraw
}

type chunkGPU struct {
	ter, wat *meshObj
	terVer   uint32
	sets     [3][2]*instSet
}

type partGPU struct {
	mesh      *gpuMesh
	inst, vao js.Value
	animated  bool
	uploaded  bool
}

type lmGPU struct{ parts []partGPU }

type gpuMesh struct {
	vbo, ibo js.Value
	n        int
	refs     int
}

type animalGPU struct {
	mesh      *gpuMesh
	inst, vao js.Value
	capBytes  int
}

// uploader copies Go memory into a reusable Uint8Array.
type uploader struct {
	u8  js.Value
	cap int
}

func (u *uploader) bytes(b []byte) js.Value {
	if len(b) > u.cap {
		u.cap = max(len(b), u.cap*2, 1<<16)
		u.u8 = js.Global().Get("Uint8Array").New(u.cap)
	}
	js.CopyBytesToJS(u.u8, b)
	return u.u8
}

func f32bytes(f []float32) []byte {
	if len(f) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&f[0])), len(f)*4)
}

func u32bytes(f []uint32) []byte {
	if len(f) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&f[0])), len(f)*4)
}

// Stats are per-frame counters.
type Stats struct {
	DrawCalls, Triangles, Chunks, Uploads int
	SceneW, SceneH                        int
	GPUMeshes                             int
}

// Renderer owns the WebGL2 context and every GPU resource.
type Renderer struct {
	canvas, gl js.Value
	c          glc
	up         uploader
	Stats      Stats

	meshProg, shadowProg, skyProg, partProg, cpuProg, brightProg, blurProg, postProg *program
	ubo, identity, emptyVAO, partVAO, cpuVAO                                         js.Value
	frame                                                                            FrameData

	sceneFBO, sceneColor, sceneDepth                 js.Value
	shadowFBO, shadowTex                             js.Value
	reflFBO, reflColor, reflDepth                    js.Value
	bloomFBO                                         [2]js.Value
	bloomTex                                         [2]js.Value
	dummyShadow, dummyTex, rippleTex                 js.Value
	shadowRes, reflW, reflH, bloomW, bloomH, rippleN int
	canvasW, canvasH, sceneW, sceneH                 int
	scale                                            float64

	meshes                     map[*mesh.Mesh]*gpuMesh
	lms                        map[any]*lmGPU
	animals                    map[*mesh.Mesh]*animalGPU
	partIDs, partLines, cpuBuf js.Value
	cpuCap                     int
	maxParticles               int

	cull     bool
	lost     bool
	reinit   bool
	handlers []js.Func
	uploads  int
	// Capture requests a PNG data URL after the next frame.
	Capture   bool
	CaptureCB func(dataURL string)
}

// MaxChunkUploads bounds GPU uploads per frame to keep frames smooth.
const MaxChunkUploads = 3

// New creates a renderer for a canvas.
func New(canvas js.Value) (*Renderer, error) {
	r := &Renderer{canvas: canvas, meshes: map[*mesh.Mesh]*gpuMesh{}, lms: map[any]*lmGPU{}, animals: map[*mesh.Mesh]*animalGPU{}, scale: 1, maxParticles: 24000}
	attrs := js.Global().Get("Object").New()
	attrs.Set("antialias", false)
	attrs.Set("alpha", false)
	attrs.Set("depth", false)
	attrs.Set("stencil", false)
	attrs.Set("powerPreference", "high-performance")
	attrs.Set("preserveDrawingBuffer", false)
	gl := canvas.Call("getContext", "webgl2", attrs)
	if gl.IsNull() || gl.IsUndefined() {
		return nil, fmt.Errorf("WebGL2 is not available")
	}
	r.gl = gl
	r.loadConstants()
	canvas.Call("addEventListener", "webglcontextlost", r.fn(func(this js.Value, a []js.Value) any {
		a[0].Call("preventDefault")
		r.lost = true
		return nil
	}))
	canvas.Call("addEventListener", "webglcontextrestored", r.fn(func(this js.Value, a []js.Value) any {
		r.lost = false
		r.reinit = true
		return nil
	}))
	if err := r.initGL(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Renderer) fn(f func(js.Value, []js.Value) any) js.Func {
	h := js.FuncOf(f)
	r.handlers = append(r.handlers, h)
	return h
}

// Lost reports whether the WebGL context is currently lost.
func (r *Renderer) Lost() bool { return r.lost }

// NeedsReinit reports (once) that a restored context requires re-uploading world data.
func (r *Renderer) NeedsReinit() bool {
	if r.reinit && !r.lost {
		r.reinit = false
		_ = r.initGL()
		return true
	}
	return false
}

func (r *Renderer) loadConstants() {
	g := r.gl
	i := func(n string) int { return g.Get(n).Int() }
	r.c = glc{
		TRIANGLES: i("TRIANGLES"), POINTS: i("POINTS"), LINES: i("LINES"), ARRAY_BUFFER: i("ARRAY_BUFFER"), ELEMENT_ARRAY_BUFFER: i("ELEMENT_ARRAY_BUFFER"),
		UNIFORM_BUFFER: i("UNIFORM_BUFFER"), STATIC_DRAW: i("STATIC_DRAW"), DYNAMIC_DRAW: i("DYNAMIC_DRAW"), FLOAT: i("FLOAT"), UNSIGNED_INT: i("UNSIGNED_INT"),
		TEXTURE_2D: i("TEXTURE_2D"), TEXTURE0: i("TEXTURE0"), FRAMEBUFFER: i("FRAMEBUFFER"), DEPTH_TEST: i("DEPTH_TEST"), CULL_FACE: i("CULL_FACE"), BLEND: i("BLEND"),
		COLOR_BUFFER_BIT: i("COLOR_BUFFER_BIT"), DEPTH_BUFFER_BIT: i("DEPTH_BUFFER_BIT"), LEQUAL: i("LEQUAL"), SRC_ALPHA: i("SRC_ALPHA"),
		ONE_MINUS_SRC_ALPHA: i("ONE_MINUS_SRC_ALPHA"), ONE: i("ONE"), CW: i("CW"), CCW: i("CCW"), BACK: i("BACK"), LINEAR: i("LINEAR"), NEAREST: i("NEAREST"),
		CLAMP_TO_EDGE: i("CLAMP_TO_EDGE"), RGBA: i("RGBA"), RGBA8: i("RGBA8"), DEPTH_COMPONENT24: i("DEPTH_COMPONENT24"), DEPTH_COMPONENT: i("DEPTH_COMPONENT"),
		UNSIGNED_BYTE: i("UNSIGNED_BYTE"), COMPARE_REF_TO_TEXTURE: i("COMPARE_REF_TO_TEXTURE"), TEXTURE_COMPARE_MODE: i("TEXTURE_COMPARE_MODE"),
		TEXTURE_COMPARE_FUNC: i("TEXTURE_COMPARE_FUNC"), TEXTURE_MIN_FILTER: i("TEXTURE_MIN_FILTER"), TEXTURE_MAG_FILTER: i("TEXTURE_MAG_FILTER"),
		TEXTURE_WRAP_S: i("TEXTURE_WRAP_S"), TEXTURE_WRAP_T: i("TEXTURE_WRAP_T"), DEPTH_ATTACHMENT: i("DEPTH_ATTACHMENT"), COLOR_ATTACHMENT0: i("COLOR_ATTACHMENT0"),
		R16F: i("R16F"), RED: i("RED"), HALF_FLOAT: i("HALF_FLOAT"), FRAMEBUFFER_COMPLETE: i("FRAMEBUFFER_COMPLETE"), VERTEX_SHADER: i("VERTEX_SHADER"),
		FRAGMENT_SHADER: i("FRAGMENT_SHADER"), COMPILE_STATUS: i("COMPILE_STATUS"), LINK_STATUS: i("LINK_STATUS"), UNPACK_ALIGNMENT: i("UNPACK_ALIGNMENT"),
		RENDERBUFFER: i("RENDERBUFFER"), SCISSOR_TEST: i("SCISSOR_TEST"), POLYGON_OFFSET_FILL: i("POLYGON_OFFSET_FILL"), LESS: i("LESS"),
	}
}

func (r *Renderer) compile(kind int, src string) (js.Value, error) {
	g := r.gl
	sh := g.Call("createShader", kind)
	g.Call("shaderSource", sh, src)
	g.Call("compileShader", sh)
	if !g.Call("getShaderParameter", sh, r.c.COMPILE_STATUS).Bool() {
		log := g.Call("getShaderInfoLog", sh).String()
		lines := strings.Split(src, "\n")
		return sh, fmt.Errorf("shader compile error: %s\n(first lines: %s)", log, strings.Join(lines[:min(6, len(lines))], " | "))
	}
	return sh, nil
}

func (r *Renderer) program(vs, fs string) (*program, error) {
	g := r.gl
	v, err := r.compile(r.c.VERTEX_SHADER, vs)
	if err != nil {
		return nil, fmt.Errorf("vertex: %w", err)
	}
	f, err := r.compile(r.c.FRAGMENT_SHADER, fs)
	if err != nil {
		return nil, fmt.Errorf("fragment: %w", err)
	}
	p := g.Call("createProgram")
	g.Call("attachShader", p, v)
	g.Call("attachShader", p, f)
	g.Call("linkProgram", p)
	if !g.Call("getProgramParameter", p, r.c.LINK_STATUS).Bool() {
		return nil, fmt.Errorf("program link error: %s", g.Call("getProgramInfoLog", p).String())
	}
	g.Call("deleteShader", v)
	g.Call("deleteShader", f)
	idx := g.Call("getUniformBlockIndex", p, "Frame")
	if idx.Int() != int(^uint32(0)) && idx.Type() == js.TypeNumber && idx.Float() < 4e9 {
		g.Call("uniformBlockBinding", p, idx, 0)
	}
	return &program{p: p, locs: map[string]js.Value{}}, nil
}

func (r *Renderer) loc(p *program, name string) js.Value {
	if l, ok := p.locs[name]; ok {
		return l
	}
	l := r.gl.Call("getUniformLocation", p.p, name)
	p.locs[name] = l
	return l
}

func (r *Renderer) use(p *program) { r.gl.Call("useProgram", p.p) }

func (r *Renderer) initGL() error {
	g := r.gl
	var err error
	if r.meshProg, err = r.program(shaders.Source(shaders.MeshVert, true), shaders.Source(shaders.MeshFrag, true)); err != nil {
		return fmt.Errorf("mesh program: %w", err)
	}
	if r.shadowProg, err = r.program(shaders.Source(shaders.MeshVert, true, "SHADOW"), shaders.Source(shaders.MeshFrag, true, "SHADOW")); err != nil {
		return fmt.Errorf("shadow program: %w", err)
	}
	if r.skyProg, err = r.program(shaders.Source(shaders.FullscreenVert, true), shaders.Source(shaders.SkyFrag, true)); err != nil {
		return fmt.Errorf("sky program: %w", err)
	}
	if r.partProg, err = r.program(shaders.Source(shaders.ParticlesVert, true), shaders.Source(shaders.ParticlesFrag, true)); err != nil {
		return fmt.Errorf("particle program: %w", err)
	}
	if r.cpuProg, err = r.program(shaders.Source(shaders.CPUParticlesVert, true), shaders.Source(shaders.CPUParticlesFrag, true)); err != nil {
		return fmt.Errorf("cpu particle program: %w", err)
	}
	if r.brightProg, err = r.program(shaders.Source(shaders.FullscreenVert, true), shaders.Source(shaders.BrightFrag, true)); err != nil {
		return fmt.Errorf("bright program: %w", err)
	}
	if r.blurProg, err = r.program(shaders.Source(shaders.FullscreenVert, true), shaders.Source(shaders.BlurFrag, true)); err != nil {
		return fmt.Errorf("blur program: %w", err)
	}
	if r.postProg, err = r.program(shaders.Source(shaders.FullscreenVert, true), shaders.Source(shaders.PostFrag, true)); err != nil {
		return fmt.Errorf("post program: %w", err)
	}
	// sampler bindings never change
	r.use(r.meshProg)
	g.Call("uniform1i", r.loc(r.meshProg, "uShadow"), 0)
	g.Call("uniform1i", r.loc(r.meshProg, "uRipple"), 1)
	g.Call("uniform1i", r.loc(r.meshProg, "uReflect"), 2)
	r.use(r.brightProg)
	g.Call("uniform1i", r.loc(r.brightProg, "uTex"), 0)
	r.use(r.blurProg)
	g.Call("uniform1i", r.loc(r.blurProg, "uTex"), 0)
	r.use(r.postProg)
	g.Call("uniform1i", r.loc(r.postProg, "uScene"), 0)
	g.Call("uniform1i", r.loc(r.postProg, "uBloom"), 1)
	g.Call("uniform1i", r.loc(r.postProg, "uDepth"), 2)

	r.ubo = g.Call("createBuffer")
	g.Call("bindBuffer", r.c.UNIFORM_BUFFER, r.ubo)
	g.Call("bufferData", r.c.UNIFORM_BUFFER, FrameFloats*4, r.c.DYNAMIC_DRAW)
	g.Call("bindBufferBase", r.c.UNIFORM_BUFFER, 0, r.ubo)
	r.emptyVAO = g.Call("createVertexArray")
	r.partVAO = g.Call("createVertexArray")
	r.cpuVAO = g.Call("createVertexArray")

	// identity instance used by world-space meshes (terrain, water)
	r.identity = g.Call("createBuffer")
	g.Call("bindBuffer", r.c.ARRAY_BUFFER, r.identity)
	r.upload(r.c.ARRAY_BUFFER, f32bytes([]float32{0, 0, 0, 1, 0, 0, -1000, 0}), r.c.STATIC_DRAW)

	// particle id buffers
	ids := make([]float32, r.maxParticles*2)
	lines := make([]float32, r.maxParticles*4)
	for i := 0; i < r.maxParticles; i++ {
		ids[i*2] = float32(i)
		lines[i*4], lines[i*4+2], lines[i*4+3] = float32(i), float32(i), 1
	}
	r.partIDs, r.partLines = g.Call("createBuffer"), g.Call("createBuffer")
	g.Call("bindBuffer", r.c.ARRAY_BUFFER, r.partIDs)
	r.upload(r.c.ARRAY_BUFFER, f32bytes(ids), r.c.STATIC_DRAW)
	g.Call("bindBuffer", r.c.ARRAY_BUFFER, r.partLines)
	r.upload(r.c.ARRAY_BUFFER, f32bytes(lines), r.c.STATIC_DRAW)
	r.cpuBuf = g.Call("createBuffer")
	r.cpuCap = 0

	// dummy textures keep every sampler valid
	r.dummyShadow = r.newDepthTexture(2, 2, true)
	g.Call("bindFramebuffer", r.c.FRAMEBUFFER, js.Null())
	r.dummyTex = r.newColorTexture(2, 2, r.c.LINEAR)
	r.rippleTex = js.Null()
	r.shadowFBO, r.shadowTex, r.shadowRes = js.Null(), js.Null(), 0
	r.sceneFBO, r.reflFBO = js.Null(), js.Null()
	r.sceneW, r.sceneH, r.reflW, r.bloomW = 0, 0, 0, 0
	// everything cached against the old context is invalid
	r.meshes = map[*mesh.Mesh]*gpuMesh{}
	r.lms = map[any]*lmGPU{}
	r.animals = map[*mesh.Mesh]*animalGPU{}
	return nil
}

func (r *Renderer) upload(target int, b []byte, usage int) {
	if len(b) == 0 {
		return
	}
	r.gl.Call("bufferData", target, r.up.bytes(b), usage, 0, len(b))
}

func (r *Renderer) newDepthTexture(w, h int, compare bool) js.Value {
	g := r.gl
	t := g.Call("createTexture")
	g.Call("bindTexture", r.c.TEXTURE_2D, t)
	g.Call("texStorage2D", r.c.TEXTURE_2D, 1, r.c.DEPTH_COMPONENT24, w, h)
	f := r.c.NEAREST
	if compare {
		f = r.c.LINEAR
		g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_COMPARE_MODE, r.c.COMPARE_REF_TO_TEXTURE)
		g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_COMPARE_FUNC, r.c.LEQUAL)
	}
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_MIN_FILTER, f)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_MAG_FILTER, f)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_WRAP_S, r.c.CLAMP_TO_EDGE)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_WRAP_T, r.c.CLAMP_TO_EDGE)
	return t
}

func (r *Renderer) newColorTexture(w, h, filter int) js.Value {
	g := r.gl
	t := g.Call("createTexture")
	g.Call("bindTexture", r.c.TEXTURE_2D, t)
	g.Call("texStorage2D", r.c.TEXTURE_2D, 1, r.c.RGBA8, w, h)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_MIN_FILTER, filter)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_MAG_FILTER, filter)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_WRAP_S, r.c.CLAMP_TO_EDGE)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_WRAP_T, r.c.CLAMP_TO_EDGE)
	return t
}

func (r *Renderer) deleteFBO(fbo, tex, depth js.Value) {
	for _, v := range []js.Value{tex, depth} {
		if !v.IsNull() && !v.IsUndefined() {
			r.gl.Call("deleteTexture", v)
		}
	}
	if !fbo.IsNull() && !fbo.IsUndefined() {
		r.gl.Call("deleteFramebuffer", fbo)
	}
}

func (r *Renderer) fbo(color, depth js.Value) (js.Value, error) {
	g := r.gl
	f := g.Call("createFramebuffer")
	g.Call("bindFramebuffer", r.c.FRAMEBUFFER, f)
	if !color.IsNull() {
		g.Call("framebufferTexture2D", r.c.FRAMEBUFFER, r.c.COLOR_ATTACHMENT0, r.c.TEXTURE_2D, color, 0)
	} else {
		g.Call("drawBuffers", js.ValueOf([]any{g.Get("NONE")}))
		g.Call("readBuffer", g.Get("NONE"))
	}
	if !depth.IsNull() {
		g.Call("framebufferTexture2D", r.c.FRAMEBUFFER, r.c.DEPTH_ATTACHMENT, r.c.TEXTURE_2D, depth, 0)
	}
	if st := g.Call("checkFramebufferStatus", r.c.FRAMEBUFFER).Int(); st != r.c.FRAMEBUFFER_COMPLETE {
		return f, fmt.Errorf("framebuffer incomplete: 0x%x", st)
	}
	return f, nil
}

// Resize adapts the drawing surface. cssW/H are CSS pixels; dpr the device ratio.
func (r *Renderer) Resize(cssW, cssH int, dpr float64, scale float64) error {
	w, h := int(float64(cssW)*dpr), int(float64(cssH)*dpr)
	w, h = max(w, 64), max(h, 64)
	if w > 3840 {
		h, w = h*3840/w, 3840
	}
	if r.canvasW != w || r.canvasH != h {
		r.canvas.Set("width", w)
		r.canvas.Set("height", h)
		r.canvasW, r.canvasH = w, h
	}
	sw, sh := max(int(float64(w)*scale), 64), max(int(float64(h)*scale), 64)
	if sw == r.sceneW && sh == r.sceneH && r.sceneFBO.Truthy() {
		return nil
	}
	r.deleteFBO(r.sceneFBO, r.sceneColor, r.sceneDepth)
	r.sceneW, r.sceneH = sw, sh
	r.sceneColor = r.newColorTexture(sw, sh, r.c.LINEAR)
	r.sceneDepth = r.newDepthTexture(sw, sh, false)
	var err error
	r.sceneFBO, err = r.fbo(r.sceneColor, r.sceneDepth)
	// bloom chain at quarter resolution
	for i := 0; i < 2; i++ {
		r.deleteFBO(r.bloomFBO[i], r.bloomTex[i], js.Null())
	}
	r.bloomW, r.bloomH = max(sw/4, 16), max(sh/4, 16)
	for i := 0; i < 2; i++ {
		r.bloomTex[i] = r.newColorTexture(r.bloomW, r.bloomH, r.c.LINEAR)
		if r.bloomFBO[i], err = r.fbo(r.bloomTex[i], js.Null()); err != nil {
			return err
		}
	}
	r.gl.Call("bindFramebuffer", r.c.FRAMEBUFFER, js.Null())
	return err
}

func (r *Renderer) ensureShadow(res int) {
	if res == r.shadowRes {
		return
	}
	r.deleteFBO(r.shadowFBO, js.Null(), r.shadowTex)
	r.shadowFBO, r.shadowTex, r.shadowRes = js.Null(), js.Null(), res
	if res == 0 {
		return
	}
	r.shadowTex = r.newDepthTexture(res, res, true)
	r.shadowFBO, _ = r.fbo(js.Null(), r.shadowTex)
}

func (r *Renderer) ensureReflection(on bool, scale float64) {
	w, h := 0, 0
	if on {
		w, h = max(int(float64(r.sceneW)*scale), 64), max(int(float64(r.sceneH)*scale), 64)
	}
	if w == r.reflW && h == r.reflH && (w == 0 || r.reflFBO.Truthy()) {
		return
	}
	r.deleteFBO(r.reflFBO, r.reflColor, r.reflDepth)
	r.reflFBO, r.reflColor, r.reflDepth = js.Null(), js.Null(), js.Null()
	r.reflW, r.reflH = w, h
	if w == 0 {
		return
	}
	r.reflColor = r.newColorTexture(w, h, r.c.LINEAR)
	r.reflDepth = r.newDepthTexture(w, h, false)
	r.reflFBO, _ = r.fbo(r.reflColor, r.reflDepth)
}

func (r *Renderer) bindTex(unit int, t js.Value) {
	r.gl.Call("activeTexture", r.c.TEXTURE0+unit)
	r.gl.Call("bindTexture", r.c.TEXTURE_2D, t)
}

func (r *Renderer) setCull(on bool) {
	if on == r.cull {
		return
	}
	r.cull = on
	if on {
		r.gl.Call("enable", r.c.CULL_FACE)
	} else {
		r.gl.Call("disable", r.c.CULL_FACE)
	}
}

// ---- mesh upload ----------------------------------------------------------------------

func (r *Renderer) setupVertexAttribs() {
	g := r.gl
	const stride = mesh.Stride * 4
	type a struct{ loc, size, off int }
	for _, at := range []a{{0, 3, 0}, {1, 3, 12}, {2, 3, 24}, {3, 1, 36}, {4, 2, 40}, {5, 1, 48}} {
		g.Call("enableVertexAttribArray", at.loc)
		g.Call("vertexAttribPointer", at.loc, at.size, r.c.FLOAT, false, stride, at.off)
	}
}

func (r *Renderer) setupInstanceAttribs(buf js.Value, offset int) {
	g := r.gl
	g.Call("bindBuffer", r.c.ARRAY_BUFFER, buf)
	for i := 0; i < 2; i++ {
		g.Call("enableVertexAttribArray", 6+i)
		g.Call("vertexAttribPointer", 6+i, 4, r.c.FLOAT, false, 32, offset+i*16)
		g.Call("vertexAttribDivisor", 6+i, 1)
	}
}

func (r *Renderer) newVAO(m *gpuMesh, inst js.Value, offset int) js.Value {
	g := r.gl
	vao := g.Call("createVertexArray")
	g.Call("bindVertexArray", vao)
	g.Call("bindBuffer", r.c.ARRAY_BUFFER, m.vbo)
	r.setupVertexAttribs()
	g.Call("bindBuffer", r.c.ELEMENT_ARRAY_BUFFER, m.ibo)
	r.setupInstanceAttribs(inst, offset)
	g.Call("bindVertexArray", js.Null())
	return vao
}

func (r *Renderer) uploadMesh(m *mesh.Mesh) *gpuMesh {
	g := r.gl
	g.Call("bindVertexArray", js.Null()) // element buffer bindings are VAO state; never touch another VAO's
	gm := &gpuMesh{vbo: g.Call("createBuffer"), ibo: g.Call("createBuffer"), n: len(m.I)}
	g.Call("bindBuffer", r.c.ARRAY_BUFFER, gm.vbo)
	r.upload(r.c.ARRAY_BUFFER, f32bytes(m.V), r.c.STATIC_DRAW)
	g.Call("bindBuffer", r.c.ELEMENT_ARRAY_BUFFER, gm.ibo)
	r.upload(r.c.ELEMENT_ARRAY_BUFFER, u32bytes(m.I), r.c.STATIC_DRAW)
	return gm
}

// sharedMesh returns the cached GPU mesh for a prototype.
func (r *Renderer) sharedMesh(m *mesh.Mesh) *gpuMesh {
	if gm, ok := r.meshes[m]; ok {
		gm.refs++
		return gm
	}
	gm := r.uploadMesh(m)
	gm.refs = 1
	r.meshes[m] = gm
	return gm
}

func (r *Renderer) releaseShared(m *mesh.Mesh) {
	gm, ok := r.meshes[m]
	if !ok {
		return
	}
	gm.refs--
	if gm.refs <= 0 && !isProtoLifetime(m) {
		r.gl.Call("deleteBuffer", gm.vbo)
		r.gl.Call("deleteBuffer", gm.ibo)
		delete(r.meshes, m)
	}
}

// isProtoLifetime marks small shared prototypes (vegetation, animals) that live for the session.
func isProtoLifetime(m *mesh.Mesh) bool {
	return !strings.HasPrefix(m.Name, "terrain") && m.VertexCount() < 20000
}

func (r *Renderer) newWorldMesh(m *mesh.Mesh) *meshObj {
	gm := r.uploadMesh(m)
	vao := r.newVAO(gm, r.identity, 0)
	return &meshObj{vbo: gm.vbo, ibo: gm.ibo, vao: vao, n: gm.n}
}

func (r *Renderer) freeMeshObj(o *meshObj) {
	if o == nil {
		return
	}
	g := r.gl
	g.Call("deleteVertexArray", o.vao)
	g.Call("deleteBuffer", o.vbo)
	g.Call("deleteBuffer", o.ibo)
}

// Close releases every GPU resource and JS callback.
func (r *Renderer) Close() {
	for _, h := range r.handlers {
		h.Release()
	}
	r.handlers = nil
}

// GPUMeshCount reports shared meshes alive (leak diagnostics).
func (r *Renderer) GPUMeshCount() int { return len(r.meshes) }
