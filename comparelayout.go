package main

// Comparison layouts: variable categories on the left half of the circle, and
// on the right half either the groups that are compared (e.g. Control, OA) or,
// for one group, the files (e.g. studies), optionally split into regions.
// Each correlation is one chord from one of its categories (left; balanced so
// that every category appears there) to the right-hand block where it belongs,
// landing in that block's sub-band for the category at its other end.

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

type TimeSet struct {
	Name    string
	Parent  string // split layout: the file a region block belongs to
	Dim     bool   // toned down: another group is highlighted
	Links   []Link
	Removed Removed
}

type TimeCounts struct {
	Name   string `json:"name"`
	Counts Counts `json:"counts"`
}

var timeRing = []string{"#C9D3DF", "#E3D3BF", "#CFE3D4", "#E2CFE0", "#D9D9D9", "#F0DDC8"}
var timeDark = []string{"#3E5370", "#7A5A34", "#3B6E4A", "#6E4669", "#555555", "#8A5A2B"}

type cmpInput struct {
	times       []TimeSet // comparison: one per group; split: exactly one (the group drawn)
	order       []string
	colors      []string
	title       string
	st          Style
	split       bool     // right half = files [> regions] instead of groups
	files       []string // split: file labels in order
	regions     bool     // split: subdivide each file by region
	regionCodes []string
	noRegion    string
	regionWord  string
}

type cmpPlan struct {
	in             *cmpInput
	blocks         []TimeSet
	cats           []string // left categories (start at least one chord)
	legendCats     []string
	nL, nR         []int
	gapL, gapR     []float64 // gap before segment k (k>0)
	availL, availR float64
	prop           bool
	midGap         float64
}

func planCompare(in *cmpInput) (*cmpPlan, error) {
	p := &cmpPlan{in: in, prop: in.st.proportional(), midGap: deg(7)}
	times := balanceLeft(in.times, in.order)
	if in.split {
		var all []Link
		for _, t := range times {
			all = append(all, t.Links...)
		}
		fidx := map[string]int{}
		for i, f := range in.files {
			fidx[f] = i
		}
		ridx := map[string]int{}
		for i, r := range in.regionCodes {
			ridx[r] = i
		}
		type bk struct{ file, reg string }
		buckets := map[bk][]Link{}
		var keys []bk
		for _, l := range all {
			k := bk{l.File, ""}
			if in.regions {
				k.reg = l.G2
				if k.reg == "" {
					k.reg = in.noRegion
				}
			}
			if _, ok := buckets[k]; !ok {
				keys = append(keys, k)
			}
			buckets[k] = append(buckets[k], l)
		}
		rank := func(r string) int {
			if r == in.noRegion {
				return 1 << 20
			}
			if i, ok := ridx[r]; ok {
				return i
			}
			return 1 << 19
		}
		sort.SliceStable(keys, func(i, j int) bool {
			a, b := keys[i], keys[j]
			if a.file != b.file {
				return fidx[a.file] < fidx[b.file]
			}
			if rank(a.reg) != rank(b.reg) {
				return rank(a.reg) < rank(b.reg)
			}
			return a.reg < b.reg
		})
		for _, k := range keys {
			name := k.file
			if in.regions {
				name = k.reg
			}
			p.blocks = append(p.blocks, TimeSet{Name: name, Parent: k.file, Links: buckets[k]})
		}
	} else {
		p.blocks = times
	}
	present, left := map[string]bool{}, map[string]int{}
	total := 0
	for _, t := range p.blocks {
		for _, l := range t.Links {
			present[l.C1], present[l.C2] = true, true
			left[l.C1]++
			total++
		}
	}
	if total == 0 {
		return nil, fmt.Errorf("no correlations to draw")
	}
	for _, c := range in.order {
		if left[c] > 0 {
			p.cats = append(p.cats, c)
			p.nL = append(p.nL, left[c])
		}
		if present[c] {
			p.legendCats = append(p.legendCats, c)
		}
	}
	gapCat := deg(7)
	if in.split {
		gapCat = deg(3)
	}
	p.availL = math.Pi - p.midGap
	for k := range p.cats {
		g := 0.0
		if k > 0 {
			g = gapCat
		}
		p.gapL = append(p.gapL, g)
		p.availL -= g
	}
	p.availR = math.Pi - p.midGap
	for k, t := range p.blocks {
		g := 0.0
		if k > 0 {
			g = deg(7)
			if in.split {
				g = deg(6)
				if t.Parent == p.blocks[k-1].Parent {
					g = deg(1.5)
				}
			}
		}
		p.gapR = append(p.gapR, g)
		p.availR -= g
		p.nR = append(p.nR, len(t.Links))
	}
	return p, nil
}

