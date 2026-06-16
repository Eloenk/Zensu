package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
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

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	dlManager  *dl.Manager
	client     *api.Client
	downloadMu sync.Mutex
	resolveSem chan struct{}
}

func NewApp() *App {
	return &App{
		resolveSem: make(chan struct{}, 6),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

type AnimeResult struct {
	Session string `json:"session"`
	Title   string `json:"title"`
	Poster  string `json:"poster"`
}

func (a *App) SearchAnime(query string) ([]AnimeResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.UA == "" || cfg.CF == "" {
		return nil, fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		return nil, err
	}
	res, err := client.Search(query)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if cfg.UA == "" || cfg.CF == "" {
		return nil, fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		return nil, err
	}
	eps, err := client.GetEpisodes(slug)
	if err != nil {
		return nil, err
	}

	sanitizedTitle := sanitizeName(animeTitle)
	existingEps := make(map[float64]bool)
	if _, err := os.Stat(cfg.DownloadDir); err == nil {
		files, _ := os.ReadDir(cfg.DownloadDir)
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
	a.downloadMu.Lock()
	defer a.downloadMu.Unlock()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.UA == "" || cfg.CF == "" {
		return fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		return err
	}

	a.client = client
	if a.dlManager == nil {
		a.dlManager = dl.NewManager(cfg.MaxParallel, cfg.UA)
	}

	eps, err := client.GetEpisodes(slug)
	if err != nil {
		return err
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
				if err != nil || len(candidates) == 0 {
					epStr := fmt.Sprintf("E%02.0f", epNum)
					if math.Mod(epNum, 1) != 0 {
						epStr = fmt.Sprintf("E%.1f", epNum)
					}
					jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "failed to resolve Kwik redirect links (check cookies/User-Agent)")
					return
				}

				kwikURL := api.SelectBestKwik(candidates, cfg.Quality, cfg.Audio)
				if kwikURL == "" {
					epStr := fmt.Sprintf("E%02.0f", epNum)
					if math.Mod(epNum, 1) != 0 {
						epStr = fmt.Sprintf("E%.1f", epNum)
					}
					jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
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
					epStr := fmt.Sprintf("E%02.0f", epNum)
					if math.Mod(epNum, 1) != 0 {
						epStr = fmt.Sprintf("E%.1f", epNum)
					}
					jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "failed kwik link extraction")
					return
				}

				epStr := fmt.Sprintf("E%02.0f", epNum)
				if math.Mod(epNum, 1) != 0 {
					epStr = fmt.Sprintf("E%.1f", epNum)
				}

				sanitizedTitle := sanitizeName(animeTitle)
				outPath := filepath.Join(cfg.DownloadDir, sanitizedTitle+" "+epStr+".mp4")

				jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
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
		return "", err
	}
	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err != nil {
		return "", err
	}
	bodyBytes, err := client.GetRawBytes(posterURL)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bodyBytes), nil
}
