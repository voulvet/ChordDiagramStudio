//go:build !js

// Chord Diagram Studio
//
// Double-click (or run without arguments) to open the interface in your browser.
// Command-line use is still possible:
//
//	./chordstudio [flags] data.xlsx [more.xlsx ...]
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// splitKeep splits a comma-separated list keeping empty entries (one per file).
func splitKeep(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, x := range strings.Split(s, ",") {
		out = append(out, strings.TrimSpace(x))
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
	strength := flag.String("strength", "", "CLI: strength bands to draw, any of \"very strong,strong,moderate,weak\" (default: all)")
	out := flag.String("out", "", "CLI: folder for the output files (default: the folder of the first Excel file); file names are automatic")
	variables := flag.Bool("variables", false, "CLI: row 1/column A contain variable names (category = text before -sep)")
	sep := flag.String("sep", "_", "CLI: separator between category and the rest of a variable name")
	useSheet := flag.Bool("usesheet", false, "CLI: take categories/regions from the 'Variables' sheet")
	byGroup := flag.String("bygroup", "", "CLI: one diagram per region/group: comma-separated codes, or 'all'")
	groupLabel := flag.String("grouplabel", "Region", "CLI: name of the grouping, e.g. Region")
	ungrouped := flag.String("ungrouped", "include", "CLI: variables without a region: include (in every diagram) or exclude")
	withinGroup := flag.Bool("withingroup", false, "CLI: single diagram with only correlations within the same region")
	noRegion := flag.String("noregion", "No region", "CLI: name shown for variables without a region (rings, tables)")
	// several files
	compare := flag.String("compare", "", "CLI: another Excel file (group or time-point 2) to compare with the first; same as giving two files")
	timeNames := flag.String("timenames", "", "CLI: names of the groups of the two files, e.g. \"Week 4,Week 12\" (same as -groupnames)")
	groupNames := flag.String("groupnames", "", "CLI: group of each file, e.g. \"Control,Control,OA,OA\"; files of the same group are combined")
	fileLabels := flag.String("filelabels", "", "CLI: label of each file, e.g. \"Study A,Study B,Study A,Study B\" (default: file name)")
	layout := flag.String("layout", "", "CLI: several files: compare (groups on the right), ring (one full ring per group) or split (one per group, files on the right)")
	regions := flag.String("regions", "separate", "CLI: several files: regions of different files separate or combined")
	highlight := flag.String("highlight", "", "CLI: comparison: group to highlight (number or name); the other groups are toned down")
	// appearance
	chordWidth := flag.String("chordwidth", "auto", "CLI: chord width: auto, same (one width for every diagram of the run) or degrees, e.g. 0.2")
	labelSize := flag.Float64("labelsize", 0, "CLI: category label size in pt (0 = automatic: 12, or 21 in the comparison layouts)")
	labelWrap := flag.Int("labelwrap", 0, "CLI: characters per label line (0 = automatic)")
	scaleOn := flag.Bool("scale", false, "CLI: count scale on the rings (segments then proportional to the number of chords)")
	segments := flag.String("segments", "auto", "CLI: segment sizes: auto, equal or proportional")
	rings := flag.String("rings", "", "CLI: extra rings, any of file,region,group (ring layout: outer rings; split layout: region)")
	ringLabels := flag.String("ringlabels", "auto", "CLI: names inside the rings (inside) or outside with leader lines (outside); auto = inside when there are extra rings")
	groupWord := flag.String("groupword", "group", "CLI: what the Group column of the Categories sheet means, e.g. tissue (legend text)")
	fileWord := flag.String("fileword", "file", "CLI: what the files are, e.g. study (legend text)")
	flag.Parse()

	if flag.NArg() == 0 {
		runServer(*port, *noBrowser)
		return
	}
	die := func(a ...any) {
		fmt.Fprintln(os.Stderr, a...)
		os.Exit(1)
	}
	paths := flag.Args()
	if *compare != "" {
		paths = append(paths, *compare)
	}
	var dss []*Dataset
	var names []string
	for i, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			die(err)
		}
		rs, ps := *rSheet, *pSheet
		if i > 0 && dss[0] != nil {
			b, _ := os.ReadFile(p)
			if rs == "" {
				rs = sheetIfPresent(b, dss[0].RSheet)
			}
			if ps == "" {
				ps = sheetIfPresent(b, dss[0].PSheet)
			}
		}
		ds, err := LoadDataset(f, rs, ps)
		f.Close()
		if err != nil {
			die(p+":", err)
		}
		dss = append(dss, ds)
		names = append(names, filepath.Base(p))
	}
	ds := dss[0]
	o := Options{Alpha: *alpha, Exclude: splitList(*exclude), Order: splitList(*order), Strength: splitList(*strength),
		Sep: *sep, UseSheet: *useSheet, Ungrouped: *ungrouped, WithinGroup: *withinGroup}
	if err := validStrength(o.Strength); err != nil {
		die(err)
	}
	if *variables || *useSheet || *byGroup != "" || *withinGroup || ds.LooksVariables {
		o.Header = "variable"
	}
	dir := *out
	if dir == "" {
		dir = filepath.Dir(paths[0])
	} else if err := os.MkdirAll(dir, 0755); err != nil {
		die(err)
	}
	req := GenerateReq{Options: o, Mode: "single", GroupLabel: *groupLabel,
		Layout: *layout, Regions: *regions, NoRegion: *noRegion, Compare: len(paths) > 1,
		Style: Style{ChordWidth: *chordWidth, LabelSize: *labelSize, LabelWrap: *labelWrap, Scale: *scaleOn, Segments: *segments,
			Rings: splitList(*rings), RingLabels: *ringLabels, GroupWord: *groupWord, FileWord: *fileWord}}
	req.FileGroups = splitKeep(*groupNames)
	if req.FileGroups == nil {
		req.TimeNames = splitKeep(*timeNames)
	}
	req.FileLabels = splitKeep(*fileLabels)
	if len(req.FileGroups) > 0 && len(req.FileGroups) != len(paths) {
		die(fmt.Sprintf("-groupnames: %d names for %d files", len(req.FileGroups), len(paths)))
	}
	if len(req.FileLabels) > 0 && len(req.FileLabels) != len(paths) {
		die(fmt.Sprintf("-filelabels: %d labels for %d files", len(req.FileLabels), len(paths)))
	}
	srcs := MakeSources(names, dss, req)
	for i := 1; i < len(dss); i++ {
		if chk := CompareLabels(dss[0], dss[i]); !chk.Same && srcs[i].Label == srcs[0].Label {
			fmt.Fprintf(os.Stderr, "warning: %s and %s differ: %d variable(s) only in the first, %d only in the second\n", names[0], names[i], chk.OnlyCount[0], chk.OnlyCount[1])
		}
	}

	if *byGroup != "" {
		req.Mode = "groups"
		if strings.EqualFold(*byGroup, "all") {
			for _, g := range RegionIDs(srcs, req) {
				req.Groups = append(req.Groups, g.Name)
			}
		} else {
			req.Groups = splitList(*byGroup)
		}
	}
	if h := strings.TrimSpace(*highlight); h != "" {
		var groups []string
		for _, s := range srcs {
			if !contains(groups, s.Name) {
				groups = append(groups, s.Name)
			}
		}
		for i, g := range groups {
			if h == strconv.Itoa(i+1) || strings.EqualFold(h, g) {
				req.Highlight = i + 1
			}
		}
		if req.Highlight == 0 {
			die(fmt.Sprintf("-highlight: %q is not a group number or name (%s)", h, strings.Join(groups, ", ")))
		}
	}
	run, err := Generate(srcs, req, true)
	if err != nil {
		die(err)
	}
	for _, w := range run.Warnings {
		fmt.Fprintln(os.Stderr, "note:", w)
	}
	if run.ChordWidth > 0 {
		fmt.Printf("chord width: %.3f° in every diagram\n", run.ChordWidth)
	}
	for _, d := range run.Diagrams {
		cn := d.Counts
		fmt.Printf("%s: %d correlations (%d positive, %d negative)\n", d.Name, cn.Total, cn.Pos, cn.Neg)
		for _, b := range cn.Bands {
			fmt.Printf("   %-28s %5d  (+%d / −%d)\n", b.Label, b.Total, b.Pos, b.Neg)
		}
		for _, t := range d.Times {
			fmt.Printf("   %s: %d correlations (%d positive, %d negative)\n", t.Name, t.Counts.Total, t.Counts.Pos, t.Counts.Neg)
		}
		os.WriteFile(filepath.Join(dir, d.File+"_counts.xlsx"), d.xlsx, 0644)
		if d.Error != "" {
			fmt.Println("   nothing drawn:", d.Error)
			continue
		}
		os.WriteFile(filepath.Join(dir, d.File+".png"), d.png, 0644)
		os.WriteFile(filepath.Join(dir, d.File+".svg"), d.svg, 0644)
	}
	if run.summary != nil {
		os.WriteFile(filepath.Join(dir, run.Prefix+"_summary.xlsx"), run.summary, 0644)
	}
	if run.Changes != nil {
		c := run.ChangeInfo
		fmt.Printf("changes %s -> %s: appeared %d, disappeared %d, sign change %d (+→− %d, −→+ %d), unchanged %d",
			c.Group1, c.Group2, c.Appeared, c.Disappeared, c.SignPosNeg+c.SignNegPos, c.SignPosNeg, c.SignNegPos, c.Unchanged)
		if !c.OK {
			fmt.Print("  (CHECK FAILED)")
		}
		fmt.Println()
		os.WriteFile(filepath.Join(dir, run.Prefix+"_changes.xlsx"), run.Changes, 0644)
	}
}
