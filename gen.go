package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type Diagram struct {
	Name       string       `json:"name"`
	File       string       `json:"file"`
	Group      string       `json:"group,omitempty"` // several groups drawn one by one: the group of this diagram
	Counts     Counts       `json:"counts"`
	Error      string       `json:"error,omitempty"`
	Times      []TimeCounts `json:"times,omitempty"` // comparison: counts per group
	ChordWidth float64      `json:"chordWidth,omitempty"`
	png        []byte
	svg        []byte
	xlsx       []byte
	links      []Link
	times      []TimeSet
	draw       func(w float64, raster bool) ([]byte, []byte, *segTable, error)
	fit        func() float64
	width      float64 // radians (0 = automatic)
	segs       *segTable
	cuts       Cuts
	order      []string
	colors     []string
	title      string
}

type segTable struct {
	header []any
	rows   [][]any
}

type GenerateReq struct {
	Options     Options  `json:"options"`
	Mode        string   `json:"mode"` // "single", "series" (one per category) or "groups" (one per region/group)
	Series      []string `json:"series"`
	Groups      []string `json:"groups"`
	GroupLabel  string   `json:"groupLabel"` // e.g. "Region"
	Compare     bool     `json:"compare"`    // use the other loaded files (several files)
	TimeNames   []string `json:"timeNames"`  // group names of the files (older name; see FileGroups)
	Highlight   int      `json:"highlight"`  // comparison: k = draw group k in full colour, the others toned down; 0 = all in full colour
	SeriesApart bool     `json:"seriesExcludeOthers"`
	Prefix      string   `json:"prefix"`

	Style      Style    `json:"style"`
	Layout     string   `json:"layout"`     // several files: "compare" (groups on the right), "ring" (one full ring per group) or "split" (one per group, files on the right)
	FileGroups []string `json:"fileGroups"` // group of each file, e.g. Control, Control, OA, OA (files of one group are combined)
	FileLabels []string `json:"fileLabels"` // label of each file, e.g. Study A, Study B, Study A, Study B
	Regions    string   `json:"regions"`    // several files: "separate" (default: a region of one file is not the same as in another file) or "combined"
	NoRegion   string   `json:"noRegion"`   // name for variables without a region (default "No region")
}

