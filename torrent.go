package main

// Torrent indirme: magnet bağlantıları ve .torrent dosyaları aria2 ile indirilir.
// aria2 her iş için ayrı başlatılır ve JSON-RPC üzerinden izlenir.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const aria2URLWin = "https://github.com/aria2/aria2/releases/download/release-1.37.0/aria2-1.37.0-win-64bit-build1.zip"

// Halka açık izleyiciler: magnet bağlantılarında eş bulmayı hızlandırır.
var publicTrackers = strings.Join([]string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.stealth.si:80/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://open.demonii.com:1337/announce",
	"udp://tracker.openbittorrent.com:6969/announce",
}, ",")

func isTorrentURL(u string) bool {
	l := strings.ToLower(strings.TrimSpace(u))
	if strings.HasPrefix(l, "magnet:?") {
		return true
	}
	if p, err := url.Parse(l); err == nil && strings.HasSuffix(p.Path, ".torrent") {
		return true
	}
	return false
}

// magnetName magnet bağlantısındaki "dn" (görünen ad) değerini döndürür.
func magnetName(u string) string {
	if i := strings.Index(u, "?"); i >= 0 {
		if q, err := url.ParseQuery(u[i+1:]); err == nil {
			return q.Get("dn")
		}
	}
	return ""
}

func aria2Path() string {
	p := filepath.Join(binDir, exe("aria2c"))
	if exists(p) {
		return p
	}
	if runtime.GOOS != "windows" {
		for _, c := range []string{"/opt/homebrew/bin/aria2c", "/usr/local/bin/aria2c", "/usr/bin/aria2c"} {
			if exists(c) {
				return c
			}
		}
		if sys, err := exec.LookPath("aria2c"); err == nil {
			return sys
		}
	}
	return p
}

var aria2Mu sync.Mutex

// ensureAria2 torrent motorunu gerekirse indirir (Windows).
func ensureAria2(stage func(string)) error {
	aria2Mu.Lock()
	defer aria2Mu.Unlock()
	if exists(aria2Path()) {
		return nil
	}
	switch runtime.GOOS {
	case "windows":
		stage("Torrent motoru indiriliyor")
		zipPath := filepath.Join(binDir, "aria2.zip.part")
		if err := downloadFile(aria2URLWin, zipPath, "Torrent motoru indiriliyor"); err != nil {
			return fmt.Errorf("Torrent motoru indirilemedi: %w", err)
		}
		defer os.Remove(zipPath)
		zr, err := zip.OpenReader(zipPath)
		if err != nil {
			return err
		}
		defer zr.Close()
		for _, f := range zr.File {
			if strings.EqualFold(filepath.Base(f.Name), "aria2c.exe") {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				dst := filepath.Join(binDir, "aria2c.exe")
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
				return os.Rename(dst+".tmp", dst)
			}
		}
		return errors.New("arşivde aria2c.exe yok")
	case "darwin":
		return errors.New("Mac'te torrent için önce Terminal'de şu komutu çalıştır: brew install aria2")
	default:
		return errors.New("Torrent için sisteme aria2 kurulmalı (aria2c).")
	}
}

// ---------- aria2 JSON-RPC ----------

type aria2 struct {
	url, secret string
	client      *http.Client
}

func (a *aria2) call(method string, params ...any) (json.RawMessage, error) {
	body, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": "nova", "method": method,
		"params": append([]any{"token:" + a.secret}, params...),
	})
	resp, err := a.client.Post(a.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if r.Error != nil {
		return nil, errors.New(r.Error.Message)
	}
	return r.Result, nil
}

type aria2Status struct {
	Status          string   `json:"status"`
	TotalLength     string   `json:"totalLength"`
	CompletedLength string   `json:"completedLength"`
	DownloadSpeed   string   `json:"downloadSpeed"`
	Connections     string   `json:"connections"`
	NumSeeders      string   `json:"numSeeders"`
	FollowedBy      []string `json:"followedBy"`
	ErrorCode       string   `json:"errorCode"`
	ErrorMessage    string   `json:"errorMessage"`
	Files           []struct {
		Path     string `json:"path"`
		Selected string `json:"selected"`
	} `json:"files"`
	Bittorrent *struct {
		Info *struct {
			Name string `json:"name"`
		} `json:"info"`
	} `json:"bittorrent"`
}

func freePort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 6800 + int(time.Now().UnixNano()%1000)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func i64(s string) int64 { v, _ := strconv.ParseInt(s, 10, 64); return v }

func aria2Error(st *aria2Status) string {
	m := strings.ToLower(st.ErrorMessage)
	switch {
	case st.ErrorCode == "7" || strings.Contains(m, "no peer") || strings.Contains(m, "timeout"):
		return "Bu torrent için yeterli kaynak (eş) bulunamadı. Daha sonra tekrar dene."
	case strings.Contains(m, "not enough space") || st.ErrorCode == "9":
		return "Diskte yeterli boş alan yok."
	case strings.Contains(m, "torrent") && strings.Contains(m, "parse"):
		return "Torrent dosyası okunamadı."
	case st.ErrorMessage != "":
		return trimTo(st.ErrorMessage, 200)
	}
	return "Torrent indirilemedi (hata " + st.ErrorCode + ")."
}

