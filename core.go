package main

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"github.com/xuri/excelize/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// ---------------- palette ----------------

var pastel = []string{"#FFD59E", "#B5EAD7", "#C7B8EA", "#FFB7C5", "#A2D2FF", "#FFF1A8", "#D4F0B0", "#F9C6E0",
	"#FFC4A8", "#A8E6E2", "#E2C6F5", "#C9D9A6", "#B8C7F0", "#F5D0A9", "#D6E8F5", "#EEC9C9"}
var dark = []string{"#A0661A", "#2E8A66", "#6A52A8", "#C2435E", "#2F78B8", "#9A8414", "#5E8A2E", "#A8467A",
	"#B3542A", "#1F8580", "#8A4FAD", "#6B7A2E", "#3F57A6", "#9C6524", "#4A7A99", "#9E4A4A"}

// soft blue - white - soft red
var cmapStops = []struct {
	v   float64
	hex string
}{{-1, "#4F7FCB"}, {-0.5, "#A9C3EA"}, {0, "#FFFFFF"}, {0.5, "#F2AAA4"}, {1, "#D65F57"}}

// ---------------- data loading ----------------

type Dataset struct {
	Sheets         []string
	RSheet, PSheet string
	rRows, pRows   [][]string
	rowCat, colCat []string // header labels (category names or variable names)
	Symmetric      bool
	Categories     []string // header labels in order of first appearance
	Labels         []string // unique header labels in order (variables in variable mode)
	Vars           map[string]VarInfo
	HasVarSheet    bool
	LooksVariables bool // headers look like variable names (mostly unique)
}

// VarInfo comes from an optional "Variables" sheet (Variable, Category, Region/Group).
type VarInfo struct {
	Category string
	Group    string
}

func parseNum(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.TrimLeft(s, "<>≤= ")
	s = strings.ReplaceAll(s, ",", ".")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

func cell(rows [][]string, i, j int) string {
	if i < len(rows) && j < len(rows[i]) {
		return rows[i][j]
	}
	return ""
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, " ", " "))
}

func LoadDataset(r io.Reader, rSheet, pSheet string) (*Dataset, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("could not open the Excel file: %v", err)
	}
	defer f.Close()
	ds := &Dataset{Sheets: f.GetSheetList()}
	if len(ds.Sheets) < 2 && (rSheet == "" || pSheet == "") {
		return nil, fmt.Errorf("the file needs two sheets (r values and p-values); found %d", len(ds.Sheets))
	}
	if rSheet == "" {
		rSheet = ds.Sheets[0]
	}
	if pSheet == "" {
		pSheet = ds.Sheets[1]
	}
	ds.RSheet, ds.PSheet = rSheet, pSheet
	if ds.rRows, err = f.GetRows(rSheet, excelize.Options{RawCellValue: true}); err != nil {
		return nil, fmt.Errorf("reading sheet %q: %v", rSheet, err)
	}
	if ds.pRows, err = f.GetRows(pSheet, excelize.Options{RawCellValue: true}); err != nil {
		return nil, fmt.Errorf("reading sheet %q: %v", pSheet, err)
	}
	if len(ds.rRows) < 2 {
		return nil, fmt.Errorf("sheet %q has no data", rSheet)
	}
	for j := 1; j < len(ds.rRows[0]); j++ {
		ds.colCat = append(ds.colCat, clean(cell(ds.rRows, 0, j)))
	}
	for i := 1; i < len(ds.rRows); i++ {
		ds.rowCat = append(ds.rowCat, clean(cell(ds.rRows, i, 0)))
	}
	ds.Symmetric = len(ds.colCat) == len(ds.rowCat)
	if ds.Symmetric {
		for i := range ds.colCat {
			if ds.colCat[i] != ds.rowCat[i] {
				ds.Symmetric = false
				break
			}
		}
	}
	seen := map[string]bool{}
	for _, c := range append(append([]string{}, ds.colCat...), ds.rowCat...) {
		if c != "" && !seen[c] {
			seen[c] = true
			ds.Categories = append(ds.Categories, c)
		}
	}
	ds.Labels = ds.Categories
	ds.LooksVariables = len(ds.colCat) > 2 && float64(len(ds.Labels)) >= 0.9*float64(len(ds.colCat))
	ds.Vars = map[string]VarInfo{}
	for _, sh := range ds.Sheets {
		if !strings.EqualFold(strings.TrimSpace(sh), "variables") || sh == rSheet || sh == pSheet {
			continue
		}
		rows, err := f.GetRows(sh)
		if err != nil || len(rows) < 2 {
			continue
		}
		vc, cc, gc := -1, -1, -1
		for j, h := range rows[0] {
			switch strings.ToLower(clean(h)) {
			case "variable", "variables", "variable name", "name":
				vc = j
			case "category", "variable category":
				cc = j
			case "region", "group", "anatomical region", "location":
				gc = j
			}
		}
		if vc < 0 || cc < 0 {
			continue
		}
		for _, r := range rows[1:] {
			v := clean(cell([][]string{r}, 0, vc))
			if v == "" {
				continue
			}
			vi := VarInfo{Category: clean(cell([][]string{r}, 0, cc))}
			if gc >= 0 {
				vi.Group = clean(cell([][]string{r}, 0, gc))
			}
			ds.Vars[v] = vi
		}
		ds.HasVarSheet = len(ds.Vars) > 0
	}
	return ds, nil
}

