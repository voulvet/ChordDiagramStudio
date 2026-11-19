//go:build js && wasm

// Browser build: exposes the chord diagram engine to JavaScript.
// Everything runs locally in the visitor's browser; no data leaves the page.
package main

import (
	"encoding/json"
	"syscall/js"
)

var (
	wSess Session
	wRun  *Run
)

func jsonResult(v any) js.Value {
	b, _ := json.Marshal(v)
	return js.ValueOf(string(b))
}

func errResult(msg string) js.Value { return jsonResult(map[string]string{"error": msg}) }

func toBytes(b []byte) js.Value {
	a := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(a, b)
	return a
}

func fromJS(v js.Value) []byte {
	if v.IsNull() || v.IsUndefined() {
		return nil
	}
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

// chordLoad(slot, bytes|null, fileName, rsheet, psheet) -> JSON info
func load(_ js.Value, a []js.Value) any {
	if err := wSess.Load(a[0].Int(), fromJS(a[1]), a[2].String(), a[3].String(), a[4].String()); err != nil {
		return errResult(err.Error())
	}
	return jsonResult(wSess.Info())
}

// chordRemove(slot | -1 for all extra files) -> JSON info
func remove(_ js.Value, a []js.Value) any {
	if k := a[0].Int(); k < 0 {
		wSess.RemoveExtra()
	} else {
		wSess.Remove(k)
	}
	return jsonResult(wSess.Info())
}

// chordCategories(requestJSON) -> JSON {categories, describe}
func categories(_ js.Value, a []js.Value) any {
	var req GenerateReq
	json.Unmarshal([]byte(a[0].String()), &req)
	m, err := wSess.Categories(req)
	if err != nil {
		return errResult(err.Error())
	}
	return jsonResult(m)
}

// chordGenerate(requestJSON) -> JSON {diagrams}; only SVGs are rendered here.
func generate(_ js.Value, a []js.Value) any {
	var req GenerateReq
	if err := json.Unmarshal([]byte(a[0].String()), &req); err != nil {
		return errResult("Bad request: " + err.Error())
	}
	run, err := wSess.Generate(req, false)
	if err != nil {
		return errResult(err.Error())
	}
	wRun = run
	return jsonResult(map[string]any{"job": "web", "diagrams": run.Diagrams, "prefix": run.Prefix, "hasSummary": run.summary != nil,
		"hasChanges": run.Changes != nil, "changes": run.ChangeInfo, "warnings": run.Warnings, "chordWidth": run.ChordWidth})
}

// chordFile(index, kind) -> Uint8Array ("png" renders on demand)
func file(_ js.Value, a []js.Value) any {
	if wRun == nil {
		return js.Null()
	}
	switch a[1].String() {
	case "summary":
		if wRun.summary == nil {
			return js.Null()
		}
		return toBytes(wRun.summary)
	case "changes":
		if wRun.Changes == nil {
			return js.Null()
		}
		return toBytes(wRun.Changes)
	}
	i := a[0].Int()
	if i < 0 || i >= len(wRun.Diagrams) {
		return js.Null()
	}
	d := wRun.Diagrams[i]
	switch a[1].String() {
	case "png":
		if err := d.EnsurePNG(); err != nil || d.png == nil {
			return js.Null()
		}
		return toBytes(d.png)
	case "svg":
		if d.svg == nil {
			return js.Null()
		}
		return toBytes(d.svg)
	case "xlsx":
		return toBytes(d.xlsx)
	}
	return js.Null()
}

// chordZip() -> Uint8Array of everything (PNGs must have been rendered first)
func zipAll(_ js.Value, _ []js.Value) any {
	if wRun == nil {
		return js.Null()
	}
	for _, d := range wRun.Diagrams {
		d.EnsurePNG()
	}
	return toBytes(ZipRun(wRun))
}

func main() {
	g := js.Global()
	g.Set("chordLoad", js.FuncOf(load))
	g.Set("chordRemove", js.FuncOf(remove))
	g.Set("chordCategories", js.FuncOf(categories))
	g.Set("chordGenerate", js.FuncOf(generate))
	g.Set("chordFile", js.FuncOf(file))
	g.Set("chordZip", js.FuncOf(zipAll))
	g.Call("chordReady")
	select {}
}
