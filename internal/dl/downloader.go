package dl

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"zensu/internal/logger"
)

type Job struct {
	ID           string
	AnimeTitle   string
	EpNum        float64
	URL          string
	IsHLS        bool
	OutputPath   string
	HlsTranscode bool
	Referer      string
}

func (j *Job) GetReferer() string {
	if j.Referer != "" {
		return j.Referer
	}
	uLower := strings.ToLower(j.URL)
	if strings.Contains(uLower, "animeheaven") {
		return "https://animeheaven.me/gate.php"
	}
	if strings.Contains(uLower, "mewstream") || strings.Contains(uLower, "cloudvideo") || strings.Contains(uLower, "megaplay") || strings.Contains(uLower, "anikoto") || strings.Contains(uLower, "lostproject") {
		return "https://megaplay.buzz/"
	}
	return "https://kwik.cx/"
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

type activeJob struct {
	cancel context.CancelFunc
	runID  int64
	cmd    *exec.Cmd
}

type Manager struct {
	maxParallel int
	ua          string
	cookies     string
	client      tlsclient.HttpClient
	mu          sync.Mutex
	progress    map[string]*JobProgress
	jobsChan    chan Job

	activeJobs   map[string]activeJob
	runCounter   int64
	cancelMu     sync.Mutex
	cancelledIDs map[string]time.Time // tracks IDs that were explicitly cancelled to block re-submission with a TTL
}

func NewManager(maxParallel int, ua string, cookies string) *Manager {
	jar := tlsclient.NewCookieJar()

	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeoutSeconds(30),
		tlsclient.WithClientProfile(profiles.Chrome_124),
		tlsclient.WithCookieJar(jar),
	}

	client, err := tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
	if err != nil {
		logger.Errorf("DL_CLIENT_INIT_ERR", "Failed to create tls client: %v", err)
	}

	m := &Manager{
		maxParallel:  maxParallel,
		ua:           ua,
		cookies:      cookies,
		client:       client,
		progress:     make(map[string]*JobProgress),
		jobsChan:     make(chan Job, 1000),
		activeJobs:   make(map[string]activeJob),
		cancelledIDs: make(map[string]time.Time),
	}

	m.seedCookies("https://kwik.cx")
	m.seedCookies("https://animepahe.pw")

	m.StartWorkers()
	return m
}

