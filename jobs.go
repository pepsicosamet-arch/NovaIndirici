package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---------- info ----------

type MediaInfo struct {
	URL        string   `json:"url"`
	Title      string   `json:"title"`
	Uploader   string   `json:"uploader"`
	Thumbnail  string   `json:"thumbnail"`
	Duration   float64  `json:"duration"`
	Site       string   `json:"site"`
	IsPlaylist bool     `json:"isPlaylist"`
	Count      int      `json:"count"`
	Heights    []int    `json:"heights"`
	HasList    bool     `json:"hasList"`
	Entries    []string `json:"entries"`
	IsTorrent  bool     `json:"isTorrent"`
}

func baseArgs() []string {
	return []string{"--no-warnings", "--no-colors", "--encoding", "utf-8", "--ffmpeg-location", ffmpegPath()}
}

func ytdlpCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, ytdlpPath(), args...)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	hideWindow(cmd)
	return cmd
}

func FetchInfo(ctx context.Context, url string, playlist bool) (*MediaInfo, error) {
	if !exists(ytdlpPath()) {
		return nil, errors.New("yt-dlp henüz hazır değil.")
	}
	args := append(baseArgs(), "-J", "--flat-playlist")
	if playlist {
		args = append(args, "--yes-playlist")
	} else {
		args = append(args, "--no-playlist")
	}
	args = append(args, "--", url)
	cmd := ytdlpCmd(ctx, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New(friendlyError(stderr.String()))
	}
	var raw struct {
		Type       string  `json:"_type"`
		Title      string  `json:"title"`
		Uploader   string  `json:"uploader"`
		Channel    string  `json:"channel"`
		Thumbnail  string  `json:"thumbnail"`
		Duration   float64 `json:"duration"`
		Extractor  string  `json:"extractor_key"`
		WebURL     string  `json:"webpage_url"`
		Thumbnails []struct {
			URL string `json:"url"`
		} `json:"thumbnails"`
		Formats []struct {
			Height *int   `json:"height"`
			Vcodec string `json:"vcodec"`
		} `json:"formats"`
		Entries []struct {
			Title      string `json:"title"`
			Thumbnails []struct {
				URL string `json:"url"`
			} `json:"thumbnails"`
		} `json:"entries"`
		PlaylistCount int `json:"playlist_count"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, errors.New("Yanıt okunamadı.")
	}
	mi := &MediaInfo{URL: url, Title: raw.Title, Uploader: raw.Uploader, Thumbnail: raw.Thumbnail,
		Duration: raw.Duration, Site: raw.Extractor, HasList: strings.Contains(url, "list=")}
	if mi.Uploader == "" {
		mi.Uploader = raw.Channel
	}
	if mi.Thumbnail == "" && len(raw.Thumbnails) > 0 {
		mi.Thumbnail = raw.Thumbnails[len(raw.Thumbnails)-1].URL
	}
	if raw.Type == "playlist" {
		mi.IsPlaylist = true
		mi.Count = len(raw.Entries)
		if raw.PlaylistCount > mi.Count {
			mi.Count = raw.PlaylistCount
		}
		for i, e := range raw.Entries {
			if i >= 8 {
				break
			}
			mi.Entries = append(mi.Entries, e.Title)
			if mi.Thumbnail == "" && len(e.Thumbnails) > 0 {
				mi.Thumbnail = e.Thumbnails[len(e.Thumbnails)-1].URL
			}
		}
	}
	seen := map[int]bool{}
	for _, f := range raw.Formats {
		if f.Height != nil && *f.Height > 0 && f.Vcodec != "none" {
			h := *f.Height
			for _, std := range []int{4320, 2160, 1440, 1080, 720, 480, 360, 240, 144} {
				if h >= std-20 {
					seen[std] = true
					break
				}
			}
		}
	}
	for h := range seen {
		mi.Heights = append(mi.Heights, h)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(mi.Heights)))
	return mi, nil
}

func friendlyError(stderr string) string {
	var last string
	for _, l := range strings.Split(stderr, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "ERROR:") {
			last = strings.TrimSpace(strings.TrimPrefix(l, "ERROR:"))
		}
	}
	low := strings.ToLower(last)
	switch {
	case last == "":
		if s := strings.TrimSpace(stderr); s != "" {
			return lastLine(s)
		}
		return "Bilinmeyen bir hata oluştu."
	case strings.Contains(low, "unsupported url"):
		return "Bu bağlantı desteklenmiyor."
	case strings.Contains(low, "private video"), strings.Contains(low, "login required"), strings.Contains(low, "sign in"):
		return "Bu içerik giriş gerektiriyor ya da gizli. (" + trimTo(last, 140) + ")"
	case strings.Contains(low, "video unavailable"), strings.Contains(low, "not available"):
		return "İçerik kullanılamıyor ya da kaldırılmış."
	case strings.Contains(low, "unable to download webpage"), strings.Contains(low, "getaddrinfo"), strings.Contains(low, "timed out"):
		return "Bağlantı kurulamadı. İnternet bağlantınızı kontrol edin."
	case strings.Contains(low, "requested format is not available"):
		return "Seçilen kalite bu içerikte yok. Başka bir kalite deneyin."
	}
	return trimTo(last, 220)
}

func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ---------- jobs ----------

type JobRequest struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail"`
	Mode        string `json:"mode"`
	Quality     string `json:"quality"`
	AudioFormat string `json:"audioFormat"`
	Playlist    bool   `json:"playlist"`
	Torrent     string `json:"torrent,omitempty"` // .torrent dosyası (base64), yalnızca istekte
}

type Job struct {
	mu sync.Mutex
	JobRequest
	ID          string   `json:"id"`
	Status      string   `json:"status"` // queued starting downloading processing done error canceled
	Stage       string   `json:"stage"`
	Percent     float64  `json:"percent"`
	Speed       float64  `json:"speed"`
	ETA         int      `json:"eta"`
	Done        int64    `json:"done"`
	Total       int64    `json:"total"`
	Item        int      `json:"item"`
	Items       int      `json:"items"`
	ItemTitle   string   `json:"itemTitle"`
	Files       []string `json:"files"`
	Error       string   `json:"error"`
	Created     int64    `json:"created"`
	Finished    int64    `json:"finished"`
	cancel      context.CancelFunc
	canceled    bool
	torrentData string
}

type JobView struct {
	JobRequest
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Stage     string   `json:"stage"`
	Percent   float64  `json:"percent"`
	Speed     float64  `json:"speed"`
	ETA       int      `json:"eta"`
	Done      int64    `json:"done"`
	Total     int64    `json:"total"`
	Item      int      `json:"item"`
	Items     int      `json:"items"`
	ItemTitle string   `json:"itemTitle"`
	Files     []string `json:"files"`
	Error     string   `json:"error"`
	Created   int64    `json:"created"`
	Finished  int64    `json:"finished"`
}

func (j *Job) View() JobView {
	j.mu.Lock()
	defer j.mu.Unlock()
	return JobView{JobRequest: j.JobRequest, ID: j.ID, Status: j.Status, Stage: j.Stage, Percent: j.Percent,
		Speed: j.Speed, ETA: j.ETA, Done: j.Done, Total: j.Total, Item: j.Item, Items: j.Items,
		ItemTitle: j.ItemTitle, Files: append([]string(nil), j.Files...), Error: j.Error, Created: j.Created, Finished: j.Finished}
}

type JobManager struct {
	mu      sync.Mutex
	jobs    []*Job
	history []JobView
	seq     int
}

func NewJobManager() *JobManager { return &JobManager{} }

func (m *JobManager) histPath() string { return filepath.Join(dataDir, "history.json") }

func (m *JobManager) LoadHistory() {
	b, err := os.ReadFile(m.histPath())
	if err == nil {
		_ = json.Unmarshal(b, &m.history)
	}
}

func (m *JobManager) saveHistoryLocked() {
	if len(m.history) > 300 {
		m.history = m.history[:300]
	}
	b, _ := json.Marshal(m.history)
	if err := os.WriteFile(m.histPath(), b, 0o644); err != nil {
		log.Println("geçmiş kaydedilemedi:", err)
	}
}

func (m *JobManager) List() []JobView {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]JobView, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, j.View())
	}
	return out
}

