package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"zensu/internal/api"
	"zensu/internal/config"
	"zensu/internal/dl"
	"zensu/internal/kwik"
	"zensu/internal/logger"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	dlManager  *dl.Manager
	client     *api.Client
	downloadMu sync.Mutex
	resolveSem chan struct{}
	slugsMu    sync.Mutex
	animeSlugs map[string]string
}

func NewApp() *App {
	return &App{
		resolveSem: make(chan struct{}, 6),
		animeSlugs: make(map[string]string),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	if a.dlManager != nil {
		a.dlManager.CancelAll()
	}
}

type AnimeResult struct {
	Session string `json:"session"`
	Title   string `json:"title"`
	Poster  string `json:"poster"`
}

func (a *App) SearchAnime(query string) ([]AnimeResult, error) {
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return nil, fmt.Errorf("failed to load configuration; see henzuku.log for details")
	}
	if cfg.UA == "" || cfg.CF == "" {
		return nil, fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return nil, fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}
	res, err := client.Search(query)
	if err != nil {
		logger.Errorf("APP_SEARCH_ERR", "Search failed for query %q: %v", query, err)
		return nil, fmt.Errorf("search failed; verify your internet connection and Cloudflare clearance")
	}
	out := make([]AnimeResult, len(res))
	for i, r := range res {
		out[i] = AnimeResult{Session: r.Session, Title: r.Title, Poster: r.Poster}
	}
	return out, nil
}

type EpisodeInfo struct {
	Episode float64 `json:"episode"`
	Session string  `json:"session"`
	Exists  bool    `json:"exists"`
}

var nonAlphanumRe = regexp.MustCompile(`[^\w ,+\-()\s]`)

func sanitizeName(name string) string {
	name = nonAlphanumRe.ReplaceAllString(name, " ")
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

func (a *App) GetEpisodes(animeTitle, slug string) ([]EpisodeInfo, error) {
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return nil, fmt.Errorf("failed to load configuration; see henzuku.log")
	}
	if cfg.UA == "" || cfg.CF == "" {
		return nil, fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return nil, fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}
	eps, err := client.GetEpisodes(slug)
	if err != nil {
		logger.Errorf("APP_EPISODES_ERR", "Failed to fetch episodes for %s (%s): %v", animeTitle, slug, err)
		return nil, fmt.Errorf("failed to fetch episodes; verify connection or Cloudflare clearance")
	}

	sanitizedTitle := sanitizeName(animeTitle)
	existingEps := make(map[float64]bool)
	animeDir := filepath.Join(cfg.DownloadDir, sanitizedTitle)
	if _, err := os.Stat(animeDir); err == nil {
		files, _ := os.ReadDir(animeDir)
		pattern := fmt.Sprintf(`^%s E(\d+(\.\d+)?)\.mp4$`, regexp.QuoteMeta(sanitizedTitle))
		re, err := regexp.Compile(pattern)
		if err == nil {
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				m := re.FindStringSubmatch(f.Name())
				if len(m) > 1 {
					if val, err := strconv.ParseFloat(m[1], 64); err == nil {
						existingEps[val] = true
					}
				}
			}
		}
	}

	out := make([]EpisodeInfo, len(eps))
	for i, e := range eps {
		out[i] = EpisodeInfo{
			Episode: e.Episode,
			Session: e.Session,
			Exists:  existingEps[e.Episode],
		}
	}
	return out, nil
}

func (a *App) SelectDirectory() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Download Directory",
	})
}

func (a *App) GetConfig() (*config.Config, error) {
	return config.Load()
}

func (a *App) SaveConfig(ua, cf, downloadDir, quality, audio, domain string, maxParallel int) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.UA = ua
	cfg.CF = cf
	cfg.DownloadDir = downloadDir
	cfg.Quality = quality
	cfg.Audio = audio
	cfg.Domain = domain
	cfg.MaxParallel = maxParallel
	return cfg.Save()
}

func (a *App) GetProgress() []*dl.JobProgress {
	if a.dlManager == nil {
		return []*dl.JobProgress{}
	}
	return a.dlManager.GetProgress()
}

func (a *App) ClearProgress() {
	if a.dlManager != nil {
		a.dlManager.ClearProgress()
	}
}

