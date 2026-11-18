package main

// Shared drawing options and helpers: chord width, label size, count scale,
// segment sizes, ring levels, curved ring labels, leader-line labels and the
// colours of the outer rings.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

// Style holds the appearance options (same names in the website, the desktop
// app (JSON) and the command line).
type Style struct {
	ChordWidth string   `json:"chordWidth"` // "auto" (default), "same" (one width for the whole run) or a width in degrees
	LabelSize  float64  `json:"labelSize"`  // category label size in pt (0 = automatic: 12, or 21 in the comparison layouts)
	LabelWrap  int      `json:"labelWrap"`  // characters per line (0 = automatic)
	Scale      bool     `json:"scale"`      // count scale on the rings
	Segments   string   `json:"segments"`   // "auto" (proportional with the scale, else equal), "equal" or "proportional"
	Rings      []string `json:"rings"`      // full-ring layout: extra outer rings: "file", "region", "group"
	RingLabels string   `json:"ringLabels"` // "auto", "inside" (names curved inside the rings) or "outside"
	GroupWord  string   `json:"groupWord"`  // what the Group column means, e.g. "tissue" (legend text)
	FileWord   string   `json:"fileWord"`   // what the files are, e.g. "study" (legend text)
}

// fixedWidth parses ChordWidth: degrees (>0), same=true, or automatic (0, false).
func (s Style) fixedWidth() (float64, bool, error) {
	w := strings.ToLower(strings.TrimSpace(s.ChordWidth))
	switch w {
	case "", "auto", "automatic", "0":
		return 0, false, nil
	case "same", "run", "same width":
		return 0, true, nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSuffix(w, "°"), ",", "."), 64)
	if err != nil || v < 0 || v > 30 {
		return 0, false, fmt.Errorf("chord width must be auto, same or a number of degrees (e.g. 0.2); got %q", s.ChordWidth)
	}
	return v, false, nil
}

func (s Style) hasRing(name string) bool {
	for _, r := range s.Rings {
		if strings.EqualFold(strings.TrimSpace(r), name) {
			return true
		}
	}
	return false
}

func (s Style) proportional() bool {
	switch strings.ToLower(s.Segments) {
	case "equal":
		return false
	case "proportional":
		return true
	}
	return s.Scale
}

// inside: names drawn inside the rings? auto = inside when there are extra rings.
func (s Style) inside() bool {
	switch strings.ToLower(s.RingLabels) {
	case "inside", "on", "yes":
		return true
	case "outside", "off", "no":
		return false
	}
	return len(s.Rings) > 0
}

// usesRingLayout: the classic diagram is extended (extra rings, scale, labels
// in the rings or proportional segments).
func (s Style) usesRingLayout() bool {
	return len(s.Rings) > 0 || s.Scale || strings.EqualFold(s.RingLabels, "inside") || strings.EqualFold(s.Segments, "proportional")
}

func (s Style) labelPt(def float64) float64 {
	if s.LabelSize > 0 {
		return s.LabelSize
	}
	return def
}

// labelWrap: characters per line; automatic = 30 at 12 pt, 20 at 21 pt and larger.
func (s Style) labelWrap(pt float64) int {
	if s.LabelWrap > 0 {
		return s.LabelWrap
	}
	switch {
	case pt <= 12:
		return 30
	case pt >= 21:
		return 20
	}
	return int(math.Round(30 - (pt-12)*10/9))
}

func (s Style) groupWord() string {
	if w := strings.TrimSpace(s.GroupWord); w != "" {
		return w
	}
	return "group"
}

func (s Style) fileWord() string {
	if w := strings.TrimSpace(s.FileWord); w != "" {
		return w
	}
	return "file"
}

// ---------------- segment sizes ----------------

const chordFill = 0.55 // a chord fills 55 % of its slot