func (m *JobManager) History() []JobView {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]JobView{}, m.history...)
}

func (m *JobManager) Add(req JobRequest) *Job {
	s := settings.Snapshot()
	torrentData := req.Torrent
	req.Torrent = ""
	if req.Mode != "audio" && req.Mode != "torrent" {
		req.Mode = "video"
	}
	if req.Quality == "" {
		req.Quality = s.Quality
	}
	if req.AudioFormat == "" {
		req.AudioFormat = s.AudioFormat
	}
	m.mu.Lock()
	m.seq++
	j := &Job{JobRequest: req, ID: fmt.Sprintf("j%d-%d", time.Now().Unix(), m.seq), Status: "queued",
		Stage: "Sırada", Created: time.Now().UnixMilli()}
	j.torrentData = torrentData
	if j.Title == "" {
		j.Title = req.URL
	}
	m.jobs = append(m.jobs, j)
	m.mu.Unlock()
	m.emit(j)
	m.Pump()
	return j
}

func (m *JobManager) emit(j *Job) { hub.Broadcast("job", j.View()) }

func (m *JobManager) find(id string) *Job {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, j := range m.jobs {
		if j.ID == id {
			return j
		}
	}
	return nil
}

// Pump eş zamanlı sınıra göre sıradaki işleri başlatır.
func (m *JobManager) Pump() {
	limit := settings.Snapshot().MaxConcurrent
	m.mu.Lock()
	running := 0
	for _, j := range m.jobs {
		j.mu.Lock()
		if j.Status == "starting" || j.Status == "downloading" || j.Status == "processing" {
			running++
		}
		j.mu.Unlock()
	}
	var start []*Job
	for _, j := range m.jobs {
		if running >= limit {
			break
		}
		j.mu.Lock()
		if j.Status == "queued" {
			j.Status, j.Stage = "starting", "Başlatılıyor"
			running++
			start = append(start, j)
		}
		j.mu.Unlock()
	}
	m.mu.Unlock()
	for _, j := range start {
		m.emit(j)
		go m.run(j)
	}
}

