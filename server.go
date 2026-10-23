//go:build !js

package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"time"
)

//go:embed ui.html
var uiHTML []byte

type server struct {
	mu       sync.Mutex
	fileName string
	data     []byte
	ds       *Dataset
	jobs     map[string][]*Diagram
	jobN     int
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
	diags, err := Generate(s.ds, req, s.fileName, true)
	if err != nil {
		fail(w, err.Error())
		return
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
	data := ZipDiagrams(diags)
	name := "chord_diagrams.zip"
	if len(diags) > 0 {
		name = diags[0].File + "_all.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Write(data)
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
