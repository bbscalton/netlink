package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var version = "0.1.0"

// parseIfaces: "wan1=eth0.2,wan2=usb0:8" -> name=device[@source][:streams]
func parseIfaces(s string, defStreams int) []Iface {
	var out []Iface
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		ifc := Iface{Streams: defStreams}
		if i := strings.Index(p, "="); i >= 0 {
			ifc.Name, p = p[:i], p[i+1:]
		}
		if i := strings.LastIndex(p, ":"); i >= 0 && !strings.Contains(p[i:], ".") {
			fmt.Sscanf(p[i+1:], "%d", &ifc.Streams)
			p = p[:i]
		}
		if i := strings.Index(p, "@"); i >= 0 {
			ifc.Source, p = p[i+1:], p[:i]
		}
		ifc.Device = p
		if ifc.Name == "" {
			ifc.Name = p
		}
		out = append(out, ifc)
	}
	return out
}

func fileName(u string) string {
	b := path.Base(strings.SplitN(strings.SplitN(u, "?", 2)[0], "#", 2)[0])
	if b == "" || b == "/" || b == "." {
		b = "download.bin"
	}
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 32 {
			return '_'
		}
		return r
	}, b)
}

func main() {
	ifs := flag.String("i", "", "interfaces: name=dev[@srcip][:streams],... (empty = default route)")
	out := flag.String("o", "", "output file (CLI mode)")
	dir := flag.String("dir", "/tmp/netlink", "download directory (server mode)")
	listen := flag.String("listen", "", "serve web UI on addr, e.g. 0.0.0.0:8099")
	streams := flag.Int("streams", 4, "default streams per interface")
	ver := flag.Bool("version", false, "print version")
	flag.Parse()
	if *ver {
		fmt.Println(version)
		return
	}
	ifaces := parseIfaces(*ifs, *streams)
	if len(ifaces) == 0 {
		ifaces = []Iface{{Name: "default", Streams: *streams}}
	}
	if *listen != "" {
		os.MkdirAll(*dir, 0755)
		serve(*listen, *dir, ifaces)
		return
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: netlink [-i wan1=eth1,wan2=eth2] [-o file] URL | netlink -listen :8099 -dir /mnt/usb")
		os.Exit(2)
	}
	u := flag.Arg(0)
	if *out == "" {
		*out = fileName(u)
	}
	j := &Job{URL: u, Path: *out, Ifaces: ifaces}
	done := make(chan error, 1)
	go func() { done <- j.Run(context.Background()) }()
	t := time.NewTicker(time.Second)
	for {
		select {
		case err := <-done:
			printProgress(j)
			fmt.Fprintln(os.Stderr)
			if err != nil {
				log.Fatal(err)
			}
			el := j.Finished.Sub(j.Started).Seconds()
			fmt.Fprintf(os.Stderr, "done: %s %d bytes in %.1fs (%.2f MB/s)\n", j.Path, j.Total, el, float64(j.Total)/el/1e6)
			return
		case <-t.C:
			printProgress(j)
		}
	}
}

func printProgress(j *Job) {
	var parts []string
	for i, ifc := range j.Ifaces {
		if i < len(j.PerIface) {
			parts = append(parts, fmt.Sprintf("%s:%.1fMB", ifc.Name, float64(j.PerIface[i])/1e6))
		}
	}
	fmt.Fprintf(os.Stderr, "\r%s %.1f/%.1f MB [%s]   ", j.Status, float64(j.Downloaded())/1e6, float64(j.Total)/1e6, strings.Join(parts, " "))
}

type jobView struct {
	ID, URL, File, Status, Err string
	Total, Done                int64
	Ranges                     bool
	PerIface                   map[string]int64
	Seconds                    float64
}

