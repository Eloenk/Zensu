package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"zensu/internal/api"
	"zensu/internal/browser"
	"zensu/internal/config"
	"zensu/internal/dl"
	"zensu/internal/kwik"
	"zensu/internal/logger"
	"zensu/internal/notify"
	"zensu/internal/tracker"
	"zensu/internal/tray"
	"zensu/internal/updater"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const AppVersion = "1.5.0"

type App struct {
	ctx        context.Context
	dlManager  *dl.Manager
	client     *api.Client
	trackerMgr *tracker.Manager
	downloadMu sync.Mutex
	resolveSem chan struct{}
	slugsMu    sync.Mutex
	animeSlugs map[string]string
}

func NewApp() *App {
	return &App{
		resolveSem: make(chan struct{}, 6),
		animeSlugs: make(map[string]string),
		trackerMgr: tracker.NewManager(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	updater.CleanupOldBinaries()

	tray.Init(ctx, func() {
		if a.ctx != nil {
			wailsRuntime.WindowShow(a.ctx)
			wailsRuntime.WindowUnminimise(a.ctx)
		}
	}, func() {
		if a.ctx != nil {
			wailsRuntime.EventsEmit(a.ctx, "trigger-app-update-check")
		}
	}, func() {
		if a.ctx != nil {
			a.shutdown(a.ctx)
			wailsRuntime.Quit(a.ctx)
		} else {
			os.Exit(0)
		}
	})

	go a.autoCheckAndResolveCredentials()
	go a.startBackgroundMonitor()
}

func (a *App) autoCheckAndResolveCredentials() {
	// Wait a moment for Wails to initialize and show the UI before launching Chrome
	time.Sleep(1 * time.Second)

	logger.Infof("APP_STARTUP_CHECK", "Checking Cloudflare clearance credentials...")
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return
	}

	needsSolve := cfg.UA == "" || cfg.CF == ""
	if !needsSolve {
		client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.GetProviderDomain("animepahe"))
		if err == nil {
			if connErr := client.TestConnection(); connErr != nil {
				logger.Warnf("APP_STARTUP_CONN_FAIL", "Connection test failed: %v", connErr)
				needsSolve = true
			} else {
				logger.Infof("APP_STARTUP_CONN_OK", "Connection test passed! Clearance is valid.")
			}
		} else {
			needsSolve = true
		}
	}

	if needsSolve {
		logger.Infof("APP_STARTUP_RESOLVE", "Clearance credentials missing or invalid. Launching Browser to automatically resolve...")
		credentials, err := browser.FetchCredentials(cfg.Domain, cfg.Browser, cfg.BrowserPath, cfg.CF)
		if err != nil {
			logger.Errorf("APP_STARTUP_BROWSER_ERR", "Failed to automatically resolve credentials via Browser: %v", err)
			return
		}

		cfg.UA = credentials.UA
		cfg.CF = credentials.CF
		cfg.Cookies = "cf_clearance=" + credentials.CF
		if err := cfg.Save(); err != nil {
			logger.Errorf("APP_CONFIG_SAVE_ERR", "Failed to save auto-resolved config: %v", err)
			return
		}

		logger.Infof("APP_STARTUP_RESOLVE_OK", "Successfully resolved and saved clearance credentials.")
		// Emit Wails event to tell the frontend to reload settings input fields
		wailsRuntime.EventsEmit(a.ctx, "credentials_updated", map[string]string{
			"ua": credentials.UA,
			"cf": credentials.CF,
		})
	}
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

func (a *App) SearchAnime(query string, provider string) ([]AnimeResult, error) {
	logger.Infof("APP_SEARCH", "Searching for anime with query %q (provider: %q)", query, provider)
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return nil, fmt.Errorf("failed to load configuration; see henzuku.log for details")
	}

	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		p = cfg.Provider
	}
	if p != "anikoto" && p != "animeheaven" {
		p = "animepahe"
	}

	if p == "animepahe" && (cfg.UA == "" || cfg.CF == "") {
		return nil, fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}

	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.GetProviderDomain(p))
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return nil, fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}

	var res []api.SearchResult
	if p == "anikoto" {
		res, err = client.SearchAnikoto(query)
	} else if p == "animeheaven" {
		res, err = client.SearchAnimeHeaven(query)
	} else {
		res, err = client.Search(query)
	}

	if err != nil {
		logger.Errorf("APP_SEARCH_ERR", "Search failed for query %q (%s): %v", query, p, err)
		return nil, fmt.Errorf("search failed; verify your internet connection")
	}
	logger.Infof("APP_SEARCH_OK", "Found %d result(s) for query %q (%s)", len(res), query, p)
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

