package notify

import (
	"fmt"
	"os/exec"
	"runtime"
	"syscall"

	"zensu/internal/logger"
)

// SendToast displays a native Windows desktop notification toast.
func SendToast(title, message string) {
	if runtime.GOOS != "windows" {
		logger.Infof("NOTIFY", "[%s] %s", title, message)
		return
	}

	go func() {
		// Escape single quotes for PowerShell string literal
		psTitle := escapePS(title)
		psMsg := escapePS(message)

		psScript := fmt.Sprintf(`
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
[Windows.Data.Xml.Dom.XmlDocument, Windows.Data.Xml.Dom.XmlDocument, ContentType = WindowsRuntime] | Out-Null

$template = @"
<toast>
    <visual>
        <binding template="ToastGeneric">
            <text>%s</text>
            <text>%s</text>
        </binding>
    </visual>
</toast>
"@

$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($template)
$toast = [Windows.UI.Notifications.ToastNotification]::new($xml)
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("Zensu Anime Downloader").Show($toast)
`, psTitle, psMsg)

		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", psScript)
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}
		if err := cmd.Run(); err != nil {
			logger.Warnf("NOTIFY_ERR", "Failed to show Windows toast: %v", err)
		} else {
			logger.Infof("NOTIFY_SENT", "Notification sent: %s - %s", title, message)
		}
	}()
}

func escapePS(s string) string {
	s = fmt.Sprintf("%v", s)
	var out []rune
	for _, r := range s {
		if r == '\'' || r == '"' || r == '`' || r == '$' {
			out = append(out, ' ')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}
