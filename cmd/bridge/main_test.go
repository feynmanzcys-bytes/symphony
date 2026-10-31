package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// built is the bridge built once for these tests, the way .github/release.sh builds it.
var built struct {
	once     sync.Once
	dir, exe string
	out      []byte
	err      error
}

func TestMain(m *testing.M) {
	code := m.Run()
	if built.dir != "" {
		os.RemoveAll(built.dir)
	}
	os.Exit(code)
}

func buildBridge(t *testing.T) string {
	t.Helper()
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("needs the go command")
	}
	built.once.Do(func() {
		if built.dir, built.err = os.MkdirTemp("", "symphony-test"); built.err != nil {
			return
		}
		built.exe = filepath.Join(built.dir, "symphony")
		if runtime.GOOS == "windows" {
			built.exe += ".exe"
		}
		built.out, built.err = exec.Command(gobin, "build", "-trimpath", "-ldflags", "-s -w -X main.version=v9.8.7", "-o", built.exe, ".").CombinedOutput()
	})
	if built.err != nil {
		t.Fatalf("go build: %v\n%s", built.err, built.out)
	}
	return built.exe
}

// TestVersion checks that -version prints the version set with -ldflags, so a renamed variable can't quietly turn
// releases into "dev".
func TestVersion(t *testing.T) {
	out, err := exec.Command(buildBridge(t), "-version").Output()
	if err != nil {
		t.Fatalf("-version: %v", err)
	}
	if want := "symphony v9.8.7 (" + runtime.GOOS + "/" + runtime.GOARCH + ")\n"; string(out) != want {
		t.Errorf("-version printed %q, want %q", out, want)
	}
}

// TestOpen runs the bridge with a stand-in browser that notes what it was asked to open: the office opens once the
// bridge is up; started again at the same address, it opens the running bridge's office and exits; with -open=false
// nothing opens.
func TestOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in browser is a shell script")
	}
	exe := buildBridge(t)
	dir := t.TempDir()
	opened := filepath.Join(dir, "opened")
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$@\" >> '" + strings.ReplaceAll(opened, "'", `'\''`) + "'\n"
	for _, name := range []string{"xdg-open", "open"} { // Linux, macOS
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	addr := freeAddr(t)
	office := "http://" + addr + "/"
	cmd := func(ctx context.Context, args ...string) *exec.Cmd {
		// no claude here, so the bridge doesn't look for the mod
		c := exec.CommandContext(ctx, exe, append([]string{"-addr", addr, "-data", filepath.Join(dir, "data"), "-claude", filepath.Join(dir, "no-claude")}, args...)...)
		c.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "DISPLAY=:0", "SYMPHONY_LANG=en")
		return c
	}
	asked := func() []string {
		data, _ := os.ReadFile(opened)
		return strings.Fields(string(data))
	}

	first := cmd(context.Background())
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Process.Kill(); first.Wait() })
	waitFor(t, 10*time.Second, "the office opens once the bridge is up", func() bool { return slices.Equal(asked(), []string{office}) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := cmd(ctx).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "already running at "+office) {
		t.Fatalf("started again, the bridge should say it's already running and exit: %v\n%s", err, out)
	}
	waitFor(t, 10*time.Second, "started again, it opens the running bridge's office", func() bool { return slices.Equal(asked(), []string{office, office}) })

	if out, err := cmd(ctx, "-open=false").CombinedOutput(); err != nil || !strings.Contains(string(out), "already running") {
		t.Fatalf("with -open=false it should still find the running bridge and exit: %v\n%s", err, out)
	}
	time.Sleep(time.Second)
	if got := asked(); len(got) != 2 {
		t.Errorf("-open=false opened something: %q", got)
	}
	if !bridgeAt("http://" + addr) {
		t.Error("the first bridge should still be running")
	}
}

// TestBackground starts the bridge with -background, as the mod does: the command returns once the bridge answers,
// even with a pipe on its output (the bridge writes to bridge.log instead), and the bridge keeps running after it;
// a second -background finds it and starts nothing.
func TestBackground(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stops the bridge with pkill")
	}
	exe := buildBridge(t)
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	t.Cleanup(func() { exec.Command("pkill", "-f", "--", "-data="+data).Run() })
	addr := freeAddr(t)
	office := "http://" + addr + "/"
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	run := func() string {
		cmd := exec.CommandContext(ctx, exe, "-background", "-open=false", "-addr", addr, "-data="+data, "-claude", filepath.Join(dir, "no-claude"))
		cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("-background: %v\n%s", err, out)
		}
		return string(out)
	}

	if out := run(); !strings.Contains(out, "runs in the background at "+office) {
		t.Fatalf("-background should say where the bridge runs:\n%s", out)
	}
	if !bridgeAt("http://" + addr) {
		t.Fatal("the bridge should keep running after -background returns")
	}
	if log, err := os.ReadFile(filepath.Join(data, "bridge.log")); err != nil || !strings.Contains(string(log), "symphony bridge v9.8.7: http://"+addr) {
		t.Errorf("bridge.log should have the bridge's own log: %v\n%s", err, log)
	}
	if out := run(); !strings.Contains(out, "already running at "+office) {
		t.Fatalf("a second -background should find the running bridge:\n%s", out)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("still waiting after %s: %s", timeout, what)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func TestAutostartArgs(t *testing.T) {
	wd, _ := os.Getwd()
	got := autostartArgs(map[string]string{"addr": "127.0.0.1:7790", "web": "web", "claude": "claude", "lang": "ja", "open": "false"}, "data")
	want := []string{"-data=" + filepath.Join(wd, "data"), "-addr=127.0.0.1:7790", "-web=" + filepath.Join(wd, "web"), "-claude=claude", "-lang=ja"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
	got = autostartArgs(map[string]string{"claude": "./bin/claude", "mod": "/mods/x"}, "/data")
	if want := []string{"-data=/data", "-mod=/mods/x", "-claude=" + filepath.Join(wd, "bin", "claude")}; !slices.Equal(got, want) {
		t.Errorf("a claude with a folder should become absolute: got %q, want %q", got, want)
	}
}

// TestAutostartRefuses checks the two mistakes -autostart turns down before touching anything: a go run build, which
// is deleted afterwards, and a mode other than on or off.
func TestAutostartRefuses(t *testing.T) {
	gobin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("needs the go command")
	}
	// were anything written anyway, it would land here, not in the real config; go keeps its own caches
	home := t.TempDir()
	env := append(os.Environ(), "SYMPHONY_LANG=en", "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
	caches, err := exec.Command(gobin, "env", "GOCACHE", "GOMODCACHE", "GOPATH").Output()
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		env = append(env, name+"="+strings.Split(strings.TrimSpace(string(caches)), "\n")[i])
	}
	for _, c := range []struct {
		cmd  *exec.Cmd
		want string
	}{
		{exec.Command(gobin, "run", ".", "-autostart", "on"), "-autostart needs the symphony program itself, not go run"},
		{exec.Command(buildBridge(t), "-autostart", "maybe"), "-autostart takes on or off, not maybe"},
	} {
		c.cmd.Env = env
		out, err := c.cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), c.want) {
			t.Errorf("%q: want a failure saying %q, got %v:\n%s", c.cmd.Args, c.want, err, out)
		}
	}
	// where the entry would go: the go command keeps its telemetry in the config folder too (~/.config/go on Linux,
	// ~/Library/Application Support/go on macOS), so not the whole of it
	for _, dir := range []string{filepath.Join(home, ".config", "systemd"), filepath.Join(home, "Library", "LaunchAgents")} {
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("a refused -autostart wrote %s", dir)
		}
	}
}