// Run is the result of one Generate call.
type Run struct {
	Diagrams   []*Diagram     `json:"diagrams"`
	Changes    []byte         `json:"-"`
	ChangeInfo *ChangeSummary `json:"changes,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
	Prefix     string         `json:"prefix"`
	ChordWidth float64        `json:"chordWidth,omitempty"` // degrees, when one width was used for the whole run
	summary    []byte
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func fileSafe(s string) string {
	s = strings.Trim(unsafeChars.ReplaceAllString(s, "_"), "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func baseName(fileName string) string {
	base := fileName
	for _, ext := range []string{".xlsx", ".xlsm", ".XLSX", ".XLSM"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// ResolvePrefix returns the output file-name prefix.
func ResolvePrefix(req GenerateReq, fileName string) string {
	prefix := fileSafe(req.Prefix)
	if prefix == "" {
		prefix = fileSafe(baseName(fileName)) + "_chord"
	}
	return prefix
}

// Source is one Excel file. Name = its group (files of the same group are
// combined), Label = its file label (e.g. the study).
type Source struct {
	Name     string
	Label    string
	FileName string
	DS       *Dataset
}

// MakeSources attaches group names and file labels to the loaded files.
func MakeSources(names []string, dss []*Dataset, req GenerateReq) []Source {
	groups := req.FileGroups
	if len(groups) == 0 {
		groups = req.TimeNames
	}
	var out []Source
	for i, ds := range dss {
		s := Source{FileName: names[i], DS: ds}
		if i < len(groups) && strings.TrimSpace(groups[i]) != "" {
			s.Name = strings.TrimSpace(groups[i])
		} else if len(req.FileGroups) > 0 {
			s.Name = "Group"
		} else {
			s.Name = baseName(names[i])
			if s.Name == "" {
				s.Name = fmt.Sprintf("Group %d", i+1)
			}
		}
		if i < len(req.FileLabels) && strings.TrimSpace(req.FileLabels[i]) != "" {
			s.Label = strings.TrimSpace(req.FileLabels[i])
		} else {
			s.Label = baseName(names[i])
		}
		out = append(out, s)
	}
	return out
}

func timeName(names []string, i int, fileName string) string {
	if i < len(names) && strings.TrimSpace(names[i]) != "" {
		return strings.TrimSpace(names[i])
	}
	if b := baseName(fileName); b != "" {
		return b
	}
	return fmt.Sprintf("Group %d", i+1)
}

// Generate builds one diagram or a series. One file: the classic diagram (or
// the ring layout). Several files: files of the same group are combined;
// groups are compared (layout "compare") or drawn one by one ("ring", "split").
// With raster=false only SVGs are produced; call (*Diagram).EnsurePNG later.
func Generate(srcs []Source, req GenerateReq, raster bool) (*Run, error) {
	if len(srcs) == 0 {
		return nil, fmt.Errorf("No file loaded.")
	}
	o := req.Options
	if o.Alpha <= 0 {
		o.Alpha = 0.05
	}
	if o.Cuts.VeryStrong == 0 {
		o.Cuts = Cuts{0.8, 0.6, 0.4}
	}
	st := req.Style
	fixedDeg, same, err := st.fixedWidth()
	if err != nil {
		return nil, err
	}
	noRegion := strings.TrimSpace(req.NoRegion)
	if noRegion == "" {
		noRegion = "No region"
	}
	label := strings.TrimSpace(req.GroupLabel)
	if label == "" {
		label = "Group"
	}
	run := &Run{}

	// groups and files
	var groups []string
	groupSrcs := map[string][]int{}
	for i, s := range srcs {
		if _, ok := groupSrcs[s.Name]; !ok {
			groups = append(groups, s.Name)
		}
		groupSrcs[s.Name] = append(groupSrcs[s.Name], i)
	}
	var files []string
	for _, s := range srcs {
		if !contains(files, s.Label) {
			files = append(files, s.Label)
		}
	}
	combined := false // several files in one group
	for _, g := range groups {
		if len(groupSrcs[g]) > 1 {
			combined = true
		}
	}
	layout := strings.ToLower(strings.TrimSpace(req.Layout))
	if layout == "" {
		layout = "ring"
		if len(groups) > 1 {
			layout = "compare"
		}
	}
	switch layout {
	case "compare":
		if len(groups) < 2 {
			return nil, fmt.Errorf("The comparison layout needs files from at least two groups (e.g. Control and OA). Give the files different group names, or choose another layout.")
		}
	case "ring", "split":
	default:
		return nil, fmt.Errorf("unknown layout %q (compare, ring or split)", req.Layout)
	}
	// file labels matter when several files are combined, or for the split layout
	useFiles := combined || layout == "split"
	separate := !strings.EqualFold(req.Regions, "combined") && useFiles && len(files) > 1
	regionKey := func(file, code string) string {
		if separate && file != "" {
			return file + " " + code
		}
		return code
	}
	// region codes in order of appearance
	var regionCodes []string
	var regionIDs []string
	for _, s := range srcs {
		for _, g := range s.DS.Describe(&o).Groups {
			if !contains(regionCodes, g.Name) {
				regionCodes = append(regionCodes, g.Name)
			}
		}
	}
	for _, f := range files {
		for _, s := range srcs {
			if s.Label != f {
				continue
			}
			for _, g := range s.DS.Describe(&o).Groups {
				if id := regionKey(f, g.Name); !contains(regionIDs, id) {
					regionIDs = append(regionIDs, id)
				}
			}
		}
	}

	prefix := PrefixFor(srcs, req)
	hl := 0
	if layout == "compare" && req.Highlight >= 1 && req.Highlight <= len(groups) {
		hl = req.Highlight
		prefix += "_highlight_" + fileSafe(groups[hl-1])
	}
	run.Prefix = prefix

	// ring order: union over all sources; colours follow included categories
	excl := map[string]bool{}
	for _, e := range o.Exclude {
		excl[clean(e)] = true
	}
	if len(o.CatInfo) == 0 {
		for _, s := range srcs {
			if len(s.DS.Cats) > 0 {
				o.CatInfo = s.DS.Cats
				break
			}
		}
	}
	var allOrder []string
	for _, src := range srcs {
		_, ord, _ := src.DS.Compute(&o)
		for _, c := range ord {
			if !contains(allOrder, c) {
				allOrder = append(allOrder, c)
			}
		}
	}
	var fullOrder []string
	for _, c := range allOrder {
		if !excl[c] {
			fullOrder = append(fullOrder, c)
		}
	}
	catLabel, catGroup := o.catMaps()
	if st.hasRing("group") && layout == "ring" && len(catGroup) == 0 {
		return nil, fmt.Errorf("The group ring needs a group for the categories: add a \"Categories\" sheet (Category, Label, Group) or fill in the Group column of the category table.")
	}

	// links of one diagram, per group
	linksFor := func(opt Options, regionID string) (map[string][]Link, map[string]Removed) {
		byG := map[string][]Link{}
		rmG := map[string]Removed{}
		for _, s := range srcs {
			op := opt
			if regionID != "" {
				if separate {
					if !strings.HasPrefix(regionID, s.Label+" ") {
						continue
					}
					op.Group = strings.TrimPrefix(regionID, s.Label+" ")
				} else {
					op.Group = regionID
				}
			}
			links, _, rm := s.DS.Compute(&op)
			idx := map[string]int{}
			for i, c := range allOrder {
				idx[c] = i
			}
			for i := range links {
				l := &links[i]
				if idx[l.C1] > idx[l.C2] {
					l.swap()
				}
				if useFiles || len(srcs) > 1 {
					l.File = s.Label
				}
			}
			byG[s.Name] = append(byG[s.Name], links...)
			r := rmG[s.Name]
			r.Read += rm.Read
			r.Within += rm.Within
			r.Excluded += rm.Excluded
			r.Apart += rm.Apart
			r.NotSig += rm.NotSig
			r.NoP += rm.NoP
			r.OutOfFocus += rm.OutOfFocus
			r.OtherGroup += rm.OtherGroup
			rmG[s.Name] = r
		}
		return byG, rmG
	}

	var diags []*Diagram
	make1 := func(name, file, title string, opt Options, regionID string) {
		byG, rmG := linksFor(opt, regionID)
		if layout == "compare" {
			d := &Diagram{Name: name, File: file, order: allOrder, colors: fullOrder, title: title, cuts: opt.Cuts}
			var times []TimeSet
			var all []Link
			var rmAll Removed
			for gi, g := range groups {
				times = append(times, TimeSet{Name: g, Links: byG[g], Removed: rmG[g], Dim: hl > 0 && gi+1 != hl})
				d.Times = append(d.Times, TimeCounts{Name: g, Counts: CountLinks(byG[g], allOrder, opt.Cuts, rmG[g])})
				all = append(all, byG[g]...)
				rm := rmG[g]
				rmAll.Read += rm.Read
				rmAll.Within += rm.Within
				rmAll.Excluded += rm.Excluded
				rmAll.Apart += rm.Apart
				rmAll.NotSig += rm.NotSig
				rmAll.NoP += rm.NoP
				rmAll.OutOfFocus += rm.OutOfFocus
				rmAll.OtherGroup += rm.OtherGroup
			}
			d.times, d.links = times, all
			d.Counts = CountLinks(all, allOrder, opt.Cuts, rmAll)
			if len(all) == 0 {
				d.Error = "No significant correlations to draw with these settings."
			} else {
				in := &cmpInput{times: times, order: allOrder, colors: fullOrder, title: title, st: st, noRegion: noRegion, regionWord: label}
				plan, err := planCompare(in)
				if err != nil {
					d.Error = err.Error()
				} else {
					d.fit = plan.fit
					d.draw = func(w float64, raster bool) ([]byte, []byte, *segTable, error) {
						r, err := plan.render(w, raster)
						if err != nil {
							return nil, nil, nil, err
						}
						return r.png, r.svg, &segTable{r.header, r.rows}, nil
					}
				}
			}
			diags = append(diags, d)
			return
		}
		for _, g := range groups {
			links := byG[g]
			n, f, t := name, file, title
			if len(groups) > 1 {
				n, f = name+" · "+g, file+"_"+fileSafe(g)
				if strings.Contains(t, "{group name}") {
					t = strings.ReplaceAll(t, "{group name}", g)
				} else {
					t = g + ": " + t
				}
			}
			t = strings.ReplaceAll(t, "{group name}", g)
			_, order, _ := srcs[groupSrcs[g][0]].DS.Compute(&opt)
			for _, c := range allOrder { // categories of the other files of the group
				if !contains(order, c) {
					order = append(order, c)
				}
			}
			d := &Diagram{Name: n, File: f, Group: g, Counts: CountLinks(links, order, opt.Cuts, rmG[g]),
				links: links, order: order, colors: fullOrder, title: t, cuts: opt.Cuts}
			if len(groups) == 1 {
				d.Group = ""
			}
			if len(links) == 0 {
				d.Error = "No significant correlations to draw with these settings."
				diags = append(diags, d)
				continue
			}
			switch {
			case layout == "split":
				var gfiles []string
				for _, i := range groupSrcs[g] {
					if !contains(gfiles, srcs[i].Label) {
						gfiles = append(gfiles, srcs[i].Label)
					}
				}
				in := &cmpInput{times: []TimeSet{{Name: g, Links: links}}, order: order, colors: fullOrder, title: t, st: st,
					split: true, files: gfiles, regions: st.hasRing("region"), regionCodes: regionCodes, noRegion: noRegion, regionWord: label}
				plan, err := planCompare(in)
				if err != nil {
					d.Error = err.Error()
					break
				}
				d.fit = plan.fit
				d.draw = func(w float64, raster bool) ([]byte, []byte, *segTable, error) {
					r, err := plan.render(w, raster)
					if err != nil {
						return nil, nil, nil, err
					}
					return r.png, r.svg, &segTable{r.header, r.rows}, nil
				}
			case st.usesRingLayout():
				var gfiles []string
				for _, i := range groupSrcs[g] {
					if !contains(gfiles, srcs[i].Label) {
						gfiles = append(gfiles, srcs[i].Label)
					}
				}
				if !useFiles {
					gfiles = nil
				}
				in := &ringInput{links: links, order: order, colors: fullOrder, title: t, st: st, catLabel: catLabel, catGroup: catGroup,
					files: gfiles, regions: regionCodes, regionKey: func(file, code string) string {
						if st.hasRing("file") {
							return code
						}
						return regionKey(file, code)
					}, noRegion: noRegion, regionWord: label}
				plan, err := planRing(in)
				if err != nil {
					d.Error = err.Error()
					break
				}
				d.fit = plan.fit
				d.draw = func(w float64, raster bool) ([]byte, []byte, *segTable, error) {
					r, err := plan.render(w, raster)
					if err != nil {
						return nil, nil, nil, err
					}
					return r.png, r.svg, &segTable{r.header, r.rows}, nil
				}
			default:
				lp := st.labelPt(12)
				wn := st.labelWrap(lp)
				d.fit = func() float64 { return classicFit(links, order) }
				d.draw = func(w float64, raster bool) ([]byte, []byte, *segTable, error) {
					png, svg, err := RenderClassic(links, order, fullOrder, t, w, lp, wn, raster)
					return png, svg, nil, err
				}
			}
			diags = append(diags, d)
		}
	}

	title := strings.TrimSpace(o.Title)
	switch req.Mode {
	case "groups":
		if len(req.Groups) == 0 {
			return nil, fmt.Errorf("Tick at least one %s for the diagrams.", strings.ToLower(label))
		}
		if title == "" {
			title = label + " {group}: significant correlations between variable categories"
		}
		dropLabel := false
		for _, g := range req.Groups {
			if strings.Contains(strings.ToLower(g), strings.ToLower(label)) {
				dropLabel = true
			}
		}
		for _, g := range req.Groups {
			opt := o
			opt.WithinGroup = false
			gname := label + " " + g
			if dropLabel { // group names already say what they are, e.g. "top region"
				gname = strings.ToUpper(g[:1]) + g[1:]
			}
			t := strings.ReplaceAll(strings.ReplaceAll(title, "{group}", g), "{category}", g)
			if strings.HasPrefix(title, label+" {group}") && gname != label+" "+g {
				t = gname + strings.TrimPrefix(strings.ReplaceAll(title, "{category}", g), label+" {group}")
			}
			make1(gname, prefix+"_"+fileSafe(gname), t, opt, g)
		}
	case "series":
		if len(req.Series) == 0 {
			return nil, fmt.Errorf("Tick at least one category for the series.")
		}
		if title == "" {
			title = "{category}: significant correlations"
		}
		for _, cat := range req.Series {
			opt := o
			opt.Focus = cat
			opt.Exclude = append([]string{}, o.Exclude...)
			if req.SeriesApart {
				for _, other := range req.Series {
					if other != cat {
						opt.Exclude = append(opt.Exclude, other)
					}
				}
			}
			make1(cat, prefix+"_"+fileSafe(cat), strings.ReplaceAll(title, "{category}", cat), opt, "")
		}
	default:
		if title == "" {
			title = "Significant correlations between variable categories"
			if layout == "compare" {
				title = "Significant correlations between variable categories: comparison of groups"
			}
		}
		make1("All categories", prefix, strings.ReplaceAll(title, "{category}", "All categories"), o, "")
	}

	// chord width: automatic, fixed, or the widest that fits every diagram of the run
	w := fixedDeg * math.Pi / 180
	if same {
		minFit := math.Inf(1)
		for _, d := range diags {
			if d.fit != nil && d.Error == "" {
				minFit = math.Min(minFit, d.fit())
			}
		}
		if !math.IsInf(minFit, 1) {
			w = floorDeg(minFit) * math.Pi / 180
			run.ChordWidth = floorDeg(minFit)
		}
	}
	for _, d := range diags {
		if d.draw == nil || d.Error != "" {
			continue
		}
		d.width = w
		if w > 0 {
			d.ChordWidth = math.Round(w*180/math.Pi*1000) / 1000
		}
		png, svg, segs, err := d.draw(w, raster)
		if err != nil {
			d.Error = err.Error()
			continue
		}
		d.png, d.svg, d.segs = png, svg, segs
	}

	// warnings (data hygiene)
	var dss []*Dataset
	for _, s := range srcs {
		dss = append(dss, s.DS)
	}
	run.Warnings = NameWarnings(dss)

	for _, d := range diags {
		if d.times != nil {
			d.xlsx = CompareXLSX(d, o.Cuts, d.segs, run.Warnings)
		} else {
			d.xlsx, _ = CountsXLSX(d.Counts, d.links, o.Cuts, d.segs, run.Warnings)
		}
	}

	// changes between two groups
	if len(groups) == 2 {
		var ci changeInput
		for k := 0; k < 2; k++ {
			ci.names[k] = groups[k]
			for _, i := range groupSrcs[groups[k]] {
				ci.srcs[k] = append(ci.srcs[k], srcs[i])
				lab := ""
				if combined {
					lab = srcs[i].Label
				}
				ci.labels[k] = append(ci.labels[k], lab)
			}
		}
		ci.opt, ci.noRegion, ci.ungrouped, ci.cuts, ci.separate = o, noRegion, o.Ungrouped, o.Cuts, separate && combined
		ci.regionKey = func(file, code string) string {
			if ci.separate && file != "" {
				return file + " " + code
			}
			return code
		}
		if o.Header == "variable" {
			for _, f := range files {
				for _, s := range srcs {
					if s.Label != f {
						continue
					}
					for _, g := range s.DS.Describe(&o).Groups {
						lab := ""
						if combined {
							lab = f
						}
						if id := ci.regionKey(lab, g.Name); !contains(ci.regions, id) {
							ci.regions = append(ci.regions, id)
						}
					}
				}
			}
		}
		run.ChangeInfo, run.Changes = ComputeChanges(ci)
	}
	run.Diagrams = diags
	if len(diags) > 1 {
		run.summary = SummaryXLSX(diags, o.Cuts)
	}
	return run, nil
}

// RegionIDs lists the regions for "one diagram per region" over all files:
// codes, or "file code" when the regions of different files are kept separate.
func RegionIDs(srcs []Source, req GenerateReq) []NameCount {
	o := req.Options
	var groups, files []string
	gcount := map[string]int{}
	for _, s := range srcs {
		if !contains(groups, s.Name) {
			groups = append(groups, s.Name)
		}
		gcount[s.Name]++
		if !contains(files, s.Label) {
			files = append(files, s.Label)
		}
	}
	combined := false
	for _, g := range groups {
		if gcount[g] > 1 {
			combined = true
		}
	}
	useFiles := combined || strings.EqualFold(req.Layout, "split")
	separate := !strings.EqualFold(req.Regions, "combined") && useFiles && len(files) > 1
	var out []NameCount
	idx := map[string]int{}
	for _, f := range files {
		for _, s := range srcs {
			if s.Label != f {
				continue
			}
			for _, g := range s.DS.Describe(&o).Groups {
				id := g.Name
				if separate {
					id = f + " " + g.Name
				}
				if k, ok := idx[id]; ok {
					if g.Count > out[k].Count {
						out[k].Count = g.Count
					}
					continue
				}
				idx[id] = len(out)
				out = append(out, NameCount{id, g.Count})
			}
		}
	}
	return out
}

// EnsurePNG rasterises the diagram if that has not been done yet.
func (d *Diagram) EnsurePNG() error {
	if d.png != nil || d.Error != "" || d.draw == nil {
		return nil
	}
	png, _, _, err := d.draw(d.width, true)
	if err != nil {
		return err
	}
	d.png = png
	return nil
}

// SummaryXLSX compares several diagrams (a series or one per group).
func SummaryXLSX(diags []*Diagram, cuts Cuts) []byte {
	if cuts.VeryStrong == 0 {
		cuts = Cuts{0.8, 0.6, 0.4}
	}
	if len(diags) > 0 && diags[0].Times != nil {
		return CompareSummaryXLSX(diags, cuts)
	}
	x := excelize.NewFile()
	defer x.Close()
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	short := []string{"Very strong", "Strong", "Moderate", "Weak"}
	sh := "Summary"
	x.SetSheetName("Sheet1", sh)
	h := []any{"Diagram", "Total", "Positive", "Negative"}
	for _, b := range short {
		h = append(h, b+" +", b+" −")
	}
	x.SetSheetRow(sh, "A1", &h)
	for i, d := range diags {
		c := d.Counts
		row := []any{d.Name, c.Total, c.Pos, c.Neg}
		for _, b := range c.Bands {
			row = append(row, b.Pos, b.Neg)
		}
		x.SetSheetRow(sh, fmt.Sprintf("A%d", i+2), &row)
	}
	x.SetRowStyle(sh, 1, 1, bold)
	x.SetColWidth(sh, "A", "A", 30)
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	n := len(diags) + 3
	x.SetSheetRow(sh, fmt.Sprintf("A%d", n), &[]any{"Strength by |r|: very strong ≥ " + f(cuts.VeryStrong) + ", strong ≥ " + f(cuts.Strong) + ", moderate ≥ " + f(cuts.Moderate) + ", weak below."})
	if diags[0].ChordWidth > 0 {
		x.SetSheetRow(sh, fmt.Sprintf("A%d", n+1), &[]any{fmt.Sprintf("Chord width: %.3f° in every diagram.", diags[0].ChordWidth)})
	}

	pair := "By category pair"
	x.NewSheet(pair)
	ph := append([]any{"Diagram", "Category 1", "Category 2"}, h[1:]...)
	x.SetSheetRow(pair, "A1", &ph)
	r := 2
	for _, d := range diags {
		for _, p := range d.Counts.Pairs {
			row := []any{d.Name, p.A, p.B, p.Total, p.Pos, p.Neg}
			for _, b := range p.Bands {
				row = append(row, b.Pos, b.Neg)
			}
			x.SetSheetRow(pair, fmt.Sprintf("A%d", r), &row)
			r++
		}
	}
	x.SetRowStyle(pair, 1, 1, bold)
	x.SetColWidth(pair, "A", "C", 26)

	cat := "By category"
	x.NewSheet(cat)
	ch := append([]any{"Diagram", "Category"}, h[1:]...)
	x.SetSheetRow(cat, "A1", &ch)
	r = 2
	for _, d := range diags {
		for _, p := range d.Counts.PerCat {
			row := []any{d.Name, p.A, p.Total, p.Pos, p.Neg}
			for _, b := range p.Bands {
				row = append(row, b.Pos, b.Neg)
			}
			x.SetSheetRow(cat, fmt.Sprintf("A%d", r), &row)
			r++
		}
	}
	x.SetRowStyle(cat, 1, 1, bold)
	x.SetColWidth(cat, "A", "B", 26)

	for _, d := range diags {
		writeLinks(x, "All correlations", d.Name, d.links, cuts, bold)
	}
	var buf bytes.Buffer
	x.Write(&buf)
	return buf.Bytes()
}

// ZipRun packs every file of a run.
func ZipRun(run *Run) []byte {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, b []byte) {
		if b != nil {
			f, _ := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
			f.Write(b)
		}
	}
	for _, d := range run.Diagrams {
		add(d.File+".png", d.png)
		add(d.File+".svg", d.svg)
		add(d.File+"_counts.xlsx", d.xlsx)
	}
	add(run.Prefix+"_summary.xlsx", run.summary)
	add(run.Prefix+"_changes.xlsx", run.Changes)
	z.Close()
	return buf.Bytes()
}

// PrefixFor returns the output prefix for one file or several files.
func PrefixFor(srcs []Source, req GenerateReq) string {
	if len(srcs) > 1 && strings.TrimSpace(req.Prefix) == "" {
		return ResolvePrefix(GenerateReq{}, srcs[0].FileName) + "_groups"
	}
	return ResolvePrefix(req, srcs[0].FileName)
}

// LabelCheck reports whether two files contain the same variables.
type LabelCheck struct {
	Same      bool     `json:"same"`
	N1        int      `json:"n1"`
	N2        int      `json:"n2"`
	Only1     []string `json:"only1"`
	Only2     []string `json:"only2"`
	OnlyCount [2]int   `json:"onlyCount"`
}

// CompareLabels compares the variables of two files (names matched without
// regard to case, spaces and underscores).
func CompareLabels(a, b *Dataset) LabelCheck {
	in := func(ls []string) map[string]bool {
		m := map[string]bool{}
		for _, l := range ls {
			m[normKey(l)] = true
		}
		return m
	}
	ma, mb := in(a.Labels), in(b.Labels)
	c := LabelCheck{N1: len(a.Labels), N2: len(b.Labels)}
	for _, l := range a.Labels {
		if !mb[normKey(l)] {
			c.OnlyCount[0]++
			if len(c.Only1) < 8 {
				c.Only1 = append(c.Only1, l)
			}
		}
	}
	for _, l := range b.Labels {
		if !ma[normKey(l)] {
			c.OnlyCount[1]++
			if len(c.Only2) < 8 {
				c.Only2 = append(c.Only2, l)
			}
		}
	}
	c.Same = c.OnlyCount[0] == 0 && c.OnlyCount[1] == 0
	return c
}

// sheetIfPresent returns name if the workbook has a sheet with that name, else "".
func sheetIfPresent(data []byte, name string) string {
	data, _ = repairSheetNames(data)
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return ""
	}
	defer f.Close()
	for _, s := range f.GetSheetList() {
		if s == name {
			return name
		}
	}
	return ""
}
