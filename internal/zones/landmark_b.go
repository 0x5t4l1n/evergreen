package zones

import (
	"math"

	"evergreen/internal/mesh"
)

// bridge builds a plank-and-rope bridge between two points and registers it as walkable.
func (l *lmb) bridge(a, c v3, width float64) {
	b := l.op
	d := c.Sub(a)
	length := d.Len()
	n := int(length / 0.7)
	dir := d.Norm()
	side := v3{X: -dir.Z, Z: dir.X}.Norm()
	yaw := math.Atan2(dir.X, dir.Z)
	for i := 0; i < n; i++ {
		t := (float64(i) + 0.5) / float64(n)
		p := a.Add(d.Scale(t))
		p.Y -= 0.7 * math.Sin(t*math.Pi) * 0 // decks stay flat to walk on; sag is in the ropes
		st := woodS
		st.SwayK = float32(0.25 * math.Sin(t*math.Pi))
		b.BoxYaw(p, v3{X: width, Y: 0.06, Z: 0.3}, yaw, st)
	}
	for _, s := range []float64{-1, 1} {
		var path []v3
		var radii []float64
		for i := 0; i <= 8; i++ {
			t := float64(i) / 8
			p := a.Add(d.Scale(t)).Add(side.Scale(s * width))
			p.Y += 1.05 - 0.35*math.Sin(t*math.Pi)
			path = append(path, p)
			radii = append(radii, 0.045)
		}
		b.Tube(path, radii, 4, ropeS, false)
		for i := 0; i <= int(length/3); i++ {
			t := float64(i) / math.Max(1, math.Floor(length/3))
			p := a.Add(d.Scale(t)).Add(side.Scale(s * width))
			b.Beam(p, p.Add(v3{Y: 1.05 - 0.35*math.Sin(t*math.Pi)}), 0.04, ropeS)
		}
	}
	l.walkSeg(a.X, a.Z, c.X, c.Z, width, a.Y+0.05, c.Y+0.05)
	if length > 12 {
		m := a.Add(d.Scale(0.5))
		l.lantern(m.X+side.X*width, m.Y+1.5, m.Z+side.Z*width, true)
	}
}

func (l *lmb) railing(cx, cz, y, r float64, skip []float64) {
	const posts = 28
	for k := 0; k < posts; k++ {
		a := float64(k) / posts * 2 * math.Pi
		blocked := false
		for _, sa := range skip {
			dd := math.Abs(math.Remainder(a-sa, 2*math.Pi))
			if dd < 0.32 {
				blocked = true
			}
		}
		if blocked {
			continue
		}
		x, z := cx+math.Cos(a)*r, cz+math.Sin(a)*r
		l.op.Beam(v3{X: x, Y: y, Z: z}, v3{X: x, Y: y + 1.2, Z: z}, 0.06, woodDk)
		l.circle(x, z, 0.35, y, y+1.4, 0)
	}
	var path []v3
	var radii []float64
	for k := 0; k <= posts; k++ {
		a := float64(k) / posts * 2 * math.Pi
		blocked := false
		for _, sa := range skip {
			if math.Abs(math.Remainder(a-sa, 2*math.Pi)) < 0.32 {
				blocked = true
			}
		}
		if blocked {
			if len(path) > 1 {
				l.op.Tube(path, radii, 4, ropeS, false)
			}
			path, radii = nil, nil
			continue
		}
		path = append(path, v3{X: cx + math.Cos(a)*r, Y: y + 1.15, Z: cz + math.Sin(a)*r})
		radii = append(radii, 0.04)
	}
	if len(path) > 1 {
		l.op.Tube(path, radii, 4, ropeS, false)
	}
}

func (l *lmb) house(cx, cz, y, hx, hz, h float64, yaw float64) {
	b := l.op
	b.BoxYaw(v3{X: cx, Y: y + h/2, Z: cz}, v3{X: hx, Y: h / 2, Z: hz}, yaw, woodS)
	roof := mesh.Style{Color: [3]float32{0.2, 0.34, 0.14}, Mat: mesh.MatFoliage, Jitter: 0.08}
	b.Cone(v3{X: cx, Y: y + h, Z: cz}, 2.4, math.Max(hx, hz)*1.5, 4, roof)
	s, c := math.Sin(yaw), math.Cos(yaw)
	win := emissive([3]float32{1, 0.72, 0.38}, 0.3)
	for _, sgn := range []float64{-1, 1} {
		wx, wz := cx+sgn*hx*0.5*c, cz-sgn*hx*0.5*s
		b.BoxYaw(v3{X: wx + s*(hz+0.03), Y: y + h*0.55, Z: wz + c*(hz+0.03)}, v3{X: 0.38, Y: 0.4, Z: 0.04}, yaw, win)
	}
	b.BoxYaw(v3{X: cx + s*(hz+0.04), Y: y + 1.0, Z: cz + c*(hz+0.04)}, v3{X: 0.5, Y: 1.0, Z: 0.04}, yaw, woodDk)
	l.light(cx+s*(hz+1.5), y+h*0.6, cz+c*(hz+1.5), 1, 0.72, 0.35, 12, 0.3)
	l.box(cx, cz, hx, hz, y, y+h+1, 0)
}

