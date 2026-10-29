//go:build windows

package bridge

import (
	"os/exec"
	"syscall"
)

// newConsole 让 claude 在自己的新控制台窗口里跑（Windows 11 默认终端是 Windows Terminal 时会开在那里）。
func newConsole(cmd *exec.Cmd) {
	const createNewConsole = 0x00000010
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
}