func (a *App) GetEpisodes(animeTitle, slug string, provider string) ([]EpisodeInfo, error) {
	logger.Infof("APP_EPISODES", "Fetching episodes for %q (slug: %s, provider: %s)", animeTitle, slug, provider)
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return nil, fmt.Errorf("failed to load configuration; see henzuku.log")
	}

	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" && a.trackerMgr != nil {
		for _, item := range a.trackerMgr.GetList() {
			if item.Title == animeTitle && item.Provider != "" {
				p = item.Provider
				break
			}
		}
	}
	if p == "" {
		p = cfg.Provider
	}
	if p != "anikoto" && p != "animeheaven" {
		p = "animepahe"
	}

	if p == "animepahe" && (cfg.UA == "" || cfg.CF == "") {
		return nil, fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}

	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.GetProviderDomain(p))
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return nil, fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}

	var eps []api.Episode
	if p == "anikoto" {
		eps, err = client.GetAnikotoEpisodes(slug)
	} else if p == "animeheaven" {
		eps, err = client.GetAnimeHeavenEpisodes(slug)
	} else {
		eps, err = client.GetEpisodes(slug)
	}

	// Auto-retry once if Cloudflare clearance cookie expired
	if err != nil && p == "animepahe" && (strings.Contains(err.Error(), "cookies expired") || strings.Contains(err.Error(), "HTML")) {
		logger.Warnf("APP_EPISODES_CF_RETRY", "Cloudflare cookie expired for %s; attempting automatic credential resolution...", animeTitle)
		a.autoCheckAndResolveCredentials()
		if cfgRetry, errRetry := config.Load(); errRetry == nil {
			if clientRetry, errClient := api.NewClient(cfgRetry.UA, cfgRetry.Cookies, cfgRetry.GetProviderDomain(p)); errClient == nil {
				eps, err = clientRetry.GetEpisodes(slug)
			}
		}
	}

	if err != nil {
		logger.Errorf("APP_EPISODES_ERR", "Failed to fetch episodes for %s (%s): %v", animeTitle, slug, err)
		return nil, fmt.Errorf("failed to fetch episodes; verify connection")
	}
	logger.Infof("APP_EPISODES_OK", "Fetched %d episode(s) for %q", len(eps), animeTitle)

	existingEps := scanExistingEpisodes(cfg.DownloadDir, animeTitle)
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

type animeTitleInfo struct {
	cleanTitle string
	season     int
	part       int
}