func buildTreehouse(z Zone) *Landmark {
	l := newLMB(z)
	b := l.op
	type tr struct{ u, v, y float64 }
	trunks := []tr{{0, 0, 16}}
	for i := 0; i < 5; i++ {
		a := (float64(i)/5)*2*math.Pi + l.rng.Range(-0.15, 0.15)
		r := l.rng.Range(26, 32)
		trunks = append(trunks, tr{math.Cos(a) * r, math.Sin(a) * r, 16 + float64(i%3)*1.5})
	}
	leaf := mesh.Style{Color: [3]float32{0.07, 0.34, 0.14}, Mat: mesh.MatFoliage, SwayK: 0.4, SwayH: 70, Jitter: 0.08}
	for i, t := range trunks {
		bark := woodDk
		bark.Mat = mesh.MatBark
		var path []v3
		var radii []float64
		for k := 0; k <= 10; k++ {
			f := float64(k) / 10
			path = append(path, v3{X: t.u + math.Sin(f*3+float64(i))*0.8, Y: -1 + 68*f, Z: t.v + math.Cos(f*2.4+float64(i))*0.8})
			radii = append(radii, 2.7-1.2*f+1.0*math.Pow(1-f, 5))
		}
		b.Tube(path, radii, 12, bark, true)
		for k := 0; k < 5; k++ {
			a := float64(k)/5*2*math.Pi + float64(i)
			d := v3{X: math.Cos(a), Z: math.Sin(a)}
			b.Tube([]v3{{X: t.u + d.X*1.2, Y: 3, Z: t.v + d.Z*1.2}, {X: t.u + d.X*3, Y: 0.6, Z: t.v + d.Z*3}, {X: t.u + d.X*5, Y: -0.4, Z: t.v + d.Z*5}}, []float64{0.3, 0.5, 0.35}, 6, bark, false)
		}
		for k := 0; k < 7; k++ {
			a := float64(k)/7*2*math.Pi + l.rng.Range(0, 1)
			b.Blob(v3{X: t.u + math.Cos(a)*8, Y: 60 + l.rng.Range(-3, 8), Z: t.v + math.Sin(a)*8}, v3{X: 9, Y: 4.5, Z: 9}, 5, 8, 0.3, 1, leaf)
		}
		b.Blob(v3{X: t.u, Y: 70, Z: t.v}, v3{X: 11, Y: 5, Z: 11}, 5, 9, 0.3, 1, leaf)
		l.circle(t.u, t.v, 2.9, -2, 70, 0)
		// platform
		b.Cylinder(v3{X: t.u, Y: t.y - 0.5, Z: t.v}, 0.5, 8.2, 8.2, 20, woodS)
		b.Cylinder(v3{X: t.u, Y: t.y - 1.5, Z: t.v}, 1.0, 3.4, 5.6, 14, woodDk)
		l.walkDisc(t.u, t.v, 8.1, t.y)
	}
	// bridges: hub to every ring trunk and ring to ring
	skips := make([][]float64, len(trunks))
	link := func(i, j int) {
		a, c := trunks[i], trunks[j]
		d := math.Atan2(c.v-a.v, c.u-a.u)
		pa := v3{X: a.u + math.Cos(d)*8.3, Y: a.y, Z: a.v + math.Sin(d)*8.3}
		pc := v3{X: c.u - math.Cos(d)*8.3, Y: c.y, Z: c.v - math.Sin(d)*8.3}
		l.bridge(pa, pc, 1.1)
		skips[i] = append(skips[i], d)
		skips[j] = append(skips[j], d+math.Pi)
	}
	for j := 1; j < len(trunks); j++ {
		link(0, j)
	}
	for j := 1; j < len(trunks); j++ {
		if j < len(trunks)-1 {
			link(j, j+1)
		} else {
			link(j, 1)
		}
	}
	// spiral ramp around the hub trunk
	const segs = 64
	rampR := 12.5
	a0 := 0.4
	turns := 2.6
	var rail []v3
	var rr []float64
	topAngle := 0.0
	for k := 0; k < segs; k++ {
		t0, t1 := float64(k)/segs, float64(k+1)/segs
		an0, an1 := a0+t0*turns*2*math.Pi, a0+t1*turns*2*math.Pi
		y0, y1 := 0.3+t0*15.7, 0.3+t1*15.7
		p0 := v3{X: math.Cos(an0) * rampR, Y: y0, Z: math.Sin(an0) * rampR}
		p1 := v3{X: math.Cos(an1) * rampR, Y: y1, Z: math.Sin(an1) * rampR}
		mid := p0.Add(p1).Scale(0.5)
		yaw := math.Atan2(p1.X-p0.X, p1.Z-p0.Z)
		pl := woodS
		b.BoxYaw(v3{X: mid.X, Y: mid.Y - 0.1, Z: mid.Z}, v3{X: 1.0, Y: 0.1, Z: p1.Sub(p0).Len()/2 + 0.05}, yaw, pl)
		l.walkSeg(p0.X, p0.Z, p1.X, p1.Z, 1.0, y0, y1)
		if k%8 == 0 {
			b.Tube([]v3{{X: p0.X, Y: y0 - 0.2, Z: p0.Z}, {X: p0.X, Y: -0.5, Z: p0.Z}}, []float64{0.2, 0.25}, 5, woodDk, false)
		}
		if k%16 == 5 {
			l.lantern(math.Cos(an0)*(rampR+1.0), y0+1.4, math.Sin(an0)*(rampR+1.0), true)
		}
		ro := rampR + 1.0
		rail = append(rail, v3{X: math.Cos(an0) * ro, Y: y0 + 1.1, Z: math.Sin(an0) * ro})
		rr = append(rr, 0.04)
		l.circle(math.Cos(an0)*(rampR+1.25), math.Sin(an0)*(rampR+1.25), 0.3, y0, y0+1.5, 0)
		topAngle = an1
	}
	b.Tube(rail, rr, 4, ropeS, false)
	// landing from ramp end to the hub platform
	p := v3{X: math.Cos(topAngle) * rampR, Y: 16, Z: math.Sin(topAngle) * rampR}
	q := v3{X: math.Cos(topAngle) * 8.2, Y: 16, Z: math.Sin(topAngle) * 8.2}
	l.bridge(p, q, 1.0)
	skips[0] = append(skips[0], topAngle)
	// railings and houses
	for i, t := range trunks {
		l.railing(t.u, t.v, t.y, 8.0, skips[i])
		ha := float64(i)*1.3 + 0.7
		hx, hz := 2.7, 2.1
		l.house(t.u+math.Cos(ha)*4.6, t.v+math.Sin(ha)*4.6, t.y, hx, hz, 3.2, ha)
		for k := 0; k < 3; k++ {
			a := ha + math.Pi + float64(k)*0.9 - 0.9
			l.lantern(t.u+math.Cos(a)*7.4, t.y+2.2, t.v+math.Sin(a)*7.4, k%2 == 0)
		}
	}
	l.hangingVines(0, 40, 0, 20, 30, 4, 14)
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "glow", U: 0, Y: 18, V: 0, Spread: 38, Rate: 30}, Emitter{Kind: "pollen", U: 0, Y: 10, V: 0, Spread: 50, Rate: 20})
	return l.finish(95)
}

// ---- Ruins ---------------------------------------------------------------

func (l *lmb) glyph(cx, cy, cz float64, face int, size float64) {
	var right, nrm v3
	switch face {
	case 0:
		right, nrm = v3{X: 1}, v3{Z: 1}
	case 1:
		right, nrm = v3{X: -1}, v3{Z: -1}
	case 2:
		right, nrm = v3{Z: -1}, v3{X: 1}
	default:
		right, nrm = v3{Z: 1}, v3{X: -1}
	}
	st := emissive([3]float32{0.35, 1, 0.8}, 0.1)
	pt := func(a, b float64) v3 {
		return v3{X: cx, Y: cy, Z: cz}.Add(right.Scale(a * size)).Add(v3{Y: b * size}).Add(nrm.Scale(0.04))
	}
	n := 3 + l.rng.Intn(3)
	px, py := float64(l.rng.Intn(3)-1)*0.4, float64(l.rng.Intn(3)-1)*0.4
	for i := 0; i < n; i++ {
		nx, ny := float64(l.rng.Intn(3)-1)*0.4, float64(l.rng.Intn(3)-1)*0.4
		if nx == px && ny == py {
			ny = -py + 0.4
		}
		l.op.Beam(pt(px, py), pt(nx, ny), 0.018*size, st)
		px, py = nx, ny
	}
	l.op.Blob(pt(px, py), v3{X: 0.03 * size, Y: 0.03 * size, Z: 0.03 * size}, 3, 5, 0, 1, st)
}

