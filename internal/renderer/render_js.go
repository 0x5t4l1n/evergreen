//go:build js && wasm

package renderer

import (
	"math"
	"sort"
	"syscall/js"

	"evergreen/internal/engine"
	"evergreen/internal/mesh"
	"evergreen/internal/terrain"
	"evergreen/internal/world"
	"evergreen/internal/zones"
)

type chunkView struct {
	c *world.Chunk
	d float64 // distance from the camera in chunk units
}

type transItem struct {
	vao   js.Value
	count int
	dist  float64
}

func (r *Renderer) uploadFrame() {
	g := r.gl
	g.Call("bindBuffer", r.c.UNIFORM_BUFFER, r.ubo)
	b := f32bytes(r.frame[:])
	g.Call("bufferSubData", r.c.UNIFORM_BUFFER, 0, r.up.bytes(b), 0, len(b))
}

func (r *Renderer) draw(vao js.Value, indices, instances int) {
	r.gl.Call("bindVertexArray", vao)
	r.gl.Call("drawElementsInstanced", r.c.TRIANGLES, indices, r.c.UNSIGNED_INT, 0, instances)
	r.Stats.DrawCalls++
	r.Stats.Triangles += indices / 3 * instances
}

// ReleaseChunk frees the GPU data attached to a chunk.
func (r *Renderer) ReleaseChunk(c *world.Chunk) {
	cg, ok := c.GPU.(*chunkGPU)
	if !ok || cg == nil {
		c.GPU = nil
		return
	}
	r.freeChunkGPU(cg)
	c.GPU = nil
}

func (r *Renderer) freeChunkGPU(cg *chunkGPU) {
	r.freeMeshObj(cg.ter)
	r.freeMeshObj(cg.wat)
	for l := range cg.sets {
		for lod := range cg.sets[l] {
			r.freeSet(cg.sets[l][lod])
		}
	}
}

func (r *Renderer) freeSet(s *instSet) {
	if s == nil {
		return
	}
	for _, d := range s.draws {
		r.gl.Call("deleteVertexArray", d.vao)
	}
	r.gl.Call("deleteBuffer", s.buf)
}

// ReleaseLandmark frees a landmark's GPU data.
func (r *Renderer) ReleaseLandmark(lm *zones.Landmark) {
	g, ok := r.lms[lm]
	if !ok {
		return
	}
	for _, p := range g.parts {
		r.gl.Call("deleteVertexArray", p.vao)
		r.gl.Call("deleteBuffer", p.inst)
		r.gl.Call("deleteBuffer", p.mesh.vbo)
		r.gl.Call("deleteBuffer", p.mesh.ibo)
	}
	delete(r.lms, lm)
}

// DropAllChunks clears GPU handles after a context restore.
func (r *Renderer) DropAllChunks(chunks map[world.Key]*world.Chunk) {
	for _, c := range chunks {
		c.GPU = nil
	}
}

func (r *Renderer) chunkGPUFor(c *world.Chunk) *chunkGPU {
	cg, _ := c.GPU.(*chunkGPU)
	if cg == nil {
		cg = &chunkGPU{}
		c.GPU = cg
	}
	if cg.terVer != c.Version {
		if r.uploads >= MaxChunkUploads {
			return cg
		}
		r.uploads++
		r.freeMeshObj(cg.ter)
		r.freeMeshObj(cg.wat)
		cg.ter, cg.wat = nil, nil
		if c.Terrain != nil {
			cg.ter = r.newWorldMesh(c.Terrain)
		}
		if c.Water != nil {
			cg.wat = r.newWorldMesh(c.Water)
		}
		cg.terVer = c.Version
	}
	return cg
}