func parseAnimeTitleInfo(title string) animeTitleInfo {
	lower := strings.ToLower(title)

	season := 1
	seasonRe := regexp.MustCompile(`(?i)(?:season\s*(\d+)|s(\d{1,2})|\b(\d+)(?:st|nd|rd|th)\s*season)`)
	if m := seasonRe.FindStringSubmatch(lower); len(m) > 0 {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				if val, err := strconv.Atoi(m[i]); err == nil && val > 0 {
					season = val
					break
				}
			}
		}
	} else {
		romanRe := regexp.MustCompile(`(?i)\b(?:season\s*)?(ii|iii|iv|v|vi)\b`)
		if rm := romanRe.FindStringSubmatch(lower); len(rm) > 1 {
			switch strings.ToLower(rm[1]) {
			case "ii":
				season = 2
			case "iii":
				season = 3
			case "iv":
				season = 4
			case "v":
				season = 5
			case "vi":
				season = 6
			}
		}
	}

	part := 0
	partRe := regexp.MustCompile(`(?i)(?:part\s*(\d+)|cour\s*(\d+))`)
	if m := partRe.FindStringSubmatch(lower); len(m) > 0 {
		for i := 1; i < len(m); i++ {
			if m[i] != "" {
				if val, err := strconv.Atoi(m[i]); err == nil && val > 0 {
					part = val
					break
				}
			}
		}
	}

	cleaned := seasonRe.ReplaceAllString(lower, " ")
	cleaned = partRe.ReplaceAllString(cleaned, " ")
	cleaned = regexp.MustCompile(`(?i)\b(ii|iii|iv|v|vi)\b`).ReplaceAllString(cleaned, " ")
	cleaned = sanitizeName(cleaned)

	return animeTitleInfo{
		cleanTitle: cleaned,
		season:     season,
		part:       part,
	}
}

func isMatchingAnime(targetTitleInfo animeTitleInfo, candidateName string) bool {
	candInfo := parseAnimeTitleInfo(candidateName)

	if targetTitleInfo.season != candInfo.season {
		return false
	}

	if targetTitleInfo.part != 0 && candInfo.part != 0 && targetTitleInfo.part != candInfo.part {
		return false
	}

	targetWords := strings.Fields(targetTitleInfo.cleanTitle)
	if len(targetWords) == 0 {
		return true
	}

	var significantTarget []string
	stopWords := map[string]bool{"the": true, "a": true, "an": true, "of": true, "in": true, "to": true, "and": true, "no": true}
	for _, w := range targetWords {
		w = strings.ToLower(w)
		if len(w) >= 3 && !stopWords[w] {
			significantTarget = append(significantTarget, w)
		}
	}

	if len(significantTarget) == 0 {
		significantTarget = targetWords
	}

	candText := strings.ToLower(candInfo.cleanTitle)
	matchedCount := 0
	for _, tw := range significantTarget {
		if strings.Contains(candText, tw) {
			matchedCount++
		}
	}

	return float64(matchedCount)/float64(len(significantTarget)) >= 0.6
}

func extractEpisodeNumbers(filename string) []float64 {
	nameWithoutExt := filename
	if ext := filepath.Ext(filename); ext != "" {
		nameWithoutExt = filename[:len(filename)-len(ext)]
	}

	qualityRe := regexp.MustCompile(`(?i)\[?(?:1080p|720p|480p|360p|2160p|4k|8k|x264|x265|h264|h265|hevc|aac|dts|flac|10bit|8bit|web-dl|webrip|hdtv|bdrip|bluray)\]?`)
	cleaned := qualityRe.ReplaceAllString(nameWithoutExt, " ")

	seasonRe := regexp.MustCompile(`(?i)(?:season\s*\d+|s\d{1,2}|\b\d+(?:st|nd|rd|th)\s*season|\bpart\s*\d+|\bcour\s*\d+|\b(?:ii|iii|iv|v|vi)\b)`)
	cleaned = seasonRe.ReplaceAllString(cleaned, " ")

	var eps []float64

	explicitEpRe := regexp.MustCompile(`(?i)(?:^|[\s_\-\.\(\[])(?:E|Ep|Episode|#)\s*(\d+(?:\.\d+)?)`)
	matches := explicitEpRe.FindAllStringSubmatch(cleaned, -1)
	for _, m := range matches {
		if len(m) > 1 {
			if val, err := strconv.ParseFloat(m[1], 64); err == nil && val > 0 && val < 5000 {
				eps = append(eps, val)
			}
		}
	}

	if len(eps) == 0 {
		standaloneEpRe := regexp.MustCompile(`(?i)(?:^|[\s_\-\(\[])(?:-\s*)?(\d{1,4}(?:\.\d+)?)(?:[\s_\-\)\.]|$)`)
		matches = standaloneEpRe.FindAllStringSubmatch(cleaned, -1)
		for _, m := range matches {
			if len(m) > 1 {
				if val, err := strconv.ParseFloat(m[1], 64); err == nil && val > 0 && val < 5000 {
					eps = append(eps, val)
				}
			}
		}
	}

	return eps
}

