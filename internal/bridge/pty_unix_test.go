//go:build darwin || linux

package bridge

import (
	"errors"
	"os"
	"path/filepath"
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
