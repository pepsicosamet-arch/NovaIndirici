package main

import (
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	ytdlpURLWin  = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
	ytdlpURLUnix = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
	ffmpegURLWin = "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip"
	ytdlpURLMac  = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
	// macOS için hazır FFmpeg (ffmpeg-static projesi), mimariye göre seçilir.
	ffmpegMacBase = "https://github.com/eugeneware/ffmpeg-static/releases/download/b6.0/"
)

func exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func ytdlpPath() string { return filepath.Join(binDir, exe("yt-dlp")) }
func ffmpegPath() string {
	p := filepath.Join(binDir, exe("ffmpeg"))
	if _, err := os.Stat(p); err != nil && runtime.GOOS != "windows" {
		if sys, e := exec.LookPath("ffmpeg"); e == nil {
			return sys
		}
	}
	return p
}

func exists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Size() > 0
}

type ToolStatus struct {
	Ytdlp   bool   `json:"ytdlp"`
	Ffmpeg  bool   `json:"ffmpeg"`
	Ready   bool   `json:"ready"`
	Busy    bool   `json:"busy"`
	Stage   string `json:"stage"`
	Percent int    `json:"percent"`
	Error   string `json:"error"`
	Version string `json:"version"`
}

var (
	setupMu    sync.Mutex
	setupState = struct {
		busy    bool
		stage   string
		percent int
		err     string
	}{}
	verMu    sync.Mutex
	verCache string
)

func toolStatus() ToolStatus {
	setupMu.Lock()
	defer setupMu.Unlock()
	y := exists(ytdlpPath())
	f := exists(ffmpegPath())
	return ToolStatus{
		Ytdlp: y, Ffmpeg: f, Ready: y && f,
		Busy: setupState.busy, Stage: setupState.stage, Percent: setupState.percent, Error: setupState.err,
		Version: ytdlpVersionCached(),
	}
}

func ytdlpVersionCached() string {
	verMu.Lock()
	defer verMu.Unlock()
	return verCache
}

func ytdlpVersion() string {
	if !exists(ytdlpPath()) {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ytdlpPath(), "--version")
	hideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(string(out))
	verMu.Lock()
	verCache = v
	verMu.Unlock()
	return v
}

func setStage(stage string, pct int) {
	setupMu.Lock()
	setupState.stage, setupState.percent = stage, pct
	setupMu.Unlock()
	hub.Broadcast("tools", toolStatusNoLock())
}

// toolStatusNoLock kilit dışında çağrılır (toolStatus kendi kilidini alır).
func toolStatusNoLock() ToolStatus { return toolStatus() }

// EnsureTools eksik yt-dlp ve ffmpeg dosyalarını indirir.
func EnsureTools() {
	setupMu.Lock()
	if setupState.busy {
		setupMu.Unlock()
		return
	}
	setupState.busy, setupState.err = true, ""
	setupMu.Unlock()

	err := ensureTools()

	setupMu.Lock()
	setupState.busy = false
	if err != nil {
		setupState.err = err.Error()
		log.Println("kurulum hatası:", err)
	} else {
		setupState.stage, setupState.percent = "Hazır", 100
	}
	setupMu.Unlock()
	ytdlpVersion()
	hub.Broadcast("tools", toolStatus())
}

func ensureTools() error {
	if !exists(ytdlpPath()) {
		u := ytdlpURLUnix
		switch runtime.GOOS {
		case "windows":
			u = ytdlpURLWin
		case "darwin":
			u = ytdlpURLMac
		}
		if err := downloadFile(u, ytdlpPath(), "yt-dlp indiriliyor"); err != nil {
			return fmt.Errorf("yt-dlp indirilemedi: %w", err)
		}
		_ = os.Chmod(ytdlpPath(), 0o755)
	}
	if !exists(ffmpegPath()) {
		if runtime.GOOS == "darwin" {
			return ensureFFmpegMac()
		}
		if runtime.GOOS != "windows" {
			return errors.New("ffmpeg bulunamadı; sisteminize ffmpeg kurun")
		}
		zipPath := filepath.Join(binDir, "ffmpeg.zip.part")
		if err := downloadFile(ffmpegURLWin, zipPath, "FFmpeg indiriliyor"); err != nil {
			return fmt.Errorf("FFmpeg indirilemedi: %w", err)
		}
		setStage("FFmpeg çıkarılıyor", 100)
		if err := extractFFmpeg(zipPath); err != nil {
			return fmt.Errorf("FFmpeg çıkarılamadı: %w", err)
		}
		_ = os.Remove(zipPath)
	}
	return nil
}

type progressWriter struct {
	total, done int64
	stage       string
	last        time.Time
	lastPct     int
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	if p.total > 0 {
		pct := int(p.done * 100 / p.total)
		if pct != p.lastPct && time.Since(p.last) > 200*time.Millisecond {
			p.last, p.lastPct = time.Now(), pct
			setStage(p.stage, pct)
		}
	}
	return len(b), nil
}

func downloadFile(url, dest, stage string) error {
	setStage(stage, 0)
	client := &http.Client{Timeout: 30 * time.Minute}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "NOVA-Indirici/"+AppVersion)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	pw := &progressWriter{total: resp.ContentLength, stage: stage}
	_, err = io.Copy(io.MultiWriter(f, pw), resp.Body)
	f.Close()
	if err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Remove(dest)
	return os.Rename(tmp, dest)
}

func extractFFmpeg(zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	want := map[string]bool{"ffmpeg.exe": true, "ffprobe.exe": true}
	found := 0
	for _, f := range zr.File {
		name := filepath.Base(f.Name)
		if !want[name] || !strings.Contains(f.Name, "/bin/") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		dst := filepath.Join(binDir, name)
		out, err := os.Create(dst + ".tmp")
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
		_ = os.Remove(dst)
		if err := os.Rename(dst+".tmp", dst); err != nil {
			return err
		}
		found++
	}
	if found == 0 {
		return errors.New("arşivde ffmpeg.exe yok")
	}
	return nil
}

func UpdateYtdlp() (string, error) {
	if !exists(ytdlpPath()) {
		return "", errors.New("yt-dlp kurulu değil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ytdlpPath(), "-U")
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	s := strings.TrimSpace(string(out))
	if err != nil {
		return s, fmt.Errorf("güncelleme başarısız: %s", lastLine(s))
	}
	return s, nil
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// ensureFFmpegMac ffmpeg ve ffprobe'u (gzip) indirip açar.
func ensureFFmpegMac() error {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		dst := filepath.Join(binDir, name)
		if exists(dst) {
			continue
		}
		gz := dst + ".gz.part"
		label := "FFmpeg indiriliyor"
		if name == "ffprobe" {
			label = "FFprobe indiriliyor"
		}
		if err := downloadFile(ffmpegMacBase+name+"-darwin-"+arch+".gz", gz, label); err != nil {
			return fmt.Errorf("%s indirilemedi: %w", name, err)
		}
		if err := gunzipTo(gz, dst); err != nil {
			return fmt.Errorf("%s açılamadı: %w", name, err)
		}
		_ = os.Remove(gz)
		_ = os.Chmod(dst, 0o755)
	}
	return nil
}

func gunzipTo(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	out, err := os.Create(dst + ".tmp")
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(dst+".tmp", dst)
}
