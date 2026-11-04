// NOVA İndirici — Samet Şahin
package main

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// AppVersion derleme sırasında -ldflags "-X main.AppVersion=..." ile ayarlanır.
var AppVersion = "1.1.0"

const (
	AppName  = "NOVA İndirici"
	PrefPort = 47319
)

//go:embed web
var webFS embed.FS

var (
	dataDir    string
	binDir     string
	token      string
	settings   *Settings
	settingsMu sync.Mutex
	jobs       *JobManager
	hub        = NewHub()
)

// ---------- settings ----------

type Settings struct {
	DownloadDir   string `json:"downloadDir"`
	Mode          string `json:"mode"`        // video | audio
	Quality       string `json:"quality"`     // best | 2160 | 1440 | 1080 | 720 | 480 | 360
	AudioFormat   string `json:"audioFormat"` // mp3 | m4a | opus | flac
	MaxConcurrent int    `json:"maxConcurrent"`
	EmbedThumb    bool   `json:"embedThumb"`
	Subtitles     bool   `json:"subtitles"`
}

func defaultSettings() *Settings {
	home, _ := os.UserHomeDir()
	return &Settings{
		DownloadDir:   filepath.Join(home, "Downloads", "NOVA"),
		Mode:          "video",
		Quality:       "best",
		AudioFormat:   "mp3",
		MaxConcurrent: 2,
		EmbedThumb:    true,
	}
}

func loadSettings() *Settings {
	s := defaultSettings()
	b, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if err == nil {
		_ = json.Unmarshal(b, s)
	}
	if s.MaxConcurrent < 1 || s.MaxConcurrent > 5 {
		s.MaxConcurrent = 2
	}
	if s.DownloadDir == "" {
		s.DownloadDir = defaultSettings().DownloadDir
	}
	return s
}

func (s *Settings) Save() error {
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0o644)
}

func (s *Settings) Snapshot() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return Settings{DownloadDir: s.DownloadDir, Mode: s.Mode, Quality: s.Quality, AudioFormat: s.AudioFormat,
		MaxConcurrent: s.MaxConcurrent, EmbedThumb: s.EmbedThumb, Subtitles: s.Subtitles}
}

// ---------- SSE hub ----------

type Hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	idle    *time.Timer
	seen    bool
}

func NewHub() *Hub { return &Hub{clients: map[chan []byte]struct{}{}} }

func (h *Hub) Broadcast(kind string, payload any) {
	b, _ := json.Marshal(map[string]any{"type": kind, "data": payload})
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- b:
		default:
		}
	}
}

func (h *Hub) add(c chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[c] = struct{}{}
	h.seen = true
	if h.idle != nil {
		h.idle.Stop()
		h.idle = nil
	}
}

func (h *Hub) remove(c chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	if len(h.clients) == 0 && os.Getenv("NOVA_NO_AUTOEXIT") == "" {
		// Pencere kapandıysa uygulamayı kapat (yenileme için kısa bir pay bırak).
		h.idle = time.AfterFunc(10*time.Second, func() {
			h.mu.Lock()
			n := len(h.clients)
			h.mu.Unlock()
			if n == 0 {
				log.Println("pencere kapandı, çıkılıyor")
				jobs.CancelAll()
				os.Exit(0)
			}
		})
	}
}

// ---------- helpers ----------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readBody(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func randToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// guard: yalnızca yerel istekler ve doğru anahtar
func guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if host != "127.0.0.1" && host != "localhost" {
			fail(w, 403, "forbidden")
			return
		}
		t := r.Header.Get("X-Nova-Token")
		if t == "" {
			t = r.URL.Query().Get("t")
		}
		if t != token {
			fail(w, 403, "forbidden")
			return
		}
		next(w, r)
	}
}

// ---------- main ----------

