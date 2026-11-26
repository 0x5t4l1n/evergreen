//go:build js && wasm

package app

import (
	"fmt"
	"syscall/js"
)

const storageKey = "evergreen.state.v1"

func storageGet() string {
	defer func() { _ = recover() }()
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return ""
	}
	v := ls.Call("getItem", storageKey)
	if v.IsNull() || v.IsUndefined() {
		return ""
	}
	return v.String()
}

func storageSet(s string) {
	defer func() { _ = recover() }()
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", storageKey, s)
	}
}

func ui() js.Value { return js.Global().Get("evergreenUI") }

func uiCall(method string, args ...any) {
	defer func() { _ = recover() }()
	u := ui()
	if u.Truthy() && u.Get(method).Truthy() {
		u.Call(method, args...)
	}
}

func prefersReducedMotion() bool {
	defer func() { _ = recover() }()
	mq := js.Global().Call("matchMedia", "(prefers-reduced-motion: reduce)")
	return mq.Truthy() && mq.Get("matches").Bool()
}

func nowLocal() (y, mo, d, h, mi, s int) {
	t := js.Global().Get("Date").New()
	return t.Call("getFullYear").Int(), t.Call("getMonth").Int() + 1, t.Call("getDate").Int(), t.Call("getHours").Int(), t.Call("getMinutes").Int(), t.Call("getSeconds").Int()
}

func download(dataURL, name string) {
	doc := js.Global().Get("document")
	a := doc.Call("createElement", "a")
	a.Set("href", dataURL)
	a.Set("download", name)
	a.Get("style").Set("display", "none")
	doc.Get("body").Call("appendChild", a)
	a.Call("click")
	doc.Get("body").Call("removeChild", a)
}

// fetchText GETs url and calls done with the body or an error.
func fetchText(url string, done func(body string, err error)) {
	var onOK, onBody, onErr js.Func
	release := func() { onOK.Release(); onBody.Release(); onErr.Release() }
	onErr = js.FuncOf(func(this js.Value, a []js.Value) any {
		release()
		msg := "network error"
		if len(a) > 0 && a[0].Truthy() {
			msg = a[0].Call("toString").String()
		}
		done("", fmt.Errorf("%s", msg))
		return nil
	})
	onBody = js.FuncOf(func(this js.Value, a []js.Value) any {
		release()
		done(a[0].String(), nil)
		return nil
	})
	onOK = js.FuncOf(func(this js.Value, a []js.Value) any {
		resp := a[0]
		if !resp.Get("ok").Bool() {
			release()
			done("", fmt.Errorf("HTTP %d", resp.Get("status").Int()))
			return nil
		}
		resp.Call("text").Call("then", onBody).Call("catch", onErr)
		return nil
	})
	js.Global().Call("fetch", url, map[string]any{"cache": "no-cache"}).Call("then", onOK).Call("catch", onErr)
}