func (r *Renderer) instSetFor(c *world.Chunk, cg *chunkGPU, layer world.Layer, lod int) *instSet {
	if s := cg.sets[layer][lod]; s != nil {
		return s
	}
	if r.uploads >= MaxChunkUploads+2 {
		return nil
	}
	r.uploads++
	batches := c.Batches(layer, lod)
	var data []float32
	for _, b := range batches {
		data = append(data, b.Data...)
	}
	set := &instSet{}
	if len(data) > 0 {
		set.buf = r.gl.Call("createBuffer")
		r.gl.Call("bindBuffer", r.c.ARRAY_BUFFER, set.buf)
		r.upload(r.c.ARRAY_BUFFER, f32bytes(data), r.c.STATIC_DRAW)
		off := 0
		for _, b := range batches {
			gm := r.sharedMesh(b.Mesh)
			vao := r.newVAO(gm, set.buf, off)
			set.draws = append(set.draws, instDraw{vao: vao, indices: gm.n, count: b.Count, twoSided: b.Mesh.TwoSided, noShadow: b.Mesh.NoShadow})
			off += len(b.Data) * 4
		}
	} else {
		set.buf = js.Null()
	}
	c.DropBatches()
	cg.sets[layer][lod] = set
	return set
}

func (r *Renderer) lmFor(lm *zones.Landmark) *lmGPU {
	if g, ok := r.lms[lm]; ok {
		return g
	}
	g := &lmGPU{}
	for _, p := range lm.Parts {
		gm := r.uploadMesh(p.Mesh)
		inst := r.gl.Call("createBuffer")
		r.gl.Call("bindBuffer", r.c.ARRAY_BUFFER, inst)
		r.gl.Call("bufferData", r.c.ARRAY_BUFFER, 32, r.c.DYNAMIC_DRAW)
		g.parts = append(g.parts, partGPU{mesh: gm, inst: inst, vao: r.newVAO(gm, inst, 0), animated: p.Anim != zones.AnimNone})
	}
	r.lms[lm] = g
	return g
}

func (r *Renderer) writePose(lm *zones.Landmark, i int, pg *partGPU, t float64) (x, y, z, scale float64) {
	x, y, z, yaw, scale := lm.PartPose(i, t)
	if pg.animated || !pg.uploaded {
		rec := [8]float32{float32(x), float32(y), float32(z), float32(scale), float32(yaw), 0, -1000, 0}
		r.gl.Call("bindBuffer", r.c.ARRAY_BUFFER, pg.inst)
		b := f32bytes(rec[:])
		r.gl.Call("bufferSubData", r.c.ARRAY_BUFFER, 0, r.up.bytes(b), 0, len(b))
		pg.uploaded = true
	}
	return
}

// ---- main entry ---------------------------------------------------------------------------

func (r *Renderer) ensureRipple(n int) {
	if r.rippleN == n && r.rippleTex.Truthy() {
		return
	}
	g := r.gl
	if r.rippleTex.Truthy() {
		g.Call("deleteTexture", r.rippleTex)
	}
	r.rippleN = n
	r.rippleTex = g.Call("createTexture")
	g.Call("bindTexture", r.c.TEXTURE_2D, r.rippleTex)
	g.Call("texStorage2D", r.c.TEXTURE_2D, 1, r.c.R16F, n, n)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_MIN_FILTER, r.c.LINEAR)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_MAG_FILTER, r.c.LINEAR)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_WRAP_S, r.c.CLAMP_TO_EDGE)
	g.Call("texParameteri", r.c.TEXTURE_2D, r.c.TEXTURE_WRAP_T, r.c.CLAMP_TO_EDGE)
}

func (r *Renderer) uploadRipples(s *Scene) {
	if s.RippleN == 0 || len(s.Ripples) != s.RippleN*s.RippleN {
		return
	}
	r.ensureRipple(s.RippleN)
	g := r.gl
	g.Call("bindTexture", r.c.TEXTURE_2D, r.rippleTex)
	b := make([]byte, len(s.Ripples)*2)
	for i, h := range s.Ripples {
		b[i*2], b[i*2+1] = byte(h), byte(h>>8)
	}
	u8 := r.up.bytes(b)
	view := js.Global().Get("Uint16Array").New(u8.Get("buffer"), 0, len(s.Ripples))
	g.Call("pixelStorei", r.c.UNPACK_ALIGNMENT, 2)
	g.Call("texSubImage2D", r.c.TEXTURE_2D, 0, 0, 0, s.RippleN, s.RippleN, r.c.RED, r.c.HALF_FLOAT, view)
}