func (m *Manager) seedCookies(rawURL string) {
	if m.client == nil || m.cookies == "" {
		return
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return
	}
	var fCookies []*fhttp.Cookie
	for _, part := range strings.Split(m.cookies, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		fCookies = append(fCookies, &fhttp.Cookie{
			Name:  strings.TrimSpace(kv[0]),
			Value: strings.TrimSpace(kv[1]),
		})
	}
	m.client.SetCookies(u, fCookies)
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

func (m *Manager) SetMaxParallel(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if n <= m.maxParallel {
		m.maxParallel = n
		return
	}
	diff := n - m.maxParallel
	m.maxParallel = n
	for i := 0; i < diff; i++ {
		go func() {
			for job := range m.jobsChan {
				m.downloadWorker(job)
			}
		}()
	}
}

// pruneCancelledIDs removes entries from cancelledIDs that are older than 10 minutes.
// Must be called with cancelMu held.
func (m *Manager) pruneCancelledIDs() {
	now := time.Now()
	for id, t := range m.cancelledIDs {
		if now.Sub(t) > 10*time.Minute {
			delete(m.cancelledIDs, id)
		}
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

	// Block re-submission of explicitly cancelled jobs
	m.cancelMu.Lock()
	m.pruneCancelledIDs()
	_, isCancelled := m.cancelledIDs[job.ID]
	if isCancelled {
		m.cancelMu.Unlock()
		logger.Infof("QUEUE_BLOCKED", "Blocked re-submission of cancelled job: %s", job.ID)
		return
	}
	m.cancelMu.Unlock()

	// Cancel and terminate any existing active worker/process for this job ID to prevent duplicate downloads
	m.CancelJob(job.ID)
	m.cancelMu.Lock()
	delete(m.cancelledIDs, job.ID)
	m.cancelMu.Unlock()

	m.mu.Lock()
	p := &JobProgress{ID: job.ID, Anime: job.AnimeTitle, EpNum: job.EpNum}
	m.progress[job.ID] = p
	p.Status = "queued"
	p.Progress = 0
	p.Speed = ""
	p.ETA = ""
	p.Error = ""
	m.mu.Unlock()

	logger.Infof("QUEUE_SUBMIT", "Submitted job: %s", job.ID)
	m.jobsChan <- job
}

func (m *Manager) downloadWorker(job Job) {
	m.mu.Lock()
	_, exists := m.progress[job.ID]
	m.mu.Unlock()
	if !exists {
		logger.Infof("DL_CANCELLED_PRESTART", "Worker skipping cancelled job before start: %s", job.ID)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	runID := atomic.AddInt64(&m.runCounter, 1)

	m.cancelMu.Lock()
	m.activeJobs[job.ID] = activeJob{cancel: cancel, runID: runID}
	m.cancelMu.Unlock()

	defer func() {
		m.cancelMu.Lock()
		if act, ok := m.activeJobs[job.ID]; ok && act.runID == runID {
			delete(m.activeJobs, job.ID)
		}
		m.cancelMu.Unlock()
		cancel()
	}()

	dlType := "MP4"
	if job.IsHLS {
		dlType = "HLS"
	}
	logger.Infof("DL_START", "Starting %s download: %s", dlType, job.ID)
	m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "downloading", 0, "", "", "")

	var err error
	if job.IsHLS {
		err = m.downloadHLS(ctx, job)
	} else {
		err = m.downloadDirect(ctx, job)
	}

	if err != nil {
		if ctx.Err() != nil {
			logger.Infof("DL_CANCELLED", "%s download cancelled: %s", dlType, job.ID)
			return // Ignore setting status to failed to let new worker update status
		}
		logger.Errorf("DL_FAIL", "%s download failed for %s: %v", dlType, job.ID, err)
		m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "failed", 0, "", "", err.Error())
	} else {
		logger.Infof("DL_DONE", "%s download finished: %s", dlType, job.ID)
		m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "done", 100, "", "", "")
	}
}

func (m *Manager) UpdateProgress(id, anime string, epNum float64, status string, progress float64, speed, eta, errMsg string) {
	m.cancelMu.Lock()
	_, isCancelled := m.cancelledIDs[id]
	m.cancelMu.Unlock()

	if isCancelled {
		return
	}

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

func killProcessTree(pid int) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		setHideWindow(cmd.SysProcAttr)
		return cmd.Run()
	}
	return fmt.Errorf("not windows")
}

func (m *Manager) CancelJob(id string) {
	m.cancelMu.Lock()
	m.pruneCancelledIDs()
	m.cancelledIDs[id] = time.Now() // mark as cancelled to block any future re-submission
	if act, ok := m.activeJobs[id]; ok {
		act.cancel()
		if act.cmd != nil {
			if act.cmd.Process != nil {
				logger.Infof("DL_KILL_PROCESS", "Directly killing process tree for %s (PID: %d)", id, act.cmd.Process.Pid)
				if err := killProcessTree(act.cmd.Process.Pid); err != nil {
					if killErr := act.cmd.Process.Kill(); killErr != nil {
						logger.Errorf("DL_KILL_PROCESS_ERR", "Failed to kill process %d for %s: %v", act.cmd.Process.Pid, id, killErr)
					}
				}
			} else {
				logger.Warnf("DL_KILL_PROCESS_NIL_PROC", "Cannot kill process for %s: cmd.Process is nil", id)
			}
		} else {
			logger.Warnf("DL_KILL_PROCESS_NIL_CMD", "Cannot kill process for %s: cmd is nil", id)
		}
		delete(m.activeJobs, id)
	} else {
		logger.Infof("DL_CANCEL_NOT_ACTIVE", "Job %s not in activeJobs (may be in resolver phase)", id)
	}
	m.cancelMu.Unlock()

	m.mu.Lock()
	delete(m.progress, id)
	m.mu.Unlock()
}

// ClearCancelled removes IDs from the cancelled set so they can be re-downloaded.
// Called when the user explicitly initiates a fresh StartDownload.
func (m *Manager) ClearCancelled(ids ...string) {
	m.cancelMu.Lock()
	m.pruneCancelledIDs()
	for _, id := range ids {
		delete(m.cancelledIDs, id)
	}
	m.cancelMu.Unlock()
}