func buildRuins(z Zone) *Landmark {
	l := newLMB(z)
	b := l.op
	const fl = 4.2
	stone := stoneS
	// stepped pyramid
	tiers := []struct{ half, h float64 }{{17, 3.0}, {12, 3.0}, {7.5, 3.4}}
	y := fl
	for i, t := range tiers {
		b.Box(v3{Y: y + t.h/2 - 0.2, Z: 0}, v3{X: t.half, Y: t.h/2 + 0.2, Z: t.half}, stone)
		l.box(0, 0, t.half-0.3, t.half-0.3, y-1, y+t.h, 0)
		if i < len(tiers)-1 {
			l.stairs(0, t.half+6, y-0.1, 0, t.half-0.4, y+t.h, 2.4, 9, stone)
		} else {
			l.stairs(0, t.half+4, y-0.1, 0, t.half-0.4, y+t.h, 2.0, 8, stone)
		}
		y += t.h
		l.walkDisc(0, 0, t.half-0.6, y)
	}
	l.walkSeg(0, 25, 0, 17, 2.3, fl, fl)
	// shrine on the summit
	for _, s := range []float64{-3, 3} {
		for _, t := range []float64{-3, 3} {
			b.Cylinder(v3{X: s, Y: y, Z: t}, 4.2, 0.6, 0.5, 8, stone)
			l.circle(s, t, 0.7, y, y+5, 0)
		}
	}
	b.Box(v3{Y: y + 4.5, Z: 0}, v3{X: 3.9, Y: 0.35, Z: 3.9}, stone)
	b.Blob(v3{Y: y + 0.9}, v3{X: 1.2, Y: 0.9, Z: 1.2}, 5, 8, 0.1, 1, stoneS)
	b.Blob(v3{Y: y + 2.1}, v3{X: 0.35, Y: 0.35, Z: 0.35}, 4, 6, 0, 1, emissive([3]float32{0.4, 1, 0.85}, 0.1))
	l.light(0, y+2.2, 0, 0.35, 1, 0.8, 16, 0.2)
	// columns (some broken) and fallen drums
	for i := 0; i < 14; i++ {
		a := float64(i)/14*2*math.Pi + 0.2
		r := 27.0
		u, v := math.Cos(a)*r, math.Sin(a)*r
		h := 6.5
		if l.rng.Chance(0.5) {
			h = l.rng.Range(1.5, 4.5)
			b.Cylinder(v3{X: u + l.rng.Range(1, 3), Y: fl, Z: v + l.rng.Range(1, 3)}, 1.0, 0.7, 0.7, 8, stone)
		}
		b.Cylinder(v3{X: u, Y: fl - 0.2, Z: v}, h, 0.75, 0.62, 10, stone)
		b.Box(v3{X: u, Y: fl - 0.1, Z: v}, v3{X: 1.1, Y: 0.2, Z: 1.1}, stoneDk)
		if h > 6 {
			b.Box(v3{X: u, Y: fl + h, Z: v}, v3{X: 1.0, Y: 0.25, Z: 1.0}, stone)
		}
		l.circle(u, v, 0.85, fl, fl+h, 0)
		if i%3 == 0 {
			l.glyph(u+math.Cos(a)*0.75, fl+1.6, v+math.Sin(a)*0.75, 2, 0.9)
		}
	}
	// outer broken walls
	for i := 0; i < 9; i++ {
		a0 := float64(i)/9*2*math.Pi + 0.1
		a1 := a0 + 0.45
		r := 38.0
		if i == 4 {
			continue
		}
		u0, v0, u1, v1 := math.Cos(a0)*r, math.Sin(a0)*r, math.Cos(a1)*r, math.Sin(a1)*r
		segs := 5
		for k := 0; k < segs; k++ {
			t := (float64(k) + 0.5) / float64(segs)
			u, v := u0+(u1-u0)*t, v0+(v1-v0)*t
			h := l.rng.Range(0.8, 4.2)
			yaw := math.Atan2(u1-u0, v1-v0)
			b.BoxYaw(v3{X: u, Y: fl - 0.5 + h/2, Z: v}, v3{X: 0.7, Y: h / 2, Z: math.Hypot(u1-u0, v1-v0)/float64(segs)/2 + 0.05}, yaw+math.Pi/2*0, stone)
		}
		l.wall(u0, v0, u1, v1, 0.9, fl-1, fl+4)
		l.hangingVines((u0+u1)/2, fl+3.5, (v0+v1)/2, 4, 3, 1, 3)
	}
	// glyph steles near the stairs
	for _, s := range []float64{-5, 5} {
		b.Box(v3{X: s, Y: fl + 1.6, Z: 24}, v3{X: 0.6, Y: 1.6, Z: 0.5}, stoneDk)
		l.glyph(s, fl+2, 24.52, 0, 1.3)
		l.circle(s, 24, 0.7, fl, fl+3.4, 0)
	}
	// the warm glyph that opens the hidden chamber
	b.Box(v3{X: 0, Y: fl + 1.2, Z: 20.3}, v3{X: 0.9, Y: 1.2, Z: 0.4}, stoneDk)
	l.glyph(0, fl+1.5, 20.72, 0, 1.6)
	l.lm.Interact = append(l.lm.Interact, Interactable{U: 0, Y: fl + 1, V: 20.5, Radius: 4.5, Prompt: "Press E to touch the glyph", Action: "ruins.glyph"})
	// hidden chamber behind the pyramid
	cu, cv := 0.0, -32.0
	hx, hz, hh := 5.0, 5.0, 5.2
	chamber := mesh.NewBuilder("ruins-chamber", z.Seed+3)
	chamber.M.TwoSided = true
	wall := interior
	wall.Color = [3]float32{0.38, 0.36, 0.32}
	chamber.Box(v3{X: cu - hx, Y: fl + hh/2, Z: cv}, v3{X: 0.5, Y: hh / 2, Z: hz + 0.5}, wall)
	chamber.Box(v3{X: cu + hx, Y: fl + hh/2, Z: cv}, v3{X: 0.5, Y: hh / 2, Z: hz + 0.5}, wall)
	chamber.Box(v3{X: cu, Y: fl + hh/2, Z: cv - hz}, v3{X: hx, Y: hh / 2, Z: 0.5}, wall)
	chamber.Box(v3{X: cu - hx/2 - 0.9, Y: fl + hh/2, Z: cv + hz}, v3{X: hx/2 - 0.9, Y: hh / 2, Z: 0.5}, wall)
	chamber.Box(v3{X: cu + hx/2 + 0.9, Y: fl + hh/2, Z: cv + hz}, v3{X: hx/2 - 0.9, Y: hh / 2, Z: 0.5}, wall)
	chamber.Box(v3{X: cu, Y: fl + hh - 0.4, Z: cv + hz}, v3{X: 1.8, Y: 0.6, Z: 0.5}, wall)
	chamber.Box(v3{X: cu, Y: fl + hh + 0.4, Z: cv}, v3{X: hx + 1, Y: 0.5, Z: hz + 1}, stone)
	chamber.Box(v3{X: cu, Y: fl + 0.05, Z: cv}, v3{X: hx, Y: 0.1, Z: hz}, interior)
	l.op.Append(chamber.Finish(), v3{}, 0, 1)
	l.box(cu-hx, cv, 0.5, hz+0.5, fl-1, fl+hh, 0)
	l.box(cu+hx, cv, 0.5, hz+0.5, fl-1, fl+hh, 0)
	l.box(cu, cv-hz, hx, 0.5, fl-1, fl+hh, 0)
	l.box(cu-hx/2-0.9, cv+hz, hx/2-0.9, 0.5, fl-1, fl+hh, 0)
	l.box(cu+hx/2+0.9, cv+hz, hx/2-0.9, 0.5, fl-1, fl+hh, 0)
	l.box(cu, cv+hz, 1.8, 0.5, fl-1, fl+hh, 1) // door
	l.walkDisc(cu, cv, 5.2, fl+0.1)
	// altar, glowing floor sigil, wall glyphs
	l.op.Box(v3{X: cu, Y: fl + 0.7, Z: cv - 2.5}, v3{X: 1.2, Y: 0.6, Z: 0.7}, stoneDk)
	l.op.Blob(v3{X: cu, Y: fl + 1.7, Z: cv - 2.5}, v3{X: 0.35, Y: 0.35, Z: 0.35}, 4, 7, 0, 1, mesh.Style{Color: [3]float32{0.5, 1, 0.9}, Mat: mesh.MatCrystal})
	for k := 0; k < 3; k++ {
		r := 1.5 + float64(k)*0.9
		var path []v3
		var rad []float64
		for s := 0; s <= 28; s++ {
			a := float64(s) / 28 * 2 * math.Pi
			path = append(path, v3{X: cu + math.Cos(a)*r, Y: fl + 0.13, Z: cv + 1 + math.Sin(a)*r})
			rad = append(rad, 0.035)
		}
		l.op.Tube(path, rad, 4, emissive([3]float32{0.3, 0.95, 0.8}, 0.1), false)
	}
	for k := -1; k <= 1; k++ {
		l.glyph(cu+float64(k)*2.4, fl+2.6, cv-hz+0.52, 0, 1.4)
	}
	l.glyph(cu-hx+0.52, fl+2.4, cv, 2, 1.6)
	l.glyph(cu+hx-0.52, fl+2.4, cv, 3, 1.6)
	l.light(cu, fl+2.6, cv, 0.3, 1, 0.8, 14, 0.15)
	door := mesh.NewBuilder("ruins-door", z.Seed+4)
	door.Box(v3{X: cu, Y: fl + 1.8, Z: cv + hz}, v3{X: 1.8, Y: 1.9, Z: 0.45}, stoneDk)
	door.Box(v3{X: cu, Y: fl + 2.6, Z: cv + hz + 0.47}, v3{X: 0.05, Y: 1.2, Z: 0.02}, emissive([3]float32{0.35, 1, 0.8}, 0.1))
	l.lm.Parts = append(l.lm.Parts, Part{Mesh: door.Finish(), Anim: AnimDoor, Group: 1, Slide: 4.4})
	l.rockPile(0, 0, 55, 14, 0.6, 1.8, nil)
	l.hangingVines(0, fl+12, 0, 14, 16, 3, 7)
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "glow", U: 0, Y: fl + 2, V: -32, Spread: 5, Rate: 14}, Emitter{Kind: "mist", U: 0, Y: fl, V: 0, Spread: 45, Rate: 14})
	return l.finish(75)
}

