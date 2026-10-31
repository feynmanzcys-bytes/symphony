//go:build !darwin && !linux && !windows

package bridge

import "errors"

// startPTY: no pseudo-terminal support on this platform; starting sessions from the page is off.
func startPTY(dir string, env []string, argv []string, cols, rows int) (ptyProc, error) {
	return nil, errors.New("pseudo-terminals are not supported on this platform")
}
