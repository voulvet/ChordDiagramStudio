//go:build !js

// Chord Diagram Studio
//
// Double-click (or run without arguments) to open the interface in your browser.
// Command-line use is still possible:
//
//	./chordstudio [flags] data.xlsx
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func main() {
	port := flag.Int("port", 0, "port for the interface (0 = pick a free one)")
	noBrowser := flag.Bool("nobrowser", false, "do not open the browser automatically")
	rSheet := flag.String("rsheet", "", "CLI: sheet with r matrix (default first)")
	pSheet := flag.String("psheet", "", "CLI: sheet with p-values (default second)")
	alpha := flag.Float64("alpha", 0.05, "CLI: keep correlations with p <= alpha")
	exclude := flag.String("exclude", "", "CLI: comma-separated categories to leave out")
	order := flag.String("order", "", "CLI: comma-separated ring order")
	apart := flag.String("apart", "", "CLI: comma-separated keywords; categories sharing a keyword are not compared")
	merge := flag.String("merge", "", "CLI: keyword=New name[,keyword2=Name2] to combine categories")
	series := flag.String("series", "", "CLI: comma-separated categories; one diagram per category")
	title := flag.String("title", "", "CLI: figure title ({category} is replaced in series)")
	out := flag.String("out", "", "CLI: output prefix")
	variables := flag.Bool("variables", false, "CLI: row 1/column A contain variable names (category = text before -sep)")
	sep := flag.String("sep", "_", "CLI: separator between category and the rest of a variable name")
	useSheet := flag.Bool("usesheet", false, "CLI: take categories/regions from the 'Variables' sheet")
	byGroup := flag.String("bygroup", "", "CLI: one diagram per region/group: comma-separated codes, or 'all'")
	groupLabel := flag.String("grouplabel", "Region", "CLI: name of the grouping, e.g. Region")
	ungrouped := flag.String("ungrouped", "include", "CLI: variables without a region: include (in every diagram) or exclude")
	withinGroup := flag.Bool("withingroup", false, "CLI: single diagram with only correlations within the same region")
	flag.Parse()

	if flag.NArg() == 0 {
		runServer(*port, *noBrowser)
		return
	}

	in := flag.Arg(0)
	f, err := os.Open(in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	ds, err := LoadDataset(f, *rSheet, *pSheet)
	f.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	o := Options{Alpha: *alpha, Exclude: splitList(*exclude), Order: splitList(*order), Apart: splitList(*apart),
		Title: *title, Cuts: Cuts{0.8, 0.6, 0.4}, Sep: *sep, UseSheet: *useSheet, Ungrouped: *ungrouped, WithinGroup: *withinGroup}
	if *variables || *useSheet || *byGroup != "" || *withinGroup {
		o.Header = "variable"
	}
	for _, m := range splitList(*merge) {
		kv := strings.SplitN(m, "=", 2)
		r := MergeRule{Keyword: kv[0]}
		if len(kv) == 2 {
			r.Name = kv[1]
		}
		o.Merge = append(o.Merge, r)
	}
	prefix := *out
	if prefix == "" {
		prefix = strings.TrimSuffix(in, filepath.Ext(in)) + "_chord"
	}
	dir, base := filepath.Split(prefix)
	req := GenerateReq{Options: o, Mode: "single", Prefix: base, SeriesApart: true, GroupLabel: *groupLabel}
	if *series != "" {
		req.Mode, req.Series = "series", splitList(*series)
	}
	if *byGroup != "" {
		req.Mode = "groups"
		if strings.EqualFold(*byGroup, "all") {
			for _, g := range ds.Describe(&o).Groups {
				req.Groups = append(req.Groups, g.Name)
			}
		} else {
			req.Groups = splitList(*byGroup)
		}
	}
	diags, err := Generate(ds, req, filepath.Base(in), true)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, d := range diags {
		cn := d.Counts
		fmt.Printf("%s: %d correlations (%d positive, %d negative)\n", d.Name, cn.Total, cn.Pos, cn.Neg)
		for _, b := range cn.Bands {
			fmt.Printf("   %-28s %5d  (+%d / −%d)\n", b.Label, b.Total, b.Pos, b.Neg)
		}
		os.WriteFile(filepath.Join(dir, d.File+"_counts.xlsx"), d.xlsx, 0644)
		if d.Error != "" {
			fmt.Println("   nothing to draw")
			continue
		}
		os.WriteFile(filepath.Join(dir, d.File+".png"), d.png, 0644)
		os.WriteFile(filepath.Join(dir, d.File+".svg"), d.svg, 0644)
	}
	if len(diags) > 1 {
		os.WriteFile(filepath.Join(dir, ResolvePrefix(req, in)+"_summary.xlsx"), SummaryXLSX(diags, o.Cuts), 0644)
	}
}
