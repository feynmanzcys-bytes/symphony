package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
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

// standInBrowser puts a stand-in for the system's opener (xdg-open, open) in a folder for PATH; asked lists what it was
// asked to open.
func standInBrowser(t *testing.T, dir string) (bin string, asked func() []string) {
	t.Helper()
	opened := filepath.Join(dir, "opened")
	bin = filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$@\" >> '" + strings.ReplaceAll(opened, "'", `'\''`) + "'\n"
	for _, name := range []string{"xdg-open", "open"} { // Linux, macOS
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// no program is set to open .html files, so the file that opens the office stays in the data folder, never in
	// the folder of this machine's own browser if that's a snap or a flatpak (ConfinedDir)
	if err := os.WriteFile(filepath.Join(bin, "xdg-mime"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func() []string {
		data, _ := os.ReadFile(opened)
		return strings.Fields(string(data))
	}
}

// bridgeCmd runs the bridge at addr with the data folder data, a stand-in browser first on PATH (bin) and no claude,
// so it doesn't look for the mod.
func bridgeCmd(ctx context.Context, exe, addr, data, bin string, args ...string) *exec.Cmd {
	c := exec.CommandContext(ctx, exe, append([]string{"-addr", addr, "-data", data, "-claude", filepath.Join(data, "no-claude")}, args...)...)
	c.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "DISPLAY=:0", "SYMPHONY_LANG=en")
	return c
}

// TestOpen runs the bridge with a stand-in browser that notes what it was asked to open: the office opens once the
// bridge is up, signed in, through open.html in the data folder (so the key is on no command line); started again at
// the same address, it opens the running bridge's office and exits; with -open=false nothing opens.
func TestOpen(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in browser is a shell script")
	}
	exe := buildBridge(t)
	dir := t.TempDir()
	bin, asked := standInBrowser(t, dir)
	addr := freeAddr(t)
	data := filepath.Join(dir, "data")
	cmd := func(ctx context.Context, args ...string) *exec.Cmd {
		return bridgeCmd(ctx, exe, addr, data, bin, args...)
	}
	page := bridge.FileURL(runtime.GOOS, filepath.Join(data, "open.html"))

	first := cmd(context.Background())
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Process.Kill(); first.Wait() })
	waitFor(t, 10*time.Second, "the office opens once the bridge is up", func() bool { return slices.Equal(asked(), []string{page}) })
	key, err := bridge.ReadKey(data)
	if err != nil {
		t.Fatal(err)
	}
	office := "http://" + addr + "/?key=" + key
	if html, _ := os.ReadFile(filepath.Join(data, "open.html")); !strings.Contains(string(html), `content="0;url=`+office+`"`) {
		t.Fatalf("open.html should send the browser on to %s:\n%s", office, html)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := cmd(ctx).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "already running at "+office) {
		t.Fatalf("started again, the bridge should say it's already running and exit: %v\n%s", err, out)
	}
	waitFor(t, 10*time.Second, "started again, it opens the running bridge's office", func() bool { return slices.Equal(asked(), []string{page, page}) })

	if out, err := cmd(ctx, "-open=false").CombinedOutput(); err != nil || !strings.Contains(string(out), "already running") {
		t.Fatalf("with -open=false it should still find the running bridge and exit: %v\n%s", err, out)
	}
	time.Sleep(time.Second)
	if got := asked(); len(got) != 2 {
		t.Errorf("-open=false opened something: %q", got)
	}
	if bridgeAt("http://"+addr, key) != ours {
		t.Error("the first bridge should still be running")
	}
	if bridgeAt("http://"+addr, strings.Repeat("0", 64)) != another {
		t.Error("with another key, the bridge is someone else's")
	}
}