// maxSlot: the largest slot per chord end that fits. Equal segments grow when a
// category needs more room (water-filling), so only the total matters;
// proportional segments are at least minArc wide.
func maxSlot(n []int, avail float64, prop bool, minArc float64) float64 {
	tot := 0
	for _, x := range n {
		tot += x
	}
	if tot == 0 || avail <= 0 {
		return 0
	}
	if !prop || minArc <= 0 {
		return avail / float64(tot)
	}
	fixed := make([]bool, len(n))
	rem, left := avail, tot
	for {
		if left == 0 || rem <= 0 {
			return 0
		}
		s := rem / float64(left)
		grew := false
		for i, x := range n {
			if !fixed[i] && float64(x)*s < minArc {
				fixed[i] = true
				rem -= minArc
				left -= x
				grew = true
			}
		}
		if !grew {
			return s
		}
	}
}

// allocate returns the arc of every segment. slot = fixed slot per chord end
// (0 = automatic). The second value is the spacing of chord ends in
// proportional segments (also the automatic slot there).
func allocate(n []int, avail float64, prop bool, minArc, slot float64) ([]float64, float64, error) {
	arcs := make([]float64, len(n))
	smax := maxSlot(n, avail, prop, minArc)
	if slot > smax*(1+1e-9) {
		tot := 0
		for _, x := range n {
			tot += x
		}
		return nil, 0, fmt.Errorf("chord width too large: %d chord ends need %.1f° but only %.1f° are available; choose a width of at most %.3f° (or \"same\")",
			tot, slot*float64(tot)*180/math.Pi, avail*180/math.Pi, smax*chordFill*180/math.Pi)
	}
	if prop {
		for i, x := range n {
			arcs[i] = math.Max(minArc, float64(x)*smax)
			if x == 0 {
				arcs[i] = 0
			}
		}
		return arcs, smax, nil
	}
	if slot <= 0 {
		for i := range n {
			arcs[i] = avail / float64(len(n))
		}
		return arcs, 0, nil
	}
	// equal shares; a segment that needs more room grows, the others share the rest
	done := make([]bool, len(n))
	rem, left := avail, len(n)
	for left > 0 {
		share := rem / float64(left)
		grew := false
		for i, x := range n {
			if !done[i] && float64(x)*slot > share {
				arcs[i] = float64(x) * slot
				rem -= arcs[i]
				done[i] = true
				left--
				grew = true
			}
		}
		if !grew {
			for i := range n {
				if !done[i] {
					arcs[i] = share
				}
			}
			break
		}
	}
	return arcs, 0, nil
}

// floorDeg rounds a chord width down to 0.001°.
func floorDeg(rad float64) float64 { return math.Floor(rad*180/math.Pi*1000+1e-9) / 1000 }

// ---------------- text ----------------

func faceFor(sizePt float64, bold bool) font.Face {
	f := regularTTF
	if bold {
		f = boldTTF
	}
	return truetype.NewFace(f, &truetype.Options{Size: sizePt * ptpx})
}

// textWidth in canvas pixels.
func textWidth(s string, sizePt float64, bold bool) float64 {
	return float64(font.MeasureString(faceFor(sizePt, bold), s)) / 64
}

func runeWidth(r rune, sizePt float64, bold bool) float64 {
	adv, _ := faceFor(sizePt, bold).GlyphAdvance(r)
	return float64(adv) / 64
}

// rotText draws one line centred at (x, y) (pixels), rotated clockwise by deg.
func (c *canvas) rotText(x, y, deg float64, s string, sizePt float64, hex string, bold bool) {
	if c.gc != nil {
		c.gc.Push()
		c.gc.RotateAbout(gg.Radians(deg), x, y)
		c.gc.SetFontFace(faceFor(sizePt, bold))
		r, g, b := hexRGB(hex)
		c.gc.SetRGB(r, g, b)
		c.gc.DrawStringAnchored(s, x, y, 0.5, 0.35)
		c.gc.Pop()
	}
	weight := "normal"
	if bold {
		weight = "bold"
	}
	esc := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	fmt.Fprintf(&c.svg, `<text x="%.1f" y="%.1f" transform="rotate(%.2f %.1f %.1f)" font-family="Helvetica, Arial, sans-serif" font-size="%.1f" font-weight="%s" fill="%s" text-anchor="middle" dominant-baseline="central">%s</text>`+"\n",
		x, y, deg, x, y, sizePt*ptpx, weight, hex, esc.Replace(s))
}

