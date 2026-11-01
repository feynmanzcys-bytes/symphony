//go:build !windows

package bridge

import "os/exec"

// hideConsole only matters on Windows, where a console program would open a window of its own.
func hideConsole(*exec.Cmd) {}
