package main

// Session: the loaded files and the last results. Shared by the desktop app
// (server.go) and the website (wasm_main.go), so both behave the same.

import (
	"bytes"
	"fmt"
)

type loadedFile struct {
	Name string
	Data []byte
	DS   *Dataset
}

type Session struct {
	files []*loadedFile // files[0] defines the sheet choice
}

// Load reads a file into slot k (0 = first file; k = number of files adds one).
// data == nil re-reads the first file with other sheets.
func (s *Session) Load(k int, data []byte, name, rs, ps string) error {
	if k == 0 {
		if data != nil {
			if len(s.files) == 0 {
				s.files = append(s.files, &loadedFile{})
			}
			s.files[0].Data, s.files[0].Name = data, name
			rs, ps = "", ""
		}
		if len(s.files) == 0 || s.files[0].Data == nil {
			return fmt.Errorf("Choose an Excel file first.")
		}
		ds, err := LoadDataset(bytes.NewReader(s.files[0].Data), rs, ps)
		if err != nil {
			return err
		}
		s.files[0].DS = ds
		for _, f := range s.files[1:] { // keep the other files on the same sheet names when possible
			if d, err := LoadDataset(bytes.NewReader(f.Data), sheetIfPresent(f.Data, ds.RSheet), sheetIfPresent(f.Data, ds.PSheet)); err == nil {
				f.DS = d
			}
		}
		return nil
	}
	if len(s.files) == 0 || s.files[0].DS == nil {
		return fmt.Errorf("Load the first Excel file first.")
	}
	r1, p1 := sheetIfPresent(data, s.files[0].DS.RSheet), sheetIfPresent(data, s.files[0].DS.PSheet)
	ds, err := LoadDataset(bytes.NewReader(data), r1, p1)
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	f := &loadedFile{Name: name, Data: data, DS: ds}
	if k < len(s.files) {
		s.files[k] = f
	} else {
		s.files = append(s.files, f)
	}
	return nil
}

// Remove drops file k (k ≥ 1).
func (s *Session) Remove(k int) {
	if k >= 1 && k < len(s.files) {
		s.files = append(s.files[:k], s.files[k+1:]...)
	}
}

// RemoveExtra keeps only the first file.
func (s *Session) RemoveExtra() {
	if len(s.files) > 1 {
		s.files = s.files[:1]
	}
}

func (s *Session) Info() map[string]any {
	if len(s.files) == 0 || s.files[0].DS == nil {
		return map[string]any{}
	}
	ds := s.files[0].DS
	m := map[string]any{
		"file": s.files[0].Name, "sheets": ds.Sheets, "rsheet": ds.RSheet, "psheet": ds.PSheet,
		"categories": ds.Categories, "symmetric": ds.Symmetric,
		"looksVariables": ds.LooksVariables, "hasVarSheet": ds.HasVarSheet,
		"catSheet": ds.Cats,
	}
	type fi struct {
		Name  string     `json:"name"`
		Check LabelCheck `json:"check"`
	}
	var more []fi
	var dss []*Dataset
	for _, f := range s.files {
		dss = append(dss, f.DS)
	}
	for _, f := range s.files[1:] {
		more = append(more, fi{f.Name, CompareLabels(ds, f.DS)})
		if len(f.DS.Cats) > 0 && len(ds.Cats) == 0 {
			m["catSheet"] = f.DS.Cats
		}
	}
	m["files"] = more
	m["warnings"] = NameWarnings(dss)
	return m
}

// Sources returns the files used by a request: only the first file, or all of
// them when several files are used.
func (s *Session) Sources(req GenerateReq) []Source {
	var names []string
	var dss []*Dataset
	for i, f := range s.files {
		if i > 0 && !req.Compare {
			break
		}
		names = append(names, f.Name)
		dss = append(dss, f.DS)
	}
	return MakeSources(names, dss, req)
}

// Categories: categories after merging (union over the files used) and the
// description of variables and regions.
func (s *Session) Categories(req GenerateReq) (map[string]any, error) {
	if len(s.files) == 0 || s.files[0].DS == nil {
		return nil, fmt.Errorf("No file loaded.")
	}
	o := req.Options
	srcs := s.Sources(req)
	var cats []string
	for _, src := range srcs {
		for _, c := range src.DS.MergedCategories(&o) {
			if !contains(cats, c) {
				cats = append(cats, c)
			}
		}
	}
	d := srcs[0].DS.Describe(&o)
	if len(srcs) > 1 {
		cc := map[string]int{}
		var order []string
		d.Variables, d.Ungrouped, d.UngroupedEx = 0, 0, nil
		for _, src := range srcs {
			x := src.DS.Describe(&o)
			d.Variables += x.Variables
			d.Ungrouped += x.Ungrouped
			for _, e := range x.UngroupedEx {
				if len(d.UngroupedEx) < 12 && !contains(d.UngroupedEx, e) {
					d.UngroupedEx = append(d.UngroupedEx, e)
				}
			}
			for _, c := range x.Categories {
				if cc[c.Name] == 0 {
					order = append(order, c.Name)
				}
				cc[c.Name] += c.Count
			}
		}
		d.Categories = nil
		for _, c := range order {
			d.Categories = append(d.Categories, NameCount{c, cc[c]})
		}
	}
	d.Groups = RegionIDs(srcs, req)
	return map[string]any{"categories": cats, "describe": d}, nil
}

func (s *Session) Generate(req GenerateReq, raster bool) (*Run, error) {
	if len(s.files) == 0 || s.files[0].DS == nil {
		return nil, fmt.Errorf("No file loaded.")
	}
	return Generate(s.Sources(req), req, raster)
}