// runTorrent bir torrent işini baştan sona yürütür.
func (m *JobManager) runTorrent(ctx context.Context, j *Job, dest string) error {
	setStage := func(s string) {
		j.mu.Lock()
		j.Stage = s
		j.mu.Unlock()
		m.emit(j)
	}
	if err := ensureAria2(setStage); err != nil {
		return err
	}
	j.mu.Lock()
	req := j.JobRequest
	torrentData := j.torrentData
	j.mu.Unlock()

	_ = os.MkdirAll(dest, 0o755)
	port := freePort()
	secret := randToken()
	args := []string{
		"--enable-rpc", "--rpc-listen-port=" + strconv.Itoa(port), "--rpc-secret=" + secret,
		"--rpc-listen-all=false", "--dir=" + dest,
		"--seed-time=0", "--bt-save-metadata=false", "--rpc-save-upload-metadata=false", "--follow-torrent=mem",
		"--enable-dht=true", "--bt-enable-lpd=true", "--enable-peer-exchange=true",
		"--dht-file-path=" + filepath.Join(dataDir, "dht.dat"),
		"--bt-tracker=" + publicTrackers, "--bt-stop-timeout=900",
		"--file-allocation=none", "--continue=true", "--auto-file-renaming=true",
		"--max-connection-per-server=8", "--split=8", "--summary-interval=0",
		"--console-log-level=error", "--quiet=true",
		"--stop-with-process=" + strconv.Itoa(os.Getpid()),
	}
	cmd := exec.CommandContext(ctx, aria2Path(), args...)
	hideWindow(cmd)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Torrent motoru başlatılamadı: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	defer func() {
		_ = killTree(cmd)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()

	a := &aria2{url: fmt.Sprintf("http://127.0.0.1:%d/jsonrpc", port), secret: secret, client: &http.Client{Timeout: 10 * time.Second}}
	var gid string
	var err error
	for i := 0; i < 50; i++ { // motorun açılmasını bekle (en fazla ~10 sn)
		var raw json.RawMessage
		if torrentData != "" {
			raw, err = a.call("aria2.addTorrent", torrentData, []string{}, map[string]any{})
		} else {
			raw, err = a.call("aria2.addUri", []string{req.URL}, map[string]any{})
		}
		if err == nil {
			_ = json.Unmarshal(raw, &gid)
			break
		}
		if !strings.Contains(err.Error(), "connect") && !strings.Contains(err.Error(), "refused") && !strings.Contains(err.Error(), "EOF") {
			return fmt.Errorf("Torrent eklenemedi: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-done:
			return fmt.Errorf("Torrent motoru kapandı: %v %s", e, lastLine(stderr.String()))
		case <-time.After(200 * time.Millisecond):
		}
	}
	if gid == "" {
		return fmt.Errorf("Torrent motoruna bağlanılamadı: %v", err)
	}
	setStage("Bilgiler alınıyor")

	tick := time.NewTicker(700 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-done:
			return fmt.Errorf("Torrent motoru beklenmedik şekilde kapandı: %v", e)
		case <-tick.C:
		}
		raw, err := a.call("aria2.tellStatus", gid)
		if err != nil {
			continue
		}
		var st aria2Status
		if json.Unmarshal(raw, &st) != nil {
			continue
		}
		// Magnet: önce meta veri indirilir, sonra asıl indirme yeni bir gid ile sürer.
		if len(st.FollowedBy) > 0 && (st.Status == "complete" || st.Status == "active") {
			gid = st.FollowedBy[0]
			continue
		}
		total, got, speed := i64(st.TotalLength), i64(st.CompletedLength), i64(st.DownloadSpeed)
		name := ""
		if st.Bittorrent != nil && st.Bittorrent.Info != nil {
			name = st.Bittorrent.Info.Name
		}
		isMeta := strings.HasPrefix(name, "[METADATA]") || (name == "" && total == 0)
		j.mu.Lock()
		if name != "" && !strings.HasPrefix(name, "[METADATA]") {
			j.Title = name
		}
		j.Done, j.Total, j.Speed = got, total, float64(speed)
		if total > 0 {
			j.Percent = float64(got) / float64(total) * 100
		}
		j.ETA = 0
		if speed > 0 && total > got {
			j.ETA = int((total - got) / speed)
		}
		j.ItemTitle = fmt.Sprintf("%s eş · %s kaynak", st.Connections, st.NumSeeders)
		switch {
		case st.Status == "error":
			msg := aria2Error(&st)
			j.mu.Unlock()
			return errors.New(msg)
		case st.Status == "complete" && !isMeta:
			for _, f := range st.Files {
				if f.Selected != "false" && f.Path != "" && !strings.HasPrefix(filepath.Base(f.Path), "[METADATA]") {
					j.Files = append(j.Files, filepath.Clean(f.Path))
				}
			}
			j.mu.Unlock()
			m.emit(j)
			return nil
		case isMeta:
			j.Status, j.Stage = "downloading", "Eşler aranıyor"
		case speed == 0 && got == 0:
			j.Status, j.Stage = "downloading", "Eşler aranıyor"
		default:
			j.Status, j.Stage = "downloading", "İndiriliyor"
		}
		j.mu.Unlock()
		m.emit(j)
	}
}

// decodeTorrentFile tarayıcıdan gelen base64 .torrent içeriğini doğrular.
func decodeTorrentFile(b64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) < 20 || raw[0] != 'd' || !bytes.Contains(raw, []byte("4:info")) {
		return "", errors.New("Bu bir torrent dosyası değil.")
	}
	return b64, nil
}