// fit: the widest chord (radians) that fits both halves.
func (p *cmpPlan) fit() float64 {
	return chordFill * math.Min(maxSlot(p.nL, p.availL, p.prop, 0), maxSlot(p.nR, p.availR, p.prop, 0))
}

type cmpResult struct {
	png, svg []byte
	rows     [][]any
	header   []any
}

func (p *cmpPlan) render(chordW float64, raster bool) (*cmpResult, error) {
	in, st := p.in, p.in.st
	slot := 0.0
	if chordW > 0 {
		slot = chordW / chordFill
	}
	arcL, sL, err := allocate(p.nL, p.availL, p.prop, 0, slot)
	if err != nil {
		return nil, err
	}
	arcR, sR, err := allocate(p.nR, p.availR, p.prop, 0, slot)
	if err != nil {
		return nil, err
	}
	if chordW == 0 && p.prop {
		chordW = chordFill * math.Min(sL, sR)
	}
	cidx := map[string]int{}
	for i, c := range in.colors {
		cidx[c] = i
	}
	oidx := map[string]int{}
	for i, c := range in.order {
		oidx[c] = i
	}
	blocks, cats := p.blocks, p.cats
	nT, nC := len(blocks), len(cats)

	// clockwise segment index: blocks (top right -> bottom right), then
	// categories from bottom left up to top left (first category at the top)
	type span struct{ a1, a2 float64 }
	segIdx := map[string]int{}
	tKey := func(i int) string { return "\x00T" + strconv.Itoa(i) }
	for i := 0; i < nT; i++ {
		segIdx[tKey(i)] = i
	}
	for k := 0; k < nC; k++ {
		segIdx[cats[nC-1-k]] = nT + k
	}
	N := nT + nC
	segSpan := map[string]span{}
	th := math.Pi/2 - p.midGap/2
	for i := 0; i < nT; i++ {
		th -= p.gapR[i]
		segSpan[tKey(i)] = span{th, th - arcR[i]}
		th -= arcR[i]
	}
	th = -math.Pi/2 - p.midGap/2
	for k := 0; k < nC; k++ {
		ci := nC - 1 - k
		if k > 0 {
			th -= p.gapL[ci+1]
		}
		segSpan[cats[ci]] = span{th, th - arcL[ci]}
		th -= arcL[ci]
	}

	type ref struct{ t, i int }
	type end struct {
		r       ref
		partner string
		sub     string
		rv      float64
		side    int
	}
	ends := map[string][]end{}
	for ti, t := range blocks {
		for li, l := range t.Links {
			ends[l.C1] = append(ends[l.C1], end{ref{ti, li}, tKey(ti), l.C2, l.R, 0})
			ends[tKey(ti)] = append(ends[tKey(ti)], end{ref{ti, li}, l.C1, l.C2, l.R, 1})
		}
	}
	circ := func(partner string, ci int) int { return -((segIdx[partner] - ci + N) % N) }
	pos := map[[3]int]span{}
	type sub struct {
		cat    string
		a1, a2 float64
		last   string
		n      int
	}
	subRuns := map[string][]*sub{}
	for key, e := range ends {
		ci := segIdx[key]
		isBlock := strings.HasPrefix(key, "\x00T")
		sort.SliceStable(e, func(i, j int) bool {
			a, b := e[i], e[j]
			if isBlock {
				if oidx[a.sub] != oidx[b.sub] {
					return oidx[a.sub] < oidx[b.sub]
				}
				if oidx[a.partner] != oidx[b.partner] {
					return oidx[a.partner] < oidx[b.partner]
				}
				if a.rv != b.rv {
					return a.rv > b.rv
				}
				return a.r.t*100000+a.r.i < b.r.t*100000+b.r.i
			}
			if ka, kb := circ(a.partner, ci), circ(b.partner, ci); ka != kb {
				return ka < kb
			}
			if oidx[a.sub] != oidx[b.sub] {
				return oidx[a.sub] > oidx[b.sub]
			}
			if a.rv != b.rv {
				return a.rv < b.rv
			}
			return a.r.t*100000+a.r.i > b.r.t*100000+b.r.i
		})
		s := segSpan[key]
		unit := (s.a1 - s.a2) / float64(len(e))
		a := s.a1
		for _, x := range e {
			pos[[3]int{x.r.t, x.r.i, x.side}] = span{a, a - unit}
			if isBlock {
				runs := subRuns[key]
				id := x.sub
				if len(runs) == 0 || runs[len(runs)-1].last != id {
					runs = append(runs, &sub{cat: x.sub, a1: a, last: id})
				}
				runs[len(runs)-1].a2 = a - unit
				runs[len(runs)-1].n++
				subRuns[key] = runs
			}
			a -= unit
		}
	}

	// radii
	band1 := 1.065
	leftOut := 1.10
	next := 1.072
	if st.Scale {
		leftOut = 1.16
		next = 1.16
	}
	var reg0, reg1, out0, out1 float64
	if in.split && in.regions {
		reg0, reg1 = next+0.01, next+0.075
		if !st.Scale {
			reg0, reg1 = 1.08, 1.145
		}
		next = reg1 + 0.025
	}
	if in.split {
		if !st.Scale && !in.regions {
			next = 1.08
		}
		out0, out1 = next, next+0.06
	} else {
		out0, out1 = next, next+0.028
		if st.Scale {
			out1 = next + 0.04
		}
	}
	rOut := math.Max(out1, leftOut)

	// labels outside the ring
	labPt := st.labelPt(21)
	wrapN := st.labelWrap(labPt)
	var labs []*outLabel
	for _, cat := range cats {
		s := segSpan[cat]
		labs = append(labs, &outLabel{lines: wrapLines(cat, wrapN), color: dark[cidx[cat]%len(dark)], mid: (s.a1 + s.a2) / 2, size: labPt, r0: leftOut})
	}
	var fileCol map[string][2]string
	if in.split {
		var catHex []string
		for _, c := range in.colors {
			catHex = append(catHex, pastel[cidx[c]%len(pastel)])
		}
		fileCol = map[string][2]string{}
		for i, cc := range ringColours(len(in.files), catHex, nil) {
			fileCol[in.files[i]] = cc
		}
	}
	type prun struct {
		name   string
		hi, lo float64
		n      int
	}
	var parents []prun
	if in.split {
		for i, t := range blocks {
			s := segSpan[tKey(i)]
			if len(parents) > 0 && parents[len(parents)-1].name == t.Parent {
				parents[len(parents)-1].lo = s.a2
				parents[len(parents)-1].n += len(t.Links)
				continue
			}
			parents = append(parents, prun{t.Parent, s.a1, s.a2, len(t.Links)})
		}
		for _, pr := range parents {
			labs = append(labs, &outLabel{lines: append(wrapLines(pr.name, wrapN), fmt.Sprintf("(n = %d)", pr.n)), color: fileCol[pr.name][1], mid: (pr.hi + pr.lo) / 2, size: labPt, r0: out1})
		}
		if in.regions && !st.inside() {
			for i, t := range blocks {
				s := segSpan[tKey(i)]
				labs = append(labs, &outLabel{lines: []string{t.Name}, color: "#444444", mid: (s.a1 + s.a2) / 2, size: math.Max(11, 0.6*labPt), r0: reg1})
			}
		}
	} else {
		for i, t := range blocks {
			labs = append(labs, &outLabel{lines: append(wrapLines(t.Name, wrapN), fmt.Sprintf("(n = %d)", len(t.Links))), color: timeDark[i%len(timeDark)], mid: (segSpan[tKey(i)].a1 + segSpan[tKey(i)].a2) / 2, size: labPt, r0: out1})
		}
	}
	// canvas: the classic size unless the rings are larger
	maxW := 0.0
	for _, l := range labs {
		for _, ln := range l.lines {
			maxW = math.Max(maxW, textWidth(ln, l.size, true))
		}
	}
	sc := math.Min(1090, (W/2-70-maxW)/(rOut+0.22))
	classic := rOut <= 1.105 && sc >= 1090
	var cyv, ringBottom float64
	if classic {
		scale = 1090
		cyv = 1950
		ringBottom = 3300
	} else {
		scale = sc
		cyv = 330 + 260 + rOut*sc
		ringBottom = cyv + rOut*sc + 260
	}
	defer func() { scale = 1090 }()
	noteY := ringBottom + 100
	legY := noteY + 100
	legEnd := legY + float64(legendRows(p.legendCats, 13))*95
	barY := math.Max(3740, legEnd+80)
	if classic {
		noteY, legY, barY = 3400, 3500, 3740
	}
	setCanvas(barY+410, cyv)
	defer setCanvas(4150, 1950)

	c := newCanvas(raster)
	type item struct {
		t, i int
		l    Link
	}
	var items []item
	for ti, t := range blocks {
		for li, l := range t.Links {
			items = append(items, item{ti, li, l})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if da, db := blocks[a.t].Dim, blocks[b.t].Dim; da != db {
			return da
		}
		if a.t != b.t {
			return a.t < b.t
		}
		if oidx[a.l.C1] != oidx[b.l.C1] {
			return oidx[a.l.C1] < oidx[b.l.C1]
		}
		if oidx[a.l.C2] != oidx[b.l.C2] {
			return oidx[a.l.C2] < oidx[b.l.C2]
		}
		return a.l.R < b.l.R
	})
	const f = 0.275
	for _, it := range items {
		A, B := pos[[3]int{it.t, it.i, 0}], pos[[3]int{it.t, it.i, 1}]
		ma, wa := (A.a1+A.a2)/2, A.a1-A.a2
		mb, wb := (B.a1+B.a2)/2, B.a1-B.a2
		ha, hb := f*wa, f*wb
		if chordW > 0 {
			ha, hb = chordW/2, chordW/2
		}
		col, op := toHex(cmap(it.l.R)), 0.95
		if blocks[it.t].Dim {
			col, op = dimHex(col), 0.8
		}
		c.fill(ribbon(ma+ha, ma-ha, mb+hb, mb-hb), col, op)
	}
	// left rings (+ scale)
	for k, cat := range cats {
		s := segSpan[cat]
		c.fill(annulus(s.a2, s.a1, 1.02, 1.09), pastel[cidx[cat]%len(pastel)], 1)
		if st.Scale {
			c.drawScale(s.a1, (s.a1-s.a2)/float64(p.nL[k]), p.nL[k], 1.095)
		}
	}
	// right: inner band (+ scale), region ring, outer ring
	for i := 0; i < nT; i++ {
		k := tKey(i)
		s := segSpan[k]
		for _, sp := range subRuns[k] {
			c.fill(annulus(sp.a2, sp.a1, 1.02, band1), pastel[cidx[sp.cat]%len(pastel)], 1)
			if st.Scale {
				c.drawScale(sp.a1, (sp.a1-sp.a2)/float64(sp.n), sp.n, 1.07)
			}
		}
		if len(subRuns[k]) == 0 {
			c.fill(annulus(s.a2, s.a1, 1.02, band1), "#EEEEEE", 1)
		}
		if in.split && in.regions {
			fill := []string{"#ECECEC", "#DCDCDC"}[i%2]
			c.fill(annulus(s.a2, s.a1, reg0, reg1), fill, 1)
			if st.inside() {
				if lines, size, ok := fitRingLabel(blocks[i].Name, s.a1-s.a2, (reg0+reg1)/2, 11, 7, 8, false); ok {
					c.curvedText((s.a1+s.a2)/2, (reg0+reg1)/2, lines, size, "#444444", true)
				}
			}
		}
		if !in.split {
			c.fill(annulus(s.a2, s.a1, out0, out1), timeRing[i%len(timeRing)], 1)
		}
	}
	for _, pr := range parents {
		c.fill(annulus(pr.lo, pr.hi, out0, out1), fileCol[pr.name][0], 1)
	}
	yTop := (cyv - 330) / scale
	yBot := -(ringBottom - 40 - cyv) / scale
	if classic {
		yTop, yBot = 10, -10
	}
	c.placeOutside(labs, rOut, yTop, yBot, 0.30, false)
	// title and summary
	var parts []string
	sumOf := func(name string, ls []Link) string {
		pp, m := 0, 0
		for _, l := range ls {
			if l.R > 0 {
				pp++
			} else if l.R < 0 {
				m++
			}
		}
		return fmt.Sprintf("%s: %d correlations (%d positive, %d negative)", name, len(ls), pp, m)
	}
	if in.split {
		for _, f := range in.files {
			var ls []Link
			for _, t := range blocks {
				if t.Parent == f {
					ls = append(ls, t.Links...)
				}
			}
			if len(ls) > 0 {
				parts = append(parts, sumOf(f, ls))
			}
		}
	} else {
		for _, t := range blocks {
			parts = append(parts, sumOf(t.Name, t.Links))
		}
	}
	c.text(W/2, 170, append([]string{in.title}, strings.Join(parts, "   ·   ")), 15, "#000000", "middle", false)
	note := "Inner band on the right = category at the other end of the correlation"
	if st.Scale {
		note += "   ·   scale = number of chords"
	}
	if in.split && in.regions {
		for _, t := range blocks {
			if t.Name == in.noRegion && in.noRegion != "" {
				note += "   ·   " + in.noRegion + " = variables without a " + strings.ToLower(in.regionWord)
				break
			}
		}
	}
	c.text(W/2, noteY, []string{note}, 12, "#444444", "middle", false)
	drawCatLegend(c, p.legendCats, cidx, legY, 13)
	drawColorbarAt(c, barY)
	c.svg.WriteString("</svg>\n")
	res := &cmpResult{svg: []byte(c.svg.String())}
	if raster {
		var buf bytes.Buffer
		if err := c.gc.EncodePNG(&buf); err != nil {
			return nil, err
		}
		res.png = buf.Bytes()
	}
	// chord ends per segment
	res.header = []any{"Side", "Block", "Category", "Chord ends"}
	for k, cat := range cats {
		res.rows = append(res.rows, []any{"Left", "", cat, p.nL[k]})
	}
	for i, t := range blocks {
		name := t.Name
		if in.split && in.regions {
			name = t.Parent + " · " + t.Name
		}
		for _, sp := range subRuns[tKey(i)] {
			res.rows = append(res.rows, []any{"Right", name, sp.cat, sp.n})
		}
	}
	return res, nil
}