func (m *Manager) CancelAll() {
	m.cancelMu.Lock()
	for id, act := range m.activeJobs {
		act.cancel()
		if act.cmd != nil {
			if act.cmd.Process != nil {
				logger.Infof("DL_KILL_PROCESS_ALL", "Directly killing process tree for %s (PID: %d) during shutdown", id, act.cmd.Process.Pid)
				if err := killProcessTree(act.cmd.Process.Pid); err != nil {
					if killErr := act.cmd.Process.Kill(); killErr != nil {
						logger.Errorf("DL_KILL_PROCESS_ALL_ERR", "Failed to kill process %d for %s during shutdown: %v", act.cmd.Process.Pid, id, killErr)
					}
				}
			} else {
				logger.Warnf("DL_KILL_PROCESS_ALL_NIL_PROC", "Cannot kill process for %s during shutdown: cmd.Process is nil", id)
			}
		} else {
			logger.Warnf("DL_KILL_PROCESS_ALL_NIL_CMD", "Cannot kill process for %s during shutdown: cmd is nil", id)
		}
	}
	m.activeJobs = make(map[string]activeJob)
	m.cancelMu.Unlock()

	m.mu.Lock()
	m.progress = make(map[string]*JobProgress)
	m.mu.Unlock()
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

func (m *Manager) downloadDirect(ctx context.Context, job Job) error {
	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0755); err != nil {
		return err
	}

	tmpPath := job.OutputPath + ".tmp"

	req, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodHead, job.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", m.ua)
	req.Header.Set("Referer", "https://kwik.cx/")

	headResp, err := m.client.Do(req)
	var totalBytes int64
	if err == nil {
		totalBytes = headResp.ContentLength
		headResp.Body.Close()
	}

	const maxRetries = 5
	var downloaded int64

	if stat, err := os.Stat(tmpPath); err == nil {
		downloaded = stat.Size()
	}

	logger.Infof("DL_DIRECT_START", "Starting direct HTTP download to %s, size: %d bytes (resume offset: %d)", tmpPath, totalBytes, downloaded)
	startTime := time.Now()

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		dlReq, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodGet, job.URL, nil)
		if err != nil {
			logger.Errorf("DL_DIRECT_REQ_ERR", "Failed creating request: %v", err)
			return err
		}
		dlReq.Header.Set("User-Agent", m.ua)
		dlReq.Header.Set("Referer", "https://kwik.cx/")

		if downloaded > 0 {
			dlReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", downloaded))
		}

		resp, err := m.client.Do(dlReq)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.Warnf("DL_DIRECT_CONN_ERR", "Direct download attempt %d failed: %v", attempt, err)
			if attempt == maxRetries {
				return fmt.Errorf("download request failed: %w", err)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		logger.Infof("DL_DIRECT_RESP", "Attempt %d: HTTP %d", attempt, resp.StatusCode)

		if resp.StatusCode != fhttp.StatusOK && resp.StatusCode != fhttp.StatusPartialContent {
			resp.Body.Close()
			logger.Warnf("DL_DIRECT_BAD_STATUS", "Attempt %d: HTTP %d (expected 200 or 206)", attempt, resp.StatusCode)
			if attempt == maxRetries {
				return fmt.Errorf("bad status %d", resp.StatusCode)
			}
			time.Sleep(2 * time.Second)
			continue
		}

		var f *os.File
		if resp.StatusCode == fhttp.StatusOK {
			f, err = os.Create(tmpPath)
			downloaded = 0
		} else {
			f, err = os.OpenFile(tmpPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0644)
		}

		if err != nil {
			resp.Body.Close()
			logger.Errorf("DL_DIRECT_FILE_ERR", "Failed to open output file %s: %v", tmpPath, err)
			return fmt.Errorf("failed to open/create file: %w", err)
		}

		buf := make([]byte, 32*1024)
		pr := &progressReader{
			ctx:       ctx,
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

		if ctx.Err() != nil {
			return ctx.Err()
		}

		logger.Warnf("DL_DIRECT_WRITE_ERR", "Attempt %d write error: %v", attempt, copyErr)

		if attempt == maxRetries {
			return fmt.Errorf("write failed: %w", copyErr)
		}

		time.Sleep(2 * time.Second)
	}

	logger.Infof("DL_DIRECT_OK", "Direct download finished successfully: %s", job.OutputPath)
	printProgress(job.EpNum, downloaded, totalBytes, true)
	return os.Rename(tmpPath, job.OutputPath)
}