// tangentDeg: screen rotation of upright text tangent to the circle at angle a
// (radians, mathematical); flipped on the bottom half so it is never upside down.
func tangentDeg(a float64) float64 {
	d := 90 - a*180/math.Pi
	if math.Sin(a) < 0 {
		d += 180
	}
	return d
}

// arcText: one short text centred on the ring at angle a, radius r (world units).
func (c *canvas) arcText(a, r float64, s string, sizePt float64, hex string, bold bool) {
	p := px(polar(a, r))
	c.rotText(p.X, p.Y, tangentDeg(a), s, sizePt, hex, bold)
}

// curvedText places the letters one by one along the ring, upright: clockwise
// on the top half, anticlockwise on the bottom half. Several lines are stacked
// radially (first line outermost on the top half).
func (c *canvas) curvedText(mid, r float64, lines []string, sizePt float64, hex string, bold bool) {
	top := math.Sin(mid) >= -1e-9
	n := len(lines)
	lh := sizePt * ptpx * 1.12 / scale
	for k, line := range lines {
		off := (float64(n-1)/2 - float64(k)) * lh
		rr := r + off
		if !top {
			rr = r - off
		}
		total := textWidth(line, sizePt, bold) / scale / rr
		a := mid + total/2
		if !top {
			a = mid - total/2
		}
		for _, ch := range line {
			w := runeWidth(ch, sizePt, bold) / scale / rr
			ac := a - w/2
			if !top {
				ac = a + w/2
			}
			if ch != ' ' {
				p := px(polar(ac, rr))
				deg := 90 - ac*180/math.Pi
				if !top {
					deg = -90 - ac*180/math.Pi
				}
				c.rotText(p.X, p.Y, deg, string(ch), sizePt, hex, bold)
			}
			if top {
				a -= w
			} else {
				a += w
			}
		}
	}
}

// fitRingLabel finds lines and a size for a name inside a ring segment
// (text ≤ 92 % of the arc). One line from maxPt down to twoAt+1, then two lines
// from twoAt down to minPt. ok=false if it does not fit at all.
func fitRingLabel(name string, arc, r, maxPt, twoAt, minPt float64, twoLines bool) ([]string, float64, bool) {
	room := 0.92 * arc * r * scale
	for pt := maxPt; pt > twoAt && pt >= minPt; pt-- {
		if textWidth(name, pt, true) <= room {
			return []string{name}, pt, true
		}
	}
	if twoLines {
		if w := strings.Fields(name); len(w) > 1 {
			best, bm := []string{}, math.MaxFloat64
			for k := 1; k < len(w); k++ {
				a, b := strings.Join(w[:k], " "), strings.Join(w[k:], " ")
				if m := math.Max(float64(len(a)), float64(len(b))); m < bm {
					bm, best = m, []string{a, b}
				}
			}
			for pt := twoAt; pt >= minPt; pt-- {
				if math.Max(textWidth(best[0], pt, true), textWidth(best[1], pt, true)) <= room {
					return best, pt, true
				}
			}
		}
	}
	for pt := math.Min(twoAt, maxPt); pt >= minPt; pt-- {
		if textWidth(name, pt, true) <= room {
			return []string{name}, pt, true
		}
	}
	return nil, 0, false
}

// ---------------- count scale ----------------

