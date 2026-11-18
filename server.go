//go:build !js

package main

import (
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
	mu   sync.Mutex
	sess Session
	runs map[string]*Run
	jobN int
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

// /api/load: slot 0 = first file (or new sheets when no file is sent);
// slot k = file k of several (k = number of files adds one); remove=1 drops slot k;
// remove=all drops every file except the first.
func (s *server) handleLoad(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slot, _ := strconv.Atoi(r.FormValue("slot"))
	switch r.FormValue("remove") {
	case "all":
		s.sess.RemoveExtra()
		writeJSON(w, s.sess.Info())
		return
	case "1":
		s.sess.Remove(slot)
		writeJSON(w, s.sess.Info())
		return
	}
	var data []byte
	name := ""
	if f, h, err := r.FormFile("file"); err == nil {
		data, _ = io.ReadAll(f)
		f.Close()
		name = h.Filename
	}
	if slot > 0 && data == nil {
		fail(w, "Choose an Excel file.")
		return
	}
	if err := s.sess.Load(slot, data, name, r.FormValue("rsheet"), r.FormValue("psheet")); err != nil {
		fail(w, err.Error())
		return
	}
	writeJSON(w, s.sess.Info())
}

// /api/catsheet: read the "Categories" sheet of an Excel file.
func (s *server) handleCatSheet(w http.ResponseWriter, r *http.Request) {
	f, _, err := r.FormFile("file")
	if err != nil {
		fail(w, "Choose an Excel file.")
		return
	}
	b, _ := io.ReadAll(f)
	f.Close()
	c, err := LoadCategories(b)
	if err != nil {
		fail(w, err.Error())
		return
	}
	writeJSON(w, map[string]any{"catSheet": c})
}

func (s *server) handleCategories(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var req GenerateReq
	json.NewDecoder(r.Body).Decode(&req)
	m, err := s.sess.Categories(req)
	if err != nil {
		fail(w, err.Error())
		return
	}
	writeJSON(w, m)
}

func (s *server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var req GenerateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		fail(w, "Bad request: "+err.Error())
		return
	}
	run, err := s.sess.Generate(req, true)
	if err != nil {
		fail(w, err.Error())
		return
	}
	s.jobN++
	job := strconv.Itoa(s.jobN)
	s.runs[job] = run
	writeJSON(w, map[string]any{"job": job, "diagrams": run.Diagrams, "prefix": run.Prefix, "hasSummary": run.summary != nil,
		"hasChanges": run.Changes != nil, "changes": run.ChangeInfo, "warnings": run.Warnings, "chordWidth": run.ChordWidth})
}

func (s *server) run(job string) *Run {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[job]
}

func (s *server) handleFile(w http.ResponseWriter, r *http.Request) {
	run := s.run(r.FormValue("job"))
	if run == nil {
		http.NotFound(w, r)
		return
	}
	const xlsxType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	switch r.FormValue("kind") {
	case "summary", "changes":
		b, suffix := run.summary, "_summary.xlsx"
		if r.FormValue("kind") == "changes" {
			b, suffix = run.Changes, "_changes.xlsx"
		}
		if b == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", xlsxType)
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s%s"`, run.Prefix, suffix))
		w.Write(b)
		return
	}
	k, err := strconv.Atoi(r.FormValue("i"))
	if err != nil || k < 0 || k >= len(run.Diagrams) {
		http.NotFound(w, r)
		return
	}
	d := run.Diagrams[k]
	var b []byte
	var ct, ext string
	switch r.FormValue("kind") {
	case "png":
		b, ct, ext = d.png, "image/png", ".png"
	case "svg":
		b, ct, ext = d.svg, "image/svg+xml", ".svg"
	case "xlsx":
		b, ct, ext = d.xlsx, xlsxType, "_counts.xlsx"
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
	run := s.run(r.FormValue("job"))
	if run == nil {
		http.NotFound(w, r)
		return
	}
	name := run.Prefix + "_all.zip"
	if len(run.Diagrams) > 1 {
		name = run.Prefix + "_series_all.zip"
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Write(ZipRun(run))
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
	s := &server{runs: map[string]*Run{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(uiHTML)
	})
	mux.HandleFunc("/api/load", s.handleLoad)
	mux.HandleFunc("/api/catsheet", s.handleCatSheet)
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
