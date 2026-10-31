//go:build windows

package bridge

import (
	"os/exec"
	"syscall"
)

// newConsole runs the command in a new console window of its own (in Windows Terminal when that is the default
// terminal on Windows 11).
func newConsole(cmd *exec.Cmd) {
	const createNewConsole = 0x00000010
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
}