func (m *JobManager) Cancel(id string) {
	j := m.find(id)
	if j == nil {
		return
	}
	j.mu.Lock()
	j.canceled = true
	if j.Status == "queued" {
		j.Status, j.Stage = "canceled", "İptal edildi"
	}
	c := j.cancel
	j.mu.Unlock()
	if c != nil {
		c()
	}
	m.emit(j)
}

func (m *JobManager) CancelAll() {
	for _, j := range m.List() {
		m.Cancel(j.ID)
	}
}

func (m *JobManager) Retry(id string) {
	j := m.find(id)
	if j == nil {
		return
	}
	j.mu.Lock()
	if j.Status == "error" || j.Status == "canceled" {
		j.Status, j.Stage, j.Error, j.Percent, j.canceled = "queued", "Sırada", "", 0, false
		j.Files = nil
	}
	j.mu.Unlock()
	m.emit(j)
	m.Pump()
}

func (m *JobManager) ClearFinished() {
	m.mu.Lock()
	keep := m.jobs[:0]
	for _, j := range m.jobs {
		j.mu.Lock()
		st := j.Status
		j.mu.Unlock()
		if st != "done" && st != "error" && st != "canceled" {
			keep = append(keep, j)
		}
	}
	m.jobs = keep
	m.mu.Unlock()
	hub.Broadcast("jobs", m.List())
}

func (m *JobManager) ClearHistory() {
	m.mu.Lock()
	m.history = nil
	m.saveHistoryLocked()
	m.mu.Unlock()
	hub.Broadcast("history", m.History())
}

func (m *JobManager) RemoveHistory(id string) {
	m.mu.Lock()
	out := m.history[:0]
	for _, h := range m.history {
		if h.ID != id {
			out = append(out, h)
		}
	}
	m.history = out
	m.saveHistoryLocked()
	m.mu.Unlock()
	hub.Broadcast("history", m.History())
}

func qualityArgs(req JobRequest, s Settings) []string {
	if req.Mode == "audio" {
		af := req.AudioFormat
		switch af {
		case "mp3", "m4a", "opus", "flac", "wav":
		default:
			af = "mp3"
		}
		a := []string{"-f", "ba/b", "-x", "--audio-format", af, "--audio-quality", "0"}
		if s.EmbedThumb && af != "wav" && af != "opus" {
			a = append(a, "--embed-thumbnail", "--convert-thumbnails", "jpg")
		}
		return a
	}
	sortKey := "res,ext:mp4:m4a"
	if h, err := strconv.Atoi(req.Quality); err == nil && h > 0 {
		sortKey = fmt.Sprintf("res:%d,ext:mp4:m4a", h)
	}
	a := []string{"-f", "bv*+ba/b", "-S", sortKey, "--merge-output-format", "mp4"}
	if s.EmbedThumb {
		a = append(a, "--embed-thumbnail")
	}
	if s.Subtitles {
		a = append(a, "--write-subs", "--sub-langs", "tr.*,en.*", "--embed-subs")
	}
	return a
}

