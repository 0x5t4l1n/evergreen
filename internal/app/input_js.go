//go:build js && wasm

package app

import (
	"math"
	"syscall/js"

	"evergreen/internal/camera"
)

type touchState struct {
	id     int
	startX float64
	startY float64
	x, y   float64
	role   int // 1 move, 2 look
}

// inputState accumulates DOM events between frames.
type inputState struct {
	keys     map[string]bool
	pressed  map[string]bool // edge-triggered this frame
	lookX    float64
	lookY    float64
	zoom     float64
	scroll   float64
	dragging bool
	touches  map[int]*touchState
	locked   bool
	active   bool
}

func newInput() *inputState {
	return &inputState{keys: map[string]bool{}, pressed: map[string]bool{}, touches: map[int]*touchState{}}
}

func (i *inputState) key(codes ...string) bool {
	for _, c := range codes {
		if i.keys[c] {
			return true
		}
	}
	return false
}

// consume turns accumulated events into a camera.Input and resets per-frame deltas.
func (i *inputState) consume(width, height float64) camera.Input {
	in := camera.Input{Zoom: i.zoom, Scroll: i.scroll, Sprint: i.key("ShiftLeft", "ShiftRight"), Jump: i.key("Space")}
	if i.key("KeyW", "ArrowUp") {
		in.Move++
	}
	if i.key("KeyS", "ArrowDown") {
		in.Move--
	}
	if i.key("KeyD", "ArrowRight") {
		in.Strafe++
	}
	if i.key("KeyA", "ArrowLeft") {
		in.Strafe--
	}
	if i.key("Space") {
		in.Vert++
	}
	if i.key("ControlLeft", "KeyC") {
		in.Vert--
	}
	in.LookX, in.LookY = i.lookX, i.lookY
	// arrow keys also turn the view when Shift is not held? keep strafe on A/D and turn with Q/R
	if i.key("KeyQ") {
		in.LookX -= 0.03
	}
	if i.key("KeyR") {
		in.LookX += 0.03
	}
	for _, t := range i.touches {
		switch t.role {
		case 1:
			dx, dy := (t.x-t.startX)/60, (t.y-t.startY)/60
			in.Strafe += math.Max(-1, math.Min(1, dx))
			in.Move -= math.Max(-1, math.Min(1, dy))
		}
	}
	in.Active = in.Move != 0 || in.Strafe != 0 || in.LookX != 0 || in.LookY != 0 || in.Zoom != 0 || in.Scroll != 0
	i.lookX, i.lookY, i.zoom, i.scroll = 0, 0, 0, 0
	return in
}

func (i *inputState) wasPressed(code string) bool {
	v := i.pressed[code]
	return v
}

func (i *inputState) endFrame() { clear(i.pressed) }

func isTyping(target js.Value) bool {
	if !target.Truthy() {
		return false
	}
	tag := target.Get("tagName").String()
	return tag == "INPUT" || tag == "TEXTAREA" || tag == "SELECT" || target.Get("isContentEditable").Bool()
}