// drawScale draws a count scale along a segment: hi = angle of chord 0 (the
// clockwise start), unit = angle per chord end, n chord ends. Minor tick every
// 5, a number every 25 (every 5 in segments with fewer than 25 chords); no
// number within half an interval of the segment end (except 0).
func (c *canvas) drawScale(hi, unit float64, n int, r0 float64) {
	if n <= 0 {
		return
	}
	lo := hi - unit*float64(n)
	c.fill(annulus(lo, hi, r0, r0+0.004), "#777777", 1)
	every := 25
	if n < 25 {
		every = 5
	}
	// fewer numbers where they would touch each other (dense or narrow segments)
	spacing := func(k int) float64 { return float64(k) * unit * (r0 + 0.055) * scale }
	for _, k := range []int{5, 10, 25, 50, 100, 250, 500, 1000} {
		if k >= every && spacing(every) < 38 {
			every = k
		}
	}
	numbers := float64(n)*unit*(r0+0.055)*scale >= 28 // room for at least the "0"
	for t := 0; t <= n; t += 5 {
		a := hi - float64(t)*unit
		major := t%every == 0
		r1 := r0 + 0.017
		w := 2.0
		if major {
			r1, w = r0+0.03, 3
		}
		c.polyline([]pt{px(polar(a, r0)), px(polar(a, r1))}, "#555555", w)
		if major && numbers && (t == 0 || (float64(n-t) >= 0.5*float64(every) && float64(n-t)*unit*(r0+0.055)*scale >= 24)) {
			c.arcText(a, r0+0.055, strconv.Itoa(t), 7, "#333333", false)
		}
	}
}

// ---------------- labels outside the ring ----------------

type outLabel struct {
	lines []string
	color string
	mid   float64 // angle of the segment centre
	size  float64
	y     float64
	side  float64
	r0    float64 // radius where the leader starts (0 = outer edge of the rings)
}

func (l *outLabel) height() float64 {
	return float64(len(l.lines)) * l.size * ptpx * 1.25 / scale
}