const progTpl = "download:NOVA|%(progress.status)s|%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|%(progress.speed)s|%(progress.eta)s|%(info.playlist_index)s|%(info.n_entries)s|%(info.title)s"

func (m *JobManager) run(j *Job) {
	s := settings.Snapshot()
	ctx, cancel := context.WithCancel(context.Background())
	j.mu.Lock()
	if j.canceled {
		j.mu.Unlock()
		cancel()
		return
	}
	j.cancel = cancel
	req := j.JobRequest
	j.mu.Unlock()
	defer cancel()

	if req.Mode == "torrent" {
		j.mu.Lock()
		j.Status, j.Stage = "downloading", "Torrent başlatılıyor"
		j.mu.Unlock()
		m.emit(j)
		err := m.runTorrent(ctx, j, s.DownloadDir)
		j.mu.Lock()
		canceled := j.canceled
		j.mu.Unlock()
		switch {
		case canceled:
			m.finish(j, "canceled", "")
		case err != nil:
			m.finish(j, "error", err.Error())
		default:
			m.finish(j, "done", "")
		}
		return
	}
	tmpDir := filepath.Join(dataDir, "tmp", j.ID)
	_ = os.MkdirAll(tmpDir, 0o755)
	defer os.RemoveAll(tmpDir)

	_ = os.MkdirAll(s.DownloadDir, 0o755)
	outTpl := "%(title).150B [%(id)s].%(ext)s"
	if req.Playlist {
		outTpl = "%(playlist_title,playlist_id|Liste).100B/%(playlist_index|0)03d - %(title).140B.%(ext)s"
	}
	args := baseArgs()
	args = append(args, "--newline", "--no-quiet", "--progress", "--no-mtime", "--windows-filenames",
		"--embed-metadata", "--no-part", "-P", filepath.Join(tmpDir, "out"), "-P", "temp:"+filepath.Join(tmpDir, "part"), "-o", outTpl,
		"--progress-template", progTpl,
		"--progress-template", "postprocess:NOVAPP|%(progress.status)s|%(progress.postprocessor)s",
		"--print", "after_move:NOVAFILE|%(filepath)s")
	if req.Playlist {
		args = append(args, "--yes-playlist", "--ignore-errors")
	} else {
		args = append(args, "--no-playlist")
	}
	args = append(args, qualityArgs(req, s)...)
	args = append(args, "--", req.URL)

	cmd := ytdlpCmd(ctx, args...)
	cmd.Cancel = func() error { return killTree(cmd) }
	cmd.WaitDelay = 5 * time.Second
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	log.Printf("[%s] başlıyor: %s", j.ID, req.URL)
	if err := cmd.Start(); err != nil {
		m.finish(j, "error", "yt-dlp başlatılamadı: "+err.Error())
		return
	}
	j.mu.Lock()
	j.Status, j.Stage = "downloading", "Bağlanıyor"
	j.mu.Unlock()
	m.emit(j)

	var errBuf strings.Builder
	var errMu sync.Mutex
	var wg sync.WaitGroup
	lastEmit := time.Time{}
	handle := func(r io.Reader, isErr bool) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			changed, force := m.parseLine(j, line)
			if isErr && !strings.HasPrefix(line, "NOVA") {
				errMu.Lock()
				errBuf.WriteString(line + "\n")
				errMu.Unlock()
			}
			if changed && (force || time.Since(lastEmit) > 250*time.Millisecond) {
				lastEmit = time.Now()
				m.emit(j)
			}
		}
	}
	wg.Add(2)
	go handle(stdout, false)
	go handle(stderr, true)
	wg.Wait()
	err := cmd.Wait()

	j.mu.Lock()
	canceled := j.canceled
	nfiles := len(j.Files)
	j.mu.Unlock()
	switch {
	case canceled:
		m.finish(j, "canceled", "")
	case err != nil && nfiles == 0:
		errMu.Lock()
		msg := friendlyError(errBuf.String())
		errMu.Unlock()
		m.finish(j, "error", msg)
	default:
		if err := m.moveOut(j, filepath.Join(tmpDir, "out"), s.DownloadDir); err != nil {
			m.finish(j, "error", "Dosya indirme klasörüne taşınamadı: "+err.Error())
			return
		}
		m.finish(j, "done", "")
	}
}

