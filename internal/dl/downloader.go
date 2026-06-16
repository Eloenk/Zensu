package dl

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type Job struct {
	ID         string
	AnimeTitle string
	EpNum      float64
	URL        string
	IsHLS      bool
	OutputPath string
}

type Result struct {
	Job     Job
	Err     error
	Elapsed time.Duration
}

type JobProgress struct {
	ID       string  `json:"id"`
	Anime    string  `json:"anime"`
	EpNum    float64 `json:"epNum"`
	Status   string  `json:"status"`
	Progress float64 `json:"progress"`
	Speed    string  `json:"speed"`
	ETA      string  `json:"eta"`
	Error    string  `json:"error,omitempty"`
}

type Manager struct {
	maxParallel int
	ua          string
	mu          sync.Mutex
	progress    map[string]*JobProgress
	jobsChan    chan Job
}

func NewManager(maxParallel int, ua string) *Manager {
	m := &Manager{
		maxParallel: maxParallel,
		ua:          ua,
		progress:    make(map[string]*JobProgress),
		jobsChan:    make(chan Job, 1000),
	}
	m.StartWorkers()
	return m
}

func (m *Manager) StartWorkers() {
	for i := 0; i < m.maxParallel; i++ {
		go func() {
			for job := range m.jobsChan {
				m.downloadWorker(job)
			}
		}()
	}
}

func (m *Manager) Submit(job Job) {
	if job.ID == "" {
		anime := job.AnimeTitle
		if anime == "" {
			anime = "Anime"
		}
		epStr := fmt.Sprintf("E%02.0f", job.EpNum)
		if math.Mod(job.EpNum, 1) != 0 {
			epStr = fmt.Sprintf("E%.1f", job.EpNum)
		}
		job.ID = fmt.Sprintf("%s - %s", anime, epStr)
	}
	m.mu.Lock()
	p, ok := m.progress[job.ID]
	if !ok {
		p = &JobProgress{ID: job.ID, Anime: job.AnimeTitle, EpNum: job.EpNum}
		m.progress[job.ID] = p
	}
	p.Status = "queued"
	p.Progress = 0
	p.Speed = ""
	p.ETA = ""
	p.Error = ""
	m.mu.Unlock()

	m.jobsChan <- job
}

func (m *Manager) downloadWorker(job Job) {
	m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "downloading", 0, "", "", "")
	var err error
	if job.IsHLS {
		err = m.downloadHLS(job)
	} else {
		err = m.downloadDirect(job)
	}
	if err != nil {
		m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "failed", 0, "", "", err.Error())
	} else {
		m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "done", 100, "", "", "")
	}
}

func (m *Manager) UpdateProgress(id, anime string, epNum float64, status string, progress float64, speed, eta, errMsg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.progress[id]
	if !ok {
		p = &JobProgress{ID: id, Anime: anime, EpNum: epNum}
		m.progress[id] = p
	}
	p.Status = status
	p.Progress = progress
	p.Speed = speed
	p.ETA = eta
	p.Error = errMsg
}

func (m *Manager) GetProgress() []*JobProgress {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := make([]*JobProgress, 0, len(m.progress))
	for _, p := range m.progress {
		list = append(list, p)
	}
	return list
}

func (m *Manager) ClearProgress() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.progress = make(map[string]*JobProgress)
}

