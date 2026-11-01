// symphony bridge: go run ./cmd/bridge [-addr 127.0.0.1:7788] [-data folder] [-claude claude] [-mod folder] [-web web]
// [-lang en] [-open=false] [-autostart on|off] [-background] [-version]; symphony attach [-addr host:port] [-data folder]
// <launch id> shows a session's terminal.
// Releases ship it as the symphony program (.github/release.sh).
package main

import (
	"context"
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

	var pages fs.FS = web.FS
	if *webDir != "" {
		pages = os.DirFS(*webDir)
	}

	hub := bridge.NewHub()
	if err := hub.Open(*dataDir); err != nil {
		log.Fatal(err)
	}
	srv := bridge.NewServer(hub, pages)
	srv.Key = key
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
		// the sessions are the bridge's children: stopping it ends them, cleanly rather than by surprise
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		go func() { // ended terminals keep up to 256 KiB of history each: let them go after a while
			for range time.Tick(time.Minute) {
				l.Terms.Prune()
			}
		}()
		go func() {
			<-stop
			hub.Stop() // no task ends by itself from here on: killing the members would start tidying git up just as we leave
			l.Terms.KillAll()
			os.Exit(0)
		}()
	}
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
	log.Fatal(http.Serve(ln, srv))
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

// startBackground starts a copy of this program without -background, detached from this terminal and writing its log
// to bridge.log in the data folder, and returns once it answers. The mod starts the bridge this way, so it keeps
// running after the session; when a bridge already answers at the address, there's nothing to start.
func startBackground(base, dataDir string) {
	// made before the copy starts, so both have the same one
	key := loadKey(dataDir)
	office := officeURL(base, key)
	switch bridgeAt(base, key) {
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
	var args []string
	for _, a := range os.Args[1:] {
		if name, _, _ := strings.Cut(strings.TrimLeft(a, "-"), "="); strings.HasPrefix(a, "-") && name == "background" {
			continue
		}
		args = append(args, a)
	}
	// its own output goes to the log, so whoever ran this (the mod's process.run) isn't kept waiting on a pipe
	cmd := exec.Command(exe, args...)
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
		switch bridgeAt(base, key) {
		case ours:
			log.Print(i18n.T("log.background", "url", office, "log", logPath))
			return
		case another:
			addrTaken(base)
		}
		select {
		case err := <-exited:
			// another bridge may have taken the address a moment ago; then this one stepped aside
			switch bridgeAt(base, key) {
			case ours:
				log.Print(i18n.T("log.already_running", "url", office))
				return
			case another:
				addrTaken(base)
			}
			log.Fatal(i18n.T("log.background_failed", "reason", fmt.Sprint(err), "log", logPath))
		case <-deadline:
			log.Fatal(i18n.T("log.background_slow", "url", office, "log", logPath))
		case <-time.After(200 * time.Millisecond):
		}
	}
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
	if wd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(wd, ".claude-plugin", "marketplace.json")); err == nil {
			l.Market = wd
		}
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