// ---------------- filtering ----------------

type MergeRule struct {
	Keyword string `json:"keyword"`
	Name    string `json:"name"`
}

type Options struct {
	Alpha   float64     `json:"alpha"`
	Exclude []string    `json:"exclude"` // category names (after merging) to leave out
	Merge   []MergeRule `json:"merge"`   // categories containing keyword become one category
	Apart   []string    `json:"apart"`   // keywords: categories containing the same keyword are not compared
	Order   []string    `json:"order"`   // ring order (after merging)
	Title   string      `json:"title"`
	Focus   string      `json:"-"` // only links touching this category (series mode)
	Cuts    Cuts        `json:"cuts"`

	// Variable-name mode
	Header      string `json:"header"`      // "category" (default): headers are categories; "variable": headers are variable names
	Sep         string `json:"sep"`         // category = text before this separator (default "_")
	UseSheet    bool   `json:"useSheet"`    // take category/group from the "Variables" sheet
	Ungrouped   string `json:"ungrouped"`   // variables without group: "include" (default) or "exclude"
	WithinGroup bool   `json:"withinGroup"` // only correlations between variables of the same group
	Group       string `json:"-"`           // one-diagram-per-group: the current group
}

type Cuts struct {
	VeryStrong float64 `json:"veryStrong"`
	Strong     float64 `json:"strong"`
	Moderate   float64 `json:"moderate"`
}

type Link struct {
	C1, C2 string
	R, P   float64
	V1, V2 string // variable labels (same order as C1, C2)
}

type Removed struct {
	Read       int `json:"read"`
	Within     int `json:"within"`
	Excluded   int `json:"excluded"`
	Apart      int `json:"apart"`
	NotSig     int `json:"notSignificant"`
	NoP        int `json:"missingP"`
	OutOfFocus int `json:"outOfFocus"`
	OtherGroup int `json:"otherGroup"`
}

func (o *Options) mapCat(c string) string {
	lc := strings.ToLower(c)
	for _, m := range o.Merge {
		k := strings.ToLower(strings.TrimSpace(m.Keyword))
		if k != "" && strings.Contains(lc, k) {
			if strings.TrimSpace(m.Name) != "" {
				return clean(m.Name)
			}
			return clean(m.Keyword)
		}
	}
	return c
}

// MergedCategories returns the category list after merge rules (appearance order).
func (ds *Dataset) MergedCategories(o *Options) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range ds.Labels {
		c, _ := ds.varInfo(l, o)
		if c == "" {
			continue
		}
		m := o.mapCat(c)
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func sameApartGroup(a, b string, apart []string) bool {
	la, lb := strings.ToLower(a), strings.ToLower(b)
	for _, k := range apart {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" && strings.Contains(la, k) && strings.Contains(lb, k) {
			return true
		}
	}
	return false
}

