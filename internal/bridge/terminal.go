package bridge

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"
)

// Terminal is how sessions started from the page run. The bridge hosts them itself (ptyTerminal): claude runs on
// a pseudo-terminal the bridge owns, nothing opens on the screen, and a window attaches on demand. Tests swap in
// fakes.
type Terminal interface {
	Name() string
	// Start runs argv in dir with env as extra environment variables.
	Start(l *Launch, dir string, env map[string]string, argv []string) error
	// Watch follows the new session until it comes online, the process exits or time runs out; it answers
	// "trust this folder" when a new folder asks.
	Watch(hub *Hub, l *Launch, timeout time.Duration)
	// Attach tells people how to reach the session's terminal: the command to run.
	Attach(launchID string) string
	// TrustHint is true when the first start in a new folder needs a click in a window.
	TrustHint() bool
}

// LookPath finds an executable; tests swap in a fake.
type LookPath func(file string) (string, error)

func sortedKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// ---------- the bridge's own terminals ----------

type ptyTerminal struct {
	Terms *Terms
	// start makes a terminal; tests swap in a fake
	start func(id, name, dir string, env []string, argv []string) (*Term, error)
	// program is how people are told to attach by hand (symphony attach <id>)
	program string
}

// NewPTYTerminal hosts sessions on pseudo-terminals inside the bridge.
func NewPTYTerminal() *ptyTerminal {
	return &ptyTerminal{Terms: &Terms{}, start: newTerm, program: "symphony"}
}

func (t *ptyTerminal) Name() string    { return "pty" }
func (t *ptyTerminal) TrustHint() bool { return false }
func (t *ptyTerminal) Attach(launchID string) string {
	return t.program + " attach " + launchID
}

// leakedToChild is true for an environment variable a spawned session must not inherit from the bridge: the
// bridge's own Claude Code session identity and messaging wiring. A session started from the page has to be a
// clean top-level session, as if launched from a plain terminal, or it collides with the session that started
// the bridge (the mod can start the bridge from inside one) and cross-session messaging breaks.
func leakedToChild(k string) bool {
	return strings.HasPrefix(k, "CLAUDE_CODE_") || k == "CLAUDECODE" || k == "CLAUDE_PID"
}

// sessionVars tie a process to one office session: its account, its launch, its task and the bridge it reports to. A
// bridge started from inside a session (the mod starts one that way) has that session's; a session it starts gets its
// own from the launch, and nothing else it runs gets any. SYMPHONY_DATA and SYMPHONY_LANG aren't a session's.
var sessionVars = []string{"SYMPHONY_TOKEN", "SYMPHONY_LAUNCH", "SYMPHONY_TASK", "SYMPHONY_BRIDGE"}

// envName is how a variable's name compares: as it is, but regardless of case on Windows.
func envName(goos, k string) string {
	if goos == "windows" {
		return strings.ToUpper(k)
	}
	return k
}

// StripSessionEnv is env without sessionVars, for a copy of the bridge started in the background.
func StripSessionEnv(env []string) []string {
	return stripSessionEnv(runtime.GOOS, env)
}

func stripSessionEnv(goos string, env []string) []string {
	var out []string
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); !slices.Contains(sessionVars, envName(goos, k)) {
			out = append(out, kv)
		}
	}
	return out
}

// childEnv is the bridge's environment, minus its own Claude Code session (leakedToChild), any session's
// (sessionVars) and what the session sets itself, plus the session's variables; a terminal type when none is set (a
// bridge started at login has none). Every name is there once: Windows passes a block on as it is, and a program
// there may read the first of two.
func childEnv(extra map[string]string) []string {
	return childEnvFor(runtime.GOOS, os.Environ(), extra)
}

func childEnvFor(goos string, environ []string, extra map[string]string) []string {
	own := map[string]bool{}
	for k := range extra {
		own[envName(goos, k)] = true
	}
	var env []string
	set := map[string]bool{}
	for _, kv := range stripSessionEnv(goos, environ) {
		k, v, _ := strings.Cut(kv, "=")
		name := envName(goos, k)
		if leakedToChild(name) || own[name] {
			continue
		}
		if (name == "TERM" || name == "COLORTERM") && v == "" {
			continue
		}
		set[name] = true
		env = append(env, kv)
	}
	for _, k := range sortedKeys(extra) {
		set[envName(goos, k)] = true
		env = append(env, k+"="+extra[k])
	}
	if !set["TERM"] {
		env = append(env, "TERM=xterm-256color")
	}
	if !set["COLORTERM"] {
		env = append(env, "COLORTERM=truecolor")
	}
	return env
}