func scanExistingEpisodes(downloadDir, animeTitle string) map[float64]bool {
	existingEps := make(map[float64]bool)
	targetInfo := parseAnimeTitleInfo(animeTitle)
	sanitizedTitle := sanitizeName(animeTitle)

	dirsToCheck := []string{
		filepath.Join(downloadDir, sanitizedTitle),
		filepath.Join(downloadDir, animeTitle),
		downloadDir,
	}

	if home, err := os.UserHomeDir(); err == nil {
		defaultDir := filepath.Join(home, "Videos", "Anime")
		dirsToCheck = append(dirsToCheck,
			filepath.Join(defaultDir, sanitizedTitle),
			filepath.Join(defaultDir, animeTitle),
			defaultDir,
		)

		for _, rootDir := range []string{downloadDir, defaultDir} {
			if rootDir == "" {
				continue
			}
			if subEntries, err := os.ReadDir(rootDir); err == nil {
				for _, sub := range subEntries {
					if sub.IsDir() {
						if isMatchingAnime(targetInfo, sub.Name()) {
							dirsToCheck = append(dirsToCheck, filepath.Join(rootDir, sub.Name()))
						}
					}
				}
			}
		}
	}

	visitedDirs := make(map[string]bool)
	for _, dir := range dirsToCheck {
		if dir == "" {
			continue
		}
		dir = filepath.Clean(dir)
		if visitedDirs[dir] {
			continue
		}
		visitedDirs[dir] = true

		dirName := filepath.Base(dir)
		if dir != downloadDir && !strings.HasSuffix(dir, "Anime") {
			if !isMatchingAnime(targetInfo, dirName) {
				continue
			}
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			ext := strings.ToLower(filepath.Ext(name))
			if ext != ".mp4" && ext != ".mkv" && ext != ".avi" && ext != ".webm" && ext != ".flv" && ext != ".ts" {
				continue
			}

			if dir == downloadDir || strings.HasSuffix(dir, "Anime") {
				if !isMatchingAnime(targetInfo, name) {
					continue
				}
			}

			epNums := extractEpisodeNumbers(name)
			for _, ep := range epNums {
				existingEps[ep] = true
			}
		}
	}

	return existingEps
}

func (a *App) SelectDirectory() (string, error) {
	return wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Download Directory",
	})
}

func (a *App) SelectBrowserFile() (string, error) {
	return wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Browser Executable",
		Filters: []wailsRuntime.FileFilter{
			{
				DisplayName: "Executables (*.exe)",
				Pattern:     "*.exe;*.cmd;*.bat;*.sh",
			},
			{
				DisplayName: "All Files (*.*)",
				Pattern:     "*.*",
			},
		},
	})
}

func (a *App) GetConfig() (*config.Config, error) {
	return config.Load()
}

func (a *App) FetchCredentialsFromBrowser() (map[string]string, error) {
	logger.Infof("APP_FETCH_CREDENTIALS", "Triggering browser credentials solver...")
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	credentials, err := browser.FetchCredentials(cfg.Domain, cfg.Browser, cfg.BrowserPath, cfg.CF)
	if err != nil {
		logger.Errorf("APP_FETCH_CREDENTIALS_ERR", "Failed to fetch credentials from browser: %v", err)
		return nil, fmt.Errorf("failed to fetch credentials: %w", err)
	}

	logger.Infof("APP_FETCH_CREDENTIALS_OK", "Successfully fetched credentials from Browser: UA length=%d, CF length=%d", len(credentials.UA), len(credentials.CF))
	return map[string]string{
		"ua": credentials.UA,
		"cf": credentials.CF,
	}, nil
}

