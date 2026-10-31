// symphony bridge: go run ./cmd/bridge [-addr 127.0.0.1:7788] [-data folder] [-web web] [-lang en] [-open=false]
// [-autostart on|off] [-background] [-version]; symphony attach [-addr host:port] <launch id> shows a session's terminal.
// Releases ship it as the symphony program (.github/release.sh).
package main

import (
	"context"
	"encoding/json"
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
	"strings"
	"syscall"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/autostart"
	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
	"github.com/feynmanzcys-bytes/symphony/web"
)

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
	addr := flag.String("addr", "127.0.0.1:7788", i18n.T("flag.addr"))
	dataDir := flag.String("data", defaultDataDir(), i18n.T("flag.data"))
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

	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}
	named := host != "" && host != "0.0.0.0" && host != "::"
	if !named {
		host = "127.0.0.1"
	}
	base := "http://" + net.JoinHostPort(host, port)
	if *auto != "" {
		set := map[string]string{}
		flag.Visit(func(f *flag.Flag) { set[f.Name] = f.Value.String() })
		setAutostart(*auto, autostartArgs(set, *dataDir), base, *open)
		return
	}
	if *background {
		startBackground(base, *dataDir)
		return
	}
	// The address first: when a bridge already answers there (symphony started a second time, say with a
	// double-click), there is nothing to start, so show its office and stop.
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		if !bridgeAt(base) {
			log.Fatal(err)
		}
		log.Print(i18n.T("log.already_running", "url", base+"/"))
		if *open {
			openBrowser(base + "/")
		}
		return
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
	if named {
		srv.AllowedHosts = append(srv.AllowedHosts, host)
	}
	if l := launcher(*modDir, *claude, base); l != nil {
		srv.SetLauncher(l)
		srv.Window = bridge.NewWindow(runtime.GOOS, exec.LookPath, os.Getenv)
		if exe, err := os.Executable(); err == nil {
			srv.AttachCommand = func(id string) []string { return []string{exe, "attach", "-addr", *addr, id} }
		}
		// the sessions are the bridge's children: stopping it ends them, cleanly rather than by surprise
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
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

	log.Print(i18n.T("log.listening", "version", buildVersion(), "addr", *addr, "data", *dataDir))
	if *open {
		openBrowser(base + "/")
	}
	log.Fatal(http.Serve(ln, srv))
}

// bridgeAt says whether a symphony bridge answers at base (http://host:port).
func bridgeAt(base string) bool {
	c := http.Client{Timeout: 2 * time.Second}
	res, err := c.Get(base + "/api/state")
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var state struct {
		Type string `json:"type"`
	}
	return res.StatusCode == http.StatusOK && json.NewDecoder(res.Body).Decode(&state) == nil && state.Type == "snapshot"
}

// startBackground starts a copy of this program without -background, detached from this terminal and writing its log
// to bridge.log in the data folder, and returns once it answers. The mod starts the bridge this way, so it keeps
// running after the session; when a bridge already answers at the address, there's nothing to start.
func startBackground(base, dataDir string) {
	if bridgeAt(base) {
		log.Print(i18n.T("log.already_running", "url", base+"/"))
		return
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
	for !bridgeAt(base) {
		select {
		case err := <-exited:
			// another bridge may have taken the address a moment ago; then this one stepped aside
			if bridgeAt(base) {
				log.Print(i18n.T("log.already_running", "url", base+"/"))
				return
			}
			log.Fatal(i18n.T("log.background_failed", "reason", fmt.Sprint(err), "log", logPath))
		case <-deadline:
			log.Fatal(i18n.T("log.background_slow", "url", base+"/", "log", logPath))
		case <-time.After(200 * time.Millisecond):
		}
	}
	log.Print(i18n.T("log.background", "url", base+"/", "log", logPath))
}

// waitBridge waits for a bridge to answer at base, and says whether one did in time.
func waitBridge(base string, timeout time.Duration) bool {
	for deadline := time.Now().Add(timeout); !bridgeAt(base); time.Sleep(200 * time.Millisecond) {
		if time.Now().After(deadline) {
			return false
		}
	}
	return true
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
func setAutostart(mode string, args []string, base string, open bool) {
	sys := autostart.Default("symphony")
	switch mode {
	case "on":
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
		where, err := sys.Enable(autostart.Entry{Exe: exe, Args: args, Path: os.Getenv("PATH")})
		if err != nil {
			log.Fatal(i18n.T("log.autostart_failed", "reason", err.Error()))
		}
		if !waitBridge(base, 15*time.Second) {
			log.Print(i18n.T("log.autostart_slow", "where", where, "url", base+"/"))
			return
		}
		log.Print(i18n.T("log.autostart_on", "where", where, "url", base+"/"))
		if open {
			openBrowser(base + "/")
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

// openBrowser shows url in the default browser. Without one (no graphical session) the address in the log has to do.
func openBrowser(url string) {
	argv, ok := bridge.BrowserCommand(runtime.GOOS, url, os.Getenv, exec.LookPath)
	if !ok {
		return
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		log.Print(i18n.T("log.open_failed", "url", url, "reason", err.Error()))
		return
	}
	go cmd.Wait()
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

// defaultDataDir is SYMPHONY_DATA, else symphony in the user config folder. The environment variable reaches a bridge
// started by the mod too, which gets no -data.
func defaultDataDir() string {
	if dir := os.Getenv("SYMPHONY_DATA"); dir != "" {
		return dir
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "symphony")
	}
	return ".symphony"
}

// errorText is an error in the bridge's language when it carries a code the catalog knows, its own text otherwise.
func errorText(err error) string {
	var e *bridge.Error
	if errors.As(err, &e) && i18n.Has("error."+e.Code) {
		return i18n.TMap("error."+e.Code, e.Params)
	}
	return err.Error()
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
