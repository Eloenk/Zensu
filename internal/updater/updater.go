package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"zensu/internal/logger"
)

type ReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type GitHubRelease struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	PublishedAt string         `json:"published_at"`
	Assets      []ReleaseAsset `json:"assets"`
}

type UpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseNotes   string `json:"releaseNotes"`
	DownloadURL    string `json:"downloadUrl"`
}

func CheckUpdate(currentVersion string) (*UpdateInfo, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/Eloenk/Zensu/releases/latest", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create release request: %w", err)
	}
	req.Header.Set("User-Agent", "Zensu-App-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger.Errorf("UPDATER_ERR", "Failed to query GitHub Releases API: %v", err)
		return nil, fmt.Errorf("network error querying updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub release API returned status %d", resp.StatusCode)
	}

	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("failed to decode release payload: %w", err)
	}

	latestVersion := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	cleanCurrent := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")

	isNewer := isVersionNewer(latestVersion, cleanCurrent)

	downloadURL := ""
	for _, asset := range rel.Assets {
		if strings.HasSuffix(asset.Name, ".exe") || strings.EqualFold(asset.Name, "zensu.exe") {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	return &UpdateInfo{
		Available:      isNewer && downloadURL != "",
		CurrentVersion: cleanCurrent,
		LatestVersion:  latestVersion,
		ReleaseNotes:   rel.Body,
		DownloadURL:    downloadURL,
	}, nil
}

func isVersionNewer(latest, current string) bool {
	lParts := parseSemverParts(latest)
	cParts := parseSemverParts(current)

	for i := 0; i < len(lParts) || i < len(cParts); i++ {
		l := 0
		c := 0
		if i < len(lParts) {
			l = lParts[i]
		}
		if i < len(cParts) {
			c = cParts[i]
		}
		if l > c {
			return true
		}
		if l < c {
			return false
		}
	}
	return false
}

func parseSemverParts(v string) []int {
	parts := strings.Split(v, ".")
	res := make([]int, len(parts))
	for i, p := range parts {
		num, _ := strconv.Atoi(p)
		res[i] = num
	}
	return res
}

func DownloadAndApplyUpdate(downloadURL string) error {
	if downloadURL == "" {
		return fmt.Errorf("download URL is empty")
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	tmpDir := os.TempDir()
	newBinaryPath := filepath.Join(tmpDir, "zensu_update.exe")

	out, err := os.Create(newBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		out.Close()
		return fmt.Errorf("failed to download update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		out.Close()
		return fmt.Errorf("download server returned HTTP %d", resp.StatusCode)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		return fmt.Errorf("failed to write update binary: %w", err)
	}
	out.Close()

	if runtime.GOOS == "windows" {
		batPath := filepath.Join(tmpDir, "zensu_updater.bat")
		batContent := fmt.Sprintf(`@echo off
timeout /t 2 /nobreak > nul
move /y "%s" "%s"
start "" "%s"
del "%%~f0"
`, newBinaryPath, execPath, execPath)

		if err := os.WriteFile(batPath, []byte(batContent), 0755); err != nil {
			return fmt.Errorf("failed to write updater script: %w", err)
		}

		cmd := exec.Command("cmd.exe", "/C", batPath)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to launch updater script: %w", err)
		}

		os.Exit(0)
	} else {
		if err := os.Rename(newBinaryPath, execPath); err != nil {
			return fmt.Errorf("failed to replace binary: %w", err)
		}
		cmd := exec.Command(execPath)
		_ = cmd.Start()
		os.Exit(0)
	}

	return nil
}

func CleanupOldBinaries() {
	tmpDir := os.TempDir()
	_ = os.Remove(filepath.Join(tmpDir, "zensu_update.exe"))
	_ = os.Remove(filepath.Join(tmpDir, "zensu_updater.bat"))
}
