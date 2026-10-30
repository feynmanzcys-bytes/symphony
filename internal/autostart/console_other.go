//go:build !windows

package autostart

import "os/exec"

// newConsole only matters on Windows: elsewhere systemd or launchd start the bridge, not Start.
func newConsole(*exec.Cmd) {}
