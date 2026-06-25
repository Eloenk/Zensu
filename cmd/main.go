package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"zensu/internal/api"
	"zensu/internal/chrome"
	"zensu/internal/config"
	"zensu/internal/dl"
	"zensu/internal/kwik"
	"zensu/internal/logger"
	"zensu/internal/ui"
)

var nonAlphanumRe = regexp.MustCompile(`[^\w ,+\-()\s]`)

func sanitizeName(name string) string {
	name = nonAlphanumRe.ReplaceAllString(name, " ")
	name = regexp.MustCompile(`\s+`).ReplaceAllString(name, " ")
	return strings.TrimSpace(name)
}

func main() {
	if err := logger.Init(); err != nil {
		fmt.Printf("Error initializing logger: %v\n", err)
	}
	defer logger.Close()

	cfg, err := config.Load()
	if err != nil {
		fatalf("config error: %v\n", err)
	}

	needsSolve := cfg.UA == "" || cfg.CF == ""
	var client *api.Client

	if !needsSolve {
		var clientErr error
		client, clientErr = api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
		if clientErr == nil {
			fmt.Println("  \033[32m[INFO]\033[0m Testing connection to domain...")
			if connErr := client.TestConnection(); connErr != nil {
				logger.Warnf("CLI_STARTUP_CONN_FAIL", "Connection test failed: %v", connErr)
				fmt.Printf("  \033[33m[WARN]\033[0m Connection test failed: %v\n", connErr)
				needsSolve = true
			} else {
				fmt.Println("  \033[32m[SUCCESS]\033[0m Connection test passed! Credentials are valid.")
			}
		} else {
			needsSolve = true
		}
	}

	if needsSolve {
		if cfg.UA == "" || cfg.CF == "" {
			fmt.Println("  \033[33m[INFO]\033[0m Missing Cloudflare credentials (UA or cf_clearance).")
		} else {
			fmt.Println("  \033[33m[INFO]\033[0m Clearance cookies are expired or invalid.")
		}
		if err := refreshCredentials(cfg); err != nil {
			fatalf("failed to resolve Cloudflare credentials: %v\n", err)
		}

		var clientErr error
		client, clientErr = api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
		if clientErr != nil {
			fatalf("failed to init client: %v\n", clientErr)
		}
	}

	extractor := kwik.NewExtractor(cfg.UA, cfg.Cookies)
	manager := dl.NewManager(cfg.MaxParallel, cfg.UA)

	// Handle signals for graceful shutdown on Ctrl+C / terminal close
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n  [INFO] Received interrupt signal. Cleaning up active downloads...")
		manager.CancelAll()
		os.Exit(0)
	}()

	fmt.Println()
	fmt.Println("  \033[1;36manimepahe-dl\033[0m")
	fmt.Println()

	searchTerm, err := ui.PromptAnimeName()
	checkErr(err)

	fmt.Println("\n  \033[32m[INFO]\033[0m Searching...")
	results, err := client.Search(searchTerm)
	if err != nil && strings.Contains(err.Error(), "403") {
		fmt.Println("  \033[33m[WARN]\033[0m Cloudflare clearance expired or invalid.")
		if err := refreshCredentials(cfg); err != nil {
			fatalf("failed to refresh credentials: %v\n", err)
		}
		// Reinitialize clients with the new cookies
		client, err = api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
		if err != nil {
			fatalf("failed to re-init client: %v\n", err)
		}
		extractor = kwik.NewExtractor(cfg.UA, cfg.Cookies)
		manager = dl.NewManager(cfg.MaxParallel, cfg.UA)
		results, err = client.Search(searchTerm)
	}
	if err != nil {
		fatalf("search failed: %v\n", err)
	}
	if len(results) == 0 {
		fatalf("no results found for %q\n", searchTerm)
	}

	titles := make([]string, len(results))
	for i, r := range results {
		titles[i] = r.Title
	}
	idx, err := ui.PromptSelectAnime(titles)
	checkErr(err)

	chosen := results[idx]
	slug := chosen.Session
	animeName := sanitizeName(chosen.Title)

	fmt.Println("\n  \033[32m[INFO]\033[0m Fetching episodes...")
	episodes, err := client.GetEpisodes(slug)
	if err != nil && strings.Contains(err.Error(), "403") {
		fmt.Println("  \033[33m[WARN]\033[0m Cloudflare clearance expired or invalid.")
		if err := refreshCredentials(cfg); err != nil {
			fatalf("failed to refresh credentials: %v\n", err)
		}
		// Reinitialize clients with the new cookies
		client, err = api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
		if err != nil {
			fatalf("failed to re-init client: %v\n", err)
		}
		extractor = kwik.NewExtractor(cfg.UA, cfg.Cookies)
		manager = dl.NewManager(cfg.MaxParallel, cfg.UA)
		episodes, err = client.GetEpisodes(slug)
	}
	if err != nil {
		fatalf("failed to fetch episodes: %v\n", err)
	}
	if len(episodes) == 0 {
		fatalf("no episodes found\n")
	}

	allNums := make([]float64, len(episodes))
	epMap := make(map[float64]api.Episode)
	for i, ep := range episodes {
		allNums[i] = ep.Episode
		epMap[ep.Episode] = ep
	}

	downloadDir := cfg.DownloadDir
	if strings.HasPrefix(downloadDir, "~/") {
		home, _ := os.UserHomeDir()
		downloadDir = home + downloadDir[1:]
	}
	outDir := filepath.Join(downloadDir, animeName)
	fmt.Printf("\n  Download folder: %s\n", downloadDir)

	existingMap := make(map[float64]bool)
	if files, err := os.ReadDir(outDir); err == nil {
		pattern := regexp.MustCompile("^" + regexp.QuoteMeta(animeName) + ` E(\d+(?:\.\d+)?)\.mp4$`)
		for _, file := range files {
			if file.IsDir() {
				continue
			}
			m := pattern.FindStringSubmatch(file.Name())
			if len(m) == 2 {
				if num, err := strconv.ParseFloat(m[1], 64); err == nil {
					existingMap[num] = true
				}
			}
		}
	}

	selected, err := ui.PromptEpisodes(allNums, existingMap)
	checkErr(err)

	resolution := cfg.Quality
	audio := cfg.Audio

	epSummary := ""
	if len(selected) > 10 {
		epSummary = fmt.Sprintf("E%.0f – E%.0f (%d episodes)", selected[0], selected[len(selected)-1], len(selected))
	} else {
		parts := make([]string, len(selected))
		for i, n := range selected {
			parts[i] = fmt.Sprintf("%.0f", n)
		}
		epSummary = "E" + strings.Join(parts, ", E")
	}

	fmt.Printf("\n  Anime:    %s\n", animeName)
	fmt.Printf("  Episodes: %s\n", epSummary)
	fmt.Printf("  Quality:  %sp / %s\n", resolution, audio)
	fmt.Printf("  Save to:  %s\n", outDir)



	if err := os.MkdirAll(outDir, 0755); err != nil {
		fatalf("failed to create output dir: %v\n", err)
	}

	fmt.Println()

	jobs := make(chan dl.Job, len(selected))

	var resolveWg sync.WaitGroup

	resolveSem := make(chan struct{}, 6)

	resolveWg.Add(len(selected))
	go func() {
		for _, epNum := range selected {
			epNum := epNum
			resolveSem <- struct{}{}
			go func() {
				defer resolveWg.Done()
				defer func() { <-resolveSem }()

				ep, ok := epMap[epNum]
				if !ok {
					fmt.Printf("  \033[33m[WARN]\033[0m E%.0f not found in episode list\n", epNum)
					return
				}

				fmt.Printf("  \033[32m[INFO]\033[0m E%.0f resolving link...\n", epNum)

				var candidates []api.KwikCandidate
				var err error
				for attempt := 1; attempt <= 6; attempt++ {
					candidates, err = client.GetKwikLinks(slug, ep.Session)
					if err == nil && len(candidates) > 0 {
						break
					}
					if attempt < 6 {
						time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
						continue
					}
				}
				if err != nil {
					fmt.Printf("  \033[31m[ERROR]\033[0m E%.0f get kwik links: %v\n", epNum, err)
					return
				}
				if len(candidates) == 0 {
					fmt.Printf("  \033[33m[WARN]\033[0m E%.0f no kwik links found\n", epNum)
					return
				}

				kwikURL := api.SelectBestKwik(candidates, resolution, audio)
				if kwikURL == "" {
					fmt.Printf("  \033[33m[WARN]\033[0m E%.0f no matching link for %sp/%s\n", epNum, resolution, audio)
					return
				}

				var dlURL string
				var isHLS bool
				for attempt := 1; attempt <= 6; attempt++ {
					dlURL, isHLS, err = extractor.GetDownloadURL(kwikURL)
					if err == nil && dlURL != "" {
						break
					}
					if attempt < 6 {
						time.Sleep(time.Duration(attempt) * 2000 * time.Millisecond)
						continue
					}
				}
				if err != nil {
					fmt.Printf("  \033[31m[ERROR]\033[0m E%.0f kwik extract: %v\n", epNum, err)
					return
				}

				epStr := fmt.Sprintf("E%02.0f", epNum)
				if math.Mod(epNum, 1) != 0 {

					epStr = fmt.Sprintf("E%.1f", epNum)
				}

				ext := ".mp4"
				if isHLS {
					ext = ".mp4"
				}
				outPath := filepath.Join(outDir, animeName+" "+epStr+ext)

				fmt.Printf("  \033[32m[INFO]\033[0m E%.0f link resolved → sending to downloader\n", epNum)
				jobs <- dl.Job{
					EpNum:      epNum,
					URL:        dlURL,
					IsHLS:      isHLS,
					OutputPath: outPath,
				}
			}()
		}

		resolveWg.Wait()
		close(jobs)
	}()

	dlResults := manager.RunAll(jobs, len(selected))

	success := 0
	failed := 0
	for res := range dlResults {
		if res.Err != nil {
			fmt.Printf("  \033[31m[ERROR]\033[0m E%.0f failed: %v\n", res.Job.EpNum, res.Err)
			failed++
		} else {
			fmt.Printf("  \033[32m[DONE]\033[0m  E%.0f saved in %s\n", res.Job.EpNum, res.Elapsed.Round(1e8))
			success++
		}
	}

	fmt.Println()
	fmt.Printf("  \033[1;32mAll done!\033[0m  %d downloaded", success)
	if failed > 0 {
		fmt.Printf(", \033[31m%d failed\033[0m", failed)
	}
	fmt.Println()
	fmt.Println()
}

func checkErr(err error) {
	if err != nil {
		fatalf("%v\n", err)
	}
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	logger.Error("FATAL", msg)
	fmt.Fprintf(os.Stderr, "  \033[31m[ERROR]\033[0m "+format, args...)
	os.Exit(1)
}

func refreshCredentials(cfg *config.Config) error {
	fmt.Println("  \033[33m[INFO]\033[0m Launching Chrome to solve Cloudflare challenge...")
	fmt.Println("         (Please click/solve any verification challenge if prompted)")
	credentials, err := chrome.FetchCredentials(cfg.Domain)
	if err != nil {
		return err
	}
	cfg.UA = credentials.UA
	cfg.CF = credentials.CF
	cfg.Cookies = "cf_clearance=" + credentials.CF
	if err := cfg.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	fmt.Println("  \033[32m[SUCCESS]\033[0m Credentials fetched and saved successfully!")
	return nil
}

