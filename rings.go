package main

// Full-ring layout with optional extra rings. From the inside out:
// count scale > variable category > category group > region > file.
// Every correlation is one chord between the segments of its two variables.

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strings"
)

type ringInput struct {
	links      []Link
	order      []string // ring order of the categories
	colors     []string // colour order (fixed colours per category)
	title      string
	st         Style
	catLabel   map[string]string
	catGroup   map[string]string
	files      []string // file labels in order (several files)
	regions    []string // region codes in order
	regionKey  func(file, code string) string
	noRegion   string // name for variables without a region
	regionWord string // e.g. "Region"
}

type ringEnd struct {
	link, side int
	partner    int // segment index of the other end
	r          float64
}

type ringSeg struct {
	file, region, group, cat string
	ends                     []ringEnd
	hi, lo                   float64
}

type ringPlan struct {
	in                           *ringInput
	segs                         []*ringSeg
	gaps                         []float64 // gap before segment i; gaps[0] = between the last and the first
	avail                        float64
	n                            []int
	prop, inside                 bool
	minArc                       float64
	hasFile, hasRegion, hasGroup bool
	endSeg                       map[[2]int]int // (link, side) -> segment
}

func deg(x float64) float64 { return x * math.Pi / 180 }

func planRing(in *ringInput) (*ringPlan, error) {
	if len(in.links) == 0 {
		return nil, fmt.Errorf("no correlations to draw")
	}
	st := in.st
	p := &ringPlan{in: in, prop: st.proportional(), inside: st.inside(),
		hasFile: st.hasRing("file") && len(in.files) > 0, hasRegion: st.hasRing("region"), hasGroup: st.hasRing("group")}
	fidx := map[string]int{}
	for i, f := range in.files {
		fidx[f] = i
	}
	cidx := map[string]int{}
	for i, c := range in.order {
		cidx[c] = i
	}
	gidx := map[string]int{}
	for _, c := range in.order {
		g := in.catGroup[c]
		if _, ok := gidx[g]; !ok && g != "" {
			gidx[g] = len(gidx)
		}
	}
	ridx := map[string]int{}
	for i, r := range in.regions {
		ridx[r] = i
	}
	regionOf := func(file, code string) string {
		if code == "" {
			code = in.noRegion
		}
		if in.regionKey != nil {
			return in.regionKey(file, code)
		}
		return code
	}
	key := func(l Link, side int) ringSeg {
		cat, g := l.C1, l.G1
		if side == 1 {
			cat, g = l.C2, l.G2
		}
		s := ringSeg{cat: cat}
		if p.hasFile {
			s.file = l.File
		}
		if p.hasRegion {
			s.region = regionOf(l.File, g)
		}
		if p.hasGroup {
			s.group = in.catGroup[cat]
		}
		return s
	}
	// region order: file, then the order of the codes, variables without a region last
	regRank := func(name string) [3]int {
		f, code := 0, name
		for i, fl := range in.files {
			if strings.HasPrefix(name, fl+" ") {
				f, code = i, strings.TrimPrefix(name, fl+" ")
			}
		}
		if code == in.noRegion {
			return [3]int{f, 1 << 20, 0}
		}
		if i, ok := ridx[code]; ok {
			return [3]int{f, i, 0}
		}
		return [3]int{f, 1 << 19, 0}
	}
	segOf := map[string]int{}
	p.endSeg = map[[2]int]int{}
	for i, l := range in.links {
		for side := 0; side < 2; side++ {
			s := key(l, side)
			k := s.file + "\x00" + s.region + "\x00" + s.group + "\x00" + s.cat
			j, ok := segOf[k]
			if !ok {
				j = len(p.segs)
				segOf[k] = j
				cp := s
				p.segs = append(p.segs, &cp)
			}
			p.segs[j].ends = append(p.segs[j].ends, ringEnd{link: i, side: side, r: l.R})
		}
	}
	less := func(a, b *ringSeg) bool {
		if a.file != b.file {
			return fidx[a.file] < fidx[b.file]
		}
		if a.region != b.region {
			ra, rb := regRank(a.region), regRank(b.region)
			if ra != rb {
				return ra[0] < rb[0] || (ra[0] == rb[0] && ra[1] < rb[1])
			}
			return a.region < b.region
		}
		if a.group != b.group {
			ga, oka := gidx[a.group]
			gb, okb := gidx[b.group]
			if !oka {
				ga = 1 << 20
			}
			if !okb {
				gb = 1 << 20
			}
			return ga < gb
		}
		return cidx[a.cat] < cidx[b.cat]
	}
	old := append([]*ringSeg{}, p.segs...)
	sort.SliceStable(p.segs, func(i, j int) bool { return less(p.segs[i], p.segs[j]) })
	newIdx := map[*ringSeg]int{}
	for i, s := range p.segs {
		newIdx[s] = i
	}
	for oi, s := range old {
		for _, e := range s.ends {
			p.endSeg[[2]int{e.link, e.side}] = newIdx[old[oi]]
		}
	}
	for _, s := range p.segs {
		for k := range s.ends {
			e := &s.ends[k]
			e.partner = p.endSeg[[2]int{e.link, 1 - e.side}]
		}
		p.n = append(p.n, len(s.ends))
	}
	// gaps
	levels := p.hasFile || p.hasRegion || p.hasGroup
	total := 0.0
	for i, s := range p.segs {
		q := p.segs[(i-1+len(p.segs))%len(p.segs)]
		g := 7.0
		switch {
		case p.hasFile && s.file != q.file:
			g = 8
		case p.hasRegion && s.region != q.region:
			g = 3
		case p.hasGroup && s.group != q.group:
			g = 6
			if p.hasRegion {
				g = 2.5
			}
		case levels:
			g = 1.5
		case p.inside:
			g = 3
		}
		if len(p.segs) == 1 {
			g = 7
		}
		p.gaps = append(p.gaps, deg(g))
		total += deg(g)
	}
	p.avail = 2*math.Pi - total
	if p.inside && !p.hasFile && !p.hasRegion && p.prop {
		p.minArc = deg(24)
		if p.minArc*float64(len(p.segs)) > 0.8*p.avail {
			p.minArc = 0.8 * p.avail / float64(len(p.segs))
		}
	}
	return p, nil
}

