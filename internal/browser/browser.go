package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

type Credentials struct {
	UA string
	CF string
}

type Target struct {
	ID                  string `json:"id"`
	Type                string `json:"type"`
	URL                 string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type CDPRequest struct {
	ID     int            `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type CDPResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *CDPError       `json:"error"`
}

type CDPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Cookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type GetCookiesResult struct {
	Cookies []Cookie `json:"cookies"`
}

type EvaluateResult struct {
	Result struct {
		Type  string `json:"type"`
		Value string `json:"value"`
	} `json:"result"`
}

var Candidates = map[string]map[string][]string{
	"chrome": {
		"windows": {"Google/Chrome/Application/chrome.exe"},
		"linux":   {"/usr/bin/google-chrome", "/usr/bin/google-chrome-stable"},
	},
	"brave": {
		"windows": {"BraveSoftware/Brave-Browser/Application/brave.exe"},
		"linux":   {"/usr/bin/brave-browser", "/usr/bin/brave"},
	},
	"edge": {
		"windows": {"Microsoft/Edge/Application/msedge.exe"},
		"linux":   {"/usr/bin/microsoft-edge", "/usr/bin/microsoft-edge-stable"},
	},
	"chromium": {
		"windows": {},
		"linux":   {"/usr/bin/chromium", "/usr/bin/chromium-browser", "/snap/bin/chromium"},
	},
	"vivaldi": {
		"windows": {"Vivaldi/Application/vivaldi.exe"},
		"linux":   {"/usr/bin/vivaldi", "/usr/bin/vivaldi-stable"},
	},
	"opera": {
		"windows": {"Opera/launcher.exe"},
		"linux":   {"/usr/bin/opera"},
	},
}

func IsCDPReady(port int) bool {
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func resolveWindowsPaths(relPath string) []string {
	var paths []string
	programFiles := os.Getenv("ProgramFiles")
	programFilesX86 := os.Getenv("ProgramFiles(x86)")
	localAppData := os.Getenv("LocalAppData")

	if programFiles != "" {
		paths = append(paths, filepath.Join(programFiles, relPath))
	}
	if programFilesX86 != "" {
		paths = append(paths, filepath.Join(programFilesX86, relPath))
	}
	if localAppData != "" {
		paths = append(paths, filepath.Join(localAppData, relPath))
		paths = append(paths, filepath.Join(localAppData, "Programs", relPath))
	}
	return paths
}

func FindBrowserPath(browserType string, customPath string) (string, error) {
	if customPath != "" {
		if _, err := os.Stat(customPath); err == nil {
			return customPath, nil
		}
		return "", fmt.Errorf("custom browser path does not exist: %s", customPath)
	}

	if browserType == "" {
		browserType = "auto"
	}
	browserType = strings.ToLower(browserType)

	// Try default system browser first if type is "auto"
	if browserType == "auto" {
		if defaultPath, err := getDefaultBrowserPath(); err == nil && defaultPath != "" {
			if _, err := os.Stat(defaultPath); err == nil {
				// Don't use Safari as default since it lacks CDP support
				if !strings.Contains(strings.ToLower(defaultPath), "safari") {
					return defaultPath, nil
				}
			}
		}
		// Fallback to searching popular browsers in order
		order := []string{"chrome", "brave", "edge", "chromium", "vivaldi", "opera"}
		for _, b := range order {
			if path, err := FindCandidatePath(b); err == nil && path != "" {
				return path, nil
			}
		}
		return "", fmt.Errorf("no supported browser found on system")
	}

	return FindCandidatePath(browserType)
}

func FindCandidatePath(browserType string) (string, error) {
	bMap, ok := Candidates[browserType]
	if !ok {
		return "", fmt.Errorf("unsupported browser type: %s", browserType)
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		for _, rel := range bMap["windows"] {
			candidates = append(candidates, resolveWindowsPaths(rel)...)
		}
	} else {
		candidates = bMap["linux"]
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}

	// Try simple environment lookup
	if browserType == "chrome" {
		if path := os.Getenv("CHROME_PATH"); path != "" {
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
	}

	// Try checking the system PATH directly
	var binName string
	switch browserType {
	case "chrome":
		binName = "google-chrome"
		if runtime.GOOS == "windows" {
			binName = "chrome.exe"
		}
	case "brave":
		binName = "brave-browser"
		if runtime.GOOS == "windows" {
			binName = "brave.exe"
		}
	case "edge":
		binName = "microsoft-edge"
		if runtime.GOOS == "windows" {
			binName = "msedge.exe"
		}
	case "chromium":
		binName = "chromium"
	case "vivaldi":
		binName = "vivaldi"
	case "opera":
		binName = "opera"
	}
	if binName != "" {
		if p, err := exec.LookPath(binName); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("%s executable not found", browserType)
}

func getLaunchArgs(browserPath string, port int, profileDir string, targetURL string) []string {
	return []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-debugging-address=127.0.0.1",
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--no-first-run",
		"--no-default-browser-check",
		targetURL,
	}
}

func launchBrowser(browserPath string, port int, profileDir string, targetURL string) (*exec.Cmd, error) {
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return nil, err
	}

	args := getLaunchArgs(browserPath, port, profileDir, targetURL)
	cmd := exec.Command(browserPath, args...)
	setSysProcAttr(cmd)

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func getDomainHost(rawURL string) string {
	rawURL = strings.ToLower(rawURL)
	rawURL = strings.TrimPrefix(rawURL, "https://")
	rawURL = strings.TrimPrefix(rawURL, "http://")
	rawURL = strings.TrimPrefix(rawURL, "www.")
	idx := strings.IndexAny(rawURL, "/?#")
	if idx != -1 {
		rawURL = rawURL[:idx]
	}
	return rawURL
}

func getTargetTab(port int, domain string) (*Target, error) {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json", port))
	if err != nil {
		return nil, fmt.Errorf("failed to query targets: %w", err)
	}
	defer resp.Body.Close()

	var targets []Target
	if err := json.NewDecoder(resp.Body).Decode(&targets); err != nil {
		return nil, fmt.Errorf("failed to parse targets JSON: %w", err)
	}

	domainHost := getDomainHost(domain)
	for _, t := range targets {
		if t.Type == "page" && strings.Contains(strings.ToLower(t.URL), domainHost) && t.WebSocketDebuggerURL != "" {
			return &t, nil
		}
	}

	// Open a new tab
	newTabURL := fmt.Sprintf("http://127.0.0.1:%d/json/new?%s", port, url.QueryEscape(domain))
	req, err := http.NewRequest("PUT", newTabURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create PUT request for new tab: %w", err)
	}
	newResp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to open new tab: %w", err)
	}
	defer newResp.Body.Close()

	var newTarget Target
	if err := json.NewDecoder(newResp.Body).Decode(&newTarget); err != nil {
		return nil, fmt.Errorf("failed to parse new target JSON: %w", err)
	}
	if newTarget.WebSocketDebuggerURL == "" {
		return nil, fmt.Errorf("opened tab did not provide a WebSocket debugger URL")
	}

	return &newTarget, nil
}

func closeTargetTab(port int, targetID string) error {
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/close/%s", port, targetID))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func sendAndReceive(conn *websocket.Conn, method string, params map[string]any, id int) (json.RawMessage, error) {
	req := CDPRequest{
		ID:     id,
		Method: method,
		Params: params,
	}
	if err := conn.WriteJSON(req); err != nil {
		return nil, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	for {
		var resp CDPResponse
		if err := conn.ReadJSON(&resp); err != nil {
			return nil, err
		}
		if resp.ID == id {
			if resp.Error != nil {
				return nil, fmt.Errorf("cdp error: %s (code %d)", resp.Error.Message, resp.Error.Code)
			}
			return resp.Result, nil
		}
	}
}

func getCookies(conn *websocket.Conn, domain string) ([]Cookie, error) {
	params := map[string]any{
		"urls": []string{domain},
	}
	resultRaw, err := sendAndReceive(conn, "Network.getCookies", params, 1)
	if err != nil {
		return nil, err
	}

	var result GetCookiesResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return nil, err
	}
	return result.Cookies, nil
}

func getUserAgent(conn *websocket.Conn) (string, error) {
	params := map[string]any{
		"expression": "navigator.userAgent",
	}
	resultRaw, err := sendAndReceive(conn, "Runtime.evaluate", params, 2)
	if err != nil {
		return "", err
	}

	var result EvaluateResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return "", err
	}
	return result.Result.Value, nil
}

func getPageTitle(conn *websocket.Conn) (string, error) {
	params := map[string]any{
		"expression": "document.title",
	}
	resultRaw, err := sendAndReceive(conn, "Runtime.evaluate", params, 3)
	if err != nil {
		return "", err
	}

	var result EvaluateResult
	if err := json.Unmarshal(resultRaw, &result); err != nil {
		return "", err
	}
	return result.Result.Value, nil
}

func pollCookiesAndUA(wsURL string, domain string, oldCookie string) (*Credentials, error) {
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial failed: %w", err)
	}
	defer conn.Close()

	timeout := time.After(3 * time.Minute)
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for Cloudflare clearance cookies (3 minutes)")
		case <-ticker.C:
			// 1. Verify that the challenge/verification page is not active by checking the document title
			title, _ := getPageTitle(conn)
			titleLower := strings.ToLower(title)
			if strings.Contains(titleLower, "just a moment") ||
				strings.Contains(titleLower, "cloudflare") ||
				strings.Contains(titleLower, "attention required") ||
				title == "" {
				continue // Keep waiting
			}

			// 2. Fetch the clearance cookie
			cookies, err := getCookies(conn, domain)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch cookies via cdp: %w", err)
			}

			var cfClearance string
			for _, c := range cookies {
				if c.Name == "cf_clearance" {
					cfClearance = c.Value
					break
				}
			}

			// 3. Ensure the cookie is found and has been updated (if oldCookie was provided)
			if cfClearance != "" {
				if oldCookie != "" && cfClearance == oldCookie {
					continue // Still the old cookie, wait for refresh
				}

				ua, err := getUserAgent(conn)
				if err != nil {
					return nil, fmt.Errorf("failed to fetch user agent via cdp: %w", err)
				}
				if ua != "" {
					return &Credentials{
						UA: ua,
						CF: cfClearance,
					}, nil
				}
			}
		}
	}
}

func cleanProfileDir(dir string) {
	if dir == "" {
		return
	}
	for i := 0; i < 10; i++ {
		if err := os.RemoveAll(dir); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func FetchCredentials(domain string, browserType string, customPath string, oldCookie string) (*Credentials, error) {
	port := 9322
	spawned := false
	var cmd *exec.Cmd
	var profileDir string

	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}

	if !IsCDPReady(port) {
		browserPath, err := FindBrowserPath(browserType, customPath)
		if err != nil {
			return nil, fmt.Errorf("could not find compatible browser executable: %w", err)
		}

		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user config dir: %w", err)
		}
		profileDir = filepath.Join(userConfigDir, "zensu", "browser-profile-isolated")

		cmd, err = launchBrowser(browserPath, port, profileDir, domain)
		if err != nil {
			return nil, fmt.Errorf("failed to launch browser: %w", err)
		}
		spawned = true

		ready := false
		for i := 0; i < 30; i++ {
			if IsCDPReady(port) {
				ready = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !ready {
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
			cleanProfileDir(profileDir)
			return nil, fmt.Errorf("browser started but CDP did not become active on port %d within 15 seconds", port)
		}
	}

	target, err := getTargetTab(port, domain)
	if err != nil {
		if spawned && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			cleanProfileDir(profileDir)
		}
		return nil, err
	}

	credentials, err := pollCookiesAndUA(target.WebSocketDebuggerURL, domain, oldCookie)
	if err != nil {
		if spawned && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			cleanProfileDir(profileDir)
		} else if !spawned {
			_ = closeTargetTab(port, target.ID)
		}
		return nil, err
	}

	if spawned && cmd != nil && cmd.Process != nil {
		time.Sleep(500 * time.Millisecond)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		cleanProfileDir(profileDir)
	} else {
		_ = closeTargetTab(port, target.ID)
	}

	return credentials, nil
}