func selectMasterVariant(basePlaylistURL, content string) string {
	base, err := url.Parse(basePlaylistURL)
	if err != nil {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#EXT-X-STREAM-INF:") {
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next != "" && !strings.HasPrefix(next, "#") {
					if u, err := url.Parse(next); err == nil {
						return base.ResolveReference(u).String()
					}
					break
				}
			}
		}
	}
	return ""
}

func (m *Manager) fetchM3U8Content(ctx context.Context, playlistURL string, ua string, referer string) (string, error) {
	req, err := fhttp.NewRequestWithContext(ctx, "GET", playlistURL, nil)
	if err != nil {
		return "", err
	}
	if referer == "" {
		uLower := strings.ToLower(playlistURL)
		if strings.Contains(uLower, "mewstream") || strings.Contains(uLower, "cloudvideo") || strings.Contains(uLower, "megaplay") || strings.Contains(uLower, "anikoto") || strings.Contains(uLower, "lostproject") || strings.Contains(uLower, "akirax") {
			referer = "https://megaplay.buzz/"
		} else {
			referer = "https://kwik.cx/"
		}
	}
	req.Header.Set("Referer", referer)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	} else {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != fhttp.StatusOK {
		return "", fmt.Errorf("status code %d", resp.StatusCode)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	content := string(bodyBytes)
	if strings.Contains(content, "#EXT-X-STREAM-INF") {
		subURL := selectMasterVariant(playlistURL, content)
		if subURL != "" && subURL != playlistURL {
			return m.fetchM3U8Content(ctx, subURL, ua, referer)
		}
	}

	return content, nil
}

func parseM3U8(playlistURL string, content string) ([]string, []float64, string, string, error) {
	var urls []string
	var durations []float64
	var keyURL string
	var keyLine string

	base, err := url.Parse(playlistURL)
	if err != nil {
		return nil, nil, "", "", err
	}

	lines := strings.Split(content, "\n")
	var currentDuration float64
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#EXT-X-KEY:") {
			idx := strings.Index(line, `URI="`)
			if idx != -1 {
				start := idx + 5
				end := strings.Index(line[start:], `"`)
				if end != -1 {
					kURL := line[start : start+end]
					if u, err := url.Parse(kURL); err == nil {
						keyURL = base.ResolveReference(u).String()
					}
					keyLine = line[:start] + "key.key" + line[start+end:]
				}
			} else {
				keyLine = line
			}
		} else if strings.HasPrefix(line, "#EXTINF:") {
			commaIdx := strings.Index(line, ",")
			var durStr string
			if commaIdx != -1 {
				durStr = line[8:commaIdx]
			} else {
				durStr = line[8:]
			}
			fmt.Sscanf(durStr, "%f", &currentDuration)
		} else if !strings.HasPrefix(line, "#") {
			u, err := url.Parse(line)
			if err != nil {
				continue
			}
			resolved := base.ResolveReference(u).String()
			urls = append(urls, resolved)
			durations = append(durations, currentDuration)
			currentDuration = 0
		}
	}
	return urls, durations, keyURL, keyLine, nil
}

func (m *Manager) getM3U8Duration(playlistURL string, ua string) float64 {
	content, err := m.fetchM3U8Content(context.Background(), playlistURL, ua, "")
	if err != nil {
		return 1440
	}
	_, durations, _, _, err := parseM3U8(playlistURL, content)
	if err != nil {
		return 1440
	}
	totalDuration := 0.0
	for _, dur := range durations {
		totalDuration += dur
	}
	if totalDuration == 0 {
		return 1440
	}
	return totalDuration
}

type segmentProgressReader struct {
	r          io.Reader
	onProgress func(n int)
}

func (spr *segmentProgressReader) Read(p []byte) (int, error) {
	n, err := spr.r.Read(p)
	if n > 0 {
		spr.onProgress(n)
	}
	return n, err
}

