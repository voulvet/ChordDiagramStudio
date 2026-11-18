package main

// Correlations that change between two groups (e.g. Control -> OA):
// appeared (significant only in group 2), disappeared (only in group 1) and
// sign change (significant in both, opposite sign). Variables are matched by
// name (case, spaces and underscores do not matter) within the same file label.

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

type candPair struct {
	va, vb, ca, cb, ga, gb string
	r, p                   float64
	hasP                   bool
}

// candidatePairs: every correlation between different categories that passes
// the category filters (exclude, combine, keep apart), significant or not.
func (ds *Dataset) candidatePairs(o *Options) []candPair {
	excl := map[string]bool{}
	for _, e := range o.Exclude {
		excl[clean(e)] = true
	}
	var out []candPair
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
			a, b := o.mapCat(a0), o.mapCat(b0)
			if excl[a] || excl[b] || a == b || sameApartGroup(a, b, o.Apart) {
				continue
			}
			p, okp := parseNum(ps)
			out = append(out, candPair{va, vb, a, b, ga, gb, r, p, okp})
		}
	}
	return out
}

type ChangeRow struct {
	Change     string
	Strength   float64
	V1, C1     string
	V2, C2     string
	Region     string
	File       string
	R1, P1     float64
	R2, P2     float64
	Has1, Has2 bool
	none       bool // both variables without a region
}

type ChangeSummary struct {
	Group1, Group2 string
	Appeared       int  `json:"appeared"`
	Disappeared    int  `json:"disappeared"`
	SignPosNeg     int  `json:"signPosNeg"`
	SignNegPos     int  `json:"signNegPos"`
	Unchanged      int  `json:"unchanged"`
	Sig1, Sig2     int  // significant in group 1 / group 2
	OK             bool `json:"ok"`
}

type changeInput struct {
	srcs      [2][]Source // files of group 1 and group 2
	names     [2]string
	labels    [2][]string // file label of each source used for matching ("" = single file per group)
	opt       Options
	regionKey func(file, code string) string
	noRegion  string
	regions   []string // region identities in order (for the region sheets)
	separate  bool     // regions of different files are different regions (identity "file code")
	ungrouped string
	cuts      Cuts
}

type pairVal struct {
	r, p    float64
	sig     bool
	present bool
	va, vb  string
	ca, cb  string
	ga, gb  string
}