// Render draws one frame.
func (r *Renderer) Render(s *Scene) error {
	if r.lost || !r.sceneFBO.Truthy() {
		return nil
	}
	g := r.gl
	r.Stats = Stats{SceneW: r.sceneW, SceneH: r.sceneH, GPUMeshes: len(r.meshes)}
	r.uploads = 0
	q := s.Quality
	r.ensureShadow(q.ShadowRes)
	wantRefl := q.Reflection > 0 && !math.IsNaN(s.WaterPlane) && !s.Underwater
	reflScale := 0.5
	if q.Reflection == 1 {
		reflScale = 0.33
	}
	r.ensureReflection(wantRefl, reflScale)
	r.uploadRipples(s)

	cc := s.Cam
	camChunkX, camChunkZ := cc.Pos.X/terrain.ChunkSize-0.5, cc.Pos.Z/terrain.ChunkSize-0.5
	views := make([]chunkView, 0, len(s.Chunks))
	for _, c := range s.Chunks {
		if !c.Ready {
			continue
		}
		d := math.Hypot(float64(c.CX)-camChunkX, float64(c.CZ)-camChunkZ)
		views = append(views, chunkView{c, d})
	}
	r.Stats.Chunks = len(views)

	g.Call("enable", r.c.DEPTH_TEST)
	g.Call("depthFunc", r.c.LEQUAL)
	g.Call("cullFace", r.c.BACK)
	r.cull = false
	r.setCull(true)

	// 1. shadow map
	if q.ShadowRes > 0 && r.shadowFBO.Truthy() {
		r.frame = s.Build(PassShadow, r.sceneW, r.sceneH)
		r.uploadFrame()
		g.Call("bindFramebuffer", r.c.FRAMEBUFFER, r.shadowFBO)
		g.Call("viewport", 0, 0, q.ShadowRes, q.ShadowRes)
		g.Call("depthMask", true)
		g.Call("clear", r.c.DEPTH_BUFFER_BIT)
		g.Call("enable", r.c.POLYGON_OFFSET_FILL)
		g.Call("polygonOffset", 1.5, 3)
		r.use(r.shadowProg)
		r.drawWorld(s, views, PassShadow, engine.Frustum{}, q.ShadowDist+70)
		g.Call("disable", r.c.POLYGON_OFFSET_FILL)
	}
	// 2. planar reflection
	reflOn := wantRefl && r.reflFBO.Truthy()
	if reflOn {
		r.frame = s.Build(PassReflect, r.reflW, r.reflH)
		r.uploadFrame()
		g.Call("bindFramebuffer", r.c.FRAMEBUFFER, r.reflFBO)
		g.Call("viewport", 0, 0, r.reflW, r.reflH)
		g.Call("depthMask", true)
		fog := s.Atmos.Fog
		g.Call("clearColor", fog[0], fog[1], fog[2], 1)
		g.Call("clear", r.c.COLOR_BUFFER_BIT|r.c.DEPTH_BUFFER_BIT)
		r.bindTex(0, r.dummyShadow)
		r.bindTex(1, r.dummyTex)
		r.bindTex(2, r.dummyTex)
		r.drawSky()
		g.Call("frontFace", r.c.CW)
		r.use(r.meshProg)
		r.drawWorld(s, views, PassReflect, engine.Frustum{}, math.Min(float64(q.ViewChunks)*terrain.ChunkSize, 360))
		g.Call("frontFace", r.c.CCW)
	}
	// 3. main pass
	r.frame = s.Build(PassMain, r.sceneW, r.sceneH)
	r.uploadFrame()
	g.Call("bindFramebuffer", r.c.FRAMEBUFFER, r.sceneFBO)
	g.Call("viewport", 0, 0, r.sceneW, r.sceneH)
	g.Call("depthMask", true)
	fog := s.Atmos.Fog
	g.Call("clearColor", fog[0], fog[1], fog[2], 1)
	g.Call("clear", r.c.COLOR_BUFFER_BIT|r.c.DEPTH_BUFFER_BIT)
	if q.ShadowRes > 0 && r.shadowTex.Truthy() {
		r.bindTex(0, r.shadowTex)
	} else {
		r.bindTex(0, r.dummyShadow)
	}
	if r.rippleTex.Truthy() {
		r.bindTex(1, r.rippleTex)
	} else {
		r.bindTex(1, r.dummyTex)
	}
	if reflOn {
		r.bindTex(2, r.reflColor)
	} else {
		r.bindTex(2, r.dummyTex)
	}
	r.drawSky()
	r.use(r.meshProg)
	frustum := engine.NewFrustum(cc.Proj.Mul(cc.View))
	trans := r.drawWorld(s, views, PassMain, frustum, float64(q.ViewChunks+1)*terrain.ChunkSize)
	r.drawTransparent(trans)
	r.drawParticles(s)

	// 4. post
	r.post(s)
	if r.Capture {
		r.Capture = false
		if r.CaptureCB != nil {
			r.CaptureCB(r.canvas.Call("toDataURL", "image/png").String())
		}
	}
	return nil
}