func main() {
	if handleCLI() {
		return
	}
	base, err := os.UserCacheDir() // Windows: %LOCALAPPDATA%
	if runtime.GOOS == "darwin" {
		base, err = os.UserConfigDir() // macOS: ~/Library/Application Support
	}
	if err != nil {
		base = os.TempDir()
	}
	dataDir = filepath.Join(base, "NovaIndirici")
	binDir = filepath.Join(dataDir, "bin")
	if v := os.Getenv("NOVA_DATA"); v != "" {
		dataDir = v
		binDir = filepath.Join(v, "bin")
	}
	_ = os.MkdirAll(binDir, 0o755)

	lf, err := os.OpenFile(filepath.Join(dataDir, "nova.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err == nil {
		log.SetOutput(lf)
	}
	log.Printf("%s %s başladı (%s)", AppName, AppVersion, time.Now().Format(time.RFC3339))

	// Tek örnek: tercih edilen port doluysa ve NOVA çalışıyorsa onu öne getir.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", PrefPort))
	if err != nil {
		if b, e := os.ReadFile(filepath.Join(dataDir, "instance.json")); e == nil {
			var inst struct{ URL string }
			if json.Unmarshal(b, &inst) == nil && inst.URL != "" && pingNova(PrefPort) {
				openWindow(inst.URL)
				return
			}
		}
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fatal("Sunucu başlatılamadı: " + err.Error())
		}
	}
	port := ln.Addr().(*net.TCPAddr).Port

	token = randToken()
	settings = loadSettings()
	jobs = NewJobManager()
	jobs.LoadHistory()
	_ = os.RemoveAll(filepath.Join(dataDir, "tmp"))
	go ytdlpVersion()
	go updateLoop()

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	static := http.FileServer(http.FS(sub))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		static.ServeHTTP(w, r)
	})
	mux.HandleFunc("/api/ping", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"app": "nova"})
	})
	mux.HandleFunc("/api/state", guard(handleState))
	mux.HandleFunc("/api/events", guard(handleEvents))
	mux.HandleFunc("/api/setup", guard(handleSetup))
	mux.HandleFunc("/api/info", guard(handleInfo))
	mux.HandleFunc("/api/download", guard(handleDownload))
	mux.HandleFunc("/api/cancel", guard(handleCancel))
	mux.HandleFunc("/api/retry", guard(handleRetry))
	mux.HandleFunc("/api/clear", guard(handleClear))
	mux.HandleFunc("/api/history/remove", guard(handleHistoryRemove))
	mux.HandleFunc("/api/settings", guard(handleSettings))
	mux.HandleFunc("/api/pick-folder", guard(handlePickFolder))
	mux.HandleFunc("/api/open", guard(handleOpen))
	mux.HandleFunc("/api/open-url", guard(handleOpenURL))
	mux.HandleFunc("/api/update-ytdlp", guard(handleUpdateYtdlp))
	mux.HandleFunc("/api/update/check", guard(handleUpdateCheck))
	mux.HandleFunc("/api/update/install", guard(handleUpdateInstall))

	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/#t=%s", port, token)
	b, _ := json.Marshal(map[string]string{"URL": url})
	_ = os.WriteFile(filepath.Join(dataDir, "instance.json"), b, 0o600)

	go func() {
		time.Sleep(150 * time.Millisecond)
		openWindow(url)
	}()
	log.Println("dinleniyor:", ln.Addr())
	if os.Getenv("NOVA_PRINT_URL") != "" {
		fmt.Println(url)
	}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fatal(err.Error())
	}
}

func pingNova(port int) bool {
	c := http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var v map[string]string
	return json.NewDecoder(resp.Body).Decode(&v) == nil && v["app"] == "nova"
}

func fatal(msg string) {
	log.Println("HATA:", msg)
	showError(AppName, msg)
	os.Exit(1)
}

// ---------- handlers ----------

func handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{
		"version":  AppVersion,
		"os":       runtime.GOOS,
		"update":   updateSnapshot(),
		"tools":    toolStatus(),
		"settings": settings.Snapshot(),
		"jobs":     jobs.List(),
		"history":  jobs.History(),
	})
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		fail(w, 500, "stream unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	c := make(chan []byte, 64)
	hub.add(c)
	defer hub.remove(c)
	fmt.Fprintf(w, ": hello\n\n")
	fl.Flush()
	tick := time.NewTicker(20 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case b := <-c:
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
		case <-tick.C:
			fmt.Fprintf(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		fail(w, 405, "POST")
		return
	}
	go EnsureTools()
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL      string `json:"url"`
		Playlist bool   `json:"playlist"`
	}
	if err := readBody(r, &req); err != nil || strings.TrimSpace(req.URL) == "" {
		fail(w, 400, "Bağlantı boş.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	info, err := FetchInfo(ctx, strings.TrimSpace(req.URL), req.Playlist)
	if err != nil {
		fail(w, 422, err.Error())
		return
	}
	writeJSON(w, 200, info)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	var req JobRequest
	if err := readBody(r, &req); err != nil || strings.TrimSpace(req.URL) == "" {
		fail(w, 400, "Bağlantı boş.")
		return
	}
	if !toolStatus().Ready {
		fail(w, 409, "Bileşenler henüz hazır değil.")
		return
	}
	j := jobs.Add(req)
	writeJSON(w, 200, j.View())
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string }
	_ = readBody(r, &req)
	jobs.Cancel(req.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleRetry(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string }
	_ = readBody(r, &req)
	jobs.Retry(req.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleClear(w http.ResponseWriter, r *http.Request) {
	var req struct{ What string }
	_ = readBody(r, &req)
	if req.What == "history" {
		jobs.ClearHistory()
	} else {
		jobs.ClearFinished()
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleHistoryRemove(w http.ResponseWriter, r *http.Request) {
	var req struct{ ID string }
	_ = readBody(r, &req)
	jobs.RemoveHistory(req.ID)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		writeJSON(w, 200, settings.Snapshot())
		return
	}
	var in Settings
	if err := readBody(r, &in); err != nil {
		fail(w, 400, "Geçersiz ayar.")
		return
	}
	settingsMu.Lock()
	if in.DownloadDir != "" {
		settings.DownloadDir = in.DownloadDir
	}
	if in.Mode == "video" || in.Mode == "audio" {
		settings.Mode = in.Mode
	}
	if in.Quality != "" {
		settings.Quality = in.Quality
	}
	if in.AudioFormat != "" {
		settings.AudioFormat = in.AudioFormat
	}
	if in.MaxConcurrent >= 1 && in.MaxConcurrent <= 5 {
		settings.MaxConcurrent = in.MaxConcurrent
	}
	settings.EmbedThumb = in.EmbedThumb
	settings.Subtitles = in.Subtitles
	err := settings.Save()
	settingsMu.Unlock()
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	jobs.Pump()
	writeJSON(w, 200, settings.Snapshot())
}

func handlePickFolder(w http.ResponseWriter, r *http.Request) {
	p, err := pickFolder(settings.Snapshot().DownloadDir)
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"path": p})
}

func handleOpen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path   string `json:"path"`
		Reveal bool   `json:"reveal"`
	}
	_ = readBody(r, &req)
	p := req.Path
	if p == "" {
		p = settings.Snapshot().DownloadDir
		_ = os.MkdirAll(p, 0o755)
	}
	if _, err := os.Stat(p); err != nil {
		fail(w, 404, "Dosya bulunamadı (taşınmış ya da silinmiş olabilir).")
		return
	}
	if err := openPath(p, req.Reveal); err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleOpenURL(w http.ResponseWriter, r *http.Request) {
	var req struct{ URL string }
	_ = readBody(r, &req)
	if !strings.HasPrefix(req.URL, "https://") && !strings.HasPrefix(req.URL, "mailto:samet.sahin.iletisim@gmail.com") {
		fail(w, 400, "Geçersiz bağlantı.")
		return
	}
	_ = openURL(req.URL)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func handleUpdateYtdlp(w http.ResponseWriter, r *http.Request) {
	out, err := UpdateYtdlp()
	if err != nil {
		fail(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"output": out, "version": ytdlpVersion()})
}