// Compute applies all filters and returns the links plus the full ring order.
func (ds *Dataset) Compute(o *Options) ([]Link, []string, Removed) {
	var rm Removed
	excl := map[string]bool{}
	for _, e := range o.Exclude {
		excl[clean(e)] = true
	}
	type raw struct {
		a, b   string
		r, p   float64
		va, vb string
	}
	var raws []raw
	for i := 0; i < len(ds.rowCat); i++ {
		for j := 0; j < len(ds.colCat); j++ {
			if ds.Symmetric && j <= i {
				continue
			}
			rs := cell(ds.rRows, i+1, j+1)
			ps := cell(ds.pRows, i+1, j+1)
			if ds.Symmetric {
				if strings.TrimSpace(rs) == "" {
					rs = cell(ds.rRows, j+1, i+1)
				}
				if strings.TrimSpace(ps) == "" {
					ps = cell(ds.pRows, j+1, i+1)
				}
			}
			r, ok := parseNum(rs)
			if !ok {
				continue
			}
			va, vb := ds.rowCat[i], ds.colCat[j]
			a0, ga := ds.varInfo(va, o)
			b0, gb := ds.varInfo(vb, o)
			if a0 == "" || b0 == "" {
				continue
			}
			rm.Read++
			a, b := o.mapCat(a0), o.mapCat(b0)
			if excl[a] || excl[b] {
				rm.Excluded++
				continue
			}
			if a == b {
				rm.Within++
				continue
			}
			if sameApartGroup(a, b, o.Apart) {
				rm.Apart++
				continue
			}
			if o.Focus != "" && a != o.Focus && b != o.Focus {
				rm.OutOfFocus++
				continue
			}
			if !o.groupOK(ga, gb) {
				rm.OtherGroup++
				continue
			}
			p, ok := parseNum(ps)
			if !ok {
				rm.NoP++
				continue
			}
			if p > o.Alpha {
				rm.NotSig++
				continue
			}
			raws = append(raws, raw{a, b, r, p, va, vb})
		}
	}
	// ring order: user order, then remaining categories in appearance order
	var order []string
	seen := map[string]bool{}
	add := func(c string) {
		if c != "" && !seen[c] {
			seen[c] = true
			order = append(order, c)
		}
	}
	for _, c := range o.Order {
		add(clean(c))
	}
	for _, c := range ds.MergedCategories(o) {
		add(c)
	}
	idx := map[string]int{}
	for i, c := range order {
		idx[c] = i
	}
	links := make([]Link, 0, len(raws))
	for _, x := range raws {
		a, b, va, vb := x.a, x.b, x.va, x.vb
		if idx[a] > idx[b] {
			a, b, va, vb = b, a, vb, va
		}
		links = append(links, Link{C1: a, C2: b, R: x.r, P: x.p, V1: va, V2: vb})
	}
	return links, order, rm
}

// ---------------- counting ----------------

type Band struct {
	Label string `json:"label"`
	Total int    `json:"total"`
	Pos   int    `json:"pos"`
	Neg   int    `json:"neg"`
}

type PairCount struct {
	A     string `json:"a"`
	B     string `json:"b"`
	Total int    `json:"total"`
	Pos   int    `json:"pos"`
	Neg   int    `json:"neg"`
	Bands []Band `json:"bands"`
}

type Counts struct {
	Total   int         `json:"total"`
	Pos     int         `json:"pos"`
	Neg     int         `json:"neg"`
	Bands   []Band      `json:"bands"`
	Pairs   []PairCount `json:"pairs"`
	PerCat  []PairCount `json:"perCategory"` // A = category, B empty
	Removed Removed     `json:"removed"`
}

func bandLabels(c Cuts) []string {
	f := func(x float64) string { return strconv.FormatFloat(x, 'f', 2, 64) }
	return []string{
		fmt.Sprintf("Very strong (|r| ≥ %s)", f(c.VeryStrong)),
		fmt.Sprintf("Strong (%s – <%s)", f(c.Strong), f(c.VeryStrong)),
		fmt.Sprintf("Moderate (%s – <%s)", f(c.Moderate), f(c.Strong)),
		fmt.Sprintf("Weak (< %s)", f(c.Moderate)),
	}
}

func bandOf(r float64, c Cuts) int {
	a := math.Abs(r)
	switch {
	case a >= c.VeryStrong:
		return 0
	case a >= c.Strong:
		return 1
	case a >= c.Moderate:
		return 2
	}
	return 3
}

func newBands(c Cuts) []Band {
	var b []Band
	for _, l := range bandLabels(c) {
		b = append(b, Band{Label: l})
	}
	return b
}

func addTo(pc *PairCount, r float64, c Cuts) {
	pc.Total++
	k := bandOf(r, c)
	pc.Bands[k].Total++
	if r > 0 {
		pc.Pos++
		pc.Bands[k].Pos++
	} else if r < 0 {
		pc.Neg++
		pc.Bands[k].Neg++
	}
}

