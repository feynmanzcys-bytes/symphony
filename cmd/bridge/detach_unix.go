//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach runs the bridge in a session of its own, so closing the terminal or ending the Claude Code session that
// started it leaves it running.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