func (m *Manager) RunAll(jobs <-chan Job, total int) <-chan Result {
	results := make(chan Result, total)
	var wg sync.WaitGroup

	go func() {
		for job := range jobs {
			wg.Add(1)
			job := job
			go func() {
				defer wg.Done()
				m.Submit(job)
				// Wait for this specific job to finish so we can return its status/result
				for {
					m.mu.Lock()
					p, ok := m.progress[job.ID]
					m.mu.Unlock()
					if ok && (p.Status == "done" || p.Status == "failed") {
						var err error
						if p.Status == "failed" {
							err = fmt.Errorf("%s", p.Error)
						}
						results <- Result{
							Job: job,
							Err: err,
						}
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}()
		}
		wg.Wait()
		close(results)
	}()

	return results
}

func (m *Manager) downloadDirect(job Job) error {
	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0755); err != nil {
		return err
	}

	tmpPath := job.OutputPath + ".tmp"

	req, err := http.NewRequest(http.MethodHead, job.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", m.ua)
	req.Header.Set("Referer", "https://kwik.cx/")

	client := &http.Client{Timeout: 30 * time.Second}
	headResp, err := client.Do(req)
	var totalBytes int64
	if err == nil {
		totalBytes = headResp.ContentLength
		headResp.Body.Close()
	}

	dlClient := &http.Client{Timeout: 0}
	const maxRetries = 5
	var downloaded int64

	if stat, err := os.Stat(tmpPath); err == nil {
		downloaded = stat.Size()
	}

	startTime := time.Now()

	for attempt := 1; attempt <= maxRetries; attempt++ {
		dlReq, err := http.NewRequest(http.MethodGet, job.URL, nil)
		if err != nil {
			return err
		}
		dlReq.Header.Set("User-Agent", m.ua)
		dlReq.Header.Set("Referer", "https://kwik.cx/")

		if downloaded > 0 {
			dlReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
		}

		resp, err := dlClient.Do(dlReq)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("download request failed: %w", err)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			resp.Body.Close()
			if attempt == maxRetries {
				return fmt.Errorf("bad status %d", resp.StatusCode)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		var f *os.File
		if resp.StatusCode == http.StatusOK {
			f, err = os.Create(tmpPath)
			downloaded = 0
		} else {
			f, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		}

		if err != nil {
			resp.Body.Close()
			return fmt.Errorf("failed to open/create file: %w", err)
		}

		buf := make([]byte, 32*1024)
		pr := &progressReader{
			r:         resp.Body,
			buf:       buf,
			id:        job.ID,
			anime:     job.AnimeTitle,
			epNum:     job.EpNum,
			total:     totalBytes,
			written:   &downloaded,
			lastPrint: time.Now(),
			start:     startTime,
			manager:   m,
		}

		_, copyErr := io.Copy(f, pr)
		f.Close()
		resp.Body.Close()

		if copyErr == nil {
			break
		}

		if attempt == maxRetries {
			return fmt.Errorf("write failed: %w", copyErr)
		}

		time.Sleep(2 * time.Second)
	}

	printProgress(job.EpNum, downloaded, totalBytes, true)
	return os.Rename(tmpPath, job.OutputPath)
}

func (m *Manager) downloadHLS(job Job) error {
	if err := EnsureFFmpegOnce(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0755); err != nil {
		return err
	}

	fmt.Printf("\r\033[K  E%02.0f  [HLS] downloading via ffmpeg...\n", job.EpNum)

	binaryName := "ffmpeg"
	if runtime.GOOS == "windows" {
		binaryName = "ffmpeg.exe"
	}

	ffmpegPath := ""
	if exe, err := os.Executable(); err == nil {
		localPath := filepath.Join(filepath.Dir(exe), "bin", binaryName)
		if isFfmpegCallable(localPath) {
			if abs, err := filepath.Abs(localPath); err == nil {
				ffmpegPath = abs
			}
		}
	}
	if ffmpegPath == "" {
		localPath := filepath.Join("bin", binaryName)
		if isFfmpegCallable(localPath) {
			if abs, err := filepath.Abs(localPath); err == nil {
				ffmpegPath = abs
			}
		}
	}
	if ffmpegPath == "" {
		if p, err := exec.LookPath("ffmpeg"); err == nil {
			if isFfmpegCallable(p) {
				if abs, err := filepath.Abs(p); err == nil {
					ffmpegPath = abs
				} else {
					ffmpegPath = p
				}
			}
		}
	}
	if ffmpegPath == "" {
		ffmpegPath = "ffmpeg"
	}

	args := []string{
		"-allowed_extensions", "ALL",
		"-extension_picky", "0",
		"-reconnect", "1",
		"-reconnect_at_eof", "1",
		"-reconnect_streamed", "1",
		"-reconnect_delay_max", "5",
		"-headers", "Referer: https://kwik.cx/\r\n",
		"-i", job.URL,
		"-c", "copy",
		"-v", "error",
		"-y", job.OutputPath,
	}

	cmd := exec.Command(ffmpegPath, args...)
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type progressReader struct {
	r         io.Reader
	buf       []byte
	id        string
	anime     string
	epNum     float64
	total     int64
	written   *int64
	lastPrint time.Time
	start     time.Time
	manager   *Manager
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.r.Read(p)
	if n > 0 {
		atomic.AddInt64(pr.written, int64(n))
		now := time.Now()
		if now.Sub(pr.lastPrint) > 200*time.Millisecond {
			pr.lastPrint = now
			downloaded := atomic.LoadInt64(pr.written)

			pct := 0.0
			if pr.total > 0 {
				pct = float64(downloaded) / float64(pr.total) * 100
			}

			elapsed := time.Since(pr.start).Seconds()
			speed := ""
			eta := ""
			if elapsed > 0 {
				bps := float64(downloaded) / elapsed
				speed = humanBytes(int64(bps)) + "/s"
				if pr.total > 0 && bps > 0 {
					remainingSec := float64(pr.total-downloaded) / bps
					if remainingSec < 60 {
						eta = fmt.Sprintf("%.0fs", remainingSec)
					} else {
						eta = fmt.Sprintf("%.0fm %.0fs", remainingSec/60, remainingSec-float64(int(remainingSec/60)*60))
					}
				}
			}

			pr.manager.UpdateProgress(pr.id, pr.anime, pr.epNum, "downloading", pct, speed, eta, "")
			printProgress(pr.epNum, downloaded, pr.total, false)
		}
	}
	return n, err
}

func printProgress(epNum float64, downloaded, total int64, done bool) {
	if total <= 0 {
		fmt.Printf("\r\033[K  E%02.0f  downloaded %s", epNum, humanBytes(downloaded))
		return
	}

	pct := float64(downloaded) / float64(total) * 100
	bar := progressBar(pct, 30)
	dl := humanBytes(downloaded)
	tot := humanBytes(total)

	if done {
		fmt.Printf("\r\033[K  E%02.0f  [%s] 100%%  %s / %s  ✓\n", epNum, bar, dl, tot)
	} else {
		fmt.Printf("\r\033[K  E%02.0f  [%s] %5.1f%%  %s / %s", epNum, bar, pct, dl, tot)
	}
}

func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
