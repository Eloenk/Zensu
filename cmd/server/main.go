package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"zensu/internal/api"
	"zensu/internal/chrome"
	"zensu/internal/config"
	"zensu/internal/kwik"
	"zensu/internal/logger"
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
		fmt.Printf("[ERROR] config error: %v\n", err)
		os.Exit(1)
	}

	needsSolve := cfg.UA == "" || cfg.CF == ""
	var client *api.Client

	if !needsSolve {
		var clientErr error
		client, clientErr = api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
		if clientErr == nil {
			fmt.Println("  \033[32m[INFO]\033[0m Testing connection to domain...")
			if connErr := client.TestConnection(); connErr != nil {
				logger.Warnf("SERVER_STARTUP_CONN_FAIL", "Connection test failed: %v", connErr)
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
			fmt.Printf("[ERROR] failed to resolve Cloudflare credentials: %v\n", err)
			os.Exit(1)
		}

		var clientErr error
		client, clientErr = api.NewClient(cfg.UA, cfg.Cookies, cfg.Domain)
		if clientErr != nil {
			fmt.Printf("[ERROR] failed to init client: %v\n", clientErr)
			os.Exit(1)
		}
	}

	extractor := kwik.NewExtractor(cfg.UA, cfg.Cookies)

	// Set up router
	mux := http.NewServeMux()
	mux.HandleFunc("/api/search", handleSearch(client))
	mux.HandleFunc("/api/episodes", handleEpisodes(client))
	mux.HandleFunc("/api/stream", handleStream(client, extractor, cfg))

	port := 8080
	serverAddr := fmt.Sprintf("127.0.0.1:%d", port)
	server := &http.Server{
		Addr:    serverAddr,
		Handler: corsMiddleware(mux),
	}

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\n  [INFO] Shutting down streaming server...")
		server.Close()
		os.Exit(0)
	}()

	fmt.Println()
	fmt.Printf("  \033[1;36mZensu Streaming Server running at http://%s\033[0m\n", serverAddr)
	fmt.Println("  Press Ctrl+C to stop.")
	fmt.Println()

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		fmt.Printf("[ERROR] server failed: %v\n", err)
		os.Exit(1)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "*")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleSearch(client *api.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		if q == "" {
			http.Error(w, "missing parameter 'q'", http.StatusBadRequest)
			return
		}

		logger.Infof("SERVER_SEARCH", "Searching for anime matching %q", q)
		results, err := client.Search(q)
		if err != nil {
			logger.Errorf("SERVER_SEARCH_ERR", "Search failed: %v", err)
			http.Error(w, fmt.Sprintf("search failed: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

func handleEpisodes(client *api.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.URL.Query().Get("slug")
		if slug == "" {
			http.Error(w, "missing parameter 'slug'", http.StatusBadRequest)
			return
		}

		logger.Infof("SERVER_EPISODES", "Fetching episodes for slug %s", slug)
		episodes, err := client.GetEpisodes(slug)
		if err != nil {
			logger.Errorf("SERVER_EPISODES_ERR", "Failed to fetch episodes: %v", err)
			http.Error(w, fmt.Sprintf("failed to fetch episodes: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(episodes)
	}
}

func handleStream(client *api.Client, extractor *kwik.Extractor, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.URL.Query().Get("slug")
		epSession := r.URL.Query().Get("session")
		title := r.URL.Query().Get("title")

		if slug == "" || epSession == "" {
			http.Error(w, "missing required parameters 'slug' and 'session'", http.StatusBadRequest)
			return
		}

		quality := r.URL.Query().Get("quality")
		if quality == "" {
			quality = cfg.Quality
		}
		audio := r.URL.Query().Get("audio")
		if audio == "" {
			audio = cfg.Audio
		}

		logger.Infof("SERVER_STREAM_REQ", "Stream requested for slug: %s, session: %s, title: %q", slug, epSession, title)

		// 1. If Title is provided, check if the file is already downloaded locally
		if title != "" {
			sanitizedTitle := sanitizeName(title)
			episodes, err := client.GetEpisodes(slug)
			if err == nil {
				var epNum float64
				found := false
				for _, ep := range episodes {
					if ep.Session == epSession {
						epNum = ep.Episode
						found = true
						break
					}
				}

				if found {
					epStr := fmt.Sprintf("E%02.0f", epNum)
					if math.Mod(epNum, 1) != 0 {
						epStr = fmt.Sprintf("E%.1f", epNum)
					}

					downloadDir := cfg.DownloadDir
					if strings.HasPrefix(downloadDir, "~/") {
						home, _ := os.UserHomeDir()
						downloadDir = home + downloadDir[1:]
					}

					localPath := filepath.Join(downloadDir, sanitizedTitle, sanitizedTitle+" "+epStr+".mp4")
					if _, err := os.Stat(localPath); err == nil {
						logger.Infof("SERVER_STREAM_STATIC", "Serving local static file: %s", localPath)
						http.ServeFile(w, r, localPath)
						return
					}
				}
			}
		}

		// 2. Fallback: Resolve Kwik link and stream/proxy on-the-fly
		candidates, err := client.GetKwikLinks(slug, epSession)
		if err != nil {
			logger.Errorf("SERVER_STREAM_KWIK_ERR", "Failed to resolve kwik links: %v", err)
			http.Error(w, fmt.Sprintf("failed to resolve links: %v", err), http.StatusInternalServerError)
			return
		}
		if len(candidates) == 0 {
			http.Error(w, "no links found for requested episode", http.StatusNotFound)
			return
		}

		kwikURL := api.SelectBestKwik(candidates, quality, audio)
		if kwikURL == "" {
			http.Error(w, "no candidate matching requested quality/audio", http.StatusNotFound)
			return
		}

		dlURL, _, err := extractor.GetDownloadURL(kwikURL)
		if err != nil {
			logger.Errorf("SERVER_STREAM_EXTRACT_ERR", "Failed to extract direct download URL: %v", err)
			http.Error(w, fmt.Sprintf("extraction failed: %v", err), http.StatusInternalServerError)
			return
		}

		logger.Infof("SERVER_STREAM_PROXY", "Proxying remote stream: %s", dlURL)
		proxyStream(w, r, dlURL, cfg.UA)
	}
}

func proxyStream(w http.ResponseWriter, r *http.Request, rawURL string, ua string) {
	ctx := r.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header.Set("User-Agent", ua)
	req.Header.Set("Referer", "https://kwik.cx/")
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("SERVER_PROXY_ERR", "Proxy request failed: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy streaming relevant headers back to the client response
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	if rangeHeader := resp.Header.Get("Content-Range"); rangeHeader != "" {
		w.Header().Set("Content-Range", rangeHeader)
	}
	if lengthHeader := resp.Header.Get("Content-Length"); lengthHeader != "" {
		w.Header().Set("Content-Length", lengthHeader)
	}
	if acceptRanges := resp.Header.Get("Accept-Ranges"); acceptRanges != "" {
		w.Header().Set("Accept-Ranges", acceptRanges)
	}

	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
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
