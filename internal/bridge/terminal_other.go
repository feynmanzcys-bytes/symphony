//go:build !windows

package bridge

import "os/exec"

// newConsole only matters on Windows; other platforms use tmux or a graphical terminal and never reach the console backend.
func newConsole(*exec.Cmd) {}
