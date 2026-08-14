//go:build windows
// +build windows

package export

import (
	"os/exec"
	"syscall"
)

func SetHideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000 | 0x00004000,
	}
}
