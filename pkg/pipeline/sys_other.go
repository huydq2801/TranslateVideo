//go:build !windows
// +build !windows

package pipeline

import "os/exec"

func SetHideWindow(cmd *exec.Cmd) {}