// placeOutside spreads the labels on both sides (keeping their ring order) and
// draws them with leader lines that stay outside the ring: when a label had to
// move away from its segment, the leader first goes out horizontally to a point
// outside the circle, then vertically, then to the label. rOut = outer edge of
// the rings, yTop/yBot = limits in world units.
func (c *canvas) placeOutside(labs []*outLabel, rOut, yTop, yBot, minGap float64, routed bool) {
	for _, l := range labs {
		if l.r0 == 0 {
			l.r0 = rOut
		}
		p := polar(l.mid, l.r0+0.03)
		if !routed {
			p = polar(l.mid, l.r0+0.02)
		}
		l.side = 1
		if p.X < 0 || (math.Abs(p.X) < 1e-9 && math.Cos(l.mid) < 0) {
			l.side = -1
		}
		l.y = p.Y
	}
	extra := 0.05
	if !routed {
		extra = 0.06
	}
	gap := func(a, b *outLabel) float64 { return math.Max(minGap, (a.height()+b.height())/2+extra) }
	for _, side := range []float64{1, -1} {
		var L []*outLabel
		for _, l := range labs {
			if l.side == side {
				L = append(L, l)
			}
		}
		sort.SliceStable(L, func(i, j int) bool { return L[i].y > L[j].y })
		for j := range L {
			if j == 0 {
				L[0].y = math.Min(L[0].y, yTop-L[0].height()/2)
			} else if L[j-1].y-L[j].y < gap(L[j-1], L[j]) {
				L[j].y = L[j-1].y - gap(L[j-1], L[j])
			}
		}
		if k := len(L) - 1; k >= 0 && L[k].y-L[k].height()/2 < yBot {
			L[k].y = yBot + L[k].height()/2
			for j := k - 1; j >= 0; j-- {
				L[j].y = math.Max(L[j].y, L[j+1].y+gap(L[j], L[j+1]))
			}
		}
	}
	// leader geometry: a label that had to move away from its segment gets a
	// leader that goes out horizontally to a point outside the circle (ex), then
	// vertically, then to the label. The vertical runs are ordered so that two
	// leaders never cross: the further a segment lies from its label, the
	// further out its vertical run.
	type geo struct {
		l      *outLabel
		a0     pt
		lx, ex float64
		bend   bool
	}
	var gs []*geo
	for _, l := range labs {
		a0 := polar(l.mid, l.r0+0.01)
		minX := rOut + 0.16
		if !routed {
			a0 = polar(l.mid, l.r0)
			minX = math.Max(1.25, rOut+0.15)
		}
		lx := math.Max(math.Abs(a0.X)+0.08, minX)
		ymin := math.Min(math.Abs(a0.Y), math.Abs(l.y))
		if a0.Y*l.y < 0 {
			ymin = 0
		}
		ex := math.Max(math.Abs(a0.X)+0.04, math.Sqrt(math.Max(0, (rOut+0.06)*(rOut+0.06)-ymin*ymin)))
		g := &geo{l: l, a0: a0, lx: lx, ex: ex, bend: math.Abs(a0.Y-l.y) > 0.02 && ex > math.Abs(a0.X)+0.04}
		gs = append(gs, g)
	}
	for _, side := range []float64{1, -1} {
		for _, up := range []bool{true, false} {
			var G []*geo
			for _, g := range gs {
				if g.l.side == side && g.bend && (g.l.y > g.a0.Y) == up {
					G = append(G, g)
				}
			}
			// moving up: start with the highest segment; moving down: the lowest
			sort.SliceStable(G, func(i, j int) bool {
				if up {
					return G[i].a0.Y > G[j].a0.Y
				}
				return G[i].a0.Y < G[j].a0.Y
			})
			for k := 1; k < len(G); k++ {
				G[k].ex = math.Max(G[k].ex, G[k-1].ex+0.035)
			}
		}
	}
	for _, g := range gs {
		l, a0 := g.l, g.a0
		lx := l.side * g.lx
		if g.bend {
			if g.ex+0.05 > math.Abs(lx) {
				lx = l.side * (g.ex + 0.05)
			}
			c.polyline([]pt{px(a0), px(pt{l.side * g.ex, a0.Y}), px(pt{l.side * g.ex, l.y}), px(pt{lx, l.y})}, l.color, 4)
		} else {
			c.polyline([]pt{px(a0), px(pt{a0.X + l.side*0.04, l.y}), px(pt{lx, l.y})}, l.color, 4)
		}
		anchor := "start"
		if l.side < 0 {
			anchor = "end"
		}
		tp := px(pt{lx + l.side*0.03, l.y})
		c.text(tp.X, tp.Y, l.lines, l.size, l.color, anchor, true)
	}
}

// ---------------- outer ring colours ----------------

// ringPalette: light ring colour and dark text colour. Chosen so that they can
// be told apart from the pastel category colours.
var ringPalette = [][2]string{
	{"#C9D3DF", "#3E5370"}, // slate
	{"#E3D3BF", "#7A5A34"}, // beige
	{"#CFE3D4", "#3B6E4A"}, // sage
	{"#E2CFE0", "#6E4669"}, // mauve
	{"#DCD8D2", "#57514A"}, // warm grey
	{"#D3D8DE", "#465059"}, // cool grey
	{"#C8DEDD", "#2F6563"}, // teal grey
	{"#E8D2D0", "#7A4542"}, // rose grey
	{"#E4DDBF", "#6B5F2E"}, // sand
	{"#D9D1E6", "#54466E"}, // lilac grey
	{"#CBCBCB", "#3F3F3F"}, // grey
	{"#E6E6E6", "#4D4D4D"}, // light grey
}

