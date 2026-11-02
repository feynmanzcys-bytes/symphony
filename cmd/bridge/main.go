// symphony bridge: go run ./cmd/bridge [-addr 127.0.0.1:7788] [-data folder] [-claude claude] [-mod folder] [-web web]
// [-lang en] [-open=false] [-autostart on|off] [-background] [-version]; symphony attach [-addr host:port] [-data folder]
// <launch id> shows a session's terminal.
// Releases ship it as the symphony program (.github/release.sh).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/autostart"
	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
	"github.com/feynmanzcys-bytes/symphony/web"
)

// defaultAddr is where the bridge listens unless -addr says otherwise.
const defaultAddr = "127.0.0.1:7788"

// version is set by release builds: -ldflags "-X main.version=v0.3.0".
var version string

// buildVersion is the release's version; other builds use what go build stamped from git, or "dev".
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

func main() {
	// The flag help is translated, so the language has to be known before the flags exist:
	// the environment first, then -lang, read here by hand.
	i18n.SetLang(i18n.Detect(os.Getenv))
	if l := langArg(os.Args[1:]); l != "" {
		i18n.SetLang(l)
	}
	if len(os.Args) > 1 && os.Args[1] == "attach" {
		attach(os.Args[2:])
		return
	}
	addr := flag.String("addr", defaultAddr, i18n.T("flag.addr"))
	dataDir := flag.String("data", bridge.DefaultDataDir(), i18n.T("flag.data"))
	webDir := flag.String("web", "", i18n.T("flag.web"))
	modDir := flag.String("mod", "", i18n.T("flag.mod"))
	claude := flag.String("claude", "claude", i18n.T("flag.claude"))
	flag.String("terminal", "", i18n.T("flag.terminal")) // ignored since 0.6 (sessions run inside the bridge); kept so older -autostart entries still start
	flag.String("lang", i18n.Lang(), i18n.T("flag.lang", "langs", strings.Join(i18n.Langs, ", ")))
	open := flag.Bool("open", true, i18n.T("flag.open"))
	auto := flag.String("autostart", "", i18n.T("flag.autostart"))
	background := flag.Bool("background", false, i18n.T("flag.background"))
	showVersion := flag.Bool("version", false, i18n.T("flag.version"))
	stopFlag := flag.Bool("stop", false, i18n.T("flag.stop"))
	flag.Parse()
	if *showVersion {
		fmt.Printf("symphony %s (%s/%s)\n", buildVersion(), runtime.GOOS, runtime.GOARCH)
		return
	}

	host, port, ok := localAddr(*addr)
	if !ok {
		log.Fatal(i18n.T("log.addr_not_local", "addr", *addr))
	}
	base := "http://" + net.JoinHostPort(host, port)
	if *auto != "" {
		set := map[string]string{}
		flag.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() })
		setAutostart(*auto, autostartArgs(set, *dataDir), base, *dataDir, *open)
		return
	}
	if *stopFlag {
		stopBridge(base, *dataDir)
		return
	}
	if *background {
		startBackground(base, *dataDir)
		return
	}
	key := loadKey(*dataDir)
	office := officeURL(base, key)
	// The address first: when this data folder's bridge already answers there (symphony started a second time, say
	// with a double-click), there is nothing to start, so show its office and stop.
	ln, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		switch bridgeAt(base, key) {
		case ours:
			log.Print(i18n.T("log.already_running", "url", office))
			if *open {
				openOffice(*dataDir, office)
			}
			return
		case another:
			addrTaken(base)
		}
		log.Fatal(err)
	}
	// One bridge per data folder, whatever its address: a second one would take the first one's running tasks for
	// left over and end them, and write over its files. Taken before anything there is read.
	if _, err := bridge.LockData(*dataDir, net.JoinHostPort(host, port)); err != nil {
		dataTaken(*dataDir, key, *open, err)
	}

	var pages fs.FS = web.FS
	if *webDir != "" {
		pages = os.DirFS(*webDir)
	}

	hub := bridge.NewHub()
	hub.Warn = func(line string) { log.Print(line) }
	if err := hub.Open(*dataDir); err != nil {
		var damaged *bridge.DamagedFile
		if errors.As(err, &damaged) {
			log.Fatal(i18n.T("log.data_damaged", "file", damaged.Path, "reason", damaged.Err.Error()))
		}
		log.Fatal(i18n.T("log.data_failed", "dir", *dataDir, "reason", err.Error()))
	}
	srv := bridge.NewServer(hub, pages)
	srv.Key = key
	var terms *bridge.Terms // the sessions' terminals, when the bridge can start sessions
	if l := launcher(*modDir, *claude, base); l != nil {
		l.AttachHint = attachHint(net.JoinHostPort(host, port), *dataDir)
		srv.SetLauncher(l)
		srv.Window = bridge.NewWindow(runtime.GOOS, exec.LookPath, os.Getenv)
		if exe, err := os.Executable(); err == nil {
			// the data folder, where attach finds the key (the key itself stays off the command line); absolute, as the
			// window starts in another folder
			data, _ := filepath.Abs(*dataDir)
			srv.AttachCommand = func(id string) []string {
				return []string{exe, "attach", "-addr", net.JoinHostPort(host, port), "-data", data, id}
			}
		}
		terms = l.Terms
		go func() { // ended terminals keep up to 256 KiB of history each: let them go after a while
			for range time.Tick(time.Minute) {
				l.Terms.Prune()
			}
		}()
	}
	// Stopping the bridge (SIGINT, SIGTERM, SIGHUP, or symphony -stop through POST /api/shutdown) ends the sessions, its
	// children, cleanly rather than by surprise: no task ends by itself from here on (killing the members would start
	// tidying git up just as we leave), what waits to be written is written, then the sessions end.
	var stopping sync.Once
	stopAll := func() {
		hub.Stop()
		// what couldn't be written, after one more try; a disk that hangs doesn't keep the sessions running
		flushed := make(chan error, 1)
		go func() { flushed <- hub.Flush() }()
		select {
		case err := <-flushed:
			if err != nil {
				log.Print(i18n.T("log.unsaved", "reason", err.Error()))
			}
		case <-time.After(10 * time.Second):
			log.Print(i18n.T("log.unsaved", "reason", "still writing after 10 s"))
		}
		if terms != nil {
			terms.KillAll()
		}
	}
	shutdown := func() {
		stopping.Do(func() {
			stopAll()
			os.Exit(0)
		})
	}
	srv.Shutdown = shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	// a hangup too: the terminal of a bridge running in it was closed; under nohup it stays ignored, as asked
	if !signal.Ignored(syscall.SIGHUP) {
		signal.Notify(stop, syscall.SIGHUP)
	}
	go func() {
		<-stop
		go shutdown()
		// a second signal while the stop runs (a task's git work, a disk that hangs): don't wait for it
		<-stop
		log.Print(i18n.T("log.stop_forced"))
		if terms != nil {
			terms.KillAll()
		}
		os.Exit(1)
	}()
	go func() {
		for range time.Tick(time.Minute) {
			hub.Prune()
		}
	}()
	go func() {
		for range time.Tick(5 * time.Second) {
			hub.Reap() // sessions without a heartbeat for too long go off duty
		}
	}()

	log.Print(i18n.T("log.listening", "version", buildVersion(), "url", office, "data", *dataDir))
	if *open {
		go openOffice(*dataDir, office) // the bridge answers meanwhile, however long finding the browser takes
	}
	err = httpServer(srv).Serve(ln)
	stopping.Do(stopAll) // as shutdown does, but the bridge exits with the error
	log.Fatal(err)
}

