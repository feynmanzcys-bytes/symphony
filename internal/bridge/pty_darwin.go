//go:build darwin

package bridge

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"time"
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

// listProcs is every process of this machine, as ps tells it; nil when ps fails.
func listProcs() []procInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// one -o each: ps takes the rest of an -o after "=" as the column's header
	cmd := exec.CommandContext(ctx, "/bin/ps", "-A", "-o", "pid=", "-o", "ppid=", "-o", "pgid=", "-o", "lstart=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parsePS(string(out))
}