func (a *App) GetDetectedBrowsers() ([]map[string]string, error) {
	supported := []struct {
		ID   string
		Name string
	}{
		{"chrome", "Google Chrome"},
		{"brave", "Brave"},
		{"edge", "Microsoft Edge"},
		{"chromium", "Chromium"},
		{"vivaldi", "Vivaldi"},
		{"opera", "Opera"},
	}

	detected := []map[string]string{
		{"id": "auto", "name": "Auto (Default)"},
	}

	for _, b := range supported {
		path, err := browser.FindCandidatePath(b.ID)
		if err == nil && path != "" {
			detected = append(detected, map[string]string{
				"id":   b.ID,
				"name": b.Name,
			})
		}
	}

	return detected, nil
}

func (a *App) SaveConfig(newUA, newCF, newDir, newQuality, newAudio, newDomain, domainAnikoto, domainAnimeHeaven, browserType, browserPath string, maxParallel int, hlsTranscode, minimizeToTray, enableMonitor, autoDownloadTracked, autoCheckUpdates bool, pollIntervalMinutes int, provider string) error {
	logger.Infof("APP_SAVE_SETTINGS", "Saving application settings...")
	cfg, err := config.Load()
	if err != nil {
		logger.Errorf("APP_CONFIG_ERR", "Failed to load config: %v", err)
		return fmt.Errorf("failed to load configuration; see henzuku.log")
	}

	cfg.UA = strings.TrimSpace(newUA)
	cfg.CF = strings.TrimSpace(newCF)
	cfg.Cookies = "cf_clearance=" + cfg.CF
	if newDir != "" {
		cfg.DownloadDir = strings.TrimSpace(newDir)
	}
	cfg.Quality = strings.TrimSpace(newQuality)
	cfg.Audio = strings.TrimSpace(newAudio)
	if newDomain != "" {
		cfg.Domain = strings.TrimSpace(newDomain)
	}
	if domainAnikoto != "" {
		cfg.DomainAnikoto = strings.TrimSpace(domainAnikoto)
	}
	if domainAnimeHeaven != "" {
		cfg.DomainAnimeHeaven = strings.TrimSpace(domainAnimeHeaven)
	}
	cfg.Browser = browserType
	cfg.BrowserPath = browserPath
	cfg.MaxParallel = maxParallel
	cfg.HlsTranscode = hlsTranscode
	cfg.MinimizeToTray = minimizeToTray
	cfg.EnableBackgroundMonitor = enableMonitor
	cfg.AutoDownloadTracked = autoDownloadTracked
	cfg.AutoCheckUpdates = autoCheckUpdates
	cfg.PollIntervalMinutes = pollIntervalMinutes
	if provider != "" {
		cfg.Provider = strings.TrimSpace(provider)
	}

	if err := cfg.Save(); err != nil {
		logger.Errorf("APP_SAVE_SETTINGS_ERR", "Failed to save settings: %v", err)
		return fmt.Errorf("failed to save settings: %w", err)
	}

	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
	if err == nil {
		a.client = client
	}
	if a.dlManager != nil {
		a.dlManager.SetMaxParallel(cfg.MaxParallel)
	}

	logger.Infof("APP_SAVE_SETTINGS_OK", "Settings saved successfully")
	return nil
}

func (a *App) CheckAppUpdate() (*updater.UpdateInfo, error) {
	return updater.CheckUpdate(AppVersion)
}