// httpServer serves h with time limits on what a client may keep waiting: a request's head, an idle connection. None
// on reading a body or writing an answer: the pages' event stream and the mod's long polls stay open.
func httpServer(h http.Handler) *http.Server {
	return &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
}

// localAddr splits -addr into its host and port. The host has to be this machine's own (127.0.0.1, localhost or ::1):
// the bridge never listens where other machines reach it. localhost becomes 127.0.0.1: listening on localhost takes
// one of its two addresses, and a browser that tries the other (::1) first would reach whoever holds that one. The port
// is a number, not 0: the office's address has to be known before the bridge listens.
func localAddr(addr string) (host, port string, ok bool) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", false
	}
	if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		return "", "", false
	}
	switch host {
	case "localhost":
		return "127.0.0.1", port, true
	case "127.0.0.1", "::1":
		return host, port, true
	}
	return "", "", false
}

// loadKey is the data folder's page key, made on the first start; the bridge can't run without it.
func loadKey(dataDir string) string {
	key, err := bridge.LoadKey(dataDir)
	if err != nil {
		log.Fatal(i18n.T("log.key_failed", "dir", dataDir, "reason", err.Error()))
	}
	return key
}

// officeURL is the office's address with the page key, which signs a browser in: for the log, and for open.html.
func officeURL(base, key string) string { return base + "/?key=" + key }