// ---- Observatory ---------------------------------------------------------

func buildObservatory(z Zone) *Landmark {
	l := newLMB(z)
	b := l.op
	const top = 70.0
	b.Cylinder(v3{Y: top - 0.2}, 4.4, 7.6, 7.2, 20, mesh.Style{Color: [3]float32{0.62, 0.62, 0.58}, Mat: mesh.MatStone, Jitter: 0.03})
	b.Box(v3{X: 0, Y: top + 1.4, Z: 7.4}, v3{X: 1.1, Y: 1.4, Z: 0.35}, woodDk)
	b.Cylinder(v3{Y: top + 4.1}, 0.3, 8.2, 8.2, 24, metalS)
	l.circle(0, 0, 8.4, top-1, top+14, 0)
	// deck
	b.Cylinder(v3{Y: top - 0.45}, 0.5, 19.5, 19.5, 28, woodS)
	l.walkDisc(0, 0, 19.2, top+0.05)
	for k := 0; k < 24; k++ {
		a := float64(k) / 24 * 2 * math.Pi
		u, v := math.Cos(a)*19, math.Sin(a)*19
		b.Beam(v3{X: u, Y: top, Z: v}, v3{X: u, Y: top + 1.15, Z: v}, 0.07, metalDk)
		l.circle(u, v, 0.4, top, top+1.3, 0)
		if k%6 == 0 {
			l.lantern(u, top+2.3, v, false)
			b.Beam(v3{X: u, Y: top + 1.1, Z: v}, v3{X: u, Y: top + 2.0, Z: v}, 0.06, metalDk)
		}
	}
	var rail []v3
	var rr []float64
	for k := 0; k <= 24; k++ {
		a := float64(k) / 24 * 2 * math.Pi
		rail = append(rail, v3{X: math.Cos(a) * 19, Y: top + 1.15, Z: math.Sin(a) * 19})
		rr = append(rr, 0.05)
	}
	b.Tube(rail, rr, 4, metalDk, false)
	// telescope on a tripod
	tu, tv := 12.0, 4.0
	for k := 0; k < 3; k++ {
		a := float64(k)/3*2*math.Pi + 0.5
		b.Tube([]v3{{X: tu, Y: top + 1.7, Z: tv}, {X: tu + math.Cos(a)*1.1, Y: top, Z: tv + math.Sin(a)*1.1}}, []float64{0.05, 0.06}, 5, metalDk, true)
	}
	dir := v3{X: -0.45, Y: 0.8, Z: -0.4}.Norm()
	p0 := v3{X: tu, Y: top + 1.7, Z: tv}.Add(dir.Scale(-1.0))
	b.Tube([]v3{p0, p0.Add(dir.Scale(1.2)), p0.Add(dir.Scale(2.6))}, []float64{0.2, 0.22, 0.2}, 12, mesh.Style{Color: [3]float32{0.85, 0.86, 0.88}, Mat: mesh.MatMetal}, true)
	b.Tube([]v3{p0.Add(dir.Scale(2.6)), p0.Add(dir.Scale(2.8))}, []float64{0.23, 0.23}, 12, metalDk, true)
	b.Tube([]v3{p0.Add(dir.Scale(0.4)).Add(v3{X: 0.2, Y: 0.1}), p0.Add(dir.Scale(1.0)).Add(v3{X: 0.25, Y: 0.1})}, []float64{0.05, 0.05}, 5, metalDk, true)
	l.circle(tu, tv, 0.9, top, top+3, 0)
	l.lm.Interact = append(l.lm.Interact, Interactable{U: tu, Y: top + 1, V: tv, Radius: 4, Prompt: "Press E to look through the telescope", Action: "observatory.telescope"})
	// benches & instruments
	for _, a := range []float64{0.5, 2.1, 3.6, 5.2} {
		u, v := math.Cos(a)*16, math.Sin(a)*16
		b.BoxYaw(v3{X: u, Y: top + 0.45, Z: v}, v3{X: 0.9, Y: 0.06, Z: 0.3}, -a+math.Pi/2, woodS)
		b.BoxYaw(v3{X: u, Y: top + 0.2, Z: v}, v3{X: 0.8, Y: 0.2, Z: 0.1}, -a+math.Pi/2, woodDk)
	}
	// wind mast with spinning anemometer
	mu, mv := -12.0, -6.0
	b.Tube([]v3{{X: mu, Y: top, Z: mv}, {X: mu, Y: top + 8, Z: mv}}, []float64{0.1, 0.06}, 6, metalS, true)
	b.Blob(v3{X: mu, Y: top + 8.3, Z: mv}, v3{X: 0.12, Y: 0.12, Z: 0.12}, 3, 5, 0, 1, emissive([3]float32{1, 0.2, 0.2}, 0.2))
	l.light(mu, top+8.4, mv, 1, 0.2, 0.2, 9, 0.5)
	anem := mesh.NewBuilder("anemometer", z.Seed+8)
	for k := 0; k < 3; k++ {
		a := float64(k) / 3 * 2 * math.Pi
		anem.Beam(v3{}, v3{X: math.Cos(a) * 0.5, Z: math.Sin(a) * 0.5}, 0.02, metalDk)
		anem.Blob(v3{X: math.Cos(a) * 0.55, Z: math.Sin(a) * 0.55}, v3{X: 0.12, Y: 0.1, Z: 0.12}, 4, 6, 0, 1, metalS)
	}
	l.lm.Parts = append(l.lm.Parts, Part{Mesh: anem.Finish(), U: mu, Y: top + 8.0, V: mv, Anim: AnimSpin, Rate: 4})
	// rotating dome
	dome := mesh.NewBuilder("dome", z.Seed+9)
	dome.M.TwoSided = true
	dm := mesh.Style{Color: [3]float32{0.88, 0.9, 0.92}, Mat: mesh.MatMetal, Jitter: 0.015}
	dome.Blob(v3{}, v3{X: 7.8, Y: 6.4, Z: 7.8}, 14, 28, 0, 1, dm)
	dome.Box(v3{X: 4.6, Y: 3.4, Z: 0}, v3{X: 3.6, Y: 3.2, Z: 0.85}, emissive([3]float32{0.01, 0.015, 0.05}, 0))
	dome.Cylinder(v3{Y: -0.3}, 0.6, 8, 8, 28, metalS)
	l.lm.Parts = append(l.lm.Parts, Part{Mesh: dome.Finish(), Y: top + 4.4, Anim: AnimSpin, Rate: 0.06})
	l.light(0, top+3, 8.6, 1, 0.82, 0.55, 12, 0.2)
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "glow", U: 0, Y: top + 4, V: 0, Spread: 25, Rate: 10})
	return l.finish(40)
}