func (m *Manager) downloadHLS(ctx context.Context, job Job) error {
	if err := EnsureFFmpegOnce(); err != nil {
		logger.Errorf("DL_HLS_FFMPEG_ERR", "FFmpeg checks failed: %v", err)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(job.OutputPath), 0755); err != nil {
		logger.Errorf("DL_HLS_DIR_ERR", "Failed creating directory: %v", err)
		return err
	}

	ua := m.ua
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	}

	fmt.Printf("\r\033[K  E%02.0f  [HLS] fetching playlist...\n", job.EpNum)

	playlistContent, err := m.fetchM3U8Content(ctx, job.URL, ua, job.GetReferer())
	if err != nil {
		logger.Errorf("DL_HLS_PLAYLIST_ERR", "Failed to fetch playlist: %v", err)
		return err
	}

	urls, durations, keyURL, keyLine, err := parseM3U8(job.URL, playlistContent)
	if err != nil {
		logger.Errorf("DL_HLS_PARSE_ERR", "Failed to parse playlist: %v", err)
		return err
	}

	tempDir, err := os.MkdirTemp("", "zensu-hls-*")
	if err != nil {
		logger.Errorf("DL_HLS_TEMP_ERR", "Failed to create temp directory: %v", err)
		return err
	}
	defer os.RemoveAll(tempDir)

	if keyURL != "" {
		req, err := fhttp.NewRequestWithContext(ctx, "GET", keyURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Referer", job.GetReferer())
		req.Header.Set("User-Agent", ua)

		resp, err := m.client.Do(req)
		if err != nil || resp.StatusCode != fhttp.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			logger.Errorf("DL_HLS_KEY_ERR", "Failed to fetch decryption key from %s: %v", keyURL, err)
			return fmt.Errorf("failed to fetch HLS decryption key: %v", err)
		}

		keyPath := filepath.Join(tempDir, "key.key")
		keyFile, err := os.Create(keyPath)
		if err != nil {
			resp.Body.Close()
			return err
		}
		_, err = io.Copy(keyFile, resp.Body)
		resp.Body.Close()
		keyFile.Close()
		if err != nil {
			return err
		}
	}

	logger.Infof("DL_HLS_START", "Starting native HLS download of %d segments", len(urls))

	var totalBytesDownloaded int64
	var completedSegmentsBytes int64
	startTime := time.Now()
	lastPrintTime := time.Now()

	for idx, segmentURL := range urls {
		if ctx.Err() != nil {
			logger.Warnf("DL_HLS_CANCELLED", "HLS download cancelled at segment %d: %s", idx, job.ID)
			return ctx.Err()
		}

		segmentPath := filepath.Join(tempDir, fmt.Sprintf("segment_%d.ts", idx))
		var segmentSuccess bool
		var lastSegmentErr error

		for retry := 0; retry < 5; retry++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			req, err := fhttp.NewRequestWithContext(ctx, "GET", segmentURL, nil)
			if err != nil {
				lastSegmentErr = err
				break
			}
			req.Header.Set("Referer", job.GetReferer())
			req.Header.Set("User-Agent", ua)

			resp, err := m.client.Do(req)
			if err != nil {
				lastSegmentErr = err
				time.Sleep(1 * time.Second)
				continue
			}

			if resp.StatusCode != fhttp.StatusOK {
				lastSegmentErr = fmt.Errorf("status %d", resp.StatusCode)
				resp.Body.Close()
				time.Sleep(1 * time.Second)
				continue
			}

			segmentFile, err := os.Create(segmentPath)
			if err != nil {
				resp.Body.Close()
				lastSegmentErr = err
				break
			}

			var currentSegmentBytes int64
			spr := &segmentProgressReader{
				r: resp.Body,
				onProgress: func(n int) {
					atomic.AddInt64(&currentSegmentBytes, int64(n))
					totalProgressBytes := atomic.LoadInt64(&completedSegmentsBytes) + atomic.LoadInt64(&currentSegmentBytes)

					now := time.Now()
					if now.Sub(lastPrintTime) > 250*time.Millisecond {
						lastPrintTime = now
						elapsed := time.Since(startTime).Seconds()
						speed := ""
						eta := ""
						if elapsed > 0 {
							bps := float64(totalProgressBytes) / elapsed
							speed = humanBytes(int64(bps)) + "/s"
							if idx > 0 {
								remainingSec := float64(len(urls)-idx) * elapsed / float64(idx)
								if remainingSec < 60 {
									eta = fmt.Sprintf("%.0fs", remainingSec)
								} else {
									eta = fmt.Sprintf("%.0fm %.0fs", remainingSec/60, remainingSec-float64(int(remainingSec/60)*60))
								}
							}
						}

						pct := (float64(idx) / float64(len(urls))) * 100.0
						if pct > 100 {
							pct = 100
						}
						m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "downloading", pct, speed, eta, "")
						printProgress(job.EpNum, totalProgressBytes, 0, false)
					}
				},
			}

			_, copyErr := io.Copy(segmentFile, spr)
			resp.Body.Close()
			segmentFile.Close()

			if copyErr != nil {
				lastSegmentErr = copyErr
				time.Sleep(1 * time.Second)
				continue
			}

			atomic.AddInt64(&completedSegmentsBytes, currentSegmentBytes)
			atomic.StoreInt64(&totalBytesDownloaded, atomic.LoadInt64(&completedSegmentsBytes))
			segmentSuccess = true
			break
		}

		if !segmentSuccess {
			logger.Errorf("DL_HLS_SEG_ERR", "Failed to download segment %d: %v", idx, lastSegmentErr)
			return fmt.Errorf("failed to download segment %d: %w", idx, lastSegmentErr)
		}
	}

	// 3. Write local playlist
	localM3U8Path := filepath.Join(tempDir, "playlist.m3u8")
	m3u8File, err := os.Create(localM3U8Path)
	if err != nil {
		return err
	}

	m3u8File.WriteString("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:20\n#EXT-X-MEDIA-SEQUENCE:0\n")
	if keyLine != "" {
		m3u8File.WriteString(keyLine + "\n")
	}
	for idx, dur := range durations {
		m3u8File.WriteString(fmt.Sprintf("#EXTINF:%f,\nsegment_%d.ts\n", dur, idx))
	}
	m3u8File.WriteString("#EXT-X-ENDLIST\n")
	m3u8File.Close()

	fmt.Printf("\r\033[K  E%02.0f  [HLS] packaging via ffmpeg...\n", job.EpNum)

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
		ffmpegPath = "ffmpeg"
	}

	var ffmpegArgs []string
	ffmpegArgs = append(ffmpegArgs,
		"-allowed_extensions", "ALL",
		"-protocol_whitelist", "file,crypto",
		"-fflags", "+genpts",
		"-i", "playlist.m3u8",
	)
	if job.HlsTranscode {
		ffmpegArgs = append(ffmpegArgs, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac")
	} else {
		ffmpegArgs = append(ffmpegArgs, "-c:v", "copy", "-c:a", "aac")
	}
	ffmpegArgs = append(ffmpegArgs,
		"-bsf:a", "aac_adtstoasc",
		"-max_muxing_queue_size", "2048",
		"-y", job.OutputPath,
	)

	cmd := exec.CommandContext(ctx, ffmpegPath, ffmpegArgs...)
	cmd.Dir = tempDir
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	setHideWindow(cmd.SysProcAttr)

	m.cancelMu.Lock()
	if act, ok := m.activeJobs[job.ID]; ok {
		act.cmd = cmd
		m.activeJobs[job.ID] = act
	}
	m.cancelMu.Unlock()

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	m.UpdateProgress(job.ID, job.AnimeTitle, job.EpNum, "processing", 100, "Local packaging", "", "")

	if err := cmd.Run(); err != nil {
		errStr := stderr.String()
		logger.Errorf("DL_HLS_PACK_ERR", "Local packaging failed: %v (stderr: %s)", err, strings.TrimSpace(errStr))
		return fmt.Errorf("local packaging failed: %w (stderr: %s)", err, strings.TrimSpace(errStr))
	}

	logger.Infof("DL_HLS_OK", "HLS download finished successfully: %s", job.OutputPath)
	printProgress(job.EpNum, totalBytesDownloaded, totalBytesDownloaded, true)
	return nil
}

type progressReader struct {
	ctx       context.Context
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
	if pr.ctx.Err() != nil {
		return 0, pr.ctx.Err()
	}
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