func (a *App) InstallAppUpdate(downloadURL string) error {
	return updater.DownloadAndApplyUpdate(downloadURL)
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

func (a *App) StartDownload(animeTitle, slug string, provider string, epNums []float64) error {
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

	p := strings.ToLower(strings.TrimSpace(provider))
	if p == "" {
		p = cfg.Provider
	}
	if p != "anikoto" && p != "animeheaven" {
		p = "animepahe"
	}

	if p == "animepahe" && (cfg.UA == "" || cfg.CF == "") {
		return fmt.Errorf("please configure User-Agent and Cloudflare clearance in Settings first")
	}

	client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.GetProviderDomain(p))
	if err != nil {
		logger.Errorf("APP_CLIENT_ERR", "Failed to initialize API client: %v", err)
		return fmt.Errorf("failed to initialize client; check settings or see henzuku.log")
	}

	a.client = client
	if a.dlManager == nil {
		a.dlManager = dl.NewManager(cfg.MaxParallel, cfg.UA, cfg.Cookies)
	} else {
		a.dlManager.SetMaxParallel(cfg.MaxParallel)
		a.dlManager.UpdateConfig(cfg.UA, cfg.Cookies)
	}

	logger.Infof("DOWNLOAD_BATCH_START", "Starting download batch of %d episodes for anime %q (slug: %q, provider: %q)", len(epNums), animeTitle, slug, p)

	// Clear cancelled state and pre-populate queue with status "queued" using ID (Anime Title + EpNum)
	var jobIDs []string
	for _, epNum := range epNums {
		epStr := fmt.Sprintf("E%02.0f", epNum)
		if math.Mod(epNum, 1) != 0 {
			epStr = fmt.Sprintf("E%.1f", epNum)
		}
		jobID := fmt.Sprintf("%s - %s", animeTitle, epStr)
		jobIDs = append(jobIDs, jobID)
	}
	a.dlManager.ClearCancelled(jobIDs...)
	for i, epNum := range epNums {
		a.dlManager.UpdateProgress(jobIDs[i], animeTitle, epNum, "queued", 0, "", "", "")
	}

	go func() {
		var eps []api.Episode
		var err error
		if p == "anikoto" {
			eps, err = client.GetAnikotoEpisodes(slug)
		} else if p == "animeheaven" {
			eps, err = client.GetAnimeHeavenEpisodes(slug)
		} else {
			eps, err = client.GetEpisodes(slug)
		}

		if err != nil {
			logger.Errorf("APP_EPISODES_ERR", "Failed to fetch episodes for %s (%s): %v", animeTitle, slug, err)
			for i, epNum := range epNums {
				a.dlManager.UpdateProgress(jobIDs[i], animeTitle, epNum, "failed", 0, "", "", "failed to retrieve release list")
			}
			return
		}

		epMap := make(map[float64]api.Episode)
		for _, e := range eps {
			epMap[e.Episode] = e
		}

		var resolveWg sync.WaitGroup
		resolveWg.Add(len(epNums))
		for i, epNum := range epNums {
			epNum := epNum
			jobID := jobIDs[i]
			a.resolveSem <- struct{}{}
			go func() {
				defer resolveWg.Done()
				defer func() { <-a.resolveSem }()

				ep, ok := epMap[epNum]
				if !ok {
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "episode not found in release list")
					return
				}

				epStr := fmt.Sprintf("E%02.0f", epNum)
				if math.Mod(epNum, 1) != 0 {
					epStr = fmt.Sprintf("E%.1f", epNum)
				}

				logger.Infof("RESOLVE_START", "Resolving stream links for %s (provider: %s)...", jobID, p)

				var dlURL string
				var isHLS bool

				if p == "anikoto" {
					for attempt := 1; attempt <= 6; attempt++ {
						dlURL, isHLS, err = client.GetAnikotoStreamURL(slug, ep.Session, cfg.Audio)
						if err == nil && dlURL != "" {
							break
						}
						if attempt < 6 {
							time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
						}
					}
				} else if p == "animeheaven" {
					for attempt := 1; attempt <= 6; attempt++ {
						mirrors, _, getErr := client.GetAnimeHeavenStream(ep.Session, slug)
						if getErr == nil && len(mirrors) > 0 {
							dlURL = mirrors[0]
							isHLS = strings.Contains(dlURL, ".m3u8")
							err = nil
							break
						}
						err = getErr
						if attempt < 6 {
							time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
						}
					}
				} else {
					var candidates []api.KwikCandidate
					for attempt := 1; attempt <= 6; attempt++ {
						candidates, err = client.GetKwikLinks(slug, ep.Session)
						if err == nil && len(candidates) > 0 {
							break
						}
						if attempt < 6 {
							time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
						}
					}

					if err != nil || len(candidates) == 0 {
						logger.Errorf("APP_KWIK_RESOLVE_ERR", "Failed to resolve kwik redirect links for %s: %v", jobID, err)
						a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "failed to resolve Kwik redirect links")
						return
					}

					kwikURL := api.SelectBestKwik(candidates, cfg.Quality, cfg.Audio)
					if kwikURL == "" {
						logger.Errorf("APP_KWIK_SELECT_ERR", "No candidate matching %sp/%s found for %s", cfg.Quality, cfg.Audio, jobID)
						a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "no link matching selected quality/audio found")
						return
					}

					extractor := kwik.NewExtractor(cfg.UA, cfg.Cookies)
					for attempt := 1; attempt <= 6; attempt++ {
						dlURL, isHLS, err = extractor.GetDownloadURL(kwikURL)
						if err == nil && dlURL != "" {
							break
						}
						if attempt < 6 {
							time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
						}
					}
				}

				if err != nil || dlURL == "" {
					logger.Errorf("APP_EXTRACT_ERR", "Failed link extraction for %s (%s): %v", jobID, p, err)
					a.dlManager.UpdateProgress(jobID, animeTitle, epNum, "failed", 0, "", "", "failed stream link extraction")
					return
				}

				logger.Infof("RESOLVE_OK", "Successfully resolved download URL for %s (HLS: %t)", jobID, isHLS)

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

				jobReferer := ""
				if p == "anikoto" {
					jobReferer = "https://megaplay.buzz/"
				} else if p == "animeheaven" {
					jobReferer = "https://animeheaven.me/gate.php"
				}

				a.dlManager.Submit(dl.Job{
					ID:           jobID,
					AnimeTitle:   animeTitle,
					EpNum:        epNum,
					URL:          dlURL,
					IsHLS:        isHLS,
					OutputPath:   outPath,
					HlsTranscode: cfg.HlsTranscode,
					Referer:      jobReferer,
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
	_, err = net.LookupHost(u.Host)
	return err == nil
}

func (a *App) RetryFailed(animeTitle string) error {
	logger.Infof("APP_RETRY_FAILED", "Retrying failed downloads for anime %q", animeTitle)
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
		if p.Anime == animeTitle && p.Status == "failed" {
			epNums = append(epNums, p.EpNum)
		}
	}

	if len(epNums) == 0 {
		return fmt.Errorf("no failed or active downloads to retry for this anime")
	}

	return a.StartDownload(animeTitle, slug, "", epNums)
}

func (a *App) CancelAnimeDownloads(animeTitle string) error {
	logger.Infof("APP_CANCEL_DOWNLOADS", "Cancelling all downloads for anime %q", animeTitle)
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

func (a *App) OpenDownloadFolder() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}
	dir := cfg.DownloadDir
	if dir == "" {
		return fmt.Errorf("download directory is not configured")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create download directory: %v", err)
		}
	}
	return exec.Command("explorer", dir).Start()
}

