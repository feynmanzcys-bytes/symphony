//go:build windows

package autostart

import (
	"os/exec"
	"syscall"
)

// newConsole gives the bridge a console window of its own, like the one it gets at login; closing it stops the bridge.
func newConsole(cmd *exec.Cmd) {
	const createNewConsole = 0x00000010
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
}
