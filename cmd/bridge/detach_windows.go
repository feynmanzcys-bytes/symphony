//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach runs the bridge without a console window, apart from the console or session that started it.
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP, HideWindow: true}
}