// TestAddrTaken: when something other than this data folder's bridge answers at the address (another program, or a
// bridge with another data folder), the bridge says so and exits 1 without opening anything, -background too.
func TestAddrTaken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in browser is a shell script")
	}
	exe := buildBridge(t)
	dir := t.TempDir()
	bin, asked := standInBrowser(t, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	refused := func(addr string, args ...string) {
		t.Helper()
		out, err := bridgeCmd(ctx, exe, addr, filepath.Join(dir, "mine"), bin, args...).CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(out), addr+" is taken: another program, or a symphony bridge with another data folder") {
			t.Fatalf("%v: want exit 1 saying %s is taken, got %v:\n%s", args, addr, err, out)
		}
	}

	// what every request to the squatters carried: the page key must never be among it
	var heard strings.Builder
	var mu sync.Mutex
	listen := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			fmt.Fprintf(&heard, "%s %s %v %s\n", r.Method, r.URL, r.Header, body)
			mu.Unlock()
			h(w, r)
		}
	}
	for _, h := range []http.HandlerFunc{
		func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"hello":"world"}`)) },
		// one that answers everything as a bridge would, a made-up proof included
		func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"type":"snapshot","proof":"` + strings.Repeat("ab", 32) + `"}`))
		},
	} {
		other := httptest.NewServer(listen(h))
		addr := strings.TrimPrefix(other.URL, "http://")
		refused(addr)
		refused(addr, "-background")
		other.Close()
	}
	mine, err := bridge.ReadKey(filepath.Join(dir, "mine"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(heard.String(), mine) || heard.Len() == 0 {
		t.Fatalf("the page key went to a program at the address, or nothing was asked:\n%s", heard.String())
	}

	addr := freeAddr(t)
	theirs := bridgeCmd(context.Background(), exe, addr, filepath.Join(dir, "theirs"), bin, "-open=false")
	if err := theirs.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { theirs.Process.Kill(); theirs.Wait() })
	waitFor(t, 10*time.Second, "the other bridge is up", func() bool {
		key, err := bridge.ReadKey(filepath.Join(dir, "theirs"))
		return err == nil && bridgeAt("http://"+addr, key) == ours
	})
	refused(addr)
	if got := asked(); len(got) != 0 {
		t.Errorf("nothing should open when the address is taken: %q", got)
	}
}

// TestDataInUse: one bridge per data folder. Started at another address with the same data folder, a bridge says
// where that folder's bridge runs, opens its office and exits 1 without touching the data; -background too, at once.
func TestDataInUse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in browser is a shell script")
	}
	exe := buildBridge(t)
	dir := t.TempDir()
	bin, asked := standInBrowser(t, dir)
	data := filepath.Join(dir, "data")
	addr, other := freeAddr(t), freeAddr(t)
	first := bridgeCmd(context.Background(), exe, addr, data, bin, "-open=false")
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { first.Process.Kill(); first.Wait() })
	var key string
	waitFor(t, 10*time.Second, "the first bridge is up", func() bool {
		k, err := bridge.ReadKey(data)
		key = k
		return err == nil && bridgeAt("http://"+addr, k) == ours
	})
	office := "http://" + addr + "/?key=" + key
	// the data as the first bridge keeps it (what opens the office and the background log aside)
	files := func() string {
		var list []string
		entries, _ := os.ReadDir(data)
		for _, e := range entries {
			if fi, err := e.Info(); err == nil && e.Name() != "open.html" && e.Name() != "bridge.log" {
				list = append(list, fmt.Sprint(e.Name(), fi.Size(), fi.ModTime().UnixNano()))
			}
		}
		return strings.Join(list, "\n")
	}
	before := files()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	refused := func(args ...string) {
		t.Helper()
		started := time.Now()
		out, err := bridgeCmd(ctx, exe, other, data, bin, args...).CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 || !strings.Contains(string(out), "is in use by its bridge at "+office) {
			t.Fatalf("%v: want exit 1 saying where the data folder's bridge runs, got %v:\n%s", args, err, out)
		}
		if time.Since(started) > 10*time.Second {
			t.Errorf("%v took %s", args, time.Since(started))
		}
		if bridgeAt("http://"+other, key) != nobody {
			t.Fatalf("%v: nothing should be left at %s", args, other)
		}
	}
	refused()
	page := bridge.FileURL(runtime.GOOS, filepath.Join(data, "open.html"))
	waitFor(t, 10*time.Second, "the running bridge's office opens", func() bool { return slices.Equal(asked(), []string{page}) })
	refused("-background", "-open=false")
	if got := bridge.BridgeAddr(data); got != addr {
		t.Errorf("bridge.addr should still name the first bridge: %q", got)
	}
	if after := files(); after != before {
		t.Errorf("the refused bridges touched the data:\n%s\n---\n%s", before, after)
	}
	if bridgeAt("http://"+addr, key) != ours {
		t.Error("the first bridge should still be running")
	}
	// -stop with the data folder finds its bridge wherever it runs
	out, err := bridgeCmd(ctx, exe, other, data, bin, "-stop").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "stopped the symphony bridge at "+addr) {
		t.Fatalf("-stop at %s with the data folder: %v\n%s", other, err, out)
	}
}