// hueSat returns hue (degrees) and saturation (HSL) of a colour.
func hueSat(hex string) (float64, float64) {
	r, g, b := hexRGB(hex)
	mx, mn := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l := (mx + mn) / 2
	d := mx - mn
	if d < 1e-9 {
		return 0, 0
	}
	s := d / (1 - math.Abs(2*l-1))
	var h float64
	switch mx {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s
}

// colourDistance: how different two colours look in hue (0–180); greyish
// colours count as clearly different from any hue.
func colourDistance(a, b string) float64 {
	ha, sa := hueSat(a)
	hb, sb := hueSat(b)
	if sa < 0.18 || sb < 0.18 {
		if sa < 0.18 && sb < 0.18 {
			ra, _, _ := hexRGB(a)
			rb, _, _ := hexRGB(b)
			return 40 + 400*math.Abs(ra-rb) // two greys: by lightness
		}
		return 90
	}
	d := math.Abs(ha - hb)
	if d > 180 {
		d = 360 - d
	}
	return d
}

// ringColours picks k ring colours (ring, text) that differ as much as possible
// from the category colours in use and from each other. Same input, same choice.
func ringColours(k int, catHex []string, avoid [][2]string) [][2]string {
	var out [][2]string
	used := map[int]bool{}
	for len(out) < k {
		best, bestScore := -1, -1.0
		for i, cnd := range ringPalette {
			if used[i] {
				continue
			}
			score := 180.0
			for _, h := range catHex {
				score = math.Min(score, colourDistance(cnd[0], h))
			}
			for _, o := range append(append([][2]string{}, out...), avoid...) {
				score = math.Min(score, colourDistance(cnd[0], o[0])*1.2)
			}
			if score > bestScore+1e-9 {
				best, bestScore = i, score
			}
		}
		if best < 0 {
			used = map[int]bool{}
			continue
		}
		used[best] = true
		out = append(out, ringPalette[best])
	}
	return out
}

// legendItems draws a category legend (swatch + full name) centred at y (px)
// and returns the y below it.
func drawCatLegend(c *canvas, cats []string, cidx map[string]int, y, size float64) float64 {
	face := faceFor(size, true)
	measure := func(t string) float64 { return float64(font.MeasureString(face, t)) / 64 }
	sw, gapIn, gapOut := 60.0, 22.0, 70.0
	type item struct {
		name string
		w    float64
	}
	var rows [][]item
	var widths []float64
	cur, curW := []item{}, 0.0
	for _, cat := range cats {
		w := sw + gapIn + measure(cat)
		add := w
		if len(cur) > 0 {
			add += gapOut
		}
		if curW+add > W-600 && len(cur) > 0 {
			rows, widths = append(rows, cur), append(widths, curW)
			cur, curW, add = nil, 0, w
		}
		cur = append(cur, item{cat, w})
		curW += add
	}
	if len(cur) > 0 {
		rows, widths = append(rows, cur), append(widths, curW)
	}
	for ri, row := range rows {
		x := (W - widths[ri]) / 2
		for k, it := range row {
			if k > 0 {
				x += gapOut
			}
			c.fill([]seg{{'M', []pt{wpt(x, y-26)}}, {'L', []pt{wpt(x+sw, y-26)}}, {'L', []pt{wpt(x+sw, y+26)}}, {'L', []pt{wpt(x, y+26)}}, {'Z', nil}},
				pastel[cidx[it.name]%len(pastel)], 1)
			c.text(x+sw+gapIn, y, []string{it.name}, size, dark[cidx[it.name]%len(dark)], "start", true)
			x += it.w
		}
		y += 95
	}
	return y
}

func legendRows(cats []string, size float64) int {
	face := faceFor(size, true)
	rows, curW := 1, 0.0
	for _, cat := range cats {
		w := 60 + 22 + float64(font.MeasureString(face, cat))/64
		add := w
		if curW > 0 {
			add += 70
		}
		if curW+add > W-600 && curW > 0 {
			rows++
			curW, add = 0, w
		}
		curW += add
	}
	return rows
}

// drawColorbarAt draws the r colour bar with its top at y (px).
func drawColorbarAt(c *canvas, by float64) {
	bw, bh := 1900.0, 75.0
	bx := (W - bw) / 2
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
}