// what answers at the bridge's address
type answer int

const (
	nobody  answer = iota // nothing (yet)
	ours                  // a symphony bridge that takes this data folder's key
	another               // another program, or a symphony bridge with another data folder
)

// bridgeAt says what answers at base (http://host:port). The answer has to prove it knows the page key before the key
// goes anywhere (bridge.ProveKey), so a program that took the address never sees it; an older bridge, without page
// keys, counts as another.
func bridgeAt(base, key string) answer {
	ok, err := bridge.ProveKey(base, key)
	switch {
	case err != nil:
		return nobody
	case ok:
		return ours
	}
	return another
}

// addrTaken stops the program: something other than this data folder's bridge answers at base, so there is no office
// to open and no address to listen on.
func addrTaken(base string) {
	log.Fatal(i18n.T("log.addr_taken", "addr", strings.TrimPrefix(base, "http://")))
}

// dataTaken stops the program when the data folder can't be locked: another bridge holds it (bridge.ErrDataLocked),
// which then runs at another address. When it proves it answers at the address it noted, its office opens, as it
// would at this one.
func dataTaken(dataDir, key string, open bool, err error) {
	if !errors.Is(err, bridge.ErrDataLocked) {
		log.Fatal(i18n.T("log.data_failed", "dir", dataDir, "reason", err.Error()))
	}
	addr := runningAt(dataDir, key)
	if addr == "" {
		log.Fatal(i18n.T("log.data_locked", "dir", dataDir))
	}
	office := officeURL("http://"+addr, key)
	if open {
		openOffice(dataDir, office)
	}
	log.Fatal(i18n.T("log.data_in_use", "dir", dataDir, "url", office))
}

// runningAt is the address (host:port) of the bridge holding the data folder, as it noted it, when it proves it is
// this data folder's bridge there; else "".
func runningAt(dataDir, key string) string {
	host, port, ok := localAddr(bridge.BridgeAddr(dataDir))
	if !ok {
		return ""
	}
	addr := net.JoinHostPort(host, port)
	if bridgeAt("http://"+addr, key) != ours {
		return ""
	}
	return addr
}

