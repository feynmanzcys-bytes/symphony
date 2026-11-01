//go:build darwin || linux

package bridge

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
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

	mu     sync.Mutex
	asked  bool       // Hangup has been sent
	family []procInfo // what the process had started when it was asked to end
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

// Hangup asks the session to end, as closing its terminal would: a hangup to its process group (the child leads one,
// Setsid) and to the groups of what it started. What it started is noted first, for Kill: Claude Code runs its tools in
// sessions of their own, which a signal to its group never reaches, and once claude is gone they no longer descend
// from it.
func (p *unixProc) Hangup() error {
	if p.cmd.Process == nil {
		return errors.New("not started")
	}
	pid := p.cmd.Process.Pid
	family := descendants(listProcs(), pid)
	p.mu.Lock()
	p.asked, p.family = true, family
	p.mu.Unlock()
	if err := syscall.Kill(-pid, syscall.SIGHUP); err != nil {
		return err
	}
	signalGroups(family, pid, syscall.SIGHUP)
	return nil
}

// Kill ends the session for good: its process group, then what it had started (noted by Hangup, else now) that still
// runs, what those started since, and their process groups.
func (p *unixProc) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	pid := p.cmd.Process.Pid
	p.mu.Lock()
	asked, family := p.asked, p.family
	p.mu.Unlock()
	if !asked {
		family = descendants(listProcs(), pid)
	}
	err := syscall.Kill(-pid, syscall.SIGKILL)
	killLeftovers(family, pid)
	if err != nil {
		return p.cmd.Process.Kill()
	}
	return nil
}

// procInfo is a process of this machine: its id, its parent's, its process group's, and when it started (in the
// words of listProcs; it tells a process from a later one with the same id).
type procInfo struct {
	pid, ppid, pgid int
	start           string
}

// descendants are the processes below roots in table, by parent; the roots themselves aren't.
func descendants(table []procInfo, roots ...int) []procInfo {
	children := map[int][]procInfo{}
	for _, p := range table {
		children[p.ppid] = append(children[p.ppid], p)
	}
	seen := map[int]bool{}
	for _, r := range roots {
		seen[r] = true
	}
	var out []procInfo
	for queue := roots; len(queue) > 0; queue = queue[1:] {
		for _, c := range children[queue[0]] {
			if c.pid > 1 && !seen[c.pid] {
				seen[c.pid] = true
				out = append(out, c)
				queue = append(queue, c.pid)
			}
		}
	}
	return out
}

// signalGroups sends sig to the process groups of procs, except the session's own (pid) and the bridge's.
func signalGroups(procs []procInfo, pid int, sig syscall.Signal) {
	done := map[int]bool{pid: true, syscall.Getpgrp(): true}
	for _, p := range procs {
		if p.pgid > 1 && !done[p.pgid] {
			done[p.pgid] = true
			_ = syscall.Kill(-p.pgid, sig)
		}
	}
}

// killLeftovers kills what of family still runs (the same id and start time: an id used again since is left alone),
// what those have started since, and their process groups. Every member of those groups descends from the session:
// a group only takes processes of its own session, and these sessions were made by the session's processes.
func killLeftovers(family []procInfo, pid int) {
	if len(family) == 0 {
		return
	}
	now := listProcs()
	byPID := make(map[int]procInfo, len(now))
	for _, p := range now {
		byPID[p.pid] = p
	}
	var left []procInfo
	var roots []int
	for _, f := range family {
		if p, ok := byPID[f.pid]; ok && p.start == f.start {
			left = append(left, p)
			roots = append(roots, p.pid)
		}
	}
	left = append(left, descendants(now, roots...)...)
	self := os.Getpid()
	for _, p := range left {
		if p.pid > 1 && p.pid != self {
			_ = syscall.Kill(p.pid, syscall.SIGKILL)
		}
	}
	signalGroups(left, pid, syscall.SIGKILL)
}

// parsePS reads the output of ps -A -o pid= -o ppid= -o pgid= -o lstart=: three numbers, then the start time in words.
func parsePS(out string) []procInfo {
	var procs []procInfo
	for line := range strings.Lines(out) {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		pgid, err3 := strconv.Atoi(f[2])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		procs = append(procs, procInfo{pid: pid, ppid: ppid, pgid: pgid, start: strings.Join(f[3:], " ")})
	}
	return procs
}