// ---- Cyber Forest Station ---------------------------------------------------

func buildStation(z Zone) *Landmark {
	l := newLMB(z)
	b := l.op
	const (
		hx, hz = 12.0, 7.0
		wallH  = 4.4
		fl     = 0.45
	)
	conc := mesh.Style{Color: [3]float32{0.32, 0.35, 0.33}, Mat: mesh.MatMetal, Jitter: 0.05}
	rust := mesh.Style{Color: [3]float32{0.36, 0.25, 0.19}, Mat: mesh.MatMetal, Jitter: 0.05}
	in := interior
	// slab and roof
	b.Box(v3{Y: fl/2 - 0.1, Z: 0}, v3{X: hx + 0.6, Y: fl/2 + 0.1, Z: hz + 0.6}, conc)
	b.Box(v3{Y: fl + 0.01, Z: 0}, v3{X: hx - 0.4, Y: 0.02, Z: hz - 0.4}, in)
	b.Box(v3{Y: fl + wallH + 0.3, Z: 0}, v3{X: hx + 0.8, Y: 0.3, Z: hz + 0.8}, conc)
	// walls (outer faces rusty concrete, interior panel just inside)
	wall := func(cu, cv, hu, hv float64) {
		b.Box(v3{X: cu, Y: fl + wallH/2, Z: cv}, v3{X: hu, Y: wallH / 2, Z: hv}, rust)
		l.box(cu, cv, hu, hv, fl-0.5, fl+wallH, 0)
	}
	wall(0, -hz, hx, 0.3)
	wall(-hx, 0, 0.3, hz)
	wall(hx, 0, 0.3, hz)
	wall(-(hx+2.0)/2-0.9, hz, (hx-2.0)/2+0.1, 0.3)
	wall((hx+2.0)/2+0.9, hz, (hx-2.0)/2+0.1, 0.3)
	b.Box(v3{Y: fl + wallH - 0.5, Z: hz}, v3{X: 2.1, Y: 0.55, Z: 0.3}, rust)
	// interior wall liner so interiors use the indoor material
	b.Box(v3{Y: fl + wallH/2, Z: -hz + 0.35}, v3{X: hx - 0.4, Y: wallH / 2, Z: 0.05}, in)
	b.Box(v3{X: -hx + 0.35, Y: fl + wallH/2, Z: 0}, v3{X: 0.05, Y: wallH / 2, Z: hz - 0.4}, in)
	b.Box(v3{X: hx - 0.35, Y: fl + wallH/2, Z: 0}, v3{X: 0.05, Y: wallH / 2, Z: hz - 0.4}, in)
	b.Box(v3{Y: fl + wallH - 0.05, Z: 0}, v3{X: hx - 0.4, Y: 0.05, Z: hz - 0.4}, in)
	l.walkSeg(-hx+0.6, 0, hx-0.6, 0, hz-0.5, fl+0.02, fl+0.02)
	l.walkSeg(0, hz+0.5, 0, hz+4, 2.2, fl+0.02, fl-0.2)
	// windows (glass with rain) on the long walls
	glass := mesh.Style{Color: [3]float32{0.5, 0.7, 0.75}, Mat: mesh.MatGlass}
	for _, cx := range []float64{-8, -4.5, 4.5, 8} {
		l.tr.Box(v3{X: cx, Y: fl + 2.4, Z: hz + 0.34}, v3{X: 1.25, Y: 0.95, Z: 0.03}, glass)
		b.Beam(v3{X: cx - 1.3, Y: fl + 2.4, Z: hz + 0.32}, v3{X: cx + 1.3, Y: fl + 2.4, Z: hz + 0.32}, 0.05, metalDk)
		b.Box(v3{X: cx, Y: fl + 2.4, Z: hz + 0.36}, v3{X: 1.4, Y: 1.05, Z: 0.02}, metalDk)
		l.tr.Box(v3{X: cx, Y: fl + 2.4, Z: -hz - 0.34}, v3{X: 1.25, Y: 0.95, Z: 0.03}, glass)
	}
	l.tr.Box(v3{Y: fl + 2.1, Z: hz + 0.34}, v3{X: 1.6, Y: 1.9, Z: 0.03}, glass) // glass door
	b.Box(v3{X: -2.0, Y: fl + 2.1, Z: hz + 0.38}, v3{X: 0.12, Y: 2.1, Z: 0.1}, metalDk)
	b.Box(v3{X: 2.0, Y: fl + 2.1, Z: hz + 0.38}, v3{X: 0.12, Y: 2.1, Z: 0.1}, metalDk)
	// server racks along the back wall with blinking LEDs
	for i := 0; i < 6; i++ {
		x := -10.4 + float64(i)*1.35
		b.Box(v3{X: x, Y: fl + 1.25, Z: -hz + 1.0}, v3{X: 0.55, Y: 1.25, Z: 0.5}, metalDk)
		for r := 0; r < 9; r++ {
			col := [3]float32{0.2, 1, 0.45}
			variant := float32(0.2)
			if (i+r)%5 == 0 {
				col, variant = [3]float32{1, 0.55, 0.15}, 0.3
			}
			for c := 0; c < 3; c++ {
				b.Box(v3{X: x - 0.3 + float64(c)*0.3, Y: fl + 0.35 + float64(r)*0.24, Z: -hz + 1.51}, v3{X: 0.05, Y: 0.04, Z: 0.01}, emissive(col, variant))
			}
		}
		l.box(x, -hz+1.0, 0.55, 0.5, fl, fl+2.5, 0)
	}
	// console desk with four monitors facing the door
	b.Box(v3{X: 5.5, Y: fl + 0.5, Z: -3.4}, v3{X: 4.2, Y: 0.5, Z: 0.9}, metalS)
	b.Box(v3{X: 5.5, Y: fl + 1.02, Z: -3.4}, v3{X: 4.3, Y: 0.03, Z: 1.0}, woodDk)
	l.box(5.5, -3.4, 4.3, 1.0, fl, fl+1.1, 0)
	for i := 0; i < 4; i++ {
		x := 2.7 + float64(i)*1.85
		b.Box(v3{X: x, Y: fl + 1.65, Z: -3.8}, v3{X: 0.82, Y: 0.5, Z: 0.04}, metalDk)
		scr := mesh.Style{Color: [3]float32{1, 1, 1}, Mat: mesh.MatScreen, Variant: float32(i) * 0.1}
		b.QuadP(v3{X: x - 0.74, Y: fl + 1.2, Z: -3.75}, v3{X: x + 0.74, Y: fl + 1.2, Z: -3.75}, v3{X: x + 0.74, Y: fl + 2.1, Z: -3.75}, v3{X: x - 0.74, Y: fl + 2.1, Z: -3.75}, scr)
	}
	// wall displays: network map and radar
	b.QuadP(v3{X: -hx + 0.42, Y: fl + 1.2, Z: 3.2}, v3{X: -hx + 0.42, Y: fl + 1.2, Z: -2.6}, v3{X: -hx + 0.42, Y: fl + 3.4, Z: -2.6}, v3{X: -hx + 0.42, Y: fl + 3.4, Z: 3.2}, mesh.Style{Color: [3]float32{1, 1, 1}, Mat: mesh.MatScreen, Variant: 0.4})
	b.QuadP(v3{X: 1, Y: fl + 2.6, Z: -hz + 0.4}, v3{X: 5, Y: fl + 2.6, Z: -hz + 0.4}, v3{X: 5, Y: fl + 3.9, Z: -hz + 0.4}, v3{X: 1, Y: fl + 3.9, Z: -hz + 0.4}, mesh.Style{Color: [3]float32{1, 1, 1}, Mat: mesh.MatScreen, Variant: 0.5})
	// interactive control panel
	b.Box(v3{X: 9.2, Y: fl + 0.55, Z: 1.0}, v3{X: 0.7, Y: 0.55, Z: 0.45}, metalS)
	for k := 0; k < 6; k++ {
		col := [3]float32{0.2, 1, 0.5}
		if k%3 == 1 {
			col = [3]float32{1, 0.6, 0.15}
		}
		b.Box(v3{X: 8.85 + float64(k%3)*0.35, Y: fl + 1.12, Z: 0.75 + float64(k/3)*0.35}, v3{X: 0.08, Y: 0.03, Z: 0.08}, emissive(col, 0.2))
	}
	l.box(9.2, 1.0, 0.7, 0.45, fl, fl+1.2, 0)
	l.lm.Interact = append(l.lm.Interact, Interactable{U: 9.2, Y: fl + 1, V: 1.0, Radius: 3.2, Prompt: "Press E to use the control panel", Action: "station.console"})
	// ceiling light strips, plants
	for _, x := range []float64{-7, 0, 7} {
		b.Box(v3{X: x, Y: fl + wallH - 0.15, Z: 0}, v3{X: 1.6, Y: 0.04, Z: 0.25}, emissive([3]float32{1, 0.78, 0.5}, 0))
		l.light(x, fl+3.6, 0, 1, 0.74, 0.42, 14, 0.04)
	}
	l.light(5.5, fl+2.2, -2.0, 0.3, 0.9, 0.5, 9, 0.1)
	l.light(-1, fl+2.4, 4.5, 1, 0.7, 0.4, 9, 0.2)
	for _, p := range [][2]float64{{-10.5, 5.2}, {10.8, 5.6}, {-3.5, 5.5}} {
		b.Cylinder(v3{X: p[0], Y: fl, Z: p[1]}, 0.5, 0.35, 0.3, 8, rust)
		for k := 0; k < 7; k++ {
			frondA := float64(k) / 7 * 2 * math.Pi
			b.Blob(v3{X: p[0] + math.Cos(frondA)*0.3, Y: fl + 1.0 + float64(k%3)*0.3, Z: p[1] + math.Sin(frondA)*0.3}, v3{X: 0.35, Y: 0.55, Z: 0.35}, 4, 6, 0.2, 1, leafSt)
		}
	}
	// roof: satellite dish, antennas, beacon, moss
	dish := mesh.NewBuilder("dish", z.Seed+11)
	dish.M.TwoSided = true
	for ring := 0; ring < 6; ring++ {
		r0, r1 := 1.7*float64(ring)/6, 1.7*float64(ring+1)/6
		for s := 0; s < 16; s++ {
			a0, a1 := float64(s)/16*2*math.Pi, float64(s+1)/16*2*math.Pi
			h := func(r float64) float64 { return 0.22 * r * r }
			p := func(r, a float64) v3 { return v3{X: math.Cos(a) * r, Y: h(r), Z: math.Sin(a) * r} }
			dish.QuadP(p(r0, a0), p(r1, a0), p(r1, a1), p(r0, a1), mesh.Style{Color: [3]float32{0.82, 0.84, 0.86}, Mat: mesh.MatMetal})
		}
	}
	dish.Tube([]v3{{Y: 0.1}, {X: 0.2, Y: 1.1}}, []float64{0.04, 0.04}, 4, metalDk, true)
	b.Append(dish.Finish(), v3{X: -7, Y: fl + wallH + 2.0, Z: -2}, 0.8, 1.2)
	b.Tube([]v3{{X: -7, Y: fl + wallH + 0.6, Z: -2}, {X: -7, Y: fl + wallH + 2.0, Z: -2}}, []float64{0.2, 0.1}, 8, metalS, true)
	for k := 0; k < 4; k++ {
		x := 3 + float64(k)*2.2
		b.Tube([]v3{{X: x, Y: fl + wallH + 0.6, Z: 2}, {X: x, Y: fl + wallH + 3 + float64(k%2)*2.5, Z: 2}}, []float64{0.07, 0.03}, 5, metalDk, true)
		b.Blob(v3{X: x, Y: fl + wallH + 3.1 + float64(k%2)*2.5, Z: 2}, v3{X: 0.1, Y: 0.1, Z: 0.1}, 3, 5, 0, 1, emissive([3]float32{1, 0.15, 0.15}, 0.2))
	}
	l.light(3, fl+wallH+3.2, 2, 1, 0.1, 0.1, 8, 0.5)
	radar := mesh.NewBuilder("radar", z.Seed+13)
	radar.M.TwoSided = true
	radar.Tube([]v3{{Y: -1.6}, {Y: 0}}, []float64{0.12, 0.1}, 6, metalS, true)
	radar.Box(v3{Y: 0.3}, v3{X: 1.7, Y: 0.28, Z: 0.07}, metalS)
	radar.Box(v3{X: 1.6, Y: 0.3}, v3{X: 0.1, Y: 0.3, Z: 0.12}, metalDk)
	l.lm.Parts = append(l.lm.Parts, Part{Mesh: radar.Finish(), U: 8.5, Y: fl + wallH + 4.3, V: -3.5, Anim: AnimSpin, Rate: 1.3})
	b.Tube([]v3{{X: 8.5, Y: fl + wallH + 0.6, Z: -3.5}, {X: 8.5, Y: fl + wallH + 2.7, Z: -3.5}}, []float64{0.16, 0.1}, 6, metalS, true)
	b.Tube([]v3{{X: 8.5, Y: fl + wallH + 2.7, Z: -3.5}, {X: 8.5, Y: fl + wallH + 4.3, Z: -3.5}}, []float64{0.1, 0.1}, 6, metalS, true)
	for i := 0; i < 12; i++ {
		b.Blob(v3{X: l.rng.Range(-hx, hx), Y: fl + wallH + 0.75, Z: l.rng.Range(-hz, hz)}, v3{X: l.rng.Range(0.8, 1.8), Y: 0.2, Z: l.rng.Range(0.8, 1.8)}, 4, 7, 0.3, 1, mossS)
	}
	// vines down the walls and cables to the forest floor
	l.hangingVines(0, fl+wallH+0.4, hz+0.5, 14, hx-1, 1.8, wallH)
	l.hangingVines(0, fl+wallH+0.4, -hz-0.5, 10, hx-1, 1.8, wallH)
	l.hangingVines(hx+0.5, fl+wallH+0.4, 0, 6, hz-1, 1.8, wallH)
	l.hangingVines(-hx-0.5, fl+wallH+0.4, 0, 6, hz-1, 1.8, wallH)
	b.Tube([]v3{{X: -7, Y: fl + wallH + 0.8, Z: -2}, {X: -9, Y: fl + 2, Z: -hz - 2}, {X: -12, Y: 0.1, Z: -hz - 6}}, []float64{0.07, 0.07, 0.07}, 5, metalDk, false)
	// porch lamp and stoop
	l.lantern(-2.6, fl+3.2, hz+1.2, true)
	l.lantern(2.6, fl+3.2, hz+1.2, true)
	l.lm.Emitters = append(l.lm.Emitters, Emitter{Kind: "steam", U: 11, Y: fl + 0.5, V: 8.2, Spread: 0.4, Rate: 10}, Emitter{Kind: "mist", U: 0, Y: 1, V: 0, Spread: 40, Rate: 20})
	return l.finish(40)
}

