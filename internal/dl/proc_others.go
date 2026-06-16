//go:build !windows

package dl

import "syscall"

func setHideWindow(attr *syscall.SysProcAttr) {
	// No-op on non-Windows platforms
}
