package chrome

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func IsCDPReady(port int) bool {
	client := http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

func FindChromePath() (string, error) {
	if path := os.Getenv("CHROME_PATH"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		programFiles := os.Getenv("ProgramFiles")
		programFilesX86 := os.Getenv("ProgramFiles(x86)")
		localAppData := os.Getenv("LocalAppData")

		if programFiles != "" {
			candidates = append(candidates, filepath.Join(programFiles, "Google", "Chrome", "Application", "chrome.exe"))
		}
		if programFilesX86 != "" {
			candidates = append(candidates, filepath.Join(programFilesX86, "Google", "Chrome", "Application", "chrome.exe"))
		}
		if localAppData != "" {
			candidates = append(candidates, filepath.Join(localAppData, "Google", "Chrome", "Application", "chrome.exe"))
		}
	} else if runtime.GOOS == "darwin" {
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	} else {
		// Linux
		candidates = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("chrome executable not found")
}

func launchChrome(chromePath string, port int, profileDir string, targetURL string) (*exec.Cmd, error) {
	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return nil, err
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		fmt.Sprintf("--user-data-dir=%s", profileDir),
		"--no-first-run",
		"--no-default-browser-check",
		targetURL,
	}

	cmd := exec.Command(chromePath, args...)
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
	newTabURL := fmt.Sprintf("http://127.0.0.1:%d/json/new?%s", port, domain)
	newResp, err := client.Get(newTabURL)
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

func pollCookiesAndUA(wsURL string, domain string) (*Credentials, error) {
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

			if cfClearance != "" {
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

func FetchCredentials(domain string) (*Credentials, error) {
	port := 9222
	spawned := false
	var cmd *exec.Cmd

	if !strings.HasPrefix(domain, "http://") && !strings.HasPrefix(domain, "https://") {
		domain = "https://" + domain
	}

	if !IsCDPReady(port) {
		chromePath, err := FindChromePath()
		if err != nil {
			return nil, fmt.Errorf("could not find chrome executable: %w", err)
		}

		userConfigDir, err := os.UserConfigDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user config dir: %w", err)
		}
		profileDir := filepath.Join(userConfigDir, "zensu", "chrome-profile")

		cmd, err = launchChrome(chromePath, port, profileDir, domain)
		if err != nil {
			return nil, fmt.Errorf("failed to launch chrome: %w", err)
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
			}
			return nil, fmt.Errorf("chrome started but CDP did not become active on port %d within 15 seconds", port)
		}
	}

	target, err := getTargetTab(port, domain)
	if err != nil {
		if spawned && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil, err
	}

	credentials, err := pollCookiesAndUA(target.WebSocketDebuggerURL, domain)
	if err != nil {
		if spawned && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		} else if !spawned {
			_ = closeTargetTab(port, target.ID)
		}
		return nil, err
	}

	if spawned && cmd != nil && cmd.Process != nil {
		time.Sleep(500 * time.Millisecond)
		_ = cmd.Process.Kill()
	} else {
		_ = closeTargetTab(port, target.ID)
	}

	return credentials, nil
}
