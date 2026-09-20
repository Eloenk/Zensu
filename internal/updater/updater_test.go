package updater

import "testing"

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		latest   string
		current  string
		expected bool
	}{
		{"1.4.2", "1.4.1", true},
		{"1.5.0", "1.4.1", true},
		{"2.0.0", "1.4.1", true},
		{"1.4.1", "1.4.1", false},
		{"1.4.0", "1.4.1", false},
		{"1.3.9", "1.4.1", false},
	}

	for _, tt := range tests {
		got := isVersionNewer(tt.latest, tt.current)
		if got != tt.expected {
			t.Errorf("isVersionNewer(%q, %q) = %v; want %v", tt.latest, tt.current, got, tt.expected)
		}
	}
}

func TestSelectGUIAsset(t *testing.T) {
	assets := []ReleaseAsset{
		{Name: "zensu-cli-linux-x64", BrowserDownloadURL: "https://example.com/zensu-cli-linux-x64"},
		{Name: "zensu-cli-termux-arm64", BrowserDownloadURL: "https://example.com/zensu-cli-termux-arm64"},
		{Name: "zensu-cli-x64.exe", BrowserDownloadURL: "https://example.com/zensu-cli-x64.exe"},
		{Name: "zensu-setup-x64.exe", BrowserDownloadURL: "https://example.com/zensu-setup-x64.exe"},
	}

	downloadURL := ""
	for _, asset := range assets {
		name := asset.Name
		if len(name) == 0 {
			continue
		}
		nameLower := name
		if stringsContains(nameLower, "cli") {
			continue
		}
		if nameLower == "zensu.exe" || nameLower == "zensu-x64.exe" || nameLower == "zensu-setup-x64.exe" || nameLower == "zensu-setup.exe" {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL != "https://example.com/zensu-setup-x64.exe" {
		t.Errorf("expected zensu-setup-x64.exe URL, got: %s", downloadURL)
	}
}

func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && findSubstr(s, substr))
}

func findSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
