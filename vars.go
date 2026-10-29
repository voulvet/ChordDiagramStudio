package main

import (
	"regexp"
	"sort"
	"strings"
)

var parenGroup = regexp.MustCompile(`\(\s*([^()]+?)\s*\)\s*$`)
var shortCode = regexp.MustCompile(`^[A-Z0-9]{1,4}$`)

// detectGroup finds a group code (e.g. an anatomical region) in a variable name:
// text in brackets at the end, "Biomechanics_IM(AE)" -> AE, or a short upper-case
// code after the last separator, "Macroscopy_IndiaInk_AE" -> AE.
func detectGroup(name, sep string) string {
	if m := parenGroup.FindStringSubmatch(name); m != nil {
		return clean(m[1])
	}
	if sep != "" {
		if k := strings.LastIndex(name, sep); k >= 0 {
			last := clean(name[k+len(sep):])
			if shortCode.MatchString(last) {
				return last
			}
		}
	}
	return ""
}

func (o *Options) sep() string {
	if o.Sep == "" {
		return "_"
	}
	return o.Sep
}

// varInfo returns the category and group of a header label.
func (ds *Dataset) varInfo(label string, o *Options) (string, string) {
	if label == "" {
		return "", ""
	}
	if o.Header != "variable" {
		return label, ""
	}
	if o.UseSheet {
		if v, ok := ds.Vars[label]; ok {
			return v.Category, v.Group
		}
	}
	cat := label
	if k := strings.Index(label, o.sep()); k > 0 {
		cat = clean(label[:k])
	}
	return cat, detectGroup(label, o.sep())
}

// groupOK applies the group filters to a pair of variables.
func (o *Options) groupOK(ga, gb string) bool {
	incl := o.Ungrouped != "exclude"
	if o.Group != "" {
		okA := ga == o.Group || (ga == "" && incl)
		okB := gb == o.Group || (gb == "" && incl)
		return okA && okB
	}
	if o.WithinGroup {
		if ga == "" || gb == "" {
			return incl
		}
		return ga == gb
	}
	return true
}

type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Description struct {
	Variables   int         `json:"variables"`
	Categories  []NameCount `json:"categories"` // after merging
	Groups      []NameCount `json:"groups"`
	Ungrouped   int         `json:"ungrouped"`
	UngroupedEx []string    `json:"ungroupedExamples"`
}

// Describe summarises how variables are assigned to categories and groups.
func (ds *Dataset) Describe(o *Options) Description {
	d := Description{Variables: len(ds.Labels)}
	cc, gc := map[string]int{}, map[string]int{}
	var corder, gorder []string
	for _, l := range ds.Labels {
		c, g := ds.varInfo(l, o)
		if c == "" {
			continue
		}
		c = o.mapCat(c)
		if cc[c] == 0 {
			corder = append(corder, c)
		}
		cc[c]++
		if o.Header == "variable" {
			if g == "" {
				d.Ungrouped++
				if len(d.UngroupedEx) < 12 {
					d.UngroupedEx = append(d.UngroupedEx, l)
				}
			} else {
				if gc[g] == 0 {
					gorder = append(gorder, g)
				}
				gc[g]++
			}
		}
	}
	for _, c := range corder {
		d.Categories = append(d.Categories, NameCount{c, cc[c]})
	}
	for _, g := range gorder {
		d.Groups = append(d.Groups, NameCount{g, gc[g]})
	}
	_ = sort.Strings
	return d
}
