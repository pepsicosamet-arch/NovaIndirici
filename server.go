package main

// Web sürümü: NOVA'yı herkese açık bir sunucuda çalıştırır.
// Her ziyaretçi yalnızca kendi indirmelerini görür; dosyalar tarayıcıya
// "Bilgisayarına kaydet" ile iner ve bir süre sonra sunucudan silinir.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	serverMode  bool
	srvMaxFile  = "2G"             // tek dosya için üst sınır
	srvMaxItems = 10               // listelerde en fazla öğe
	srvPerUser  = 2                // oturum başına aynı anda iş
	srvKeep     = 30 * time.Minute // biten dosyaların saklanma süresi
)

func envInt(k string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(k)); err == nil && v > 0 {
		return v
	}
	return def
}

// sessionID ziyaretçiyi çerezle tanır; masaüstünde her zaman "".
func sessionID(w http.ResponseWriter, r *http.Request) string {
	if !serverMode {
		return ""
	}
	if c, err := r.Cookie("nova_sid"); err == nil && len(c.Value) == 32 {
		return c.Value
	}
	sid := randToken()
	http.SetCookie(w, &http.Cookie{Name: "nova_sid", Value: sid, Path: "/", MaxAge: 7 * 24 * 3600,
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: r.Header.Get("X-Forwarded-Proto") == "https"})
	r.AddCookie(&http.Cookie{Name: "nova_sid", Value: sid})
	return sid
}

func clientIP(r *http.Request) string {
	if f := r.Header.Get("X-Forwarded-For"); f != "" {
		return strings.TrimSpace(strings.Split(f, ",")[0])
	}
	h, _, _ := net.SplitHostPort(r.RemoteAddr)
	return h
}

// ---------- basit hız sınırı ----------

type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	n    int
	per  time.Duration
}

func newLimiter(n int, per time.Duration) *limiter {
	return &limiter{hits: map[string][]time.Time{}, n: n, per: per}
}

func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	h := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < l.per {
			h = append(h, t)
		}
	}
	if len(h) >= l.n {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	return true
}

// ---------- güvenlik: iç ağ adreslerini engelle ----------

