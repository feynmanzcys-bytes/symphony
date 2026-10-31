//go:build linux

package bridge

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"
)

// openPTY opens a new pseudo-terminal pair: the master, and the slave's path (/dev/pts/N).
func openPTY() (*os.File, string, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, "", err
	}
	fd := master.Fd()
	unlock := int32(0)
	if err := ioctl(fd, syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		master.Close()
		return nil, "", err
	}
	var n uint32
	if err := ioctl(fd, syscall.TIOCGPTN, unsafe.Pointer(&n)); err != nil {
		master.Close()
		return nil, "", err
	}
	return master, "/dev/pts/" + strconv.Itoa(int(n)), nil
}