// TestAddrNotLocal: the bridge listens on this machine only; any other host in -addr stops it before it starts.
func TestAddrNotLocal(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:7788": true, "localhost:7788": true, "[::1]:7788": true,
		"0.0.0.0:7788": false, "[::]:7788": false, ":7788": false, "192.168.1.5:7788": false, "example.com:7788": false,
		"127.0.0.2:7788": false, "127.0.0.1": false, "127.0.0.1:": false, "::1:7788": false,
		"127.0.0.1:0": false, "127.0.0.1:http": false, "127.0.0.1:70000": false, "127.0.0.1:-1": false,
	} {
		host, port, ok := localAddr(addr)
		// localhost is listened on as 127.0.0.1, so a browser can't end up on whoever holds ::1
		if ok != want || (ok && net.JoinHostPort(host, port) != strings.Replace(addr, "localhost", "127.0.0.1", 1)) {
			t.Errorf("localAddr(%q) = %q %q %v, want ok %v", addr, host, port, ok, want)
		}
	}
	exe := buildBridge(t)
	data := t.TempDir()
	for _, addr := range []string{"0.0.0.0:7788", ":7788"} {
		cmd := exec.Command(exe, "-addr", addr, "-data", data, "-open=false")
		cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "-addr "+addr+": the bridge listens on this machine only") {
			t.Errorf("-addr %s: want a refusal, got %v:\n%s", addr, err, out)
		}
	}
	if _, err := os.Stat(filepath.Join(data, "key")); err == nil {
		t.Error("a refused start should leave the data folder alone")
	}
	// attach checks its address the same way
	cmd := exec.Command(exe, "attach", "-addr", "0.0.0.0:7788", "x")
	cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "the bridge listens on this machine only") {
		t.Errorf("attach -addr 0.0.0.0:7788: want a refusal, got %v:\n%s", err, out)
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

	out := run()
	key, err := bridge.ReadKey(data)
	if err != nil {
		t.Fatal(err)
	}
	office := "http://" + addr + "/?key=" + key
	if !strings.Contains(out, "runs in the background at "+office) {
		t.Fatalf("-background should say where the bridge runs, signed in:\n%s", out)
	}
	if bridgeAt("http://"+addr, key) != ours {
		t.Fatal("the bridge should keep running after -background returns, with the data folder's key")
	}
	if log, err := os.ReadFile(filepath.Join(data, "bridge.log")); err != nil || !strings.Contains(string(log), "symphony bridge v9.8.7: "+office) {
		t.Errorf("bridge.log should have the bridge's own log: %v\n%s", err, log)
	}
	if fi, err := os.Stat(filepath.Join(data, "bridge.log")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("bridge.log holds the key: only the user may read it (%v %v)", fi.Mode(), err)
	}
	if out := run(); !strings.Contains(out, "already running at "+office) {
		t.Fatalf("a second -background should find the running bridge:\n%s", out)
	}

	// -stop stops it, and then finds nothing to stop
	stop := func() string {
		cmd := exec.CommandContext(ctx, exe, "-stop", "-addr", addr, "-data="+data)
		cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("-stop: %v\n%s", err, out)
		}
		return string(out)
	}
	if out := stop(); !strings.Contains(out, "stopped the symphony bridge at "+addr) {
		t.Fatalf("-stop should say it stopped the bridge:\n%s", out)
	}
	if bridgeAt("http://"+addr, key) != nobody {
		t.Fatal("after -stop nothing should answer")
	}
	if out := stop(); !strings.Contains(out, "no symphony bridge with this data folder answers at "+addr) {
		t.Fatalf("a second -stop should find nothing to stop:\n%s", out)
	}
}