var leafSt = mesh.Style{Color: [3]float32{0.1, 0.4, 0.14}, Mat: mesh.MatFoliage, SwayK: 0.5, SwayH: 2, Jitter: 0.06}

// ---- World Tree ---------------------------------------------------------------

func buildWorldTree(z Zone) *Landmark {
	l := newLMB(z)
	tree := mesh.NewBuilder("worldtree-trunk", z.Seed+21)
	tree.M.TwoSided = true
	const H = 205.0
	bark := mesh.Style{Color: [3]float32{0.27, 0.2, 0.15}, Mat: mesh.MatBark, SwayK: 0.04, SwayH: H, Jitter: 0.04}
	mossy := mesh.Style{Color: [3]float32{0.16, 0.26, 0.12}, Mat: mesh.MatBark, SwayK: 0.04, SwayH: H, Jitter: 0.05}
	var path []v3
	var radii []float64
	const n = 34
	for i := 0; i <= n; i++ {
		t := float64(i) / n
		path = append(path, v3{X: math.Sin(t*2.6)*3.5 + 0.0, Y: -4 + (H*0.72)*t, Z: math.Cos(t*2.1) * 3})
		radii = append(radii, 12.5*(1-0.55*t)+9*math.Pow(1-t, 6))
	}
	tree.Tube(path, radii, 28, bark, true)
	// moss patches on the trunk base
	for i := 0; i < 22; i++ {
		a := l.rng.Range(0, 2*math.Pi)
		y := l.rng.Range(2, 40)
		r := 11.5 * (1 - 0.55*y/(H*0.72))
		tree.Blob(v3{X: math.Cos(a) * r, Y: y, Z: math.Sin(a) * r}, v3{X: l.rng.Range(1.5, 4), Y: l.rng.Range(1.5, 4), Z: 1.5}, 4, 7, 0.4, 1, mossy)
	}
	// buttress roots: interconnected arches
	type rt struct{ a, l float64 }
	var roots []rt
	const nr = 14
	for i := 0; i < nr; i++ {
		a := float64(i)/nr*2*math.Pi + l.rng.Range(-0.12, 0.12)
		ln := l.rng.Range(42, 78)
		roots = append(roots, rt{a, ln})
		d := v3{X: math.Cos(a), Z: math.Sin(a)}
		pts := []v3{}
		rds := []float64{}
		for k := 0; k <= 10; k++ {
			t := float64(k) / 10
			h := 22*math.Pow(1-t, 2.2) - 4*t
			pts = append(pts, d.Scale(9+ln*t).Add(v3{Y: h + 1}))
			rds = append(rds, 6.2*math.Pow(1-t, 1.4)+0.7)
		}
		tree.Tube(pts, rds, 12, mossy, false)
		// glowing vein along each root
		vein := []v3{}
		vr := []float64{}
		for k := 0; k <= 10; k++ {
			p := pts[k]
			vein = append(vein, v3{X: p.X, Y: p.Y + rds[k]*0.95, Z: p.Z})
			vr = append(vr, 0.12)
		}
		tree.Tube(vein, vr, 4, emissive([3]float32{0.3, 1, 0.75}, 0.1), false)
	}
	for i := 0; i < nr; i++ {
		a0, a1 := roots[i], roots[(i+1)%nr]
		var pts []v3
		var rds []float64
		for k := 0; k <= 8; k++ {
			t := float64(k) / 8
			ang := a0.a + math.Remainder(a1.a-a0.a, 2*math.Pi)*t
			r := 28 + 16*math.Sin(t*math.Pi)
			pts = append(pts, v3{X: math.Cos(ang) * r, Y: 1.2 + 7*math.Sin(t*math.Pi), Z: math.Sin(ang) * r})
			rds = append(rds, 1.4+0.5*math.Sin(t*math.Pi))
		}
		tree.Tube(pts, rds, 8, bark, false)
	}
	// major limbs and canopy
	top := path[n]
	leaf := mesh.Style{Color: [3]float32{0.07, 0.38, 0.17}, Mat: mesh.MatFoliage, SwayK: 0.5, SwayH: 240, Jitter: 0.1}
	leafGlow := mesh.Style{Color: [3]float32{0.1, 0.5, 0.3}, Mat: mesh.MatFoliage, SwayK: 0.5, SwayH: 240, Jitter: 0.1}
	for i := 0; i < 9; i++ {
		a := float64(i)/9*2*math.Pi + l.rng.Range(-0.2, 0.2)
		from := path[24+(i%4)]
		d := v3{X: math.Cos(a), Y: l.rng.Range(0.25, 0.55), Z: math.Sin(a)}
		length := l.rng.Range(60, 100)
		tip := branch2(tree, from, d, length, 4.2, bark)
		for k := 0; k < 6; k++ {
			o := v3{X: l.rng.Range(-18, 18), Y: l.rng.Range(-4, 10), Z: l.rng.Range(-18, 18)}
			st := leaf
			if k%2 == 1 {
				st = leafGlow
			}
			tree.Blob(tip.Add(o), v3{X: l.rng.Range(14, 24), Y: l.rng.Range(7, 12), Z: l.rng.Range(14, 24)}, 5, 8, 0.3, 0.6, st)
		}
	}
	for k := 0; k < 9; k++ {
		a := float64(k) / 9 * 2 * math.Pi
		tree.Blob(top.Add(v3{X: math.Cos(a) * 30, Y: l.rng.Range(-6, 12), Z: math.Sin(a) * 30}), v3{X: 24, Y: 11, Z: 24}, 5, 9, 0.3, 0.5, leaf)
	}
	tree.Blob(top.Add(v3{Y: 20}), v3{X: 34, Y: 14, Z: 34}, 6, 10, 0.3, 0.5, leaf)
	// hanging bioluminescent vines
	vs := mesh.Style{Color: [3]float32{0.2, 0.95, 0.7}, Mat: mesh.MatGlow, SwayK: 0.5, SwayH: 40}
	for i := 0; i < 40; i++ {
		a := l.rng.Range(0, 2*math.Pi)
		r := l.rng.Range(20, 70)
		x, zz := math.Cos(a)*r, math.Sin(a)*r
		y0 := l.rng.Range(110, 170)
		ln := l.rng.Range(14, 40)
		w := 0.18
		v0 := tree.Vert(v3{X: x - w, Y: y0, Z: zz}, v3{Z: 1}, vs.Color, 0, 0, 0, vs.Mat)
		v1 := tree.Vert(v3{X: x + w, Y: y0, Z: zz}, v3{Z: 1}, vs.Color, 0, 1, 0, vs.Mat)
		v2 := tree.Vert(v3{X: x + w, Y: y0 - ln, Z: zz}, v3{Z: 1}, vs.Color, 0.8, 1, 1, vs.Mat)
		v3_ := tree.Vert(v3{X: x - w, Y: y0 - ln, Z: zz}, v3{Z: 1}, vs.Color, 0.8, 0, 1, vs.Mat)
		tree.Quad(v0, v1, v2, v3_)
	}
	tm := tree.Finish()
	tm.NoShadow = false
	l.lm.Parts = append(l.lm.Parts, Part{Mesh: tm, Anim: AnimGrow})
	// ground-level bioluminescent flora (merged; per-vertex colours)
	b := l.op
	for i := 0; i < 90; i++ {
		a := l.rng.Range(0, 2*math.Pi)
		r := l.rng.Range(24, 95)
		u, v := math.Cos(a)*r, math.Sin(a)*r
		col := [3]float32{0.2, 0.95, 0.75}
		switch i % 3 {
		case 1:
			col = [3]float32{0.45, 0.6, 1}
		case 2:
			col = [3]float32{0.7, 0.45, 1}
		}
		h := l.rng.Range(0.3, 1.1)
		b.Tube([]v3{{X: u, Y: 0, Z: v}, {X: u, Y: h, Z: v}}, []float64{0.04, 0.03}, 4, mesh.Style{Color: [3]float32{0.1, 0.3, 0.15}, Mat: mesh.MatStandard}, false)
		b.Blob(v3{X: u, Y: h + 0.1, Z: v}, v3{X: h * 0.5, Y: h * 0.35, Z: h * 0.5}, 4, 7, 0.1, 2, mesh.Style{Color: col, Mat: mesh.MatGlow})
		if i%9 == 0 {
			l.light(u, h+0.6, v, col[0], col[1], col[2], 14, 0.15)
		}
	}
	for i := 0; i < 8; i++ {
		a := float64(i) / 8 * 2 * math.Pi
		l.light(math.Cos(a)*18, 4, math.Sin(a)*18, 0.3, 1, 0.75, 22, 0.1)
	}
	l.circle(0, 0, 14.8, -5, H, 0)
	l.lm.Emitters = append(l.lm.Emitters,
		Emitter{Kind: "glow", U: 0, Y: 20, V: 0, Spread: 70, Rate: 80},
		Emitter{Kind: "glow", U: 0, Y: 140, V: 0, Spread: 60, Rate: 60},
		Emitter{Kind: "pollen", U: 0, Y: 6, V: 0, Spread: 90, Rate: 40})
	lm := l.finish(130)
	lm.MaxY = H + 40
	return lm
}

func branch2(b *mesh.Builder, from, dir v3, length, r0 float64, st mesh.Style) v3 {
	const steps = 6
	var path []v3
	var radii []float64
	p, d := from, dir.Norm()
	for i := 0; i <= steps; i++ {
		path = append(path, p)
		radii = append(radii, math.Max(0.3, r0*(1-0.8*float64(i)/steps)))
		p = p.Add(d.Scale(length / steps))
		d = v3{X: d.X, Y: d.Y - 0.05, Z: d.Z}.Norm()
	}
	b.Tube(path, radii, 8, st, false)
	return path[steps]
}