func CountLinks(links []Link, order []string, c Cuts, rm Removed) Counts {
	idx := map[string]int{}
	for i, x := range order {
		idx[x] = i
	}
	all := PairCount{Bands: newBands(c)}
	pairs := map[[2]string]*PairCount{}
	cats := map[string]*PairCount{}
	for _, l := range links {
		addTo(&all, l.R, c)
		k := [2]string{l.C1, l.C2}
		if pairs[k] == nil {
			pairs[k] = &PairCount{A: l.C1, B: l.C2, Bands: newBands(c)}
		}
		addTo(pairs[k], l.R, c)
		for _, x := range []string{l.C1, l.C2} {
			if cats[x] == nil {
				cats[x] = &PairCount{A: x, Bands: newBands(c)}
			}
			addTo(cats[x], l.R, c)
		}
	}
	out := Counts{Total: all.Total, Pos: all.Pos, Neg: all.Neg, Bands: all.Bands, Removed: rm}
	for _, p := range pairs {
		out.Pairs = append(out.Pairs, *p)
	}
	sort.Slice(out.Pairs, func(i, j int) bool {
		a, b := out.Pairs[i], out.Pairs[j]
		if idx[a.A] != idx[b.A] {
			return idx[a.A] < idx[b.A]
		}
		return idx[a.B] < idx[b.B]
	})
	for _, x := range order {
		if cats[x] != nil {
			out.PerCat = append(out.PerCat, *cats[x])
		}
	}
	return out
}

func CountsXLSX(cn Counts, links []Link, cuts Cuts) ([]byte, error) {
	x := excelize.NewFile()
	defer x.Close()
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})

	sh := "Summary"
	x.SetSheetName("Sheet1", sh)
	x.SetSheetRow(sh, "A1", &[]any{"Strength", "Total", "Positive", "Negative"})
	for i, b := range cn.Bands {
		x.SetSheetRow(sh, fmt.Sprintf("A%d", i+2), &[]any{b.Label, b.Total, b.Pos, b.Neg})
	}
	n := len(cn.Bands) + 2
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n), &[]any{"All", cn.Total, cn.Pos, cn.Neg})
	x.SetRowStyle(sh, 1, 1, bold)
	x.SetRowStyle(sh, n, n, bold)
	rm := cn.Removed
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+2), &[]any{"Correlations read", rm.Read})
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+3), &[]any{"Removed: within category", rm.Within})
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+4), &[]any{"Removed: excluded categories", rm.Excluded})
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+5), &[]any{"Removed: kept-apart categories", rm.Apart})
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+6), &[]any{"Removed: not in this diagram (series)", rm.OutOfFocus + rm.OtherGroup})
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+7), &[]any{"Removed: not significant", rm.NotSig})
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n+8), &[]any{"Removed: missing p-value", rm.NoP})
	x.SetColWidth(sh, "A", "A", 40)

	hdr := []any{"Category 1", "Category 2", "Total", "Positive", "Negative"}
	short := []string{"Very strong", "Strong", "Moderate", "Weak"}
	for _, s := range short {
		hdr = append(hdr, s+" +", s+" −")
	}
	writePairs := func(name string, rows []PairCount, single bool) {
		x.NewSheet(name)
		h := hdr
		if single {
			h = append([]any{"Category"}, hdr[2:]...)
		}
		x.SetSheetRow(name, "A1", &h)
		for i, p := range rows {
			row := []any{p.A, p.B, p.Total, p.Pos, p.Neg}
			if single {
				row = []any{p.A, p.Total, p.Pos, p.Neg}
			}
			for _, b := range p.Bands {
				row = append(row, b.Pos, b.Neg)
			}
			x.SetSheetRow(name, fmt.Sprintf("A%d", i+2), &row)
		}
		x.SetRowStyle(name, 1, 1, bold)
		x.SetColWidth(name, "A", "B", 50)
	}
	writePairs("Pairwise counts", cn.Pairs, false)
	writePairs("Per category", cn.PerCat, true)
	writeLinks(x, "All correlations", "", links, cuts, bold)
	var buf bytes.Buffer
	if err := x.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------- geometry ----------------

type pt struct{ X, Y float64 }

