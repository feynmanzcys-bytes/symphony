//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
)

// TestHangup: closing the terminal a bridge runs in (a hangup) stops it cleanly, as SIGTERM does: it exits 0. Started
// under nohup, it keeps running.
func TestHangup(t *testing.T) {
	exe := buildBridge(t)
	dir := t.TempDir()
	start := func(name string, args ...string) (*exec.Cmd, chan error, string) {
		data, addr := filepath.Join(dir, name), freeAddr(t)
		cmd := exec.Command(args[0], append(args[1:], exe, "-addr", addr, "-data", data, "-open=false", "-claude", filepath.Join(dir, "no-claude"))...)
		cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		t.Cleanup(func() { cmd.Process.Kill() })
		waitFor(t, 10*time.Second, "the bridge is up", func() bool {
			key, err := bridge.ReadKey(data)
			return err == nil && bridgeAt("http://"+addr, key) == ours
		})
		return cmd, exited, addr
	}

	cmd, exited, _ := start("a", "env")
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("after a hangup the bridge should stop cleanly, exit 0: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the bridge is still running after a hangup")
	}

	nohup, err := exec.LookPath("nohup")
	if err != nil {
		return
	}
	cmd, exited, _ = start("b", nohup)
	_ = cmd.Process.Signal(syscall.SIGHUP)
	select {
	case err := <-exited:
		t.Fatalf("under nohup a hangup should leave the bridge running: %v", err)
	case <-time.After(time.Second):
	}
}

// TestBackgroundLeavesTheSession: the copy -background starts runs in the data folder (a relative -data still finds it),
// not in the folder it was started from, and without the session variables of the session that started it.
func TestBackgroundLeavesTheSession(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("reads the bridge's folder and environment from /proc, or from ps and lsof on macOS")
	}
	exe := buildBridge(t)
	project := t.TempDir()
	data := filepath.Join(project, "data")
	t.Cleanup(func() { exec.Command("pkill", "-f", "--", project).Run() })
	addr := freeAddr(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-background", "-open=false", "-addr", addr, "-data", "data", "-claude", filepath.Join(project, "no-claude"))
	cmd.Dir = project
	cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en", "SYMPHONY_TOKEN=starter", "SYMPHONY_TASK=T9", "SYMPHONY_LAUNCH=L9",
		"SYMPHONY_BRIDGE=http://"+addr, "SYMPHONY_KEEP=yes")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("-background: %v\n%s", err, out)
	}
	key, err := bridge.ReadKey(data)
	if err != nil || bridgeAt("http://"+addr, key) != ours {
		t.Fatalf("the bridge should run with the data folder given relative to where it was started: %v", err)
	}
	_, wd, env := bridgeProcess(t, exe, project)
	if real, err := filepath.EvalSymlinks(data); err == nil && wd != data && wd != real { // macOS: /var is /private/var
		t.Errorf("the bridge runs in %s, want its data folder %s", wd, data)
	} else if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SYMPHONY_TOKEN", "SYMPHONY_TASK", "SYMPHONY_LAUNCH", "SYMPHONY_BRIDGE"} {
		if slices.ContainsFunc(env, func(kv string) bool { return strings.HasPrefix(kv, name+"=") }) {
			t.Errorf("the bridge got the starting session's %s", name)
		}
	}
	if !slices.Contains(env, "SYMPHONY_KEEP=yes") {
		t.Error("the bridge should get the rest of the environment")
	}
	stop := exec.CommandContext(ctx, exe, "-stop", "-addr", addr, "-data", data)
	stop.Env = append(os.Environ(), "SYMPHONY_LANG=en")
	if out, err := stop.CombinedOutput(); err != nil {
		t.Fatalf("-stop: %v\n%s", err, out)
	}
}

// bridgeProcess finds the bridge this test started (this program, with the test's folder in its flags) and answers its
// pid, its working folder and its environment: from /proc on Linux; on macOS from ps (-E adds the environment after the
// command, a word each: enough for names, which is all the test needs) and lsof (the cwd).
func bridgeProcess(t *testing.T, exe, project string) (pid int, wd string, env []string) {
	t.Helper()
	switch runtime.GOOS {
	case "linux":
		entries, _ := os.ReadDir("/proc")
		for _, e := range entries {
			args, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
			if err == nil && strings.HasPrefix(string(args), exe+"\x00") && strings.Contains(string(args), project) {
				pid, _ = strconv.Atoi(e.Name())
			}
		}
		if pid == 0 {
			t.Fatal("can't find the bridge")
		}
		wd, _ = os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "cwd"))
		environ, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
		if err != nil {
			t.Fatal(err)
		}
		return pid, wd, strings.Split(string(environ), "\x00")
	default: // darwin
		out, err := exec.Command("ps", "-A", "-o", "pid=", "-o", "command=").Output()
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.Lines(string(out)) {
			f := strings.Fields(line)
			if len(f) > 1 && f[1] == exe && strings.Contains(line, project) {
				pid, _ = strconv.Atoi(f[0])
			}
		}
		if pid == 0 {
			t.Fatal("can't find the bridge")
		}
		out, err = exec.Command("lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn").Output()
		if err != nil {
			t.Fatalf("lsof: %v", err)
		}
		for line := range strings.Lines(string(out)) {
			if strings.HasPrefix(line, "n") {
				wd = strings.TrimSpace(line[1:])
			}
		}
		out, err = exec.Command("ps", "-E", "-o", "command=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			t.Fatal(err)
		}
		return pid, wd, strings.Fields(string(out))
	}
}
