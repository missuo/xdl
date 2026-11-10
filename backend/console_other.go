//go:build !windows

package backend

import "os/exec"

// hideConsole does nothing outside Windows, where tools open no window.
func hideConsole(*exec.Cmd) {}
