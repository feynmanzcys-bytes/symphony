//go:build linux

package bridge

import (
	"bytes"
	"os"
	"strconv"
	"strings"
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

// listProcs is every process of this machine, read from /proc; nil when it can't be read.
func listProcs() []procInfo {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var procs []procInfo
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		if data, err := os.ReadFile("/proc/" + e.Name() + "/stat"); err == nil { // gone meanwhile, else
			if p, ok := parseStat(string(data)); ok {
				procs = append(procs, p)
			}
		}
	}
	return procs
}

// parseStat reads /proc/<pid>/stat: "pid (name) state ppid pgrp ...", the start time 22nd; the name may hold spaces
// and parentheses, so the fields are counted from its last ")".
func parseStat(stat string) (procInfo, bool) {
	open, end := strings.IndexByte(stat, '('), strings.LastIndexByte(stat, ')')
	if open < 0 || end < open {
		return procInfo{}, false
	}
	f := strings.Fields(stat[end+1:]) // from the 3rd field on
	if len(f) < 20 {
		return procInfo{}, false
	}
	pid, err1 := strconv.Atoi(strings.TrimSpace(stat[:open]))
	ppid, err2 := strconv.Atoi(f[1])
	pgid, err3 := strconv.Atoi(f[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return procInfo{}, false
	}
	return procInfo{pid: pid, ppid: ppid, pgid: pgid, start: f[19]}, true
}

// procsWithEnv is every process whose environment holds key=value: /proc/<pid>/environ, readable for this user's
// processes (a session's are), NUL-separated as it was at exec.
func procsWithEnv(key, value string) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	want := []byte(key + "=" + value)
	var pids []int
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/environ")
		if err != nil {
			continue
		}
		for _, kv := range bytes.Split(data, []byte{0}) {
			if bytes.Equal(kv, want) {
				pids = append(pids, pid)
				break
			}
		}
	}
	return pids
}
