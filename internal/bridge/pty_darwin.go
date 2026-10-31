//go:build darwin

package bridge

import (
	"os"
	"syscall"
	"unsafe"
)

// openPTY opens a new pseudo-terminal pair: the master, and the slave's path.
func openPTY() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	fd := master.Fd()
	if err := ioctl(fd, syscall.TIOCPTYGRANT, nil); err != nil {
		master.Close()
		return nil, "", err
	}
	if err := ioctl(fd, syscall.TIOCPTYUNLK, nil); err != nil {
		master.Close()
		return nil, "", err
	}
	var name [128]byte // TIOCPTYGNAME fills in the slave's path
	if err := ioctl(fd, syscall.TIOCPTYGNAME, unsafe.Pointer(&name[0])); err != nil {
		master.Close()
		return nil, "", err
	}
	n := 0
	for n < len(name) && name[n] != 0 {
		n++
	}
	return master, string(name[:n]), nil
}
