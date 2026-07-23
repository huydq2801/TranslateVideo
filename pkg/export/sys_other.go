//go:build !windows
// +build !windows

package export

import "os/exec"

func SetHideWindow(cmd *exec.Cmd) {}