type seg struct {
	op  byte
	pts []pt
}

func polar(a, r float64) pt { return pt{r * math.Cos(a), r * math.Sin(a)} }

func angDist(x, y float64) float64 {
	d := math.Mod(math.Abs(x-y), 2*math.Pi)
	if d > math.Pi {
		d = 2*math.Pi - d
	}
	return d
}

func kk(x, y float64) float64 { return math.Max(0, 0.9-math.Pow(angDist(x, y)/math.Pi, 0.6)) }

func ribbon(a1, a2, b1, b2 float64) []seg {
	return []seg{
		{'M', []pt{polar(a1, 1)}},
		{'L', []pt{polar(a2, 1)}},
		{'C', []pt{polar(a2, kk(a2, b1)), polar(b1, kk(a2, b1)), polar(b1, 1)}},
		{'L', []pt{polar(b2, 1)}},
		{'C', []pt{polar(b2, kk(b2, a1)), polar(a1, kk(b2, a1)), polar(a1, 1)}},
		{'Z', nil},
	}
}

func annulus(lo, hi, r0, r1 float64) []seg {
	n := 120
	s := []seg{}
	for i := 0; i <= n; i++ {
		a := hi - (hi-lo)*float64(i)/float64(n)
		op := byte('L')
		if i == 0 {
			op = 'M'
		}
		s = append(s, seg{op, []pt{polar(a, r1)}})
	}
	for i := 0; i <= n; i++ {
		a := lo + (hi-lo)*float64(i)/float64(n)
		s = append(s, seg{'L', []pt{polar(a, r0)}})
	}
	return append(s, seg{'Z', nil})
}

func hexRGB(h string) (float64, float64, float64) {
	h = strings.TrimPrefix(h, "#")
	v, _ := strconv.ParseUint(h, 16, 32)
	return float64(v>>16&255) / 255, float64(v>>8&255) / 255, float64(v&255) / 255
}

func cmap(r float64) (float64, float64, float64) {
	r = math.Max(-1, math.Min(1, r))
	for i := 0; i < len(cmapStops)-1; i++ {
		a, b := cmapStops[i], cmapStops[i+1]
		if r <= b.v {
			t := (r - a.v) / (b.v - a.v)
			r1, g1, b1 := hexRGB(a.hex)
			r2, g2, b2 := hexRGB(b.hex)
			return r1 + t*(r2-r1), g1 + t*(g2-g1), b1 + t*(b2-b1)
		}
	}
	return hexRGB(cmapStops[len(cmapStops)-1].hex)
}

func toHex(r, g, b float64) string {
	return fmt.Sprintf("#%02X%02X%02X", int(math.Round(r*255)), int(math.Round(g*255)), int(math.Round(b*255)))
}

// ---------------- canvas (PNG + SVG) ----------------

const (
	W     = 4800.0
	H     = 4150.0
	scale = 1090.0
	cx    = W / 2
	cy    = 1950.0
	ptpx  = 300.0 / 72.0
)

func px(p pt) pt { return pt{cx + p.X*scale, cy - p.Y*scale} }

// drawer is the subset of gg.Context we use; noDraw skips rasterising (SVG only).
type drawer interface {
	SetRGB(r, g, b float64)
	SetRGBA(r, g, b, a float64)
	Clear()
	NewSubPath()
	MoveTo(x, y float64)
	LineTo(x, y float64)
	CubicTo(x1, y1, x2, y2, x3, y3 float64)
	ClosePath()
	Fill()
	Stroke()
	SetLineWidth(w float64)
	SetFontFace(f font.Face)
	DrawStringAnchored(s string, x, y, ax, ay float64)
	DrawRectangle(x, y, w, h float64)
}

type noDraw struct{}

func (noDraw) SetRGB(r, g, b float64)                            {}
func (noDraw) SetRGBA(r, g, b, a float64)                        {}
func (noDraw) Clear()                                            {}
func (noDraw) NewSubPath()                                       {}
func (noDraw) MoveTo(x, y float64)                               {}
func (noDraw) LineTo(x, y float64)                               {}
func (noDraw) CubicTo(x1, y1, x2, y2, x3, y3 float64)            {}
func (noDraw) ClosePath()                                        {}
func (noDraw) Fill()                                             {}
func (noDraw) Stroke()                                           {}
func (noDraw) SetLineWidth(w float64)                            {}
func (noDraw) SetFontFace(f font.Face)                           {}
func (noDraw) DrawStringAnchored(s string, x, y, ax, ay float64) {}
func (noDraw) DrawRectangle(x, y, w, h float64)                  {}

