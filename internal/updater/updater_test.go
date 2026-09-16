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
