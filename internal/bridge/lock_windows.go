package bridge

import (
	"errors"
	"os"
	"syscall"
)

// errSharingViolation is ERROR_SHARING_VIOLATION: another process has the file open without sharing it.
const errSharingViolation syscall.Errno = 32

// lockPath opens path without sharing it with anyone, which is the lock: it goes with the process, so a bridge that
// crashed holds none. The handle isn't inheritable, so the sessions the bridge starts don't get it.
func lockPath(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if errors.Is(err, errSharingViolation) {
			return nil, ErrDataLocked
		}
		return nil, &os.PathError{Op: "lock", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
