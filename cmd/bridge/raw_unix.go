//go:build darwin || linux

package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
	"unsafe"
)

type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// consoleSize is this terminal's size in characters (80 x 24 when it can't be read).
func consoleSize() (cols, rows int) {
	var ws winsize
	if err := ioctl(os.Stdout.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&ws)); err != nil || ws.Col == 0 {
		return 80, 24
	}
	return int(ws.Col), int(ws.Row)
}

// watchSize calls fn with the new size whenever the window is resized.
func watchSize(fn func(cols, rows int)) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)
	for range ch {
		fn(consoleSize())
	}
}

// rawMode puts this terminal in raw mode (every key goes through as is, nothing is echoed or post-processed:
// the session's own pty does that) and returns what restores it.
func rawMode() (func(), error) {
	fd := os.Stdin.Fd()
	var old syscall.Termios
	if err := ioctl(fd, termiosGet, unsafe.Pointer(&old)); err != nil {
		return nil, err
	}
	raw := old
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cflag |= syscall.CS8
	raw.Cc[syscall.VMIN], raw.Cc[syscall.VTIME] = 1, 0
	if err := ioctl(fd, termiosSet, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() {
		_ = ioctl(fd, termiosSet, unsafe.Pointer(&old))
		time.Sleep(10 * time.Millisecond)
	}, nil
}
