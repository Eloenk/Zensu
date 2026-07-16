//go:build windows
package browser

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
}

func getDefaultBrowserPath() (string, error) {
	// Query user choice for HTTP scheme default ProgID
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close()

	progID, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return "", err
	}

	// Read the open command associated with this ProgID
	cmdKeyPath := progID + `\shell\open\command`
	kCmd, err := registry.OpenKey(registry.CLASSES_ROOT, cmdKeyPath, registry.QUERY_VALUE)
	if err != nil {
		// Fallback to local machine check if classes root fails
		kCmd, err = registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Classes\`+cmdKeyPath, registry.QUERY_VALUE)
		if err != nil {
			return "", err
		}
	}
	defer kCmd.Close()

	cmdStr, _, err := kCmd.GetStringValue("")
	if err != nil {
		return "", err
	}

	// The command is typically: "C:\Program Files\Google\Chrome\Application\chrome.exe" -- "%1"
	var path string
	parts := strings.Split(cmdStr, `"`)
	if len(parts) >= 2 {
		path = parts[1]
	} else {
		fields := strings.Fields(cmdStr)
		if len(fields) > 0 {
			path = fields[0]
		}
	}

	return filepath.Clean(path), nil
}
