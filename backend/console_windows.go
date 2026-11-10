//go:build windows

package backend

import (
	"os/exec"
	"syscall"
)

// hideConsole keeps a tool from opening a console window.
func hideConsole(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
