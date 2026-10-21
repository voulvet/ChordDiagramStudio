package main

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed ui.html
var uiHTML []byte

type Diagram struct {
	Name   string `json:"name"`
	File   string `json:"file"`
	Counts Counts `json:"counts"`
	Error  string `json:"error,omitempty"`
	png    []byte
	svg    []byte
	xlsx   []byte
}

type server struct {
	mu       sync.Mutex
	fileName string
	data     []byte
	ds       *Dataset
	jobs     map[string][]*Diagram
	jobN     int
}

type GenerateReq struct {
	Options     Options  `json:"options"`
	Mode        string   `json:"mode"` // "single" or "series"
	Series      []string `json:"series"`
	SeriesApart bool     `json:"seriesExcludeOthers"`
	Prefix      string   `json:"prefix"`
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (s *server) info() map[string]any {
	return map[string]any{
		"file": s.fileName, "sheets": s.ds.Sheets, "rsheet": s.ds.RSheet, "psheet": s.ds.PSheet,
		"categories": s.ds.Categories, "symmetric": s.ds.Symmetric,
	}
}

func (s *server) handleLoad(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs, ps := r.FormValue("rsheet"), r.FormValue("psheet")
	if f, h, err := r.FormFile("file"); err == nil {
		b, _ := io.ReadAll(f)
		f.Close()
		s.data, s.fileName = b, h.Filename
		rs, ps = "", ""
	}
	if s.data == nil {
		fail(w, "Choose an Excel file first.")
		return
	}
	ds, err := LoadDataset(bytes.NewReader(s.data), rs, ps)
	if err != nil {
		fail(w, err.Error())
		return
	}
	s.ds = ds
	writeJSON(w, s.info())
}

func (s *server) handleCategories(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ds == nil {
		fail(w, "No file loaded.")
		return
	}
	var o Options
	json.NewDecoder(r.Body).Decode(&o)
	writeJSON(w, map[string]any{"categories": s.ds.MergedCategories(&o)})
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

func fileSafe(s string) string {
	s = strings.Trim(unsafe.ReplaceAllString(s, "_"), "_")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

func (s *server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ds == nil {
		fail(w, "No file loaded.")
		return
	}
	var req GenerateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, "Bad request: "+err.Error())
		return
	}
	o := req.Options
	if o.Alpha <= 0 {
		o.Alpha = 0.05
	}
	if o.Cuts.VeryStrong == 0 {
		o.Cuts = Cuts{0.8, 0.6, 0.4}
	}
	prefix := fileSafe(req.Prefix)
	if prefix == "" {
		prefix = fileSafe(strings.TrimSuffix(s.fileName, ".xlsx")) + "_chord"
	}
	// full ring / colour order (after merge, following the user's order)
	_, fullOrder, _ := s.ds.Compute(&o)

	var diags []*Diagram
	make1 := func(name, file, title string, opt Options) {
		links, order, rm := s.ds.Compute(&opt)
		d := &Diagram{Name: name, File: file, Counts: CountLinks(links, order, opt.Cuts, rm)}
		if len(links) == 0 {
			d.Error = "No significant correlations to draw with these settings."
		} else {
			png, svg, err := Render(links, order, fullOrder, title)
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
			fail(w, "Tick at least one category for the series.")
			return
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
			t := strings.ReplaceAll(title, "{category}", cat)
			make1(cat, prefix+"_"+fileSafe(cat), t, opt)
		}
	} else {
		if title == "" {
			title = "Significant correlations between variable categories"
		}
		make1("All categories", prefix, strings.ReplaceAll(title, "{category}", "All categories"), o)
	}
	s.jobN++
	job := strconv.Itoa(s.jobN)
	s.jobs[job] = diags
	writeJSON(w, map[string]any{"job": job, "diagrams": diags})
}

func (s *server) get(job, i string) *Diagram {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.jobs[job]
	k, err := strconv.Atoi(i)
	if err != nil || k < 0 || k >= len(d) {
		return nil
	}
	return d[k]
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	d := s.get(r.FormValue("job"), r.FormValue("i"))
	if d == nil {
		http.NotFound(w, r)
		return
	}
	kind := r.FormValue("kind")
	var b []byte
	var ct, ext string
	switch kind {
	case "png":
		b, ct, ext = d.png, "image/png", ".png"
	case "svg":
		b, ct, ext = d.svg, "image/svg+xml", ".svg"
	case "xlsx":
		b, ct, ext = d.xlsx, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "_counts.xlsx"
	}
	if b == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	if r.FormValue("dl") == "1" {
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s%s"`, d.File, ext))
	}
	w.Write(b)
}

func (s *server) handleZip(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	diags := s.jobs[r.FormValue("job")]
	s.mu.Unlock()
	if diags == nil {
		http.NotFound(w, r)
		return
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	add := func(name string, b []byte) {
		if b != nil {
			f, _ := z.Create(name)
			f.Write(b)
		}
	}
	for _, d := range diags {
		add(d.File+".png", d.png)
		add(d.File+".svg", d.svg)
		add(d.File+"_counts.xlsx", d.xlsx)
	}
	z.Close()
	name := "chord_diagrams.zip"
	if len(diags) > 0 {
		name = diags[0].File + "_all.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Write(buf.Bytes())
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func runServer(port int, noBrowser bool) {
	s := &server{jobs: map[string][]*Diagram{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(uiHTML)
	})
	mux.HandleFunc("/api/load", s.handleLoad)
	mux.HandleFunc("/api/categories", s.handleCategories)
	mux.HandleFunc("/api/generate", s.handleGenerate)
	mux.HandleFunc("/api/file", s.handleFile)
	mux.HandleFunc("/api/zip", s.handleZip)
	mux.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
		go func() { time.Sleep(300 * time.Millisecond); os.Exit(0) }()
	})
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not start:", err)
		os.Exit(1)
	}
	url := fmt.Sprintf("http://%s/", ln.Addr().String())
	fmt.Println("Chord Diagram Studio is running at", url)
	fmt.Println("Keep this window open while you use it. Close it (or press Quit in the page) to stop.")
	if !noBrowser {
		openBrowser(url)
	}
	http.Serve(ln, mux)
}
