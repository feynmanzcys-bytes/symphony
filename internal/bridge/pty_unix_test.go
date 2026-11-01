//go:build darwin || linux

package bridge

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestPTYKillsTheGroup: killing a session's process kills what it started too, even what ignores the hangup.
func TestPTYKillsTheGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	proc, err := startPTY(t.TempDir(), childEnv(nil),
		[]string{"/bin/sh", "-c", `(trap "" HUP; exec sleep 1000) & echo $! > ` + shellQuote(pidFile) + `; exec sleep 1000`}, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	go func() { // keep the output flowing
		buf := make([]byte, 1024)
		for {
			if _, err := proc.Read(buf); err != nil {
				return
			}
		}
	}()
	var pid int
	waitFor(t, "the grandchild to start", func() bool {
		data, err := os.ReadFile(pidFile)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		return err == nil && pid > 0
	})
	if err := proc.Kill(); err != nil {
		t.Fatal(err)
	}
	waited := make(chan struct{})
	go func() { _ = proc.Wait(); close(waited) }()
	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("the process should be gone")
	}
	_ = proc.Close()
	waitFor(t, "the grandchild to be killed", func() bool {
		return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
	})
}

// TestDescendants follows the parents down from a session, through what moved to sessions of its own, and only there.
func TestDescendants(t *testing.T) {
	table := []procInfo{
		{pid: 1, ppid: 0, pgid: 1},
		{pid: 10, ppid: 1, pgid: 10},  // claude
		{pid: 11, ppid: 10, pgid: 10}, // in its group
		{pid: 20, ppid: 10, pgid: 20}, // a tool in a session of its own
		{pid: 21, ppid: 20, pgid: 20}, // what the tool started
		{pid: 22, ppid: 21, pgid: 22}, // and that one
		{pid: 30, ppid: 1, pgid: 30},  // someone else's
		{pid: 31, ppid: 30, pgid: 30}, // and its child
		{pid: 40, ppid: 40, pgid: 40}, // a loop never loops
		{pid: 41, ppid: 22, pgid: 41}, // deep down
	}
	var got []int
	for _, p := range descendants(table, 10) {
		got = append(got, p.pid)
	}
	slices.Sort(got)
	if want := []int{11, 20, 21, 22, 41}; !slices.Equal(got, want) {
		t.Fatalf("descendants of 10: %v, want %v", got, want)
	}
	if d := descendants(table, 40); len(d) != 0 {
		t.Fatalf("a process isn't its own descendant: %v", d)
	}
}

// TestListProcs: this test's own process is among the machine's processes, with its parent and group; on Linux ps
// (what macOS uses) agrees with /proc.
func TestListProcs(t *testing.T) {
	find := func(procs []procInfo) (procInfo, bool) {
		i := slices.IndexFunc(procs, func(p procInfo) bool { return p.pid == os.Getpid() })
		if i < 0 {
			return procInfo{}, false
		}
		return procs[i], true
	}
	me, ok := find(listProcs())
	if !ok || me.ppid != os.Getppid() || me.pgid != syscall.Getpgrp() || me.start == "" {
		t.Fatalf("this process in listProcs: %+v (found %v)", me, ok)
	}
	ps, err := exec.LookPath("ps")
	if err != nil {
		t.Skip("no ps here")
	}
	out, err := exec.Command(ps, "-A", "-o", "pid=", "-o", "ppid=", "-o", "pgid=", "-o", "lstart=").Output()
	if err != nil {
		t.Fatal(err)
	}
	them, ok := find(parsePS(string(out)))
	if !ok || them.ppid != me.ppid || them.pgid != me.pgid || len(strings.Fields(them.start)) < 4 {
		t.Fatalf("this process in ps: %+v (found %v), /proc: %+v", them, ok, me)
	}
}
