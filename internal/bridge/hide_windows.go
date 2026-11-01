//go:build windows

package bridge

import (
	"os/exec"
	"syscall"
)

// hideConsole runs a console program (git, claude plugin list) without a console window of its own: a bridge started
// with -background has no console to lend it, and Windows would open one, every time.
func hideConsole(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