type canvas struct {
	dc  drawer
	gc  *gg.Context
	svg strings.Builder
}

func newCanvas(raster bool) *canvas {
	c := &canvas{dc: noDraw{}}
	if raster {
		c.gc = gg.NewContext(int(W), int(H))
		c.dc = c.gc
	}
	c.dc.SetRGB(1, 1, 1)
	c.dc.Clear()
	fmt.Fprintf(&c.svg, `<svg xmlns="http://www.w3.org/2000/svg" width="16in" height="%.3fin" viewBox="0 0 %.0f %.0f">`+"\n", H/300, W, H)
	fmt.Fprintf(&c.svg, `<rect width="100%%" height="100%%" fill="#FFFFFF"/>`+"\n")
	return c
}

func (c *canvas) fill(s []seg, hex string, alpha float64) {
	var sb strings.Builder
	c.dc.NewSubPath()
	for _, g := range s {
		sb.WriteByte(g.op)
		var q []pt
		for _, p := range g.pts {
			q = append(q, px(p))
			fmt.Fprintf(&sb, "%.2f,%.2f ", q[len(q)-1].X, q[len(q)-1].Y)
		}
		switch g.op {
		case 'M':
			c.dc.MoveTo(q[0].X, q[0].Y)
		case 'L':
			c.dc.LineTo(q[0].X, q[0].Y)
		case 'C':
			c.dc.CubicTo(q[0].X, q[0].Y, q[1].X, q[1].Y, q[2].X, q[2].Y)
		case 'Z':
			c.dc.ClosePath()
		}
	}
	r, g, b := hexRGB(hex)
	c.dc.SetRGBA(r, g, b, alpha)
	c.dc.Fill()
	fmt.Fprintf(&c.svg, `<path d="%s" fill="%s" fill-opacity="%.2f" stroke="none"/>`+"\n", sb.String(), hex, alpha)
}

