package bridge

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Terminal is a way to open an interactive terminal that runs claude; it differs per platform:
//
//	tmux     Linux (the default when tmux is installed), optional on macOS: runs in the background,
//	         and the bridge can answer "trust this folder" for the user
//	console  Windows: starts claude directly in a new console window
//	script   macOS Terminal.app, or a graphical terminal on Linux without tmux: the terminal runs a one-off script
type Terminal interface {
	Name() string
	// Start opens a terminal in dir that runs argv; env holds extra environment variables.
	Start(l *Launch, dir string, env map[string]string, argv []string) error
	// Watch follows the new terminal until the session comes online, the process exits or time runs out.
	Watch(hub *Hub, l *Launch, timeout time.Duration)
	// Attach tells people where to find the session's terminal: a tmux command, or AttachWindow / AttachConsole.
	Attach(launchID string) string
	// TrustHint is true when the first start in a new folder needs a click on "trust this folder" in the window.
	TrustHint() bool
}

// Attach values that are not commands; the page shows them as a sentence in its own language.
const (
	AttachWindow  = "window"  // the terminal window that opened, titled with the employee's name
	AttachConsole = "console" // the console window that opened, titled with the employee's name
)

// LookPath finds an executable; tests swap in a fake.
type LookPath func(file string) (string, error)

// PickTerminal picks a terminal backend for the platform and the -terminal flag: auto | tmux | window.
func PickTerminal(goos, kind string, look LookPath) (Terminal, error) {
	has := func(bin string) bool { _, err := look(bin); return err == nil }
	if kind == "auto" {
		kind = "window"
		if goos == "linux" && has("tmux") {
			kind = "tmux"
		}
	}
	switch kind {
	case "tmux":
		if !has("tmux") {
			return nil, newError("terminal_no_tmux", "tmux not found")
		}
		return NewTmux(), nil
	case "window":
		switch goos {
		case "windows":
			return &consoleTerminal{}, nil
		case "darwin":
			return &scriptTerminal{opener: []string{"open", "-a", "Terminal"}}, nil
		default:
			for _, o := range [][]string{{"x-terminal-emulator", "-e"}, {"gnome-terminal", "--"}, {"konsole", "-e"}, {"xterm", "-e"}} {
				if has(o[0]) {
					return &scriptTerminal{opener: o}, nil
				}
			}
			return nil, newError("terminal_none", "no tmux and no graphical terminal found (x-terminal-emulator / gnome-terminal / konsole / xterm)")
		}
	}
	return nil, newError("terminal_unknown", fmt.Sprintf("unknown terminal type %q (auto | tmux | window)", kind), "kind", kind)
}

func sortedKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// waitOnline waits for the session to come online with its launch id; a value on exited means the
// process is gone (pass nil when that can't be known). timeoutCode names the error for running out
// of time, which tells people where to look.
func waitOnline(hub *Hub, l *Launch, timeout time.Duration, exited <-chan error, timeoutCode string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if hub.launchStatus(l.ID) != LaunchStarting {
			return
		}
		select {
		case err := <-exited:
			hub.failLaunch(l.ID, newError("launch_exited_early",
				fmt.Sprintf("claude exited before it came online (%v); check the options", err), "detail", fmt.Sprint(err)))
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	hub.failLaunch(l.ID, newError(timeoutCode,
		"not online after "+timeout.String()+"; the first start in a folder needs a click on \"trust this folder\" in the window that opened",
		"timeout", timeout.String()))
}

// ---------- tmux ----------

type tmuxTerminal struct {
	run func(name string, args ...string) ([]byte, error)
}

func NewTmux() Terminal {
	return &tmuxTerminal{run: func(name string, args ...string) ([]byte, error) {
		return exec.Command(name, args...).CombinedOutput()
	}}
}

func (t *tmuxTerminal) Name() string                  { return "tmux" }
func (t *tmuxTerminal) TrustHint() bool               { return false }
func (t *tmuxTerminal) Attach(launchID string) string { return "tmux attach -t " + tmuxName(launchID) }

func tmuxName(launchID string) string { return "sym-" + launchID }

// tmuxArgs builds the arguments of tmux new-session: the command follows --, without a shell.
func tmuxArgs(launchID, dir string, env map[string]string, argv []string) []string {
	args := []string{"new-session", "-d", "-s", tmuxName(launchID), "-x", "200", "-y", "50", "-c", dir}
	for _, k := range sortedKeys(env) {
		args = append(args, "-e", k+"="+env[k])
	}
	return append(append(args, "--"), argv...)
}

func (t *tmuxTerminal) Start(l *Launch, dir string, env map[string]string, argv []string) error {
	if out, err := t.run("tmux", tmuxArgs(l.ID, dir, env, argv)...); err != nil {
		detail := strings.TrimSpace(fmt.Sprintf("%v %s", err, out))
		return newError("tmux_start_failed", "tmux failed to start: "+detail, "detail", detail)
	}
	return nil
}

// Watch answers "trust this folder" when a new folder asks (the user picked the folder in the page).
func (t *tmuxTerminal) Watch(hub *Hub, l *Launch, timeout time.Duration) {
	name := tmuxName(l.ID)
	deadline := time.Now().Add(timeout)
	trusted := false
	for time.Now().Before(deadline) {
		if hub.launchStatus(l.ID) != LaunchStarting {
			return
		}
		out, err := t.run("tmux", "capture-pane", "-p", "-t", name)
		if err != nil {
			hub.failLaunch(l.ID, newError("launch_process_exited", "the claude process exited; check the options"))
			return
		}
		if !trusted && strings.Contains(string(out), "trust this folder") {
			_, _ = t.run("tmux", "send-keys", "-t", name, "Down")
			time.Sleep(200 * time.Millisecond)
			_, _ = t.run("tmux", "send-keys", "-t", name, "Enter")
			trusted = true
		}
		time.Sleep(500 * time.Millisecond)
	}
	out, _ := t.run("tmux", "capture-pane", "-p", "-t", name)
	screen := lastLines(string(out), 8)
	hub.failLaunch(l.ID, newError("launch_timeout_tmux",
		"not online after "+timeout.String()+"; last lines of the terminal:\n"+screen, "timeout", timeout.String(), "screen", screen))
}

// ---------- one-off script in a graphical terminal (macOS Terminal.app, Linux terminals) ----------

type scriptTerminal struct {
	opener []string // the command that opens a terminal; the script path goes last
	// spawn runs opener; tests swap in a fake
	spawn func(argv []string) error
}

func (t *scriptTerminal) Name() string         { return "window:" + t.opener[0] }
func (t *scriptTerminal) TrustHint() bool      { return true }
func (t *scriptTerminal) Attach(string) string { return AttachWindow }
func (t *scriptTerminal) Watch(hub *Hub, l *Launch, timeout time.Duration) {
	waitOnline(hub, l, timeout, nil, "launch_timeout_window")
}

// scriptBody is the script the terminal runs: it deletes itself first (it holds the token), then
// enters the folder and becomes claude with the environment variables.
func scriptBody(dir string, env map[string]string, argv []string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nrm -f -- \"$0\"\ncd " + shellQuote(dir) + " || exit 1\nexec env")
	for _, k := range sortedKeys(env) {
		b.WriteString(" " + shellQuote(k+"="+env[k]))
	}
	for _, a := range argv {
		b.WriteString(" " + shellQuote(a))
	}
	b.WriteString("\n")
	return b.String()
}

func (t *scriptTerminal) Start(l *Launch, dir string, env map[string]string, argv []string) error {
	f, err := os.CreateTemp("", "symphony-*.command")
	if err != nil {
		return err
	}
	path := f.Name()
	_, err = f.WriteString(scriptBody(dir, env, argv))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(path, 0o700)
	}
	if err == nil {
		spawn := t.spawn
		if spawn == nil {
			spawn = func(argv []string) error { return exec.Command(argv[0], argv[1:]...).Start() }
		}
		err = spawn(append(slices.Clone(t.opener), path))
	}
	if err != nil {
		_ = os.Remove(path)
		return newError("window_open_failed", fmt.Sprintf("can't open a terminal window (%s): %v", t.opener[0], err),
			"opener", t.opener[0], "detail", err.Error())
	}
	return nil
}

// ---------- Windows console ----------

type consoleTerminal struct {
	// start starts the process; tests swap in a fake
	start func(cmd *exec.Cmd) error
	exits map[string]chan error
}

func (t *consoleTerminal) Name() string         { return "console" }
func (t *consoleTerminal) TrustHint() bool      { return true }
func (t *consoleTerminal) Attach(string) string { return AttachConsole }

// consoleCmd starts claude directly (no cmd.exe, so non-ASCII names and spaces need no escaping)
// in a new console window.
func consoleCmd(dir string, env map[string]string, argv []string) *exec.Cmd {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	for _, k := range sortedKeys(env) {
		cmd.Env = append(cmd.Env, k+"="+env[k])
	}
	newConsole(cmd)
	return cmd
}

func (t *consoleTerminal) Start(l *Launch, dir string, env map[string]string, argv []string) error {
	cmd := consoleCmd(dir, env, argv)
	start := t.start
	if start == nil {
		start = (*exec.Cmd).Start
	}
	if err := start(cmd); err != nil {
		program := filepath.Base(argv[0])
		return newError("console_start_failed", fmt.Sprintf("can't start %s: %v", program, err), "program", program, "detail", err.Error())
	}
	exited := make(chan error, 1)
	if t.exits == nil {
		t.exits = map[string]chan error{}
	}
	t.exits[l.ID] = exited
	if cmd.Process != nil {
		go func() {
			err := cmd.Wait()
			if err == nil {
				err = errors.New("exit 0")
			}
			exited <- err
		}()
	}
	return nil
}

func (t *consoleTerminal) Watch(hub *Hub, l *Launch, timeout time.Duration) {
	waitOnline(hub, l, timeout, t.exits[l.ID], "launch_timeout_console")
}