func serve(addr, dir string, ifaces []Iface) {
	var mu sync.Mutex
	jobs := map[string]*Job{}
	var order []string
	view := func(j *Job) jobView {
		j.mu.Lock()
		st, er := j.Status, j.Err
		j.mu.Unlock()
		v := jobView{ID: j.ID, URL: j.URL, File: filepath.Base(j.Path), Status: st, Err: er, Total: j.Total, Done: j.Downloaded(), Ranges: j.Ranges, PerIface: map[string]int64{}}
		for i, ifc := range j.Ifaces {
			if i < len(j.PerIface) {
				v.PerIface[ifc.Name] = j.PerIface[i]
			}
		}
		end := time.Now()
		if !j.Finished.IsZero() {
			end = j.Finished
		}
		v.Seconds = end.Sub(j.Started).Seconds()
		return v
	}
	start := func(u string, names []string) *Job {
		var sel []Iface
		for _, ifc := range ifaces {
			for _, n := range names {
				if n == ifc.Name {
					sel = append(sel, ifc)
				}
			}
		}
		if len(sel) == 0 {
			sel = ifaces
		}
		b := make([]byte, 6)
		rand.Read(b)
		id := hex.EncodeToString(b)
		j := &Job{ID: id, URL: u, Path: filepath.Join(dir, id+"-"+fileName(u)), Ifaces: sel, Status: "queued", Started: time.Now()}
		mu.Lock()
		jobs[id] = j
		order = append(order, id)
		mu.Unlock()
		go j.Run(context.Background())
		return j
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(uiHTML))
	})
	http.HandleFunc("/api/interfaces", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ifaces)
	})
	http.HandleFunc("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			var req struct {
				URL    string   `json:"url"`
				Ifaces []string `json:"ifaces"`
			}
			if json.NewDecoder(r.Body).Decode(&req) != nil || !(strings.HasPrefix(req.URL, "http://") || strings.HasPrefix(req.URL, "https://")) {
				http.Error(w, "need http(s) url", 400)
				return
			}
			json.NewEncoder(w).Encode(view(start(req.URL, req.Ifaces)))
			return
		}
		mu.Lock()
		var vs []jobView
		for i := len(order) - 1; i >= 0; i-- {
			vs = append(vs, view(jobs[order[i]]))
		}
		mu.Unlock()
		json.NewEncoder(w).Encode(vs)
	})
	http.HandleFunc("/api/jobs/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/jobs/"), "/")
		mu.Lock()
		j := jobs[parts[0]]
		mu.Unlock()
		if j == nil {
			http.NotFound(w, r)
			return
		}
		switch {
		case r.Method == "DELETE":
			j.mu.Lock()
			if j.Status == "running" || j.Status == "probing" {
				j.Status = "cancelled"
			}
			j.mu.Unlock()
			if j.cancel != nil {
				j.cancel()
			}
			os.Remove(j.Path)
			os.Remove(j.Path + ".part")
			mu.Lock()
			delete(jobs, j.ID)
			for i, id := range order {
				if id == j.ID {
					order = append(order[:i], order[i+1:]...)
					break
				}
			}
			mu.Unlock()
		case len(parts) > 1 && parts[1] == "file":
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName(j.URL)))
			http.ServeFile(w, r, j.Path)
		default:
			json.NewEncoder(w).Encode(view(j))
		}
	})
	// Proxy-ish mode: GET /get?url=... downloads over all lines, then serves the file (Range ok).
	http.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		u := r.URL.Query().Get("url")
		if !(strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")) {
			http.Error(w, "need ?url=http(s)://...", 400)
			return
		}
		j := start(u, nil)
		for {
			j.mu.Lock()
			st := j.Status
			j.mu.Unlock()
			if st == "done" {
				break
			}
			if st == "failed" || st == "cancelled" {
				http.Error(w, j.Err, 502)
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-time.After(300 * time.Millisecond):
			}
		}
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName(u)))
		http.ServeFile(w, r, j.Path)
	})
	log.Printf("netlink %s listening on %s, dir %s, %d interface(s)", version, addr, dir, len(ifaces))
	log.Fatal(http.ListenAndServe(addr, nil))
}
