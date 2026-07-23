//go:build !windows
package main

import "os/exec"

func HideConsole(cmd *exec.Cmd) {}
