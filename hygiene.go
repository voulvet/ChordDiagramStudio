package main

// Data hygiene: repair damaged sheet names, normalise variable names and report
// names that look like the same variable written differently or like typos.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var sheetNameAttr = regexp.MustCompile(`(<sheet\b[^>]*?\bname=")([^"]*)(")`)

// repairSheetNames rewrites sheet names that Excel itself would not accept
// (longer than 31 characters or containing : \ / ? * [ ]) to a short valid
// name, so that the workbook can still be read. The sheets keep their position,
// so "first sheet = r, second sheet = p" still works.
func repairSheetNames(b []byte) ([]byte, []string) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return b, nil
	}
	var wb []byte
	for _, f := range zr.File {
		if f.Name == "xl/workbook.xml" {
			rc, err := f.Open()
			if err != nil {
				return b, nil
			}
			wb, _ = io.ReadAll(rc)
			rc.Close()
		}
	}
	if wb == nil {
		return b, nil
	}
	var warn []string
	used := map[string]bool{}
	for _, m := range sheetNameAttr.FindAllSubmatch(wb, -1) {
		used[strings.ToLower(xmlUnescape(string(m[2])))] = true
	}
	k := 0
	fixed := sheetNameAttr.ReplaceAllFunc(wb, func(m []byte) []byte {
		k++
		p := sheetNameAttr.FindSubmatch(m)
		name := xmlUnescape(string(p[2]))
		if validSheetName(name) {
			return m
		}
		nn := strings.Map(func(r rune) rune {
			if strings.ContainsRune(`:\/?*[]`, r) {
				return ' '
			}
			return r
		}, name)
		nn = strings.Join(strings.Fields(nn), " ")
		if r := []rune(nn); len(r) > 28 {
			nn = strings.TrimSpace(string(r[:28]))
		}
		if nn == "" || used[strings.ToLower(nn)] {
			nn = fmt.Sprintf("Sheet %d", k)
		}
		used[strings.ToLower(nn)] = true
		warn = append(warn, fmt.Sprintf("Sheet %d had an invalid name (%q); it was read as %q.", k, name, nn))
		return append(append(append([]byte{}, p[1]...), []byte(xmlEscape(nn))...), p[3]...)
	})
	if len(warn) == 0 {
		return b, nil
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		h := f.FileHeader
		w, err := zw.CreateHeader(&h)
		if err != nil {
			return b, nil
		}
		if f.Name == "xl/workbook.xml" {
			w.Write(fixed)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return b, nil
		}
		io.Copy(w, rc)
		rc.Close()
	}
	zw.Close()
	return out.Bytes(), warn
}

func validSheetName(s string) bool {
	if s == "" || len([]rune(s)) > 31 || strings.ContainsAny(s, `:\/?*[]`) {
		return false
	}
	return !strings.HasPrefix(s, "'") && !strings.HasSuffix(s, "'")
}

var xmlUn = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'")
var xmlEs = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")

func xmlUnescape(s string) string { return xmlUn.Replace(s) }
func xmlEscape(s string) string   { return xmlEs.Replace(s) }

// normKey is used to match variables between files: case, spaces and
// underscores do not matter ("Lining thickness" = "Lining_thickness").
func normKey(s string) string {
	s = strings.ToLower(clean(s))
	s = strings.ReplaceAll(s, "_", " ")
	return strings.Join(strings.Fields(s), " ")
}

// SpacingVariants lists names that differ only in spacing, underscores or case,
// within one file or between files (pairs as "a" / "b").
func SpacingVariants(names [][]string) []string {
	seen := map[string]map[string]bool{}
	var keys []string
	for _, list := range names {
		for _, n := range list {
			k := normKey(n)
			if seen[k] == nil {
				seen[k] = map[string]bool{}
				keys = append(keys, k)
			}
			seen[k][n] = true
		}
	}
	var out []string
	for _, k := range keys {
		if len(seen[k]) > 1 {
			var v []string
			for n := range seen[k] {
				v = append(v, fmt.Sprintf("%q", n))
			}
			sort.Strings(v)
			out = append(out, strings.Join(v, " / "))
		}
	}
	return out
}

// PossibleTypos reports pairs of words (at least 5 letters) used in variable
// names that differ by a single letter or two swapped letters, e.g.
// "Proteoglycan" / "Proteoglycna". Only a hint: the user decides.
func PossibleTypos(names [][]string) []string {
	count := map[string]int{}
	orig := map[string]string{}
	for _, list := range names {
		for _, n := range list {
			for _, w := range strings.FieldsFunc(n, func(r rune) bool { return !unicode.IsLetter(r) }) {
				lw := strings.ToLower(w)
				if len([]rune(lw)) < 5 {
					continue
				}
				if count[lw] == 0 {
					orig[lw] = w
				}
				count[lw]++
			}
		}
	}
	var words []string
	for w := range count {
		words = append(words, w)
	}
	sort.Strings(words)
	var out []string
	for i := 0; i < len(words); i++ {
		for j := i + 1; j < len(words); j++ {
			a, b := words[i], words[j]
			if d := len(a) - len(b); d > 1 || d < -1 {
				continue
			}
			if !oneEdit(a, b) || pluralPair(a, b) {
				continue
			}
			out = append(out, fmt.Sprintf("%q / %q", orig[a], orig[b]))
		}
	}
	return out
}

func pluralPair(a, b string) bool {
	return a+"s" == b || b+"s" == a || a+"es" == b || b+"es" == a
}

// oneEdit: one substitution, insertion, deletion or swap of two neighbouring letters.
func oneEdit(a, b string) bool {
	x, y := []rune(a), []rune(b)
	if len(x) == len(y) {
		var diff []int
		for i := range x {
			if x[i] != y[i] {
				diff = append(diff, i)
			}
		}
		if len(diff) == 1 {
			return true
		}
		return len(diff) == 2 && diff[1] == diff[0]+1 && x[diff[0]] == y[diff[1]] && x[diff[1]] == y[diff[0]]
	}
	if len(x) > len(y) {
		x, y = y, x
	}
	i := 0
	for i < len(x) && x[i] == y[i] {
		i++
	}
	return string(x[i:]) == string(y[i+1:])
}

// NameWarnings combines the checks for a set of datasets (one per file).
func NameWarnings(dss []*Dataset) []string {
	var w []string
	var names [][]string
	for _, ds := range dss {
		w = append(w, ds.Warnings...)
		names = append(names, ds.Labels)
	}
	if v := SpacingVariants(names); len(v) > 0 {
		for _, x := range v {
			w = append(w, "Same variable written differently (matched as one variable): "+x)
		}
	}
	for _, x := range PossibleTypos(names) {
		w = append(w, "Possible typo (check the spelling): "+x)
	}
	return w
}