func (r *Renderer) drawSky() {
	g := r.gl
	r.use(r.skyProg)
	g.Call("disable", r.c.DEPTH_TEST)
	g.Call("depthMask", false)
	g.Call("bindVertexArray", r.emptyVAO)
	g.Call("drawArrays", r.c.TRIANGLES, 0, 3)
	r.Stats.DrawCalls++
	g.Call("depthMask", true)
	g.Call("enable", r.c.DEPTH_TEST)
}

// drawWorld issues the opaque geometry for a pass and returns transparent items.
func (r *Renderer) drawWorld(s *Scene, views []chunkView, pass Pass, fr engine.Frustum, maxDist float64) []transItem {
	cam := s.Cam.Pos
	q := s.Quality
	var trans []transItem
	mainPass := pass == PassMain
	for _, v := range views {
		c := v.c
		if v.d*terrain.ChunkSize > maxDist+terrain.ChunkSize {
			continue
		}
		if mainPass {
			lo, hi := c.Bounds()
			if !fr.IntersectsAABB(lo, hi) {
				continue
			}
		}
		cg := r.chunkGPUFor(c)
		if cg.ter != nil {
			r.setCull(true)
			r.draw(cg.ter.vao, cg.ter.n, 1)
		}
		if cg.wat != nil && mainPass {
			cx, cz := c.Center()
			trans = append(trans, transItem{vao: cg.wat.vao, count: cg.wat.n, dist: math.Hypot(cx-cam.X, cz-cam.Z)})
		}
		viewR := float64(q.ViewChunks) + 0.6
		// trees
		if v.d <= viewR && (pass != PassShadow || v.d*terrain.ChunkSize < q.ShadowDist+40) {
			lod := 0
			if v.d > 2.4 || pass == PassReflect || (pass == PassShadow && v.d > 1.2) {
				lod = 1
			}
			if set := r.instSetFor(c, cg, world.LayerTrees, lod); set != nil {
				r.drawSet(set, pass)
			}
		}
		if pass == PassShadow {
			continue
		}
		if pass == PassReflect {
			continue
		}
		smallR := math.Min(float64(q.ViewChunks), q.GrassChunks*1.5)
		if v.d <= smallR {
			if set := r.instSetFor(c, cg, world.LayerSmall, 0); set != nil {
				r.drawSet(set, pass)
			}
		}
		if v.d <= q.GrassChunks {
			if set := r.instSetFor(c, cg, world.LayerGrass, 0); set != nil {
				r.drawSet(set, pass)
			}
		}
	}
	// landmarks
	for _, lm := range s.Landmarks {
		lg := r.lmFor(lm)
		dist := math.Hypot(cam.X-lm.Zone.X, cam.Z-lm.Zone.Z)
		if dist > lm.Radius+maxDist+60 {
			continue
		}
		for i := range lm.Parts {
			p := lm.Parts[i]
			if pass == PassShadow && p.Mesh.NoShadow {
				continue
			}
			if pass == PassReflect && p.Mesh.Transparent {
				continue
			}
			pg := &lg.parts[i]
			x, y, z, sc := r.writePose(lm, i, pg, s.Time)
			lo, hi := p.Mesh.Bounds()
			rad := math.Max(p.Mesh.Radius(), (hi.Y-lo.Y)/2) * sc * 1.45
			cy := y + (lo.Y+hi.Y)/2*sc
			if mainPass {
				if !fr.IntersectsAABB(engine.Vec3{X: x - rad, Y: cy - rad, Z: z - rad}, engine.Vec3{X: x + rad, Y: cy + rad, Z: z + rad}) {
					continue
				}
			} else if math.Hypot(cam.X-x, cam.Z-z) > maxDist+rad {
				continue
			}
			if p.Mesh.Transparent {
				if mainPass {
					trans = append(trans, transItem{vao: pg.vao, count: pg.mesh.n, dist: math.Hypot(cam.X-x, cam.Z-z)})
				}
				continue
			}
			r.setCull(!p.Mesh.TwoSided)
			r.draw(pg.vao, pg.mesh.n, 1)
		}
	}
	// animals
	if mainPass {
		r.setCull(false)
		for _, a := range s.Animals {
			n := len(a.Data) / 8
			if n == 0 {
				continue
			}
			ag := r.animalFor(a.Mesh)
			b := f32bytes(a.Data)
			r.gl.Call("bindBuffer", r.c.ARRAY_BUFFER, ag.inst)
			r.upload(r.c.ARRAY_BUFFER, b, r.c.DYNAMIC_DRAW)
			r.draw(ag.vao, ag.mesh.n, n)
		}
	}
	r.setCull(true)
	return trans
}

