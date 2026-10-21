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
		Title: *title, Cuts: Cuts{0.8, 0.6, 0.4}}
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
	_, full, _ := ds.Compute(&o)
	run := func(name, file, t string, opt Options) {
		links, ord, rm := ds.Compute(&opt)
		cn := CountLinks(links, ord, opt.Cuts, rm)
		fmt.Printf("%s: %d correlations (%d positive, %d negative)\n", name, cn.Total, cn.Pos, cn.Neg)
		for _, b := range cn.Bands {
			fmt.Printf("   %-28s %5d  (+%d / −%d)\n", b.Label, b.Total, b.Pos, b.Neg)
		}
		if x, err := CountsXLSX(cn); err == nil {
			os.WriteFile(file+"_counts.xlsx", x, 0644)
		}
		if len(links) == 0 {
			fmt.Println("   nothing to draw")
			return
		}
		png, svg, err := Render(links, ord, full, t)
		if err != nil {
			fmt.Println("  ", err)
			return
		}
		os.WriteFile(file+".png", png, 0644)
		os.WriteFile(file+".svg", svg, 0644)
	}
	if *series != "" {
		cats := splitList(*series)
		t := o.Title
		if t == "" {
			t = "{category}: significant correlations"
		}
		for _, c := range cats {
			opt := o
			opt.Focus = c
			opt.Exclude = append([]string{}, o.Exclude...)
			for _, x := range cats {
				if x != c {
					opt.Exclude = append(opt.Exclude, x)
				}
			}
			run(c, prefix+"_"+fileSafe(c), strings.ReplaceAll(t, "{category}", c), opt)
		}
	} else {
		t := o.Title
		if t == "" {
			t = "Significant correlations between variable categories"
		}
		run("All categories", prefix, t, o)
	}
}