// TestStop: symphony -stop ends a bridge started in a terminal (it exits 0, as with SIGTERM), refuses to show the key to
// what can't prove it has it, and with no data folder at all says there is nothing to stop.
func TestStop(t *testing.T) {
	exe := buildBridge(t)
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	addr := freeAddr(t)
	bridgeProc := exec.Command(exe, "-addr", addr, "-data", data, "-open=false", "-claude", filepath.Join(dir, "no-claude"))
	bridgeProc.Env = append(os.Environ(), "SYMPHONY_LANG=en")
	if err := bridgeProc.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- bridgeProc.Wait() }()
	t.Cleanup(func() { bridgeProc.Process.Kill() })
	waitFor(t, 10*time.Second, "the bridge is up", func() bool {
		key, err := bridge.ReadKey(data)
		return err == nil && bridgeAt("http://"+addr, key) == ours
	})
	stop := func(addr, data string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-stop", "-addr", addr, "-data", data)
		cmd.Env = append(os.Environ(), "SYMPHONY_LANG=en")
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := stop(addr, data); err != nil || !strings.Contains(out, "stopped the symphony bridge at "+addr) {
		t.Fatalf("-stop: %v\n%s", err, out)
	}
	select {
	case err := <-exited:
		if err != nil {
			t.Fatalf("the bridge should exit 0, as with SIGTERM: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the bridge is still running")
	}

	// something else at the address never sees the key, and -stop fails
	var heard strings.Builder
	var mu sync.Mutex
	squatter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		fmt.Fprintf(&heard, "%s %s %v %s\n", r.Method, r.URL, r.Header, body)
		mu.Unlock()
		w.Write([]byte(`{"type":"snapshot","proof":"` + strings.Repeat("ab", 32) + `","ok":true}`))
	}))
	defer squatter.Close()
	there := strings.TrimPrefix(squatter.URL, "http://")
	if out, err := stop(there, data); err == nil || !strings.Contains(out, there+" doesn't prove it is the bridge with this page key") {
		t.Fatalf("-stop at a squatter: want a refusal, got %v\n%s", err, out)
	}
	key, _ := bridge.ReadKey(data)
	mu.Lock()
	if strings.Contains(heard.String(), key) || strings.Contains(heard.String(), "/api/shutdown") {
		t.Fatalf("the squatter saw the key or a shutdown:\n%s", heard.String())
	}
	mu.Unlock()

	// a data folder no bridge ever used: nothing to stop, and nothing made
	empty := filepath.Join(dir, "empty")
	if out, err := stop(addr, empty); err != nil || !strings.Contains(out, "no symphony bridge with this data folder answers at "+addr) {
		t.Fatalf("-stop with a fresh data folder: %v\n%s", err, out)
	}
	if _, err := os.Stat(empty); err == nil {
		t.Fatal("-stop shouldn't make a data folder")
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
	abs := func(p string) string { a, _ := filepath.Abs(p); return a } // a rooted path has a drive on Windows
	got = autostartArgs(map[string]string{"claude": "./bin/claude", "mod": "/mods/x"}, "/data")
	if want := []string{"-data=" + abs("/data"), "-mod=" + abs("/mods/x"), "-claude=" + filepath.Join(wd, "bin", "claude")}; !slices.Equal(got, want) {
		t.Errorf("a claude with a folder should become absolute: got %q, want %q", got, want)
	}
}