// fit returns the widest chord (radians) that fits this diagram.
func (p *ringPlan) fit() float64 {
	return chordFill * maxSlot(p.n, p.avail, p.prop, p.minArc)
}

type ringRadii struct {
	scale                      float64 // inner radius of the scale (0 = none)
	cat0, cat1                 float64
	group0, group1, reg0, reg1 float64
	file0, file1, out          float64
}

func (p *ringPlan) radii() ringRadii {
	var r ringRadii
	r.cat0 = 1.02
	if p.in.st.Scale {
		r.scale = 1.01
		r.cat0 = 1.11
	}
	t := 0.07
	if p.inside {
		t = 0.10
	}
	r.cat1 = r.cat0 + t
	next := r.cat1 + 0.03
	ring := func(thick float64) (float64, float64) {
		a, b := next, next+thick
		next = b + 0.03
		return a, b
	}
	big, small := 0.06, 0.05
	if p.inside {
		big, small = 0.10, 0.075
	}
	r.out = r.cat1
	if p.hasGroup {
		r.group0, r.group1 = ring(big)
		r.out = r.group1
	}
	if p.hasRegion {
		r.reg0, r.reg1 = ring(small)
		r.out = r.reg1
	}
	if p.hasFile {
		r.file0, r.file1 = ring(big)
		r.out = r.file1
	}
	return r
}

type ringResult struct {
	png, svg []byte
	rows     [][]any // chords per ring segment (for the counts workbook)
	header   []any
}

