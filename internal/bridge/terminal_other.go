//go:build !windows

package bridge

import "os/exec"

// newConsole 只在 Windows 上有意义；别的平台用 tmux 或图形终端，不会走到 console 后端。
func newConsole(*exec.Cmd) {}
