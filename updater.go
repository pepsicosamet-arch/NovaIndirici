package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	RepoBase  = envOr("NOVA_UPDATE_BASE", "https://github.com/pepsicosamet-arch/NovaIndirici/")
	UpdateURL = envOr("NOVA_UPDATE_URL", RepoBase+"raw/main/dist/latest.json")
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type UpdateState struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`
	Available bool   `json:"available"`
	Checking  bool   `json:"checking"`
	Busy      bool   `json:"busy"`
	Percent   int    `json:"percent"`
	Stage     string `json:"stage"`
	Error     string `json:"error"`
	Checked   int64  `json:"checked"`
	url       string
}

var (
	updMu sync.Mutex
	upd   = UpdateState{}
)

func updateSnapshot() UpdateState {
	updMu.Lock()
	defer updMu.Unlock()
	s := upd
	s.Current = AppVersion
	return s
}

func updEmit() { hub.Broadcast("update", updateSnapshot()) }

// newer: a > b ise true (1.10.0 > 1.9.3).
func newer(a, b string) bool {
	pa, pb := verParts(a), verParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func verParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	for i, p := range strings.SplitN(v, ".", 3) {
		n, _ := strconv.Atoi(strings.TrimFunc(p, func(r rune) bool { return r < '0' || r > '9' }))
		out[i] = n
	}
	return out
}

type latestInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Mac     string `json:"mac"`
	Notes   string `json:"notes"`
}

// updateSources: biri çalışmazsa sıradakine geçilir.
func updateSources() []string {
	if v := os.Getenv("NOVA_UPDATE_URL"); v != "" {
		return []string{v}
	}
	return []string{
		UpdateURL,
		RepoBase + "releases/latest/download/latest.json",
		"https://novaindirici.com.tr/latest.json",
	}
}

func fetchLatest() (*latestInfo, error) {
	c := http.Client{Timeout: 15 * time.Second}
	var lastErr error = errors.New("Güncelleme sunucusuna ulaşılamadı. İnternet bağlantını kontrol et.")
	for _, src := range updateSources() {
		req, _ := http.NewRequest("GET", src+"?t="+strconv.FormatInt(time.Now().Unix()/300, 10), nil)
		req.Header.Set("User-Agent", "NOVA-Indirici/"+AppVersion)
		req.Header.Set("Cache-Control", "no-cache")
		resp, err := c.Do(req)
		if err != nil {
			log.Println("güncelleme kaynağı ulaşılamadı:", src, err)
			continue
		}
		var li latestInfo
		if resp.StatusCode == 200 {
			err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&li)
		} else {
			err = fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		resp.Body.Close()
		if err == nil && li.Version != "" {
			return &li, nil
		}
		log.Println("güncelleme kaynağı geçersiz:", src, err)
		lastErr = errors.New("Güncelleme bilgisi şu an alınamıyor. Biraz sonra tekrar dene.")
	}
	return nil, lastErr
}

func CheckUpdate() error {
	updMu.Lock()
	if upd.Checking || upd.Busy {
		updMu.Unlock()
		return nil
	}
	upd.Checking, upd.Error = true, ""
	updMu.Unlock()
	updEmit()

	err := func() error {
		li, err := fetchLatest()
		if err != nil {
			return err
		}
		updMu.Lock()
		upd.Latest, upd.Notes = li.Version, li.Notes
		upd.url = li.URL
		if runtime.GOOS == "darwin" {
			upd.url = li.Mac
		}
		// Güvenlik: yalnızca NOVA deposundan gelen kurulum dosyaları kabul edilir.
		upd.Available = newer(li.Version, AppVersion) && strings.HasPrefix(upd.url, RepoBase)
		updMu.Unlock()
		return nil
	}()

	updMu.Lock()
	upd.Checking = false
	upd.Checked = time.Now().UnixMilli()
	if err != nil {
		upd.Error = err.Error()
	}
	updMu.Unlock()
	updEmit()
	return err
}

func updateLoop() {
	time.Sleep(4 * time.Second)
	for {
		if err := CheckUpdate(); err != nil {
			log.Println("güncelleme denetimi:", err)
		}
		time.Sleep(6 * time.Hour)
	}
}

func setUpd(stage string, pct int) {
	updMu.Lock()
	upd.Stage, upd.Percent = stage, pct
	updMu.Unlock()
	updEmit()
}

// InstallUpdate yeni kurulum dosyasını indirip sessiz kurulumla çalıştırır.
func InstallUpdate() error {
	if runtime.GOOS == "darwin" {
		// Mac: yeni DMG'yi tarayıcıda indir; kullanıcı uygulamayı Applications'a sürükler.
		updMu.Lock()
		u, ok := upd.url, upd.Available
		updMu.Unlock()
		if !ok || u == "" {
			return errors.New("Kurulacak güncelleme yok.")
		}
		return openURL(u)
	}
	if runtime.GOOS != "windows" {
		return errors.New("Otomatik güncelleme yalnızca Windows'ta çalışır.")
	}
	updMu.Lock()
	if !upd.Available || upd.Busy {
		updMu.Unlock()
		return errors.New("Kurulacak güncelleme yok.")
	}
	upd.Busy, upd.Error = true, ""
	url, ver := upd.url, upd.Latest
	updMu.Unlock()

	fail := func(e error) error {
		updMu.Lock()
		upd.Busy, upd.Error, upd.Stage = false, e.Error(), ""
		updMu.Unlock()
		updEmit()
		return e
	}

	setUpd("İndiriliyor", 0)
	c := http.Client{Timeout: 20 * time.Minute}
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "NOVA-Indirici/"+AppVersion)
	resp, err := c.Do(req)
	if err != nil {
		return fail(errors.New("Güncelleme indirilemedi."))
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fail(fmt.Errorf("Güncelleme indirilemedi (HTTP %d).", resp.StatusCode))
	}
	dir := filepath.Join(dataDir, "update")
	_ = os.RemoveAll(dir)
	_ = os.MkdirAll(dir, 0o755)
	dst := filepath.Join(dir, "NOVA-Indirici-Setup-"+strings.TrimPrefix(ver, "v")+".exe")
	f, err := os.Create(dst)
	if err != nil {
		return fail(err)
	}
	var buf bytes.Buffer
	total := resp.ContentLength
	var done int64
	last := -1
	chunk := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(chunk)
		if n > 0 {
			if _, werr := f.Write(chunk[:n]); werr != nil {
				f.Close()
				return fail(werr)
			}
			if buf.Len() < 2 {
				buf.Write(chunk[:n])
			}
			done += int64(n)
			if total > 0 {
				if p := int(done * 100 / total); p != last {
					last = p
					setUpd("İndiriliyor", p)
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return fail(errors.New("İndirme yarıda kesildi."))
		}
	}
	f.Close()
	if done < 1<<20 || !bytes.HasPrefix(buf.Bytes(), []byte("MZ")) {
		return fail(errors.New("İndirilen dosya geçersiz."))
	}
	setUpd("Kuruluyor", 100)
	jobs.CancelAll()
	if err := runInstaller(dst); err != nil {
		return fail(fmt.Errorf("Kurulum başlatılamadı: %v", err))
	}
	go func() {
		time.Sleep(1500 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if err := CheckUpdate(); err != nil {
		fail(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, updateSnapshot())
}

func handleUpdateInstall(w http.ResponseWriter, r *http.Request) {
	go func() {
		if err := InstallUpdate(); err != nil {
			log.Println("güncelleme:", err)
		}
	}()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