func (r *Renderer) animalFor(m *mesh.Mesh) *animalGPU {
	if a, ok := r.animals[m]; ok {
		return a
	}
	gm := r.sharedMesh(m)
	inst := r.gl.Call("createBuffer")
	a := &animalGPU{mesh: gm, inst: inst}
	a.vao = r.newVAO(gm, inst, 0)
	r.animals[m] = a
	return a
}

func (r *Renderer) drawSet(set *instSet, pass Pass) {
	for _, d := range set.draws {
		if pass == PassShadow && d.noShadow {
			continue
		}
		r.setCull(!d.twoSided)
		r.draw(d.vao, d.indices, d.count)
	}
}

func (r *Renderer) drawTransparent(items []transItem) {
	if len(items) == 0 {
		return
	}
	g := r.gl
	sort.Slice(items, func(i, j int) bool { return items[i].dist > items[j].dist })
	g.Call("enable", r.c.BLEND)
	g.Call("blendFunc", r.c.SRC_ALPHA, r.c.ONE_MINUS_SRC_ALPHA)
	g.Call("depthMask", false)
	r.setCull(false)
	for _, it := range items {
		r.draw(it.vao, it.count, 1)
	}
	g.Call("depthMask", true)
	g.Call("disable", r.c.BLEND)
	r.setCull(true)
}

func (r *Renderer) drawParticles(s *Scene) {
	g := r.gl
	g.Call("enable", r.c.BLEND)
	g.Call("depthMask", false)
	r.setCull(false)
	r.use(r.partProg)
	kl, ll := r.loc(r.partProg, "uKind"), r.loc(r.partProg, "uLines")
	counts := []int{s.Plan.Rain, s.Plan.Snow, s.Plan.Pollen, s.Plan.Dust, s.Plan.Leaves, s.Plan.Fireflies, s.Plan.Spores}
	for kind, n := range counts {
		n = min(n, r.maxParticles)
		if n <= 0 {
			continue
		}
		additive := kind == 5 || kind == 6
		if additive {
			g.Call("blendFunc", r.c.SRC_ALPHA, r.c.ONE)
		} else {
			g.Call("blendFunc", r.c.SRC_ALPHA, r.c.ONE_MINUS_SRC_ALPHA)
		}
		g.Call("uniform1i", kl, kind)
		g.Call("bindVertexArray", r.partVAO)
		if kind == 0 {
			g.Call("bindBuffer", r.c.ARRAY_BUFFER, r.partLines)
			g.Call("enableVertexAttribArray", 0)
			g.Call("vertexAttribPointer", 0, 2, r.c.FLOAT, false, 8, 0)
			g.Call("uniform1i", ll, 1)
			g.Call("drawArrays", r.c.LINES, 0, n*2)
		} else {
			g.Call("bindBuffer", r.c.ARRAY_BUFFER, r.partIDs)
			g.Call("enableVertexAttribArray", 0)
			g.Call("vertexAttribPointer", 0, 2, r.c.FLOAT, false, 8, 0)
			g.Call("uniform1i", ll, 0)
			g.Call("drawArrays", r.c.POINTS, 0, n)
		}
		r.Stats.DrawCalls++
	}
	// localised CPU particles (mist, splashes, spores)
	if n := len(s.CPUParts) / 8; n > 0 {
		r.use(r.cpuProg)
		g.Call("blendFunc", r.c.SRC_ALPHA, r.c.ONE_MINUS_SRC_ALPHA)
		g.Call("bindVertexArray", r.cpuVAO)
		g.Call("bindBuffer", r.c.ARRAY_BUFFER, r.cpuBuf)
		b := f32bytes(s.CPUParts[:n*8])
		r.upload(r.c.ARRAY_BUFFER, b, r.c.DYNAMIC_DRAW)
		g.Call("enableVertexAttribArray", 0)
		g.Call("vertexAttribPointer", 0, 3, r.c.FLOAT, false, 32, 0)
		g.Call("enableVertexAttribArray", 1)
		g.Call("vertexAttribPointer", 1, 1, r.c.FLOAT, false, 32, 12)
		g.Call("enableVertexAttribArray", 2)
		g.Call("vertexAttribPointer", 2, 4, r.c.FLOAT, false, 32, 16)
		g.Call("drawArrays", r.c.POINTS, 0, n)
		r.Stats.DrawCalls++
	}
	g.Call("depthMask", true)
	g.Call("disable", r.c.BLEND)
	r.setCull(true)
}