// moveOut tamamlanan dosyaları geçici klasörden indirme klasörüne taşır.
func (m *JobManager) moveOut(j *Job, from, to string) error {
	j.mu.Lock()
	files := append([]string(nil), j.Files...)
	j.Stage = "Taşınıyor"
	j.mu.Unlock()
	var out []string
	for _, f := range files {
		rel, err := filepath.Rel(from, f)
		if err != nil || strings.HasPrefix(rel, "..") {
			out = append(out, f)
			continue
		}
		dst := uniquePath(filepath.Join(to, rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := moveFile(f, dst); err != nil {
			return err
		}
		out = append(out, dst)
	}
	j.mu.Lock()
	j.Files = out
	j.mu.Unlock()
	return nil
}

func uniquePath(p string) string {
	if _, err := os.Stat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(c); err != nil {
			return c
		}
	}
}

func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		os.Remove(dst)
		return err
	}
	if err := o.Close(); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
}

func atof(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

var ppNames = map[string]string{
	"Merger": "Birleştiriliyor", "FFmpegMerger": "Birleştiriliyor", "ExtractAudio": "Ses dönüştürülüyor",
	"FFmpegExtractAudio": "Ses dönüştürülüyor", "EmbedThumbnail": "Kapak ekleniyor", "FFmpegMetadata": "Bilgiler yazılıyor",
	"Metadata": "Bilgiler yazılıyor", "FFmpegEmbedSubtitle": "Altyazı ekleniyor", "MoveFiles": "Taşınıyor",
	"FFmpegThumbnailsConvertor": "Kapak hazırlanıyor", "ThumbnailsConvertor": "Kapak hazırlanıyor",
}

func (m *JobManager) parseLine(j *Job, line string) (changed, force bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	switch {
	case strings.HasPrefix(line, "NOVA|"):
		p := strings.SplitN(line, "|", 10)
		if len(p) < 10 {
			return
		}
		done := atof(p[2])
		total := atof(p[3])
		if total == 0 {
			total = atof(p[4])
		}
		j.Status = "downloading"
		j.Done, j.Total = int64(done), int64(total)
		if total > 0 {
			j.Percent = done / total * 100
		}
		if p[1] == "finished" {
			j.Percent = 100
		}
		j.Speed = atof(p[5])
		j.ETA = int(atof(p[6]))
		if idx, err := strconv.Atoi(p[7]); err == nil {
			j.Item = idx
		}
		if n, err := strconv.Atoi(p[8]); err == nil {
			j.Items = n
		}
		if p[9] != "NA" && p[9] != "" {
			j.ItemTitle = p[9]
			if j.Title == "" || j.Title == j.URL {
				j.Title = p[9]
			}
		}
		j.Stage = "İndiriliyor"
		return true, p[1] == "finished"
	case strings.HasPrefix(line, "NOVAPP|"):
		p := strings.SplitN(line, "|", 3)
		if len(p) == 3 && p[1] == "started" {
			name := ppNames[p[2]]
			if name == "" {
				name = "İşleniyor"
			}
			if p[2] == "MoveFiles" {
				return false, false
			}
			j.Status, j.Stage = "processing", name
			return true, true
		}
	case strings.HasPrefix(line, "NOVAFILE|"):
		f := strings.TrimPrefix(line, "NOVAFILE|")
		j.Files = append(j.Files, f)
		return true, true
	case strings.HasPrefix(line, "[download] Destination:") || strings.HasPrefix(line, "[info]"):
		if j.Stage == "Bağlanıyor" {
			j.Stage = "Hazırlanıyor"
			return true, true
		}
	}
	return false, false
}

func (m *JobManager) finish(j *Job, status, msg string) {
	j.mu.Lock()
	j.Status, j.Error = status, msg
	j.Finished = time.Now().UnixMilli()
	j.cancel = nil
	switch status {
	case "done":
		j.Stage, j.Percent = "Tamamlandı", 100
	case "canceled":
		j.Stage = "İptal edildi"
	case "error":
		j.Stage = "Hata"
	}
	j.mu.Unlock()
	v := j.View()
	log.Printf("[%s] %s %s", j.ID, status, msg)
	if status == "done" {
		m.mu.Lock()
		m.history = append([]JobView{v}, m.history...)
		m.saveHistoryLocked()
		m.mu.Unlock()
		hub.Broadcast("history", m.History())
	}
	m.emit(j)
	m.Pump()
}
