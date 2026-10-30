// symphony bridge: go run ./cmd/bridge [-addr 127.0.0.1:7788] [-data folder] [-web web] [-lang en]
package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
	"github.com/feynmanzcys-bytes/symphony/web"
)

func main() {
	// The flag help is translated, so the language has to be known before the flags exist:
	// the environment first, then -lang, read here by hand.
	i18n.SetLang(i18n.Detect(os.Getenv))
	if l := langArg(os.Args[1:]); l != "" {
		i18n.SetLang(l)
	}
	addr := flag.String("addr", "127.0.0.1:7788", i18n.T("flag.addr"))
	dataDir := flag.String("data", defaultDataDir(), i18n.T("flag.data"))
	webDir := flag.String("web", "", i18n.T("flag.web"))
	modDir := flag.String("mod", "", i18n.T("flag.mod"))
	claude := flag.String("claude", "claude", i18n.T("flag.claude"))
	terminal := flag.String("terminal", "auto", i18n.T("flag.terminal"))
	flag.String("lang", i18n.Lang(), i18n.T("flag.lang", "langs", strings.Join(i18n.Langs, ", ")))
	flag.Parse()

	var pages fs.FS = web.FS
	if *webDir != "" {
		pages = os.DirFS(*webDir)
	}

	hub := bridge.NewHub()
	if err := hub.Open(*dataDir); err != nil {
		log.Fatal(err)
	}
	srv := bridge.NewServer(hub, pages)
	host, port, err := net.SplitHostPort(*addr)
	if err != nil {
		log.Fatal(err)
	}
	if host != "" && host != "0.0.0.0" && host != "::" {
		srv.AllowedHosts = append(srv.AllowedHosts, host)
	} else {
		host = "127.0.0.1"
	}
	if l := launcher(*modDir, *claude, *terminal, "http://"+net.JoinHostPort(host, port)); l != nil {
		srv.SetLauncher(l)
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

	log.Print(i18n.T("log.listening", "addr", *addr, "data", *dataDir))
	log.Fatal(http.ListenAndServe(*addr, srv))
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

func defaultDataDir() string {
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

// launcher prepares what starting sessions from the page needs; without claude or a usable terminal
// the feature stays off. A missing mod doesn't turn it off: the page tells how to install it, and a
// reload after installing is enough.
func launcher(modDir, claude, terminal, bridgeURL string) *bridge.Launcher {
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
	term, err := bridge.PickTerminal(runtime.GOOS, terminal, exec.LookPath)
	if err != nil {
		log.Print(i18n.T("log.launch_off", "reason", errorText(err)))
		return nil
	}
	log.Print(i18n.T("log.launch_on", "terminal", term.Name()))
	l := bridge.NewLauncher(claudePath, abs, bridgeURL, term)
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
