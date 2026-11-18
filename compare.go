package main

// Counts workbooks of comparisons and the balanced left side (drawing: comparelayout.go).

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ---------------- counts for comparisons ----------------

func bandRow(c Counts) []any {
	row := []any{c.Total, c.Pos, c.Neg}
	for _, b := range c.Bands {
		row = append(row, b.Pos, b.Neg)
	}
	return row
}

func bandHeader() []any {
	h := []any{"Total", "Positive", "Negative"}
	for _, b := range []string{"Very strong", "Strong", "Moderate", "Weak"} {
		h = append(h, b+" +", b+" −")
	}
	return h
}

// writeCompare writes the per-time-point sheets for one or more comparison diagrams.
func writeCompare(x *excelize.File, diags []*Diagram, cuts Cuts, withDiagram bool, bold int) {
	pre := func(d *Diagram, rest ...any) []any {
		if withDiagram {
			return append([]any{d.Name}, rest...)
		}
		return rest
	}
	hpre := func(rest ...any) []any {
		if withDiagram {
			return append([]any{"Diagram"}, rest...)
		}
		return rest
	}
	sh := "Summary"
	x.SetSheetName("Sheet1", sh)
	h := hpre(append([]any{"Group"}, bandHeader()...)...)
	x.SetSheetRow(sh, "A1", &h)
	r := 2
	for _, d := range diags {
		for _, t := range d.Times {
			row := pre(d, append([]any{t.Name}, bandRow(t.Counts)...)...)
			x.SetSheetRow(sh, fmt.Sprintf("A%d", r), &row)
			r++
		}
		row := pre(d, append([]any{"All groups"}, bandRow(d.Counts)...)...)
		x.SetSheetRow(sh, fmt.Sprintf("A%d", r), &row)
		x.SetRowStyle(sh, r, r, bold)
		r++
	}
	x.SetRowStyle(sh, 1, 1, bold)
	x.SetColWidth(sh, "A", "B", 24)
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	note := []any{"Strength by |r|: very strong ≥ " + f(cuts.VeryStrong) + ", strong ≥ " + f(cuts.Strong) + ", moderate ≥ " + f(cuts.Moderate) + ", weak below."}
	x.SetSheetRow(sh, fmt.Sprintf("A%d", r+1), &note)

	pair := "By category pair"
	x.NewSheet(pair)
	ph := hpre(append([]any{"Group", "Category 1", "Category 2"}, bandHeader()...)...)
	x.SetSheetRow(pair, "A1", &ph)
	r = 2
	for _, d := range diags {
		for _, t := range d.Times {
			for _, p := range t.Counts.Pairs {
				pc := Counts{Total: p.Total, Pos: p.Pos, Neg: p.Neg, Bands: p.Bands}
				row := pre(d, append([]any{t.Name, p.A, p.B}, bandRow(pc)...)...)
				x.SetSheetRow(pair, fmt.Sprintf("A%d", r), &row)
				r++
			}
		}
	}
	x.SetRowStyle(pair, 1, 1, bold)
	x.SetColWidth(pair, "A", "D", 24)

	cat := "By category"
	x.NewSheet(cat)
	chh := hpre(append([]any{"Group", "Category"}, bandHeader()...)...)
	x.SetSheetRow(cat, "A1", &chh)
	r = 2
	for _, d := range diags {
		for _, t := range d.Times {
			for _, p := range t.Counts.PerCat {
				pc := Counts{Total: p.Total, Pos: p.Pos, Neg: p.Neg, Bands: p.Bands}
				row := pre(d, append([]any{t.Name, p.A}, bandRow(pc)...)...)
				x.SetSheetRow(cat, fmt.Sprintf("A%d", r), &row)
				r++
			}
		}
	}
	x.SetRowStyle(cat, 1, 1, bold)
	x.SetColWidth(cat, "A", "C", 24)

	all := "All correlations"
	x.NewSheet(all)
	ah := hpre("Group", "Variable 1", "Variable 2", "Category 1", "Category 2", "r", "p value", "Sign", "Strength", "Region 1", "Region 2", "File")
	x.SetSheetRow(all, "A1", &ah)
	r = 2
	for _, d := range diags {
		for _, t := range d.times {
			for _, l := range t.Links {
				sign := "Positive"
				if l.R < 0 {
					sign = "Negative"
				}
				row := pre(d, t.Name, l.V1, l.V2, l.C1, l.C2, math.Round(l.R*10000)/10000, l.P, sign, strengthName(l.R, cuts), l.G1, l.G2, l.File)
				x.SetSheetRow(all, fmt.Sprintf("A%d", r), &row)
				r++
			}
		}
	}
	x.SetRowStyle(all, 1, 1, bold)
	off := 0
	if withDiagram {
		off = 1
	}
	c1, _ := excelize.ColumnNumberToName(2 + off)
	c2, _ := excelize.ColumnNumberToName(3 + off)
	x.SetColWidth(all, c1, c2, 40)
	x.SetPanes(all, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
}

func CompareXLSX(d *Diagram, cuts Cuts, segs *segTable, warnings []string) []byte {
	x := excelize.NewFile()
	defer x.Close()
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	writeCompare(x, []*Diagram{d}, cuts, false, bold)
	writeExtras(x, segs, warnings, bold)
	var buf bytes.Buffer
	x.Write(&buf)
	return buf.Bytes()
}

func CompareSummaryXLSX(diags []*Diagram, cuts Cuts) []byte {
	x := excelize.NewFile()
	defer x.Close()
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	writeCompare(x, diags, cuts, true, bold)
	var buf bytes.Buffer
	x.Write(&buf)
	return buf.Bytes()
}

// wpt converts pixel coordinates back to world units (fill works in world units).
func wpt(x, y float64) pt { return pt{(x - cx) / scale, (cy - y) / scale} }

// wrapLines wraps text, keeping explicit line breaks.
func wrapLines(s string, width int) []string {
	var out []string
	for _, part := range strings.Split(s, "\n") {
		out = append(out, wrap(part, width)...)
	}
	return out
}

// balanceLeft chooses, for drawing only, which end of each correlation sits on
// the left, so that every category that takes part in a correlation also
// appears on the left. Category pairs are oriented like a round-robin: of the
// categories present (in ring order, n of them), the pair (i, j), i < j, starts
// at i when j-i < n/2, at j when j-i > n/2, and at i when j-i = n/2. If a
// category still starts no chord, one of its pairs is turned around (choosing
// a partner that keeps chords of its own), or, failing that, every other
// correlation of its largest pair is turned around. Counts are not affected.
func balanceLeft(times []TimeSet, order []string) []TimeSet {
	oidx := map[string]int{}
	for i, c := range order {
		oidx[c] = i
	}
	type pk [2]string // ring order: earlier, later
	key := func(l Link) pk {
		if oidx[l.C1] <= oidx[l.C2] {
			return pk{l.C1, l.C2}
		}
		return pk{l.C2, l.C1}
	}
	size := map[pk]int{}
	present := map[string]bool{}
	for _, t := range times {
		for _, l := range t.Links {
			size[key(l)]++
			present[l.C1], present[l.C2] = true, true
		}
	}
	var cats []string
	for _, c := range order {
		if present[c] {
			cats = append(cats, c)
		}
	}
	n := len(cats)
	pidx := map[string]int{}
	for i, c := range cats {
		pidx[c] = i
	}
	var pairs []pk
	for k := range size {
		pairs = append(pairs, k)
	}
	sort.Slice(pairs, func(a, b int) bool {
		if pidx[pairs[a][0]] != pidx[pairs[b][0]] {
			return pidx[pairs[a][0]] < pidx[pairs[b][0]]
		}
		return pidx[pairs[a][1]] < pidx[pairs[b][1]]
	})
	flip := map[pk]bool{}  // whole pair starts at the later category
	split := map[pk]bool{} // every other correlation of the pair is turned around
	for _, k := range pairs {
		d := pidx[k[1]] - pidx[k[0]]
		flip[k] = 2*d > n
	}
	leftOf := func(k pk) string {
		if flip[k] {
			return k[1]
		}
		return k[0]
	}
	starts := func() map[string]int {
		m := map[string]int{}
		for _, k := range pairs {
			m[leftOf(k)]++
			if split[k] {
				o := k[0]
				if leftOf(k) == k[0] {
					o = k[1]
				}
				m[o]++
			}
		}
		return m
	}
	for _, c := range cats {
		st := starts()
		if st[c] > 0 {
			continue
		}
		// turn around a pair whose current left category keeps other chords
		best, bestN := pk{}, 0
		for _, k := range pairs {
			if k[0] != c && k[1] != c {
				continue
			}
			o := leftOf(k)
			if o != c && st[o] >= 2 && size[k] > bestN {
				best, bestN = k, size[k]
			}
		}
		if bestN > 0 {
			flip[best] = !flip[best]
			continue
		}
		// otherwise split its largest pair (needs at least two correlations)
		for _, k := range pairs {
			if (k[0] == c || k[1] == c) && size[k] >= 2 && size[k] > bestN {
				best, bestN = k, size[k]
			}
		}
		if bestN > 0 {
			split[best] = true
		}
	}
	out := make([]TimeSet, len(times))
	seen := map[pk]int{}
	for ti, t := range times {
		out[ti] = t
		out[ti].Links = make([]Link, len(t.Links))
		for li, l := range t.Links {
			k := key(l)
			want := leftOf(k)
			if split[k] {
				if seen[k]%2 == 1 {
					if want == k[0] {
						want = k[1]
					} else {
						want = k[0]
					}
				}
				seen[k]++
			}
			if l.C1 != want {
				l.swap()
			}
			out[ti].Links[li] = l
		}
	}
	return out
}

// dimHex tones a chord colour down for a group that is not highlighted: mostly
// light grey, keeping a faint hint of the red/blue so the sign stays readable.
func dimHex(h string) string {
	var r, g, b int
	fmt.Sscanf(strings.TrimPrefix(h, "#"), "%02x%02x%02x", &r, &g, &b)
	mix := func(v int) int { return int(math.Round(0.22*float64(v) + 0.78*0xD4)) }
	return fmt.Sprintf("#%02X%02X%02X", mix(r), mix(g), mix(b))
}