// bindInput registers every DOM listener; the funcs live for the page lifetime.
func (a *App) bindInput() {
	win, canvas := a.win, a.canvas
	in := a.input
	on := func(target js.Value, ev string, opts any, f func(e js.Value)) {
		h := js.FuncOf(func(this js.Value, args []js.Value) any { f(args[0]); return nil })
		a.funcs = append(a.funcs, h)
		if opts == nil {
			target.Call("addEventListener", ev, h)
		} else {
			target.Call("addEventListener", ev, h, opts)
		}
	}
	gameKey := map[string]bool{"KeyW": true, "KeyA": true, "KeyS": true, "KeyD": true, "ArrowUp": true, "ArrowDown": true, "ArrowLeft": true, "ArrowRight": true, "Space": true, "ShiftLeft": true, "ShiftRight": true, "ControlLeft": true, "KeyC": true, "KeyE": true, "KeyQ": true, "KeyR": true}
	on(win, "keydown", nil, func(e js.Value) {
		if isTyping(e.Get("target")) || e.Get("ctrlKey").Bool() && e.Get("code").String() != "ControlLeft" || e.Get("metaKey").Bool() || e.Get("altKey").Bool() {
			return
		}
		code := e.Get("code").String()
		if gameKey[code] {
			e.Call("preventDefault")
		}
		if !in.keys[code] {
			in.pressed[code] = true
		}
		in.keys[code] = true
		a.userGesture()
	})
	on(win, "keyup", nil, func(e js.Value) { in.keys[e.Get("code").String()] = false })
	on(win, "blur", nil, func(e js.Value) { clear(in.keys); in.dragging = false })

	on(canvas, "mousedown", nil, func(e js.Value) {
		a.userGesture()
		in.dragging = true
		if a.cfg.CameraMode == "explore" && !in.locked {
			if rp := canvas.Get("requestPointerLock"); rp.Truthy() {
				canvas.Call("requestPointerLock")
			}
		}
	})
	on(win, "mouseup", nil, func(e js.Value) { in.dragging = false })
	on(win, "mousemove", nil, func(e js.Value) {
		if !in.locked && !in.dragging {
			return
		}
		if a.cfg.CameraMode != "explore" {
			return
		}
		in.lookX += e.Get("movementX").Float() * 0.0024
		in.lookY -= e.Get("movementY").Float() * 0.0024
	})
	on(js.Global().Get("document"), "pointerlockchange", nil, func(e js.Value) {
		in.locked = js.Global().Get("document").Get("pointerLockElement").Equal(canvas)
	})
	on(canvas, "wheel", map[string]any{"passive": false}, func(e js.Value) {
		e.Call("preventDefault")
		d := e.Get("deltaY").Float()
		if e.Get("deltaMode").Int() == 1 {
			d *= 16
		}
		switch a.cfg.CameraMode {
		case "scroll":
			in.scroll += d * 0.6
		case "explore":
			in.zoom += d
		}
		a.userGesture()
	})
	// touch: left side moves, right side looks; vertical drags scroll in scroll mode
	on(canvas, "touchstart", map[string]any{"passive": false}, func(e js.Value) {
		e.Call("preventDefault")
		a.userGesture()
		cts := e.Get("changedTouches")
		w := canvas.Get("clientWidth").Float()
		for k := 0; k < cts.Length(); k++ {
			t := cts.Index(k)
			ts := &touchState{id: t.Get("identifier").Int(), startX: t.Get("clientX").Float(), startY: t.Get("clientY").Float()}
			ts.x, ts.y = ts.startX, ts.startY
			ts.role = 2
			if ts.startX < w*0.42 && a.cfg.CameraMode == "explore" {
				ts.role = 1
			}
			in.touches[ts.id] = ts
		}
	})
	on(canvas, "touchmove", map[string]any{"passive": false}, func(e js.Value) {
		e.Call("preventDefault")
		cts := e.Get("changedTouches")
		for k := 0; k < cts.Length(); k++ {
			t := cts.Index(k)
			ts := in.touches[t.Get("identifier").Int()]
			if ts == nil {
				continue
			}
			nx, ny := t.Get("clientX").Float(), t.Get("clientY").Float()
			dx, dy := nx-ts.x, ny-ts.y
			ts.x, ts.y = nx, ny
			if ts.role == 2 {
				switch a.cfg.CameraMode {
				case "explore":
					in.lookX += dx * 0.005
					in.lookY -= dy * 0.005
				case "scroll":
					in.scroll -= dy * 2.2
				}
			}
		}
	})
	endTouch := func(e js.Value) {
		cts := e.Get("changedTouches")
		for k := 0; k < cts.Length(); k++ {
			delete(in.touches, cts.Index(k).Get("identifier").Int())
		}
	}
	on(canvas, "touchend", nil, endTouch)
	on(canvas, "touchcancel", nil, endTouch)
	on(win, "resize", nil, func(e js.Value) { a.needResize = true })
	on(win, "pagehide", nil, func(e js.Value) { a.save() })
	on(js.Global().Get("document"), "visibilitychange", nil, func(e js.Value) {
		if js.Global().Get("document").Get("hidden").Bool() {
			a.save()
		}
	})
}