func checkPublicURL(raw string) error {
	if os.Getenv("NOVA_ALLOW_PRIVATE") != "" { // yalnızca yerel test için
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("Geçerli bir bağlantı gir (https:// ile başlamalı).")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil || len(ips) == 0 {
		return errors.New("Bu adres bulunamadı.")
	}
	for _, ip := range ips {
		a := ip.IP
		if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() || a.IsUnspecified() || a.IsMulticast() {
			return errors.New("Bu adres desteklenmiyor.")
		}
	}
	return nil
}

// ---------- sunucu ----------

func runServer() {
	serverMode = true
	log.SetOutput(os.Stdout)
	dataDir = os.Getenv("NOVA_DATA")
	if dataDir == "" {
		dataDir = filepath.Join(os.TempDir(), "nova-web")
	}
	binDir = filepath.Join(dataDir, "bin")
	_ = os.MkdirAll(binDir, 0o755)
	_ = os.RemoveAll(filepath.Join(dataDir, "tmp"))
	_ = os.RemoveAll(filepath.Join(dataDir, "files"))

	srvPerUser = envInt("NOVA_PER_USER", srvPerUser)
	srvMaxItems = envInt("NOVA_MAX_ITEMS", srvMaxItems)
	if v := os.Getenv("NOVA_MAX_FILESIZE"); v != "" {
		srvMaxFile = v
	}
	settings = defaultSettings()
	settings.MaxConcurrent = envInt("NOVA_MAX_JOBS", 3)
	settings.DownloadDir = filepath.Join(dataDir, "files")
	jobs = NewJobManager()

	if !toolStatus().Ready {
		log.Println("UYARI: yt-dlp veya ffmpeg bulunamadı")
	}
	go ytdlpVersion()
	go func() { // indirme motorunu güncel tut
		for {
			time.Sleep(12 * time.Hour)
			if out, err := UpdateYtdlp(); err != nil {
				log.Println("yt-dlp güncellenemedi:", err, out)
			}
			ytdlpVersion()
		}
	}()
	go func() { // eski dosyaları temizle
		for {
			time.Sleep(time.Minute)
			jobs.janitor(srvKeep)
		}
	}()

	infoLimit := newLimiter(30, 10*time.Minute)
	dlLimit := newLimiter(20, time.Hour)

	sub, _ := fs.Sub(webFS, "web")
	static := http.FileServer(http.FS(sub))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		sessionID(w, r)
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		static.ServeHTTP(w, r)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "tools": toolStatus().Ready})
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		sid := sessionID(w, r)
		writeJSON(w, 200, map[string]any{
			"version": AppVersion, "web": true,
			"tools":    toolStatus(),
			"settings": settings.Snapshot(),
			"jobs":     jobs.ListFor(sid),
			"history":  []JobView{},
			"update":   map[string]any{"current": AppVersion},
			"limits":   map[string]any{"perUser": srvPerUser, "maxItems": srvMaxItems, "maxFile": srvMaxFile, "keepMinutes": int(srvKeep.Minutes())},
		})
	})
	mux.HandleFunc("/api/events", handleEvents)
	mux.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		sessionID(w, r)
		if !infoLimit.allow(clientIP(r)) {
			fail(w, 429, "Çok fazla istek gönderdin. Birkaç dakika sonra tekrar dene.")
			return
		}
		var req struct {
			URL      string `json:"url"`
			Playlist bool   `json:"playlist"`
		}
		if err := readBody(r, &req); err != nil || strings.TrimSpace(req.URL) == "" {
			fail(w, 400, "Bağlantı boş.")
			return
		}
		if err := checkPublicURL(req.URL); err != nil {
			fail(w, 422, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()
		info, err := FetchInfo(ctx, strings.TrimSpace(req.URL), req.Playlist)
		if err != nil {
			fail(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, info)
	})
	mux.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		sid := sessionID(w, r)
		var req JobRequest
		if err := readBody(r, &req); err != nil || strings.TrimSpace(req.URL) == "" {
			fail(w, 400, "Bağlantı boş.")
			return
		}
		if err := checkPublicURL(req.URL); err != nil {
			fail(w, 422, err.Error())
			return
		}
		if jobs.activeFor(sid) >= srvPerUser {
			fail(w, 429, fmt.Sprintf("Aynı anda en fazla %d indirme yapabilirsin. Biri bitince tekrar dene.", srvPerUser))
			return
		}
		if !dlLimit.allow(clientIP(r)) {
			fail(w, 429, "Bu saat için indirme sınırına ulaştın. Biraz sonra tekrar dene.")
			return
		}
		writeJSON(w, 200, jobs.AddFor(req, sid).View())
	})
	owned := func(fn func(id string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			sid := sessionID(w, r)
			var req struct{ ID string }
			_ = readBody(r, &req)
			if j := jobs.find(req.ID); j != nil && j.owner == sid {
				fn(req.ID)
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
		}
	}
	mux.HandleFunc("/api/cancel", owned(jobs.Cancel))
	mux.HandleFunc("/api/retry", owned(jobs.Retry))
	mux.HandleFunc("/api/clear", func(w http.ResponseWriter, r *http.Request) {
		jobs.ClearFinishedFor(sessionID(w, r))
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/settings", func(w http.ResponseWriter, r *http.Request) {
		// Web sürümünde tercihler tarayıcıda tutulur.
		writeJSON(w, 200, settings.Snapshot())
	})
	mux.HandleFunc("/api/file", func(w http.ResponseWriter, r *http.Request) {
		sid := sessionID(w, r)
		j := jobs.find(r.URL.Query().Get("id"))
		if j == nil || j.owner != sid {
			http.Error(w, "Dosya bulunamadı ya da süresi doldu.", 404)
			return
		}
		v := j.View()
		i, _ := strconv.Atoi(r.URL.Query().Get("i"))
		if v.Status != "done" || i < 0 || i >= len(v.Files) {
			http.Error(w, "Dosya hazır değil.", 404)
			return
		}
		root := filepath.Join(dataDir, "files", j.ID)
		p := filepath.Clean(v.Files[i])
		if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
			http.Error(w, "Geçersiz dosya.", 400)
			return
		}
		f, err := os.Open(p)
		if err != nil {
			http.Error(w, "Dosya bulunamadı ya da süresi doldu.", 404)
			return
		}
		defer f.Close()
		st, _ := f.Stat()
		name := filepath.Base(p)
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
		w.Header().Set("Cache-Control", "private, no-store")
		http.ServeContent(w, r, name, st.ModTime(), f)
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(w, r)
	})

	addr := ":" + strconv.Itoa(envInt("PORT", 8080))
	log.Printf("%s %s web sürümü %s adresinde (eş zamanlı %d, kişi başı %d)", AppName, AppVersion, addr, settings.MaxConcurrent, srvPerUser)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func (m *JobManager) activeFor(owner string) int {
	n := 0
	for _, j := range m.ListFor(owner) {
		switch j.Status {
		case "queued", "starting", "downloading", "processing":
			n++
		}
	}
	return n
}

// janitor süresi dolan işleri ve dosyalarını siler.
func (m *JobManager) janitor(keep time.Duration) {
	cut := time.Now().Add(-keep).UnixMilli()
	m.mu.Lock()
	keepJobs := m.jobs[:0]
	var gone []*Job
	for _, j := range m.jobs {
		v := j.View()
		if v.Finished > 0 && v.Finished < cut {
			gone = append(gone, j)
			continue
		}
		keepJobs = append(keepJobs, j)
	}
	m.jobs = keepJobs
	m.mu.Unlock()
	for _, j := range gone {
		_ = os.RemoveAll(filepath.Join(dataDir, "files", j.ID))
		hub.BroadcastTo(j.owner, "jobs", m.ListFor(j.owner))
	}
}

// lookTool sunucuda sistemde kurulu aracı bulur.
func lookTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	return ""
}
