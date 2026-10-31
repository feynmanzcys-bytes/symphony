//go:build darwin || linux

package bridge

import (
	"os"
	"os/exec"
	"syscall"
	"unsafe"
)

// winsize is struct winsize of <sys/ioctl.h>.
type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

func ioctl(fd uintptr, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// setWinsize tells the pty its size (the child gets SIGWINCH).
func setWinsize(f *os.File, cols, rows int) error {
	ws := winsize{Row: uint16(rows), Col: uint16(cols)}
	return ioctl(f.Fd(), syscall.TIOCSWINSZ, unsafe.Pointer(&ws))
}

// unixProc is a child process on a pseudo-terminal: master is both its output and its input.
type unixProc struct {
	cmd    *exec.Cmd
	master *os.File
}

// startPTY starts argv in dir on a new pseudo-terminal of cols x rows, with env as its whole environment.
func startPTY(dir string, env []string, argv []string, cols, rows int) (ptyProc, error) {
	master, slaveName, err := openPTY()
	if err != nil {
		return nil, err
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, err
	}
	defer slave.Close() // the child has its own copy once started
	_ = setWinsize(master, cols, rows)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// a session of its own with the pty as its controlling terminal (fd 0 in the child), so Ctrl-C and job control work
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		master.Close()
		return nil, err
	}
	return &unixProc{cmd: cmd, master: master}, nil
}

func (p *unixProc) Read(b []byte) (int, error)  { return p.master.Read(b) }
func (p *unixProc) Write(b []byte) (int, error) { return p.master.Write(b) }
func (p *unixProc) Resize(cols, rows int) error { return setWinsize(p.master, cols, rows) }
func (p *unixProc) Wait() error                 { return p.cmd.Wait() }
func (p *unixProc) Close() error                { return p.master.Close() }
func (p *unixProc) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	// the whole session (claude and whatever it started), as closing a terminal would
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGHUP)
	return p.cmd.Process.Kill()
}