func (t *ptyTerminal) Start(l *Launch, dir string, env map[string]string, argv []string) error {
	term, err := t.start(l.ID, l.Name, dir, childEnv(env), argv)
	if err != nil {
		program := argv[0]
		if i := strings.LastIndexAny(program, `/\`); i >= 0 {
			program = program[i+1:]
		}
		return newError("pty_start_failed", fmt.Sprintf("can't start %s on a pseudo-terminal: %v", program, err), "program", program, "detail", err.Error())
	}
	t.Terms.put(term)
	return nil
}

// Watch answers "trust this folder" when a new folder asks, and fails the launch when the process exits or
// time runs out before the session reports in. Once a desk session is online, it follows the process to its end (a
// task follows its members' processes itself, publishTask).
func (t *ptyTerminal) Watch(hub *Hub, l *Launch, timeout time.Duration) {
	term := t.Terms.Get(l.ID)
	if term == nil {
		return
	}
	t.untilOnline(hub, l, term, timeout)
	// a desk session's process is followed to its end whatever became of the launch: its session may still report in
	// after the launch failed (while the kill's hangup is in its grace, or someone uses its terminal)
	if l.Task == "" {
		<-term.Exited()
		hub.launchExited(l.ID, term.ExitErr(), term.Screen(4))
	}
}

// untilOnline waits for the launch to leave starting, answering the trust prompt on the way, and fails it when the
// process exits or time runs out; it answers the launch's status then.
func (t *ptyTerminal) untilOnline(hub *Hub, l *Launch, term *Term, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	trusted, downs := false, 0
	var asked time.Time // when the trust prompt showed
	for time.Now().Before(deadline) {
		if st := hub.launchStatus(l.ID); st != LaunchStarting {
			return st
		}
		select {
		case <-term.Exited():
			screen := term.Screen(8)
			hub.failLaunch(l.ID, newError("launch_process_exited",
				fmt.Sprintf("the claude process exited (%v); check the options. Last lines of the terminal:\n%s", term.ExitErr(), screen),
				"detail", fmt.Sprint(term.ExitErr()), "screen", screen))
			return hub.launchStatus(l.ID) // online, when the session reported in just before it went
		case <-time.After(500 * time.Millisecond):
		}
		// "trust this folder" starts on "No, exit". The answer goes down to "Yes, I trust this folder" and takes it once
		// the screen shows the cursor there, a round later: claude may miss a key that comes right after it drew the
		// question, or draw it again with the cursor back on No, and Enter there would end it
		if screen := term.Recent(12); !trusted && hasTrustPrompt(screen) {
			if asked.IsZero() {
				asked = time.Now()
			}
			switch at := trustCursor(screen); {
			case at == "yes":
				_ = term.Write([]byte("\r"))
				trusted = true
			case at == "no":
				if downs < trustDowns {
					_ = term.Write([]byte("\x1b[B")) // Down
					downs++
				}
			// the screen doesn't say where the cursor is (a console that draws only what changed, another glyph):
			// Down once claude had a moment to take keys, and Enter a round later
			case downs > 0:
				_ = term.Write([]byte("\r"))
				trusted = true
			case time.Since(asked) >= trustSettle:
				_ = term.Write([]byte("\x1b[B"))
				downs++
			}
		}
	}
	screen := term.Screen(8)
	hub.failLaunch(l.ID, newError("launch_timeout_pty",
		"not online after "+timeout.String()+"; last lines of the terminal:\n"+screen, "timeout", timeout.String(), "screen", screen))
	return hub.launchStatus(l.ID) // online, when the session reported in at the last moment
}

// ---------- opening a window on demand ----------

// Window opens a terminal window on this machine that runs a command (symphony attach): Terminal.app on macOS,
// a graphical terminal on Linux, a new console on Windows.
type Window struct {
	GOOS string
	Look LookPath
	Env  func(string) string
	// spawn starts the opener; tests swap in a fake
	spawn func(argv []string) error
}

func NewWindow(goos string, look LookPath, env func(string) string) *Window {
	return &Window{GOOS: goos, Look: look, Env: env, spawn: func(argv []string) error {
		cmd := exec.Command(argv[0], argv[1:]...)
		newConsole(cmd)
		if err := cmd.Start(); err != nil {
			return err
		}
		go cmd.Wait()
		return nil
	}}
}

// opener is the command that opens a window running the script (macOS) or the command itself (Linux);
// an error says why none is available.
func (w *Window) opener() ([]string, error) {
	switch w.GOOS {
	case "windows":
		return nil, nil
	case "darwin":
		return []string{"open", "-a", "Terminal"}, nil
	}
	if w.Env("DISPLAY") == "" && w.Env("WAYLAND_DISPLAY") == "" {
		return nil, newError("window_no_display", "no graphical session: open the terminal by hand with the attach command")
	}
	for _, o := range [][]string{{"x-terminal-emulator", "-e"}, {"gnome-terminal", "--"}, {"konsole", "-e"}, {"xterm", "-e"}} {
		if _, err := w.Look(o[0]); err == nil {
			return o, nil
		}
	}
	return nil, newError("terminal_none", "no graphical terminal found (x-terminal-emulator / gnome-terminal / konsole / xterm)")
}

// attachScript is the one-off script Terminal.app runs: it deletes itself, names the window and becomes the command.
func attachScript(title string, argv []string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nrm -f -- \"$0\"\nprintf '\\033]0;%s\\007' " + shellQuote(title) + "\nexec")
	for _, a := range argv {
		b.WriteString(" " + shellQuote(a))
	}
	b.WriteString("\n")
	return b.String()
}

// Open opens a window titled title that runs argv.
func (w *Window) Open(title string, argv []string) error {
	opener, err := w.opener()
	if err != nil {
		return err
	}
	var run []string
	switch w.GOOS {
	case "windows":
		run = argv
	case "darwin":
		f, err := os.CreateTemp("", "symphony-*.command")
		if err != nil {
			return err
		}
		path := f.Name()
		_, err = f.WriteString(attachScript(title, argv))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Chmod(path, 0o700)
		}
		if err != nil {
			_ = os.Remove(path)
			return err
		}
		run = append(slices.Clone(opener), path)
	default:
		run = append(slices.Clone(opener), argv...)
	}
	if err := w.spawn(run); err != nil {
		return newError("window_open_failed", fmt.Sprintf("can't open a terminal window (%s): %v", run[0], err),
			"opener", run[0], "detail", err.Error())
	}
	return nil
}