func (c *canvas) polyline(p []pt, hex string, width float64) {
	r, g, b := hexRGB(hex)
	c.dc.SetRGB(r, g, b)
	c.dc.SetLineWidth(width)
	var sb strings.Builder
	for i, q := range p {
		if i == 0 {
			c.dc.MoveTo(q.X, q.Y)
		} else {
			c.dc.LineTo(q.X, q.Y)
		}
		fmt.Fprintf(&sb, "%.2f,%.2f ", q.X, q.Y)
	}
	c.dc.Stroke()
	fmt.Fprintf(&c.svg, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%.1f"/>`+"\n", sb.String(), hex, width)
}

var regularTTF, boldTTF *truetype.Font

func init() {
	regularTTF, _ = truetype.Parse(goregular.TTF)
	boldTTF, _ = truetype.Parse(gobold.TTF)
}

func (c *canvas) text(x, y float64, lines []string, sizePt float64, hex, anchor string, bold bool) {
	f, weight := regularTTF, "normal"
	if bold {
		f, weight = boldTTF, "bold"
	}
	size := sizePt * ptpx
	c.dc.SetFontFace(truetype.NewFace(f, &truetype.Options{Size: size}))
	r, g, b := hexRGB(hex)
	c.dc.SetRGB(r, g, b)
	lh := size * 1.25
	top := y - lh*float64(len(lines)-1)/2
	ax := map[string]float64{"start": 0, "middle": 0.5, "end": 1}[anchor]
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	for i, l := range lines {
		ly := top + lh*float64(i)
		c.dc.DrawStringAnchored(l, x, ly, ax, 0.35)
		fmt.Fprintf(&c.svg, `<text x="%.1f" y="%.1f" font-family="Helvetica, Arial, sans-serif" font-size="%.1f" font-weight="%s" fill="%s" text-anchor="%s" dominant-baseline="central">%s</text>`+"\n",
			x, ly, size, weight, hex, anchor, esc.Replace(l))
	}
}

func wrap(s string, width int) []string {
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		if cur == "" {
			cur = w
		} else if len(cur)+1+len(w) <= width {
			cur += " " + w
		} else {
			out = append(out, cur)
			cur = w
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// colorOrder gives each category a fixed colour index (stable across a series).
// Render draws the diagram. With wantPNG=false only the SVG is produced (much faster).
func Render(links []Link, order []string, colorOrder []string, title string, wantPNG bool) ([]byte, []byte, error) {
	n := len(order)
	idx := map[string]int{}
	for i, c := range order {
		idx[c] = i
	}
	cidx := map[string]int{}
	for i, c := range colorOrder {
		cidx[c] = i
	}
	type end struct {
		partner string
		r       float64
		link    int
		side    int
	}
	ends := map[string][]end{}
	for i, l := range links {
		ends[l.C1] = append(ends[l.C1], end{l.C2, l.R, i, 0})
		ends[l.C2] = append(ends[l.C2], end{l.C1, l.R, i, 1})
	}
	var present []string
	for _, c := range order {
		if len(ends[c]) > 0 {
			present = append(present, c)
		}
	}
	if len(present) == 0 {
		return nil, nil, fmt.Errorf("no correlations to draw")
	}
	gap := 7 * math.Pi / 180
	segarc := (2*math.Pi - gap*float64(len(present))) / float64(len(present))
	type span struct{ a1, a2 float64 }
	pos := map[[2]int]span{}
	segs := map[string]span{}
	th := math.Pi / 2
	for _, c := range present {
		ci := idx[c]
		e := ends[c]
		sort.SliceStable(e, func(i, j int) bool {
			ki := -((idx[e[i].partner] - ci + n) % n)
			kj := -((idx[e[j].partner] - ci + n) % n)
			if ki != kj {
				return ki < kj
			}
			ri, rj := e[i].r, e[j].r
			if e[i].side == 1 {
				ri = -ri
			}
			if e[j].side == 1 {
				rj = -rj
			}
			return ri < rj
		})
		start := th
		unit := segarc / float64(len(e))
		for _, x := range e {
			pos[[2]int{x.link, x.side}] = span{th, th - unit}
			th -= unit
		}
		segs[c] = span{th, start}
		th -= gap
	}

	c := newCanvas(wantPNG)
	ids := make([]int, len(links))
	for i := range ids {
		ids[i] = i
	}
	sort.SliceStable(ids, func(i, j int) bool {
		a, b := links[ids[i]], links[ids[j]]
		if idx[a.C1] != idx[b.C1] {
			return idx[a.C1] < idx[b.C1]
		}
		if idx[a.C2] != idx[b.C2] {
			return idx[a.C2] < idx[b.C2]
		}
		return a.R < b.R
	})
	const f = 0.275
	for _, i := range ids {
		A, B := pos[[2]int{i, 0}], pos[[2]int{i, 1}]
		ma, wa := (A.a1+A.a2)/2, A.a1-A.a2
		mb, wb := (B.a1+B.a2)/2, B.a1-B.a2
		c.fill(ribbon(ma+f*wa, ma-f*wa, mb+f*wb, mb-f*wb), toHex(cmap(links[i].R)), 0.95)
	}
	for _, cat := range present {
		s := segs[cat]
		c.fill(annulus(s.a1, s.a2, 1.02, 1.09), pastel[cidx[cat]%len(pastel)], 1)
	}
	type lab struct {
		cat    string
		mid, y float64
		side   float64
	}
	var labs []*lab
	for _, cat := range present {
		s := segs[cat]
		mid := (s.a1 + s.a2) / 2
		p := polar(mid, 1.12)
		side := 1.0
		if p.X < 0 {
			side = -1
		}
		labs = append(labs, &lab{cat, mid, p.Y, side})
	}
	for _, side := range []float64{1, -1} {
		var L []*lab
		for _, l := range labs {
			if l.side == side {
				L = append(L, l)
			}
		}
		sort.Slice(L, func(i, j int) bool { return L[i].y > L[j].y })
		for j := 1; j < len(L); j++ {
			if L[j-1].y-L[j].y < 0.36 {
				L[j].y = L[j-1].y - 0.36
			}
		}
	}
	for _, l := range labs {
		a0 := polar(l.mid, 1.10)
		lx := l.side * math.Max(math.Abs(a0.X)+0.08, 1.25)
		dk := dark[cidx[l.cat]%len(dark)]
		c.polyline([]pt{px(a0), px(pt{a0.X + l.side*0.04, l.y}), px(pt{lx, l.y})}, dk, 4)
		anchor := "start"
		if l.side < 0 {
			anchor = "end"
		}
		tp := px(pt{lx + l.side*0.03, l.y})
		c.text(tp.X, tp.Y, wrap(l.cat, 30), 12, dk, anchor, true)
	}
	npos, nneg := 0, 0
	for _, l := range links {
		if l.R > 0 {
			npos++
		} else if l.R < 0 {
			nneg++
		}
	}
	c.text(W/2, 170, []string{title, fmt.Sprintf("%d correlations (%d positive, %d negative)", len(links), npos, nneg)}, 15, "#000000", "middle", false)

	bw, bh := 1900.0, 75.0
	bx, by := (W-bw)/2, 3740.0
	steps := 400
	for s := 0; s < steps; s++ {
		v := -1 + 2*(float64(s)+0.5)/float64(steps)
		x0 := bx + bw*float64(s)/float64(steps)
		r, g, b := cmap(v)
		c.dc.SetRGB(r, g, b)
		c.dc.DrawRectangle(x0, by, bw/float64(steps)+1, bh)
		c.dc.Fill()
	}
	c.svg.WriteString(`<defs><linearGradient id="cb" x1="0" x2="1" y1="0" y2="0">`)
	for _, st := range cmapStops {
		fmt.Fprintf(&c.svg, `<stop offset="%.3f" stop-color="%s"/>`, (st.v+1)/2, st.hex)
	}
	c.svg.WriteString("</linearGradient></defs>\n")
	fmt.Fprintf(&c.svg, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="url(#cb)"/>`+"\n", bx, by, bw, bh)
	c.polyline([]pt{{bx, by}, {bx + bw, by}, {bx + bw, by + bh}, {bx, by + bh}, {bx, by}}, "#999999", 3)
	for _, t := range []float64{-1, -0.75, -0.5, -0.25, 0, 0.25, 0.5, 0.75, 1} {
		x := bx + bw*(t+1)/2
		c.polyline([]pt{{x, by + bh}, {x, by + bh + 18}}, "#333333", 3)
		lbl := strings.Replace(strconv.FormatFloat(t, 'f', 2, 64), "-", "−", 1)
		c.text(x, by+bh+70, []string{lbl}, 10, "#000000", "middle", false)
	}
	c.text(W/2, by+bh+175, []string{"Spearman's r"}, 12, "#000000", "middle", false)
	c.svg.WriteString("</svg>\n")

	if !wantPNG {
		return nil, []byte(c.svg.String()), nil
	}
	var pngBuf bytes.Buffer
	if err := c.gc.EncodePNG(&pngBuf); err != nil {
		return nil, nil, err
	}
	return pngBuf.Bytes(), []byte(c.svg.String()), nil
}

func strengthName(r float64, c Cuts) string {
	return []string{"Very strong", "Strong", "Moderate", "Weak"}[bandOf(r, c)]
}

// writeLinks lists every correlation drawn. With diag != "" a first column names the diagram.
func writeLinks(x *excelize.File, sheet, diag string, links []Link, c Cuts, bold int) int {
	idx, _ := x.GetSheetIndex(sheet)
	row := 2
	if idx < 0 {
		x.NewSheet(sheet)
		h := []any{"Variable 1", "Variable 2", "Category 1", "Category 2", "r", "p value", "Sign", "Strength"}
		if diag != "" {
			h = append([]any{"Diagram"}, h...)
		}
		x.SetSheetRow(sheet, "A1", &h)
		x.SetRowStyle(sheet, 1, 1, bold)
		off := 0
		if diag != "" {
			x.SetColWidth(sheet, "A", "A", 22)
			off = 1
		}
		c1, _ := excelize.ColumnNumberToName(1 + off)
		c2, _ := excelize.ColumnNumberToName(2 + off)
		x.SetColWidth(sheet, c1, c2, 40)
		x.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	} else {
		rows, _ := x.GetRows(sheet)
		row = len(rows) + 1
	}
	for _, l := range links {
		sign := "Positive"
		if l.R < 0 {
			sign = "Negative"
		}
		v := []any{l.V1, l.V2, l.C1, l.C2, math.Round(l.R*10000) / 10000, l.P, sign, strengthName(l.R, c)}
		if diag != "" {
			v = append([]any{diag}, v...)
		}
		x.SetSheetRow(sheet, fmt.Sprintf("A%d", row), &v)
		row++
	}
	return row
}