// TestBackgroundArgs: the copy started in the background gets the flags it needs, folders made absolute and -open kept
// (the mod asks for -open=false), and runs in the data folder, not the folder of the session that started it; in this
// repository, it runs there, so the install hint still names it.
func TestBackgroundArgs(t *testing.T) {
	wd, _ := os.Getwd()
	abs := func(p string) string { a, _ := filepath.Abs(p); return a }
	project := t.TempDir()
	args, dir := backgroundArgs(map[string]string{"addr": "127.0.0.1:7790", "open": "false", "background": "true", "mod": "m", "lang": "ja"}, "data", project)
	want := []string{"-data=" + filepath.Join(wd, "data"), "-addr=127.0.0.1:7790", "-mod=" + filepath.Join(wd, "m"), "-lang=ja", "-open=false"}
	if !slices.Equal(args, want) || dir != filepath.Join(wd, "data") {
		t.Errorf("got %q in %s, want %q in the data folder", args, dir, want)
	}
	// another plugin marketplace is just a project
	if err := os.MkdirAll(filepath.Join(project, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".claude-plugin", "marketplace.json"), []byte(`{"plugins":[{"name":"other"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, dir := backgroundArgs(nil, "/data", project); dir != abs("/data") {
		t.Errorf("started from another marketplace: runs in %s, want the data folder", dir)
	}
	repo := filepath.Join(wd, "..", "..")
	if _, dir := backgroundArgs(nil, "/data", repo); dir != repo {
		t.Errorf("started from the repository: runs in %s, want the repository", dir)
	}
}

// TestHTTPServer: a client can't hold a connection by sending a request's head slowly, or by keeping it idle; the
// pages' event stream and the mod's long polls have no time limit.
func TestHTTPServer(t *testing.T) {
	s := httpServer(http.NotFoundHandler())
	if s.ReadHeaderTimeout <= 0 || s.IdleTimeout <= 0 || s.ReadTimeout != 0 || s.WriteTimeout != 0 {
		t.Errorf("ReadHeaderTimeout %s, IdleTimeout %s, ReadTimeout %s, WriteTimeout %s", s.ReadHeaderTimeout, s.IdleTimeout, s.ReadTimeout, s.WriteTimeout)
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
	// something else holds the address: nothing is turned on that would only step aside at every login
	squatter := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer squatter.Close()
	taken := strings.TrimPrefix(squatter.URL, "http://")
	for _, c := range []struct {
		cmd  *exec.Cmd
		want string
	}{
		{exec.Command(gobin, "run", ".", "-autostart", "on"), "-autostart needs the symphony program itself, not go run"},
		{exec.Command(buildBridge(t), "-autostart", "maybe"), "-autostart takes on or off, not maybe"},
		{exec.Command(buildBridge(t), "-autostart", "on", "-addr", taken), taken + " is taken"},
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

// The page's hint for reaching a session's terminal by hand names the address and the data folder when they aren't
// the defaults, since attach needs both to find the bridge and its key; and the program by its full path when
// "symphony" on PATH isn't this one (a release unpacked somewhere).
func TestAttachHint(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// PATH leads nowhere, or to another symphony: the full path
	t.Setenv("PATH", t.TempDir())
	if got, want := attachHint(defaultAddr, bridge.DefaultDataDir())("L3"), shellWord(self)+" attach L3"; got != want {
		t.Errorf("off PATH: got %q, want %q", got, want)
	}
	other := filepath.Join(t.TempDir(), "symphony"+exeSuffix())
	if err := os.WriteFile(other, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(other))
	if got, want := attachHint(defaultAddr, bridge.DefaultDataDir())("L4"), shellWord(self)+" attach L4"; got != want {
		t.Errorf("another symphony on PATH: got %q, want %q", got, want)
	}
	dir := filepath.Join(t.TempDir(), "my data")
	if got, want := attachHint("127.0.0.1:7799", dir)("L2"), shellWord(self)+" attach -addr 127.0.0.1:7799 -data "+shellWord(dir)+" L2"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if runtime.GOOS == "windows" {
		// plain quotes, no doubled backslashes, and "& " before a quoted program for PowerShell
		if got := shellWord(`C:\bin dir\symphony.exe`); got != `"C:\bin dir\symphony.exe"` {
			t.Errorf("a Windows path with a space: %q", got)
		}
	} else if got := shellWord(dir); got != strconv.Quote(dir) {
		t.Errorf("a path with a space: %q", got)
	}
	// PATH leads to this very program (through a link): plain "symphony"
	bin := t.TempDir()
	if err := os.Symlink(self, filepath.Join(bin, "symphony"+exeSuffix())); err != nil {
		t.Skip("no symlinks:", err) // Windows without developer mode
	}
	t.Setenv("PATH", bin)
	if got := attachHint(defaultAddr, bridge.DefaultDataDir())("L1"); got != "symphony attach L1" {
		t.Errorf("on PATH: %q", got)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
