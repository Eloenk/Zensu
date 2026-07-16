//go:build !windows && !darwin
package browser

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on Unix/Linux
}

func getDefaultBrowserPath() (string, error) {
	cmd := exec.Command("xdg-settings", "get", "default-web-browser")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	desktopFile := strings.TrimSpace(stdout.String())
	if desktopFile == "" {
		return "", os.ErrNotExist
	}

	searchPaths := []string{
		filepath.Join(os.Getenv("HOME"), ".local/share/applications"),
		"/usr/share/applications",
		"/usr/local/share/applications",
	}

	var foundPath string
	for _, dir := range searchPaths {
		path := filepath.Join(dir, desktopFile)
		if _, err := os.Stat(path); err == nil {
			foundPath = path
			break
		}
	}

	if foundPath == "" {
		binName := strings.TrimSuffix(desktopFile, ".desktop")
		if p, err := exec.LookPath(binName); err == nil {
			return p, nil
		}
		return "", os.ErrNotExist
	}

	file, err := os.Open(foundPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Exec=") {
			execCmd := strings.TrimPrefix(line, "Exec=")
			fields := strings.Fields(execCmd)
			if len(fields) > 0 {
				binary := fields[0]
				binary = strings.Trim(binary, `"'`)
				if filepath.IsAbs(binary) {
					return binary, nil
				}
				if p, err := exec.LookPath(binary); err == nil {
					return p, nil
				}
				return binary, nil
			}
		}
	}

	return "", os.ErrNotExist
}