// ComputeChanges builds the change list and the workbook.
func ComputeChanges(in changeInput) (*ChangeSummary, []byte) {
	o := in.opt
	o.Focus, o.Group, o.WithinGroup = "", "", false
	vals := [2]map[string]*pairVal{{}, {}}
	var keys []string
	seen := map[string]bool{}
	fileOf := map[string]string{}
	for g := 0; g < 2; g++ {
		for si, s := range in.srcs[g] {
			lab := in.labels[g][si]
			for _, c := range s.DS.candidatePairs(&o) {
				ka, kb := normKey(c.va), normKey(c.vb)
				va, vb, ca, cb, ga, gb := c.va, c.vb, c.ca, c.cb, c.ga, c.gb
				if kb < ka {
					ka, kb = kb, ka
					va, vb, ca, cb, ga, gb = vb, va, cb, ca, gb, ga
				}
				k := lab + "\x00" + ka + "\x00" + kb
				vals[g][k] = &pairVal{r: c.r, p: c.p, sig: c.hasP && c.p <= o.Alpha, present: true, va: va, vb: vb, ca: ca, cb: cb, ga: ga, gb: gb}
				if !seen[k] {
					seen[k] = true
					keys = append(keys, k)
					fileOf[k] = lab
				}
			}
		}
	}
	regionOf := func(file string, v *pairVal) string {
		a, b := v.ga, v.gb
		key := func(c string) string {
			if in.regionKey != nil {
				return in.regionKey(file, c)
			}
			return c
		}
		switch {
		case a == "" && b == "":
			if in.separate && file != "" {
				return file + " " + in.noRegion + " (all regions)"
			}
			return in.noRegion + " (all regions)"
		case a == "" || a == b:
			return key(b)
		case b == "":
			return key(a)
		}
		return fmt.Sprintf("Across regions (%s–%s)", key(a), key(b))
	}
	// a correlation between two variables without a region counts in every
	// region (of its own file when the regions of different files are separate)
	inRegion := func(rg, R, file string, none bool) bool {
		if rg == R {
			return true
		}
		if in.ungrouped == "exclude" || !none {
			return false
		}
		return !in.separate || file == "" || strings.HasPrefix(R, file+" ")
	}
	sum := &ChangeSummary{Group1: in.names[0], Group2: in.names[1]}
	var rows []ChangeRow
	type regd struct {
		rg, file string
		none     bool
	}
	var unchangedReg, sig1Reg, sig2Reg []regd
	for _, k := range keys {
		a, b := vals[0][k], vals[1][k]
		ref := a
		if ref == nil {
			ref = b
		}
		s1 := a != nil && a.sig
		s2 := b != nil && b.sig
		rg := regionOf(fileOf[k], ref)
		rd := regd{rg, fileOf[k], ref.ga == "" && ref.gb == ""}
		if s1 {
			sum.Sig1++
			sig1Reg = append(sig1Reg, rd)
		}
		if s2 {
			sum.Sig2++
			sig2Reg = append(sig2Reg, rd)
		}
		row := ChangeRow{V1: ref.va, C1: ref.ca, V2: ref.vb, C2: ref.cb, Region: rg, File: fileOf[k], none: rd.none}
		if a != nil {
			row.R1, row.P1, row.Has1 = a.r, a.p, true
		}
		if b != nil {
			row.R2, row.P2, row.Has2 = b.r, b.p, true
		}
		switch {
		case s1 && s2 && (a.r > 0) == (b.r > 0):
			sum.Unchanged++
			unchangedReg = append(unchangedReg, rd)
			continue
		case s1 && s2:
			if a.r > 0 {
				row.Change = "Sign change positive → negative"
				sum.SignPosNeg++
			} else {
				row.Change = "Sign change negative → positive"
				sum.SignNegPos++
			}
			row.Strength = math.Max(math.Abs(a.r), math.Abs(b.r))
		case s2:
			row.Change = "Appeared in " + in.names[1]
			row.Strength = math.Abs(b.r)
			sum.Appeared++
		case s1:
			row.Change = "Disappeared in " + in.names[1]
			row.Strength = math.Abs(a.r)
			sum.Disappeared++
		default:
			continue
		}
		rows = append(rows, row)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Strength != rows[j].Strength {
			return rows[i].Strength > rows[j].Strength
		}
		return rows[i].Change < rows[j].Change
	})
	sign := sum.SignPosNeg + sum.SignNegPos
	sum.OK = sum.Appeared+sign+sum.Unchanged == sum.Sig2 && sum.Disappeared+sign+sum.Unchanged == sum.Sig1

	// ---------------- workbook ----------------
	x := excelize.NewFile()
	defer x.Close()
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	title, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13}})
	hdr, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	fill := func(hex string) int {
		s, _ := x.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{hex}}})
		return s
	}
	num := func(hex string) int {
		f := "0.000"
		s, _ := x.NewStyle(&excelize.Style{Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{hex}}, CustomNumFmt: &f})
		return s
	}
	colour := func(ch string) string {
		switch {
		case strings.HasPrefix(ch, "Appeared"):
			return "FDE2DF"
		case strings.HasPrefix(ch, "Disappeared"):
			return "DCE6F5"
		}
		return "FFF2CC"
	}
	g1, g2 := in.names[0], in.names[1]
	sh := "Summary"
	x.SetSheetName("Sheet1", sh)
	put := func(sheet string, row int, v ...any) {
		x.SetSheetRow(sheet, "A"+strconv.Itoa(row), &v)
	}
	put(sh, 1, fmt.Sprintf("Correlations that changed from %s to %s", g1, g2))
	x.SetCellStyle(sh, "A1", "A1", title)
	f2 := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	put(sh, 2, fmt.Sprintf("Significant: p ≤ %s. Only correlations between different categories (excluded categories left out). Strength = |r| of the significant correlation (for a sign change, the larger |r|).", strconv.FormatFloat(o.Alpha, 'f', -1, 64)))
	put(sh, 3, "\"Disappeared\" only means no longer significant (p > threshold); r may still be large in either direction.")
	put(sh, 4, "Regions: variables without a region count in every region, as in the per-region diagrams. Strength bands by |r|: very strong ≥ "+f2(in.cuts.VeryStrong)+", strong ≥ "+f2(in.cuts.Strong)+", moderate ≥ "+f2(in.cuts.Moderate)+", weak below.")
	head := []any{"Change", "All regions together"}
	for _, R := range in.regions {
		head = append(head, R)
	}
	put(sh, 6, head...)
	x.SetRowStyle(sh, 6, 6, bold)
	countIn := func(pred func(ChangeRow) bool, R string) int {
		n := 0
		for _, r := range rows {
			if pred(r) && (R == "" || inRegion(r.Region, R, r.File, r.none)) {
				n++
			}
		}
		return n
	}
	types := []string{"Appeared in " + g2, "Disappeared in " + g2, "Sign change positive → negative", "Sign change negative → positive"}
	rn := 7
	for _, t := range types {
		v := []any{t, countIn(func(r ChangeRow) bool { return r.Change == t }, "")}
		for _, R := range in.regions {
			v = append(v, countIn(func(r ChangeRow) bool { return r.Change == t }, R))
		}
		put(sh, rn, v...)
		rn++
	}
	v := []any{"Total changed", len(rows)}
	for _, R := range in.regions {
		v = append(v, countIn(func(ChangeRow) bool { return true }, R))
	}
	put(sh, rn, v...)
	x.SetRowStyle(sh, rn, rn, bold)
	rn++
	cnt := func(regs []regd, R string) int {
		if R == "" {
			return len(regs)
		}
		n := 0
		for _, x := range regs {
			if inRegion(x.rg, R, x.file, x.none) {
				n++
			}
		}
		return n
	}
	addCount := func(label string, regs []regd) {
		v := []any{label, cnt(regs, "")}
		for _, R := range in.regions {
			v = append(v, cnt(regs, R))
		}
		put(sh, rn, v...)
		rn++
	}
	addCount("Unchanged (significant in both, same sign)", unchangedReg)
	rn++
	addCount("Significant in "+g1, sig1Reg)
	addCount("Significant in "+g2, sig2Reg)
	// check: appeared + sign change + unchanged = group 2; disappeared + sign change + unchanged = group 1
	check := []any{"Check (appeared + sign change + unchanged = " + g2 + "; disappeared + sign change + unchanged = " + g1 + ")"}
	allOK := true
	for _, R := range append([]string{""}, in.regions...) {
		isT := func(p string) func(ChangeRow) bool {
			return func(r ChangeRow) bool { return strings.HasPrefix(r.Change, p) }
		}
		ap, di, sg := countIn(isT("Appeared"), R), countIn(isT("Disappeared"), R), countIn(isT("Sign change"), R)
		un := cnt(unchangedReg, R)
		ok := ap+sg+un == cnt(sig2Reg, R) && di+sg+un == cnt(sig1Reg, R)
		allOK = allOK && ok
		if ok {
			check = append(check, "OK")
		} else {
			check = append(check, "MISMATCH")
		}
	}
	sum.OK = sum.OK && allOK
	put(sh, rn, check...)
	x.SetColWidth(sh, "A", "A", 48)
	if n := len(in.regions) + 2; n >= 2 {
		last, _ := excelize.ColumnNumberToName(n)
		x.SetColWidth(sh, "B", last, 18)
	}
	H := []any{"Rank", "Change", "Strength (|r|)", "Strength band", "Variable 1", "Category 1", "Variable 2", "Category 2", "Region"}
	multiFile := false
	for _, r := range rows {
		if r.File != "" {
			multiFile = true
		}
	}
	if multiFile {
		H = append(H, "File")
	}
	H = append(H, "r "+g1, "p "+g1, "r "+g2, "p "+g2, "Δr ("+g2+" − "+g1+")")
	sheet := func(name string, list []ChangeRow) {
		x.NewSheet(name)
		x.SetSheetRow(name, "A1", &H)
		lastCol, _ := excelize.ColumnNumberToName(len(H))
		x.SetCellStyle(name, "A1", lastCol+"1", hdr)
		for k, r := range list {
			row := []any{k + 1, r.Change, math.Round(r.Strength*1000) / 1000, strengthName(r.Strength, in.cuts), r.V1, r.C1, r.V2, r.C2, r.Region}
			if multiFile {
				row = append(row, r.File)
			}
			opt := func(ok bool, v float64) any {
				if !ok || math.IsNaN(v) {
					return nil
				}
				return v
			}
			row = append(row, opt(r.Has1, r.R1), opt(r.Has1, r.P1), opt(r.Has2, r.R2), opt(r.Has2, r.P2))
			if r.Has1 && r.Has2 {
				row = append(row, r.R2-r.R1)
			} else {
				row = append(row, nil)
			}
			cellA := "A" + strconv.Itoa(k+2)
			x.SetSheetRow(name, cellA, &row)
			col := colour(r.Change)
			numStart, _ := excelize.ColumnNumberToName(len(H) - 4)
			x.SetCellStyle(name, cellA, numStart+strconv.Itoa(k+2), fill(col))
			x.SetCellStyle(name, numStart+strconv.Itoa(k+2), lastCol+strconv.Itoa(k+2), num(col))
		}
		widths := []float64{6, 32, 11, 12, 40, 30, 40, 30, 26}
		for i, w := range widths {
			c, _ := excelize.ColumnNumberToName(i + 1)
			x.SetColWidth(name, c, c, w)
		}
		x.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
		x.AutoFilter(name, "A1:"+lastCol+strconv.Itoa(len(list)+1), nil)
	}
	filter := func(p func(ChangeRow) bool) []ChangeRow {
		var out []ChangeRow
		for _, r := range rows {
			if p(r) {
				out = append(out, r)
			}
		}
		return out
	}
	sheet("All changes", rows)
	sheet("Appeared", filter(func(r ChangeRow) bool { return strings.HasPrefix(r.Change, "Appeared") }))
	sheet("Disappeared", filter(func(r ChangeRow) bool { return strings.HasPrefix(r.Change, "Disappeared") }))
	sheet("Sign change", filter(func(r ChangeRow) bool { return strings.HasPrefix(r.Change, "Sign") }))
	used := map[string]bool{}
	for _, R := range in.regions {
		name := sheetSafe("Region "+R, used)
		sheet(name, filter(func(r ChangeRow) bool {
			return inRegion(r.Region, R, r.File, r.none)
		}))
	}
	var buf bytes.Buffer
	x.Write(&buf)
	return sum, buf.Bytes()
}

// sheetSafe makes a valid, unique sheet name (≤ 31 characters, no : \ / ? * [ ]).
func sheetSafe(s string, used map[string]bool) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return '-'
		}
		return r
	}, s)
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	base := s
	for k := 2; used[strings.ToLower(s)]; k++ {
		suf := fmt.Sprintf(" (%d)", k)
		r := []rune(base)
		if len(r)+len(suf) > 31 {
			r = r[:31-len(suf)]
		}
		s = string(r) + suf
	}
	used[strings.ToLower(s)] = true
	return s
}
