//go:build windows

package dl

import "syscall"

func setHideWindow(attr *syscall.SysProcAttr) {
	attr.CreationFlags = 0x08000000 // CREATE_NO_WINDOW
}