func (a *App) GetAnimeMetadata(title string) (*api.MetadataResult, error) {
	return api.FetchAnimeMetadata(title)
}

func (a *App) GetTrackedAnime() []tracker.TrackedAnime {
	if a.trackerMgr == nil {
		return []tracker.TrackedAnime{}
	}
	return a.trackerMgr.GetList()
}

func (a *App) IsAnimeTracked(title string) bool {
	if a.trackerMgr == nil {
		return false
	}
	return a.trackerMgr.IsTracked(title)
}

func (a *App) ToggleTrackAnime(title, slug, poster, provider string) (bool, error) {
	if a.trackerMgr == nil {
		return false, fmt.Errorf("tracker manager not initialized")
	}
	return a.trackerMgr.ToggleTrack(title, slug, poster, provider)
}

func (a *App) BatchTrackAnime(shows []tracker.TrackedAnime) (int, error) {
	if a.trackerMgr == nil {
		return 0, fmt.Errorf("tracker manager not initialized")
	}
	return a.trackerMgr.BatchTrack(shows)
}

func (a *App) CheckTrackedUpdatesNow() error {
	logger.Infof("TRACKER_MANUAL_CHECK", "Triggered manual check for tracked anime updates...")
	go a.performTrackedCheck()
	return nil
}

