package bridge

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// looker pretends that only the programs in bins are installed.
func looker(bins ...string) LookPath {
	return func(file string) (string, error) {
		if slices.Contains(bins, file) {
			return "/usr/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	}
}

// asError returns err as a bridge *Error, or nil when it isn't one.
func asError(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return nil
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestWindowOpeners(t *testing.T) {
	var spawned []string
	w := &Window{spawn: func(argv []string) error { spawned = argv; return nil }}
	argv := []string{"/usr/local/bin/symphony", "attach", "-addr", "127.0.0.1:7788", "id1"}

	w.GOOS, w.Look, w.Env = "linux", looker("gnome-terminal"), envOf(map[string]string{"DISPLAY": ":0"})
	if err := w.Open("alice", argv); err != nil || !slices.Equal(spawned, append([]string{"gnome-terminal", "--"}, argv...)) {
		t.Fatalf("linux: %v %v", err, spawned)
	}
	w.Look = looker()
	if e := asError(w.Open("alice", argv)); e == nil || e.Code != "terminal_none" {
		t.Fatalf("linux without a terminal: %v", e)
	}
	w.Env = envOf(nil)
	if e := asError(w.Open("alice", argv)); e == nil || e.Code != "window_no_display" {
		t.Fatalf("linux without a display: %v", e)
	}

	w.GOOS = "windows"
	if err := w.Open("alice", argv); err != nil || !slices.Equal(spawned, argv) {
		t.Fatalf("windows should run the command in a new console: %v %v", err, spawned)
	}

	w.GOOS = "darwin"
	if err := w.Open("alice", argv); err != nil || len(spawned) != 4 || spawned[0] != "open" || spawned[2] != "Terminal" || !strings.HasSuffix(spawned[3], ".command") {
		t.Fatalf("darwin should open a script in Terminal.app: %v %v", err, spawned)
	}
	w.spawn = func([]string) error { return errors.New("boom") }
	if e := asError(w.Open("alice", argv)); e == nil || e.Code != "window_open_failed" || e.Params["detail"] != "boom" {
		t.Fatalf("a failed opener: %v", e)
	}
}

func TestChildEnv(t *testing.T) {
	t.Setenv("TERM", "")
	env := childEnv(map[string]string{"SYMPHONY_TOKEN": "t", "SYMPHONY_LANG": "ja"})
	for _, want := range []string{"SYMPHONY_TOKEN=t", "SYMPHONY_LANG=ja", "TERM=xterm-256color"} {
		if !slices.Contains(env, want) {
			t.Fatalf("env lacks %s: %v", want, env)
		}
	}
	t.Setenv("TERM", "screen")
	if env := childEnv(nil); slices.Contains(env, "TERM=xterm-256color") || !slices.Contains(env, "TERM=screen") {
		t.Fatal("a TERM that is set should be kept")
	}
}

func TestPTYAttachCommand(t *testing.T) {
	if got := NewPTYTerminal().Attach("abc"); got != "symphony attach abc" {
		t.Fatalf("attach command: %q", got)
	}
}