func (a *App) StartDownload(animeTitle, slug string, epNums []float64) error {
	a.slugsMu.Lock()
	a.animeSlugs[animeTitle] = slug
	a.slugsMu.Unlock()

	a.downloadMu.Lock()
	defer a.downloadMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return fmt.Errorf("failed to load configuration; see henzuku.log")
	}
	if cfg.UA == "" || cfg.CF == "" {
		return fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}

	a.client = client
	if a.dlManager == nil {
		a.dlManager = dl.NewManager(cfg.MaxParallel, cfg.UA)
	} else {
		a.dlManager.SetMaxParallel(cfg.MaxParallel)
	}

	eps, err := client.GetEpisodes(slug)
	if err != nil {
		logger.Errorf("APP_EPISODES_ERR", "Failed to fetch episodes for %s (%s): %v", animeTitle, slug, err)
		return fmt.Errorf("failed to retrieve episodes; check connection or settings")
	}

	epMap := make(map[float64]api.Episode)
	for _, e := range eps {
		epMap[e.Episode] = e
	}

	// Pre-populate queue with status "queued" using ID (Anime Title + EpNum)
	for _, epNum := range epNums {
		epStr := fmt.Sprintf("E%02.0f", epNum)
		if math.Mod(epNum, 1) != 0 {
			epStr = fmt.Sprintf("E%.1f", epNum)
		}
		jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
		a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "queued", 0, "", "", "")
	}

	go func() {
		var resolveWg sync.WaitGroup

		resolveWg.Add(len(epNums))
		for _, epNum := range epNums {
			epNum := epNum
			a.resolveSem <- struct{}{}
			go func() {
				defer resolveWg.Done()
				defer func() { <-a.resolveSem }()

				ep, ok := epMap[epNum]
				if !ok {
					epStr := fmt.Sprintf("E%02.0f", epNum)
					if math.Mod(epNum, 1) != 0 {
						epStr = fmt.Sprintf("E%.1f", epNum)
					}
					jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "episode not found in release list")
					return
				}

				var candidates []api.KwikCandidate
				var err error
				for attempt := 1; attempt <= 6; attempt++ {
					candidates, err = a.client.GetKwikLinks(slug, ep.Session)
					if err == nil && len(candidates) > 0 {
						break
					}
					if attempt < 6 {
						time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
					}
				}

				epStr := fmt.Sprintf("E%02.0f", epNum)
				if math.Mod(epNum, 1) != 0 {
					epStr = fmt.Sprintf("E%.1f", epNum)
				}
				jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)

				if err != nil || len(candidates) == 0 {
					logger.Errorf("APP_KWIK_RESOLVE_ERR", "Failed to resolve kwik redirect links for %s: %v", jobID, err)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "failed to resolve Kwik redirect links (check cookies/User-Agent)")
					return
				}

				kwikURL := api.SelectBestKwik(candidates, cfg.Quality, cfg.Audio)
				if kwikURL == "" {
					logger.Errorf("APP_KWIK_SELECT_ERR", "No candidate matching %sp/%s found for %s", cfg.Quality, cfg.Audio, jobID)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "no link matching selected quality/audio found")
					return
				}

				extractor := kwik.NewExtractor(cfg.UA, cfg.Cookies)
				var dlURL string
				var isHLS bool
				for attempt := 1; attempt <= 6; attempt++ {
					dlURL, isHLS, err = extractor.GetDownloadURL(kwikURL)
					if err == nil && dlURL != "" {
						break
					}
					if attempt < 6 {
						time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
					}
				}
				if err != nil || dlURL == "" {
					logger.Errorf("APP_KWIK_EXTRACT_ERR", "Failed kwik link extraction for %s: %v", jobID, err)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "failed kwik link extraction")
					return
				}

				// Check if the job was cancelled/removed while resolving
				progressList := a.dlManager.GetProgress()
				cancelled := true
				for _, p := range progressList {
					if p.ID == jobID {
						cancelled = false
						break
					}
				}
				if cancelled {
					logger.Infof("RESOLVER_CANCELLED", "Discarding resolved link because job was cancelled: %s", jobID)
					return
				}

				sanitizedTitle := sanitizeName(animeTitle)
				outPath := filepath.Join(cfg.DownloadDir, sanitizedTitle, sanitizedTitle+" "+epStr+".mp4")

				a.dlManager.Submit(dl.Job{
					ID:         jobID,
					AnimeTitle: animeTitle,
					EpNum:      epNum,
					URL:        dlURL,
					IsHLS:      isHLS,
					OutputPath: outPath,
				})
			}()
		}
		resolveWg.Wait()
	}()

	return nil
}

func (a *App) GetPosterBase64(posterURL string) (string, error) {
	if posterURL == "" {
		return "", fmt.Errorf("empty url")
	}
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return "", fmt.Errorf("failed to load configuration; see henzuku.log")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return "", fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}
	bodyBytes, err := client.GetRawBytes(posterURL)
	if err != nil {
		logger.Errorf("APP_POSTER_ERR", "Failed to fetch poster bytes from %s: %v", posterURL, err)
		return "", fmt.Errorf("failed to fetch poster")
	}
	return base64.StdEncoding.EncodeToString(bodyBytes), nil
}

func (a *App) IsOnline() bool {
	cfg, err := config.Load()
	if err != nil {
		return false
	}
	u, err := url.Parse(cfg.Domain)
	if err != nil || u.Host == "" {
		return false
	}
	// Try a quick DNS lookup for the domain host to see if it resolves
	_, err = net.LookupHost(u.Host)
	return err == nil
}

func (a *App) RetryFailed(animeTitle string) error {
	a.slugsMu.Lock()
	slug, ok := a.animeSlugs[animeTitle]
	a.slugsMu.Unlock()
	if !ok {
		return fmt.Errorf("anime slug not found; search and start the download first")
	}

	if a.dlManager == nil {
		return fmt.Errorf("no downloads manager active")
	}

	progress := a.dlManager.GetProgress()
	var epNums []float64
	for _, p := range progress {
		if p.Anime == animeTitle && (p.Status == "failed" || p.Status == "downloading" || p.Status == "queued") {
			epNums = append(epNums, p.EpNum)
		}
	}

	if len(epNums) == 0 {
		return fmt.Errorf("no failed or active downloads to retry for this anime")
	}

	// Submit them again
	return a.StartDownload(animeTitle, slug, epNums)
}

func (a *App) CancelAnimeDownloads(animeTitle string) error {
	if a.dlManager == nil {
		return fmt.Errorf("no downloads manager active")
	}

	progress := a.dlManager.GetProgress()
	for _, p := range progress {
		if p.Anime == animeTitle {
			a.dlManager.CancelJob(p.ID)
		}
	}
	return nil
}