func (a *App) startBackgroundMonitor() {
	// Initial delay to let application start up cleanly
	time.Sleep(10 * time.Second)

	for {
		cfg, err := config.Load()
		if err == nil && cfg.EnableBackgroundMonitor {
			logger.Infof("BG_MONITOR", "Starting periodic background update check for tracked anime...")
			a.performTrackedCheck()
		}

		interval := 30
		if cfg != nil && cfg.PollIntervalMinutes > 0 {
			interval = cfg.PollIntervalMinutes
		}
		time.Sleep(time.Duration(interval) * time.Minute)
	}
}

func (a *App) performTrackedCheck() {
	if a.trackerMgr == nil {
		return
	}

	tracked := a.trackerMgr.GetList()
	if len(tracked) == 0 {
		return
	}

	cfg, err := config.Load()
	if err != nil {
		return
	}

	for _, item := range tracked {
		if item.AiringStatus == string(api.StatusFinished) {
			continue
		}

		prov := item.Provider
		if prov == "" {
			prov = "animepahe"
		}

		if prov == "animepahe" && (cfg.UA == "" || cfg.CF == "") {
			continue
		}

		client, err := api.NewClient(cfg.UA, cfg.Cookies, cfg.GetProviderDomain(prov))
		if err != nil {
			continue
		}

		var eps []api.Episode
		if prov == "anikoto" {
			eps, err = client.GetAnikotoEpisodes(item.Slug)
		} else if prov == "animeheaven" {
			eps, err = client.GetAnimeHeavenEpisodes(item.Slug)
		} else {
			eps, err = client.GetEpisodes(item.Slug)
		}

		if err != nil {
			logger.Warnf("BG_EPISODES_ERR", "Background check failed to fetch episodes for %s (%s): %v", item.Title, prov, err)
			continue
		}

		var newEps []float64
		maxEp := item.LastDownloadedEp

		existingEps := scanExistingEpisodes(cfg.DownloadDir, item.Title)

		for _, e := range eps {
			if e.Episode > item.LastDownloadedEp && !existingEps[e.Episode] {
				newEps = append(newEps, e.Episode)
				if e.Episode > maxEp {
					maxEp = e.Episode
				}
			}
		}

		if len(newEps) > 0 {
			msg := fmt.Sprintf("%d new episode(s) released for %s!", len(newEps), item.Title)
			logger.Infof("BG_NEW_EPISODE", "%s", msg)
			notify.SendToast("Zensu Anime Update", msg)

			if cfg.AutoDownloadTracked && item.AutoDownload {
				logger.Infof("BG_AUTO_DOWNLOAD", "Auto-starting download of %d episodes for %s", len(newEps), item.Title)
				if err := a.StartDownload(item.Title, item.Slug, "", newEps); err == nil {
					a.trackerMgr.UpdateLastDownloaded(item.Title, maxEp)
				}
			}
		}

		// Update AniList & Jikan metadata status
		if meta, err := api.FetchAnimeMetadata(item.Title); err == nil && meta != nil {
			a.trackerMgr.UpdateFullMetadata(item.Title, meta)
		}
	}
}