// startBackground starts a copy of this program without -background, detached from this terminal and writing its log
// to bridge.log in the data folder, and returns once it answers. The mod starts the bridge this way, so it keeps
// running after the session; when a bridge already answers at the address, there's nothing to start. The copy gets
// neither the folder nor the session variables of the session that started it (backgroundArgs,
// bridge.StripSessionEnv): it outlives that session, and what it runs must never pass for it.
func startBackground(base, dataDir string) {
	// made before the copy starts, so both have the same one
	key := loadKey(dataDir)
	office := officeURL(base, key)
	// "another" may be this data folder's bridge with a key written over ours: two symphony -background at once over a
	// damaged key file each write a new one (LoadKey renames over it then), and the bridge that came up has the other;
	// the key file is read again before the answer counts
	at := func() answer {
		a := bridgeAt(base, key)
		if a == another {
			if k, err := bridge.ReadKey(dataDir); err == nil && k != key {
				key, office = k, officeURL(base, k)
				a = bridgeAt(base, key)
			}
		}
		return a
	}
	switch at() {
	case ours:
		log.Print(i18n.T("log.already_running", "url", office))
		return
	case another:
		addrTaken(base)
	}
	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		log.Fatal(err)
	}
	logPath := filepath.Join(dataDir, "bridge.log")
	out, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		log.Fatal(err)
	}
	set := map[string]string{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() })
	wd, _ := os.Getwd()
	args, dir := backgroundArgs(set, dataDir, wd)
	// its own output goes to the log, so whoever ran this (the mod's process.run) isn't kept waiting on a pipe
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = bridge.StripSessionEnv(os.Environ())
	cmd.Stdout, cmd.Stderr = out, out
	detach(cmd)
	err = cmd.Start()
	out.Close()
	if err != nil {
		log.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(15 * time.Second)
	for {
		switch at() {
		case ours:
			log.Print(i18n.T("log.background", "url", office, "log", logPath))
			return
		case another:
			addrTaken(base)
		}
		select {
		case err := <-exited:
			// another bridge may have taken the address a moment ago; then this one stepped aside
			switch at() {
			case ours:
				log.Print(i18n.T("log.already_running", "url", office))
				return
			case another:
				addrTaken(base)
			}
			// or this data folder's bridge runs at another address
			if addr := runningAt(dataDir, key); addr != "" {
				log.Fatal(i18n.T("log.data_in_use", "dir", dataDir, "url", officeURL("http://"+addr, key)))
			}
			log.Fatal(i18n.T("log.background_failed", "reason", fmt.Sprint(err), "log", logPath))
		case <-deadline:
			log.Fatal(i18n.T("log.background_slow", "url", office, "log", logPath))
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// backgroundArgs are the flags of this command line that the copy started in the background gets (those autostartArgs
// keeps, folders made absolute, and -open), and the folder it runs in: the data folder, as a program's working folder
// can't be deleted or renamed on Windows; this repository when started from it, where the install hint finds the mod's
// marketplace (launcher).
func backgroundArgs(set map[string]string, dataDir, wd string) (args []string, dir string) {
	args = autostartArgs(set, dataDir)
	if v, ok := set["open"]; ok {
		args = append(args, "-open="+v)
	}
	if wd != "" && bridge.ModMarket(wd) {
		return args, wd
	}
	dir, err := filepath.Abs(dataDir)
	if err != nil {
		dir = dataDir
	}
	return args, dir
}

// stopBridge stops this data folder's bridge at base (symphony -stop), however it was started: the bridge proves it has
// the key before the key goes to it, then gets POST /api/shutdown and ends its sessions; this waits until nothing
// answers there any more. Nothing of this data folder's answering there is no error: there is nothing to stop.
func stopBridge(base, dataDir string) {
	addr := strings.TrimPrefix(base, "http://")
	key, err := bridge.ReadKey(dataDir)
	switch {
	case errors.Is(err, fs.ErrNotExist): // no bridge with this data folder ever ran
		log.Print(i18n.T("log.stop_none", "addr", addr))
		return
	case err != nil:
		log.Fatal(i18n.T("log.key_failed", "dir", dataDir, "reason", err.Error()))
	}
	// a data folder has one bridge: the one it noted, wherever that runs
	if bridgeAt(base, key) != ours {
		if at := runningAt(dataDir, key); at != "" {
			base, addr = "http://"+at, at
		}
	}
	switch bridgeAt(base, key) {
	case nobody:
		log.Print(i18n.T("log.stop_none", "addr", addr))
		return
	case another:
		log.Fatal(i18n.T("log.key_unproven", "addr", addr))
	}
	req, _ := http.NewRequest(http.MethodPost, base+"/api/shutdown", nil)
	req.Header.Set("X-Symphony-Key", key)
	c := http.Client{Timeout: 5 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		log.Fatal(i18n.T("log.stop_failed", "addr", addr, "reason", err.Error()))
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		log.Fatal(i18n.T("log.stop_failed", "addr", addr, "reason", res.Status))
	}
	// it writes what waits to be written and waits for tasks being ended (their git work) before it goes
	for deadline := time.Now().Add(60 * time.Second); bridgeAt(base, key) != nobody; time.Sleep(200 * time.Millisecond) {
		if time.Now().After(deadline) {
			log.Fatal(i18n.T("log.stop_slow", "addr", addr))
		}
	}
	log.Print(i18n.T("log.stopped", "addr", addr))
}

// waitBridge waits for something to answer at base, and says what did: this data folder's bridge, something else, or
// nobody in time.
func waitBridge(base, key string, timeout time.Duration) answer {
	for deadline := time.Now().Add(timeout); ; time.Sleep(200 * time.Millisecond) {
		if a := bridgeAt(base, key); a != nobody || time.Now().After(deadline) {
			return a
		}
	}
}

// autostartArgs are the flags of this command line that the bridge started at login gets too, with folders made
// absolute (it starts in another folder), and always the data folder, so it finds the same accounts.
func autostartArgs(set map[string]string, dataDir string) []string {
	abs := func(p string) string {
		if a, err := filepath.Abs(p); err == nil {
			return a
		}
		return p
	}
	args := []string{"-data=" + abs(dataDir)}
	for _, name := range []string{"addr", "web", "mod", "claude", "lang"} {
		v, ok := set[name]
		if !ok {
			continue
		}
		if name == "web" || name == "mod" || (name == "claude" && strings.ContainsAny(v, `/\`)) {
			v = abs(v)
		}
		args = append(args, "-"+name+"="+v)
	}
	return args
}

// setAutostart turns starting the bridge at login on or off. On, it also starts the bridge now, waits for it and
// opens its office.
func setAutostart(mode string, args []string, base, dataDir string, open bool) {
	sys := autostart.Default("symphony")
	switch mode {
	case "on":
		// made before the bridge starts, so both have the same one
		key := loadKey(dataDir)
		office := officeURL(base, key)
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err != nil {
			log.Fatal(i18n.T("log.autostart_failed", "reason", err.Error()))
		}
		// go run builds into a temporary folder that's gone afterwards
		if strings.Contains(filepath.ToSlash(exe), "/go-build") {
			log.Fatal(i18n.T("log.autostart_go_run"))
		}
		// something else at the address: the bridge started at login would only step aside, at every login
		if bridgeAt(base, key) == another {
			addrTaken(base)
		}
		// this data folder's bridge at another address: the one started at login couldn't have the folder
		if at := runningAt(dataDir, key); at != "" && "http://"+at != base {
			log.Fatal(i18n.T("log.data_in_use", "dir", dataDir, "url", officeURL("http://"+at, key)))
		}
		where, err := sys.Enable(autostart.Entry{Exe: exe, Args: args, Path: os.Getenv("PATH")})
		if err != nil {
			log.Fatal(i18n.T("log.autostart_failed", "reason", err.Error()))
		}
		switch waitBridge(base, key, 15*time.Second) {
		case nobody:
			log.Print(i18n.T("log.autostart_slow", "where", where, "url", office))
			return
		case another: // it took the address in the meantime: leave nothing behind that steps aside at every login
			_, _ = sys.Disable()
			addrTaken(base)
		}
		log.Print(i18n.T("log.autostart_on", "where", where, "url", office))
		if open {
			openOffice(dataDir, office)
		}
	case "off":
		where, err := sys.Disable()
		switch {
		case err != nil:
			log.Fatal(i18n.T("log.autostart_failed", "reason", err.Error()))
		case where == "":
			log.Print(i18n.T("log.autostart_none"))
		case runtime.GOOS == "windows":
			log.Print(i18n.T("log.autostart_off_window"))
		default:
			log.Print(i18n.T("log.autostart_off"))
		}
	default:
		log.Fatal(i18n.T("log.autostart_mode", "mode", mode))
	}
}

// openOffice shows the office in the default browser, signed in. Without a browser (no graphical session) the address
// in the log has to do. The key never goes on a command line, where other users of this machine could read it (ps):
// the browser opens open.html in the data folder, which only this user may read and which sends it on to office (the
// way Jupyter does it). A snap or flatpak browser can't read the data folder; it gets the file in its own folder.
func openOffice(dataDir, office string) {
	dir := dataDir
	if runtime.GOOS == "linux" {
		home, _ := os.UserHomeDir()
		exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
		if confined := bridge.ConfinedDir(htmlHandler(), home, exists); confined != "" {
			dir = confined
		}
	}
	path, err := bridge.WriteOpenFile(dir, office)
	if err != nil {
		log.Print(i18n.T("log.open_failed", "url", office, "reason", err.Error()))
		return
	}
	argv, ok := bridge.BrowserCommand(runtime.GOOS, bridge.FileURL(runtime.GOOS, path), os.Getenv, exec.LookPath)
	if !ok {
		return
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		log.Print(i18n.T("log.open_failed", "url", office, "reason", err.Error()))
		return
	}
	go cmd.Wait()
}

// attachHint is the command the page shows for reaching a session's terminal by hand: symphony attach <id>, with the
// address and the data folder (where attach finds the key) when they aren't the defaults.
func attachHint(addr, dataDir string) func(launchID string) string {
	cmd := "symphony attach "
	if addr != defaultAddr {
		cmd += "-addr " + addr + " "
	}
	if abs, err := filepath.Abs(dataDir); err == nil && abs != bridge.DefaultDataDir() {
		if strings.ContainsAny(abs, " '\"") {
			abs = strconv.Quote(abs)
		}
		cmd += "-data " + abs + " "
	}
	return func(launchID string) string { return cmd + launchID }
}

// htmlHandler is the desktop file of the program that opens .html files on Linux (xdg-mime), or "" when unknown.
func htmlHandler() string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "xdg-mime", "query", "default", "text/html")
	cmd.WaitDelay = time.Second // xdg-mime is a script: a child it started may hold the output after it's killed
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// langArg finds -lang in the arguments before the flags are parsed: -lang x, -lang=x, --lang x, --lang=x.
func langArg(args []string) string {
	for i, a := range args {
		if a == "--" {
			break
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "lang" {
			continue
		}
		if hasValue {
			return value
		}
		if i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// launcher prepares what starting sessions from the page needs; without claude the feature stays off. A
// missing mod doesn't turn it off: the page tells how to install it, and a reload after installing is enough.
func launcher(modDir, claude, bridgeURL string) *bridge.Launcher {
	var abs string
	if modDir != "" {
		var err error
		if abs, err = filepath.Abs(modDir); err == nil {
			_, err = os.Stat(filepath.Join(abs, ".claude-plugin", "plugin.json"))
		}
		if err != nil {
			log.Print(i18n.T("log.launch_off_mod", "dir", modDir))
			return nil
		}
	}
	claudePath, err := exec.LookPath(claude)
	if err != nil {
		log.Print(i18n.T("log.launch_off_claude", "claude", claude))
		return nil
	}
	log.Print(i18n.T("log.launch_on"))
	l := bridge.NewLauncher(claudePath, abs, bridgeURL, bridge.NewPTYTerminal())
	// run from the repository, install hints use it as a local plugin marketplace
	if wd, err := os.Getwd(); err == nil && bridge.ModMarket(wd) {
		l.Market = wd
	}
	switch m := l.Mod(context.Background()); {
	case m.Dev:
		log.Print(i18n.T("log.mod_dev", "dir", abs))
	case m.OK:
		log.Print(i18n.T("log.mod_installed", "id", m.ID, "version", m.Version))
	case m.Fix != "":
		log.Print(i18n.T("log.mod_missing", "reason", i18n.TMap("error."+m.Code, m.Params), "fix", m.Fix))
	default:
		log.Print(i18n.T("log.mod_error", "reason", i18n.TMap("error."+m.Code, m.Params)))
	}
	return l
}
