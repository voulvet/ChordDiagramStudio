package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"
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
	Mode        string   `json:"mode"` // "single" or "series"
	Series      []string `json:"series"`
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
	_, fullOrder, _ := ds.Compute(&o)

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
		d.xlsx, _ = CountsXLSX(d.Counts)
		diags = append(diags, d)
	}
	title := strings.TrimSpace(o.Title)
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

func ZipDiagrams(diags []*Diagram) []byte {
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
	z.Close()
	return buf.Bytes()
}