// render draws the diagram; chordW = drawn chord width in radians (0 = automatic).
func (p *ringPlan) render(chordW float64, raster bool) (*ringResult, error) {
	in, st := p.in, p.in.st
	slot := 0.0
	if chordW > 0 {
		slot = chordW / chordFill
	}
	arcs, spacing, err := allocate(p.n, p.avail, p.prop, p.minArc, slot)
	if err != nil {
		return nil, err
	}
	if chordW == 0 && p.prop {
		chordW = chordFill * spacing
	}
	N := len(p.segs)
	th := math.Pi/2 - p.gaps[0]/2
	type span struct{ hi, lo float64 }
	pos := map[[2]int]span{}
	for i, s := range p.segs {
		if i > 0 {
			th -= p.gaps[i]
		}
		e := s.ends
		sort.SliceStable(e, func(a, b int) bool {
			ka := -((e[a].partner - i + N) % N)
			kb := -((e[b].partner - i + N) % N)
			if ka != kb {
				return ka < kb
			}
			ra, rb := e[a].r, e[b].r
			if e[a].side == 1 {
				ra = -ra
			}
			if e[b].side == 1 {
				rb = -rb
			}
			return ra < rb
		})
		s.hi = th
		unit := arcs[i] / float64(len(e))
		for _, x := range e {
			pos[[2]int{x.link, x.side}] = span{th, th - unit}
			th -= unit
		}
		s.lo = th
	}
	R := p.radii()
	cidx := map[string]int{}
	for i, c := range in.colors {
		cidx[c] = i
	}
	labPt := st.labelPt(12)
	wrapN := st.labelWrap(labPt)
	levels := p.hasFile || p.hasRegion || p.hasGroup

	// labels outside the ring
	var outs []*outLabel
	addOut := func(lines []string, color string, mid, size float64) {
		outs = append(outs, &outLabel{lines: lines, color: color, mid: mid, size: size})
	}
	// runs of consecutive segments sharing a level value
	type run struct {
		name     string
		hi, lo   float64
		first, n int
	}
	runs := func(f func(s *ringSeg) string) []run {
		var out []run
		for i, s := range p.segs {
			k := f(s)
			if len(out) > 0 && out[len(out)-1].name == k && i == out[len(out)-1].first+out[len(out)-1].n {
				out[len(out)-1].lo = s.lo
				out[len(out)-1].n++
				continue
			}
			out = append(out, run{k, s.hi, s.lo, i, 1})
		}
		return out
	}
	fileRuns := runs(func(s *ringSeg) string { return s.file })
	regRuns := runs(func(s *ringSeg) string { return s.file + "\x00" + s.region })
	grpRuns := runs(func(s *ringSeg) string { return s.file + "\x00" + s.region + "\x00" + s.group })
	lastPart := func(k string) string { x := strings.Split(k, "\x00"); return x[len(x)-1] }

	// colours of the outer rings: groups and files, distinct from the category colours
	var catHex []string
	for _, c := range in.colors {
		catHex = append(catHex, pastel[cidx[c]%len(pastel)])
	}
	var groupNames []string
	for _, c := range in.order {
		if g := in.catGroup[c]; g != "" && !contains(groupNames, g) {
			groupNames = append(groupNames, g)
		}
	}
	gcol := map[string][2]string{}
	if p.hasGroup {
		for i, cc := range ringColours(len(groupNames), catHex, nil) {
			gcol[groupNames[i]] = cc
		}
	}
	fcol := map[string][2]string{}
	if p.hasFile {
		var avoid [][2]string
		for _, g := range groupNames {
			avoid = append(avoid, gcol[g])
		}
		for i, cc := range ringColours(len(in.files), catHex, avoid) {
			fcol[in.files[i]] = cc
		}
	}

	// outside labels decide the size of the circle
	missingCat := false
	if !p.inside {
		if !levels {
			for _, s := range p.segs {
				dk := dark[cidx[s.cat]%len(dark)]
				addOut(wrap(s.cat, wrapN), dk, (s.hi+s.lo)/2, labPt)
			}
		} else {
			if p.hasFile {
				for _, r := range fileRuns {
					addOut(wrap(r.name, wrapN), fcol[r.name][1], (r.hi+r.lo)/2, math.Max(labPt, 15))
				}
			}
			if p.hasRegion {
				for _, r := range regRuns {
					addOut([]string{lastPart(r.name)}, "#444444", (r.hi+r.lo)/2, math.Max(11, 0.7*labPt))
				}
			}
			if p.hasGroup {
				for _, r := range grpRuns {
					if g := lastPart(r.name); g != "" {
						addOut(wrap(g, wrapN), gcol[g][1], (r.hi+r.lo)/2, math.Max(labPt, 15))
					}
				}
			}
		}
	}
	maxW := 0.0
	for _, l := range outs {
		for _, ln := range l.lines {
			maxW = math.Max(maxW, textWidth(ln, l.size, true))
		}
	}
	sc := 1090.0
	if len(outs) > 0 {
		sc = math.Min(sc, (W/2-70-maxW)/(R.out+0.22))
	} else {
		sc = math.Min(sc, (W/2-120)/R.out)
	}
	scale = sc
	defer func() { scale = 1090 }()
	margin := 90.0
	if len(outs) > 0 {
		margin = 260
	}
	topPx := 330 + margin
	cyv := topPx + R.out*sc
	ringBottom := cyv + R.out*sc + margin

	// legend: categories when their names are not on the ring
	var legendCats []string
	if (!p.inside && levels) || (p.inside && (p.hasFile || p.hasRegion)) {
		for _, c := range in.order {
			for _, s := range p.segs {
				if s.cat == c {
					legendCats = append(legendCats, c)
					break
				}
			}
		}
	}
	// category labels inside the ring (decide now whether a legend is needed)
	type catLab struct {
		lines []string
		size  float64
	}
	catLabs := map[int]catLab{}
	if p.inside {
		rm := (R.cat0 + R.cat1) / 2
		for i, s := range p.segs {
			name := s.cat
			if l := in.catLabel[s.cat]; l != "" {
				name = l
			}
			if lines, size, ok := fitRingLabel(name, s.hi-s.lo, rm, 15, 13, 9, true); ok {
				catLabs[i] = catLab{lines, size}
			} else {
				missingCat = true
			}
		}
		if missingCat && legendCats == nil {
			for _, c := range in.order {
				for _, s := range p.segs {
					if s.cat == c {
						legendCats = append(legendCats, c)
						break
					}
				}
			}
		}
	}
	noteY := ringBottom + 60
	legY := noteY + 100
	legEnd := legY
	if len(legendCats) > 0 {
		legEnd = legY + float64(legendRows(legendCats, 13))*95
	}
	barY := legEnd + 80
	setCanvas(barY+410, cyv)
	defer setCanvas(4150, 1950)

	c := newCanvas(raster)
	ids := make([]int, len(in.links))
	for i := range ids {
		ids[i] = i
	}
	sort.SliceStable(ids, func(a, b int) bool {
		la, lb := in.links[ids[a]], in.links[ids[b]]
		sa0, sb0 := p.endSeg[[2]int{ids[a], 0}], p.endSeg[[2]int{ids[b], 0}]
		if sa0 != sb0 {
			return sa0 < sb0
		}
		sa1, sb1 := p.endSeg[[2]int{ids[a], 1}], p.endSeg[[2]int{ids[b], 1}]
		if sa1 != sb1 {
			return sa1 < sb1
		}
		return la.R < lb.R
	})
	for _, i := range ids {
		A, B := pos[[2]int{i, 0}], pos[[2]int{i, 1}]
		ma, wa := (A.hi+A.lo)/2, A.hi-A.lo
		mb, wb := (B.hi+B.lo)/2, B.hi-B.lo
		ha, hb := chordFill/2*wa, chordFill/2*wb
		if chordW > 0 {
			ha, hb = chordW/2, chordW/2
		}
		c.fill(ribbon(ma+ha, ma-ha, mb+hb, mb-hb), toHex(cmap(in.links[i].R)), 0.95)
	}
	// scale and category ring
	for i, s := range p.segs {
		if st.Scale {
			c.drawScale(s.hi, (s.hi-s.lo)/float64(len(s.ends)), len(s.ends), R.scale)
		}
		c.fill(annulus(s.lo, s.hi, R.cat0, R.cat1), pastel[cidx[s.cat]%len(pastel)], 1)
		if cl, ok := catLabs[i]; ok {
			c.curvedText((s.hi+s.lo)/2, (R.cat0+R.cat1)/2, cl.lines, cl.size, dark[cidx[s.cat]%len(dark)], true)
		}
	}
	// outer rings
	ringName := func(r0, r1 float64, name string, hi, lo float64, fill, ink string, maxPt, minPt float64) {
		c.fill(annulus(lo, hi, r0, r1), fill, 1)
		if !p.inside || name == "" {
			return
		}
		if lines, size, ok := fitRingLabel(name, hi-lo, (r0+r1)/2, maxPt, minPt-1, minPt, false); ok {
			c.curvedText((hi+lo)/2, (r0+r1)/2, lines, size, ink, true)
		}
	}
	if p.hasGroup {
		for _, r := range grpRuns {
			g := lastPart(r.name)
			if g == "" {
				continue
			}
			ringName(R.group0, R.group1, g, r.hi, r.lo, gcol[g][0], gcol[g][1], 21, 12)
		}
	}
	if p.hasRegion {
		for k, r := range regRuns {
			fill := []string{"#ECECEC", "#DCDCDC"}[k%2]
			ringName(R.reg0, R.reg1, lastPart(r.name), r.hi, r.lo, fill, "#444444", 11, 8)
		}
	}
	if p.hasFile {
		for _, r := range fileRuns {
			ringName(R.file0, R.file1, r.name, r.hi, r.lo, fcol[r.name][0], fcol[r.name][1], 21, 12)
		}
	}
	if len(outs) > 0 {
		c.placeOutside(outs, R.out, (cyv-330)/scale, -(ringBottom-60-cyv)/scale, 0.2, true)
	}
	// title
	npos, nneg := 0, 0
	for _, l := range in.links {
		if l.R > 0 {
			npos++
		} else if l.R < 0 {
			nneg++
		}
	}
	c.text(W/2, 170, []string{in.title, fmt.Sprintf("%d correlations (%d positive, %d negative)", len(in.links), npos, nneg)}, 15, "#000000", "middle", false)
	// note on the rings
	var parts []string
	if st.Scale {
		parts = append(parts, "number of chords")
	}
	parts = append(parts, "variable category")
	if p.hasGroup {
		parts = append(parts, strings.ToLower(st.groupWord()))
	}
	if p.hasRegion {
		parts = append(parts, strings.ToLower(in.regionWord))
	}
	if p.hasFile {
		parts = append(parts, strings.ToLower(st.fileWord()))
	}
	note := ""
	if levels || st.Scale {
		note = "Rings from inside out: " + strings.Join(parts, ", ")
	}
	if p.hasRegion {
		for _, r := range regRuns {
			if lastPart(r.name) == in.noRegion && in.noRegion != "" {
				note += "   ·   " + in.noRegion + " = variables without a " + strings.ToLower(in.regionWord)
				break
			}
		}
	}
	if note != "" {
		c.text(W/2, noteY, []string{note}, 11, "#444444", "middle", false)
	}
	if len(legendCats) > 0 {
		drawCatLegend(c, legendCats, cidx, legY, 13)
	}
	drawColorbarAt(c, barY)
	c.svg.WriteString("</svg>\n")

	res := &ringResult{svg: []byte(c.svg.String())}
	if raster {
		var buf bytes.Buffer
		if err := c.gc.EncodePNG(&buf); err != nil {
			return nil, err
		}
		res.png = buf.Bytes()
	}
	// chords per segment
	if p.hasFile {
		res.header = append(res.header, strings.Title(st.fileWord()))
	}
	if p.hasRegion {
		res.header = append(res.header, in.regionWord)
	}
	if p.hasGroup {
		res.header = append(res.header, strings.Title(st.groupWord()))
	}
	res.header = append(res.header, "Category", "Chord ends")
	for _, s := range p.segs {
		var row []any
		if p.hasFile {
			row = append(row, s.file)
		}
		if p.hasRegion {
			row = append(row, s.region)
		}
		if p.hasGroup {
			row = append(row, s.group)
		}
		row = append(row, s.cat, len(s.ends))
		res.rows = append(res.rows, row)
	}
	return res, nil
}