func (r *Renderer) fullscreen() {
	r.gl.Call("bindVertexArray", r.emptyVAO)
	r.gl.Call("drawArrays", r.c.TRIANGLES, 0, 3)
	r.Stats.DrawCalls++
}

func (r *Renderer) post(s *Scene) {
	g := r.gl
	q := s.Quality
	g.Call("disable", r.c.DEPTH_TEST)
	g.Call("depthMask", false)
	bloomOn := q.Bloom
	if bloomOn {
		g.Call("bindFramebuffer", r.c.FRAMEBUFFER, r.bloomFBO[0])
		g.Call("viewport", 0, 0, r.bloomW, r.bloomH)
		r.use(r.brightProg)
		r.bindTex(0, r.sceneColor)
		r.fullscreen()
		r.use(r.blurProg)
		dl := r.loc(r.blurProg, "uDir")
		for pass := 0; pass < 2; pass++ {
			g.Call("bindFramebuffer", r.c.FRAMEBUFFER, r.bloomFBO[1])
			r.bindTex(0, r.bloomTex[0])
			g.Call("uniform2f", dl, 1/float64(r.bloomW), 0)
			r.fullscreen()
			g.Call("bindFramebuffer", r.c.FRAMEBUFFER, r.bloomFBO[0])
			r.bindTex(0, r.bloomTex[1])
			g.Call("uniform2f", dl, 0, 1/float64(r.bloomH))
			r.fullscreen()
		}
	}
	g.Call("bindFramebuffer", r.c.FRAMEBUFFER, js.Null())
	g.Call("viewport", 0, 0, r.canvasW, r.canvasH)
	r.use(r.postProg)
	r.bindTex(0, r.sceneColor)
	if bloomOn {
		r.bindTex(1, r.bloomTex[0])
	} else {
		r.bindTex(1, r.dummyTex)
	}
	r.bindTex(2, r.sceneDepth)
	bs := 0.0
	if bloomOn {
		bs = 0.55
		if s.Atmos.Night > 0.5 {
			bs = 0.75
		}
	}
	dof := 0.0
	if q.DOF && s.Focus > 0 {
		dof = 1
	}
	rays := 0.0
	if q.GodRays {
		rays = 1
	}
	g.Call("uniform4f", r.loc(r.postProg, "uPost"), bs, rays, dof, s.Focus)
	u, v, vis := s.SunScreen(float64(r.sceneW) / float64(r.sceneH))
	g.Call("uniform4f", r.loc(r.postProg, "uSun"), u, v, vis, 0)
	r.fullscreen()
	g.Call("depthMask", true)
	g.Call("enable", r.c.DEPTH_TEST)
}
