// NOVA İndirici Kurulum — Samet Şahin
package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	AppName    = "NOVA İndirici"
	AppVersion = "1.0.0"
	ExeName    = "NOVA-Indirici.exe"
	FolderName = "NOVA Indirici"
	RegKey     = `Software\Microsoft\Windows\CurrentVersion\Uninstall\NovaIndirici`
)

//go:embed payload/NOVA-Indirici.exe
var payload []byte

//go:embed web
var webFS embed.FS

var existingDir = sync.OnceValue(existingDirRaw)

var (
	token    string
	beatMu   sync.Mutex
	lastBeat = time.Now()
	busy     bool
)

func defaultDir() string {
	if d := existingDir(); d != "" {
		return d
	}
	base, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "Programs", FolderName)
}

func normalizeDir(d string) string {
	d = filepath.Clean(strings.TrimSpace(d))
	if !strings.EqualFold(filepath.Base(d), FolderName) {
		d = filepath.Join(d, FolderName)
	}
	return d
}

// Install uygulamayı kurar ve kısayolları oluşturur.
func Install(dir string, desktop bool) (string, error) {
	dir = normalizeDir(dir)
	stopRunning()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("Klasör oluşturulamadı: %v", err)
	}
	exe := filepath.Join(dir, ExeName)
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, payload, 0o755); err != nil {
		return "", fmt.Errorf("Dosya yazılamadı: %v", err)
	}
	var err error
	for i := 0; i < 10; i++ {
		_ = os.Remove(exe)
		if err = os.Rename(tmp, exe); err == nil {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}
	if err != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("NOVA açıkken güncellenemedi; uygulamayı kapatıp tekrar deneyin")
	}
	if err := registerApp(exe, dir, desktop, int64(len(payload)/1024)); err != nil {
		return exe, fmt.Errorf("Kısayollar oluşturulamadı: %v", err)
	}
	return exe, nil
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		if (host != "127.0.0.1" && host != "localhost") || r.Header.Get("X-Setup-Token") != token {
			writeJSON(w, 403, map[string]string{"error": "forbidden"})
			return
		}
		beatMu.Lock()
		lastBeat = time.Now()
		beatMu.Unlock()
		next(w, r)
	}
}

func main() {
	silent, dirArg := false, ""
	for _, a := range os.Args[1:] {
		switch {
		case strings.EqualFold(a, "/S"):
			silent = true
		case strings.HasPrefix(strings.ToUpper(a), "/D="):
			dirArg = a[3:]
		}
	}
	if dirArg == "" {
		dirArg = defaultDir()
	}
	if silent {
		if _, err := Install(dirArg, true); err != nil {
			os.Exit(1)
		}
		return
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fallbackInstall(dirArg)
		return
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	token = hex.EncodeToString(b)

	sub, _ := fs.Sub(webFS, "web")
	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/info", guard(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"version": AppVersion, "dir": defaultDir(), "existing": existingDir() != "",
			"sizeMB": float64(len(payload)) / 1048576})
	}))
	mux.HandleFunc("/api/ping", guard(func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]bool{"ok": true}) }))
	mux.HandleFunc("/api/pick", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Dir string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		p, err := pickFolder(filepath.Dir(normalizeDir(req.Dir)))
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		if p != "" {
			p = normalizeDir(p)
		}
		writeJSON(w, 200, map[string]string{"dir": p})
	}))
	var installed string
	mux.HandleFunc("/api/install", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Dir     string `json:"dir"`
			Desktop bool   `json:"desktop"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		busy = true
		exe, err := Install(req.Dir, req.Desktop)
		busy = false
		if err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
		installed = exe
		writeJSON(w, 200, map[string]string{"exe": exe, "dir": filepath.Dir(exe)})
	}))
	mux.HandleFunc("/api/finish", guard(func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Launch bool }
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Launch && installed != "" {
			launch(installed)
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		go func() { time.Sleep(600 * time.Millisecond); closeWindow(); os.Exit(0) }()
	}))

	// Pencere kapanırsa (kalp atışı kesilirse) kurulumu sonlandır.
	go func() {
		for {
			time.Sleep(2 * time.Second)
			beatMu.Lock()
			idle := time.Since(lastBeat)
			beatMu.Unlock()
			if idle > 12*time.Second && !busy {
				os.Exit(0)
			}
		}
	}()

	url := fmt.Sprintf("http://127.0.0.1:%d/#t=%s", ln.Addr().(*net.TCPAddr).Port, token)
	if os.Getenv("NOVA_PRINT_URL") != "" {
		fmt.Println(url)
		beatMu.Lock()
		lastBeat = time.Now().Add(time.Hour)
		beatMu.Unlock()
	} else if !openWindow(url) {
		ln.Close()
		fallbackInstall(dirArg)
		return
	}
	_ = http.Serve(ln, mux)
}
