//go:build js && wasm

// Browser build: exposes the chord diagram engine to JavaScript.
// Everything runs locally in the visitor's browser; no data leaves the page.
package main

import (
	"bytes"
	"encoding/json"
	"syscall/js"
)

var (
	wFileName string
	wData     []byte
	wDS       *Dataset
	wDiags    []*Diagram
	wSummary  []byte
	wPrefix   string
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

// chordLoad(bytes|null, fileName, rsheet, psheet) -> JSON info
func load(_ js.Value, a []js.Value) any {
	rs, ps := a[2].String(), a[3].String()
	if !a[0].IsNull() && !a[0].IsUndefined() {
		b := make([]byte, a[0].Get("length").Int())
		js.CopyBytesToGo(b, a[0])
		wData, wFileName = b, a[1].String()
		rs, ps = "", ""
	}
	if wData == nil {
		return errResult("Choose an Excel file first.")
	}
	ds, err := LoadDataset(bytes.NewReader(wData), rs, ps)
	if err != nil {
		return errResult(err.Error())
	}
	wDS = ds
	return jsonResult(map[string]any{
		"file": wFileName, "sheets": ds.Sheets, "rsheet": ds.RSheet, "psheet": ds.PSheet,
		"categories": ds.Categories, "symmetric": ds.Symmetric,
		"looksVariables": ds.LooksVariables, "hasVarSheet": ds.HasVarSheet,
	})
}

// chordCategories(optionsJSON) -> JSON {categories}
func categories(_ js.Value, a []js.Value) any {
	if wDS == nil {
		return errResult("No file loaded.")
	}
	var o Options
	json.Unmarshal([]byte(a[0].String()), &o)
	return jsonResult(map[string]any{"categories": wDS.MergedCategories(&o), "describe": wDS.Describe(&o)})
}

// chordGenerate(requestJSON) -> JSON {diagrams}; only SVGs are rendered here.
func generate(_ js.Value, a []js.Value) any {
	if wDS == nil {
		return errResult("No file loaded.")
	}
	var req GenerateReq
	if err := json.Unmarshal([]byte(a[0].String()), &req); err != nil {
		return errResult("Bad request: " + err.Error())
	}
	d, err := Generate(wDS, req, wFileName, false)
	if err != nil {
		return errResult(err.Error())
	}
	wDiags = d
	wPrefix = ResolvePrefix(req, wFileName)
	wSummary = nil
	if len(d) > 1 {
		wSummary = SummaryXLSX(d, req.Options.Cuts)
	}
	return jsonResult(map[string]any{"job": "web", "diagrams": d, "prefix": wPrefix, "hasSummary": len(d) > 1})
}

// chordFile(index, kind) -> Uint8Array ("png" renders on demand)
func file(_ js.Value, a []js.Value) any {
	if a[1].String() == "summary" {
		if wSummary == nil {
			return js.Null()
		}
		return toBytes(wSummary)
	}
	i := a[0].Int()
	if i < 0 || i >= len(wDiags) {
		return js.Null()
	}
	d := wDiags[i]
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
	for _, d := range wDiags {
		d.EnsurePNG()
	}
	return toBytes(ZipDiagrams(wDiags, wSummary, wPrefix+"_summary.xlsx"))
}

func main() {
	g := js.Global()
	g.Set("chordLoad", js.FuncOf(load))
	g.Set("chordCategories", js.FuncOf(categories))
	g.Set("chordGenerate", js.FuncOf(generate))
	g.Set("chordFile", js.FuncOf(file))
	g.Set("chordZip", js.FuncOf(zipAll))
	g.Call("chordReady")
	select {}
}
