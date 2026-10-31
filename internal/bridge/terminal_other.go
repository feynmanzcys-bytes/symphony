//go:build !windows

package bridge

import "os/exec"

// newConsole only matters on Windows; other platforms open a graphical terminal that runs the command.
func newConsole(*exec.Cmd) {}
