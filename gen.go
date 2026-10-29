package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

type Diagram struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Counts Counts `json:"counts"`
	Error  string `json:"error,omitempty"`
	png    []byte
	svg    []byte
	xlsx   []byte
	links  []Link
	order  []string
	colors []string
	title  string
}

type GenerateReq struct {
	Options     Options  `json:"options"`
	Mode        string   `json:"mode"` // "single", "series" (one per category) or "groups" (one per region/group)
	Series      []string `json:"series"`
	Groups      []string `json:"groups"`
	GroupLabel  string   `json:"groupLabel"` // e.g. "Region"
	SeriesApart bool     `json:"seriesExcludeOthers"`
	Prefix      string   `json:"prefix"`
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func fileSafe(s string) string {
	s = strings.Trim(unsafeChars.ReplaceAllString(s, "_"), "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// ResolvePrefix returns the output file-name prefix.
func ResolvePrefix(req GenerateReq, fileName string) string {
	prefix := fileSafe(req.Prefix)
	if prefix == "" {
		base := fileName
		for _, ext := range []string{".xlsx", ".xlsm", ".XLSX"} {
			base = strings.TrimSuffix(base, ext)
		}
		prefix = fileSafe(base) + "_chord"
	}
	return prefix
}

// Generate builds one diagram or a series. With raster=false only SVGs are
// produced; call (*Diagram).EnsurePNG later when a PNG is needed.
func Generate(ds *Dataset, req GenerateReq, fileName string, raster bool) ([]*Diagram, error) {
	o := req.Options
	if o.Alpha <= 0 {
		o.Alpha = 0.05
	}
	if o.Cuts.VeryStrong == 0 {
		o.Cuts = Cuts{0.8, 0.6, 0.4}
	}
	prefix := ResolvePrefix(req, fileName)
	_, allOrder, _ := ds.Compute(&o)
	// colours follow the ring order of the categories that are actually included
	excl := map[string]bool{}
	for _, e := range o.Exclude {
		excl[clean(e)] = true
	}
	var fullOrder []string
	for _, c := range allOrder {
		if !excl[c] {
			fullOrder = append(fullOrder, c)
		}
	}

	var diags []*Diagram
	make1 := func(name, file, title string, opt Options) {
		links, order, rm := ds.Compute(&opt)
		d := &Diagram{Name: name, File: file, Counts: CountLinks(links, order, opt.Cuts, rm),
			links: links, order: order, colors: fullOrder, title: title}
		if len(links) == 0 {
			d.Error = "No significant correlations to draw with these settings."
		} else {
			png, svg, err := Render(links, order, fullOrder, title, raster)
			if err != nil {
				d.Error = err.Error()
			}
			d.png, d.svg = png, svg
		}
		d.xlsx, _ = CountsXLSX(d.Counts, links, opt.Cuts)
		diags = append(diags, d)
	}
	title := strings.TrimSpace(o.Title)
	label := strings.TrimSpace(req.GroupLabel)
	if label == "" {
		label = "Group"
	}
	if req.Mode == "groups" {
		if len(req.Groups) == 0 {
			return nil, fmt.Errorf("Tick at least one %s for the diagrams.", strings.ToLower(label))
		}
		if title == "" {
			title = label + " {group}: significant correlations between variable categories"
		}
		for _, g := range req.Groups {
			opt := o
			opt.Group = g
			opt.WithinGroup = false
			t := strings.ReplaceAll(strings.ReplaceAll(title, "{group}", g), "{category}", g)
			make1(label+" "+g, prefix+"_"+fileSafe(label+"_"+g), t, opt)
		}
		return diags, nil
	}
	if req.Mode == "series" {
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
			make1(cat, prefix+"_"+fileSafe(cat), strings.ReplaceAll(title, "{category}", cat), opt)
		}
	} else {
		if title == "" {
			title = "Significant correlations between variable categories"
		}
		make1("All categories", prefix, strings.ReplaceAll(title, "{category}", "All categories"), o)
	}
	return diags, nil
}

// EnsurePNG rasterises the diagram if that has not been done yet.
func (d *Diagram) EnsurePNG() error {
	if d.png != nil || len(d.links) == 0 {
		return nil
	}
	png, _, err := Render(d.links, d.order, d.colors, d.title, true)
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

func ZipDiagrams(diags []*Diagram, summary []byte, summaryName string) []byte {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, b []byte) {
		if b != nil {
			f, _ := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
			f.Write(b)
		}
	}
	for _, d := range diags {
		add(d.File+".png", d.png)
		add(d.File+".svg", d.svg)
		add(d.File+"_counts.xlsx", d.xlsx)
	}
	if summary != nil {
		add(summaryName, summary)
	}
	z.Close()
	return buf.Bytes()
}
