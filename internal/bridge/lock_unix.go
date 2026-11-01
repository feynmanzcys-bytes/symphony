//go:build !windows

package bridge

import (
	"errors"
	"os"
	"syscall"
)

// lockPath opens path and takes an exclusive lock on it (flock), which goes with the process: a bridge that crashed
// holds none. Go opens files close-on-exec, so the sessions the bridge starts don't inherit it.
func lockPath(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrDataLocked
		}
		return nil, &os.PathError{Op: "lock", Path: path, Err: err}
	}
	return f, nil
}
