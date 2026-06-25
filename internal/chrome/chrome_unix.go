//go:build !windows
package chrome

import (
	"os/exec"
)

func setSysProcAttr(cmd *exec.Cmd) {
	// No-op on non-Windows
}
