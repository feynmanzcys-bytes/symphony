package bridge

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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

	// the Terminal.app script goes into the temp folder (a folder of this test's: the script deletes itself only once
	// Terminal.app ran it, which nothing does here)
	tmp := t.TempDir()
	for _, v := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(v, tmp)
	}
	w.GOOS = "darwin"
	if err := w.Open("alice", argv); err != nil || len(spawned) != 4 || spawned[0] != "open" || spawned[2] != "Terminal" || !strings.HasSuffix(spawned[3], ".command") {
		t.Fatalf("darwin should open a script in Terminal.app: %v %v", err, spawned)
	}
	if _, err := os.Stat(spawned[3]); err != nil {
		t.Fatalf("the script should be there for Terminal.app: %v", err)
	}
	w.spawn = func([]string) error { return errors.New("boom") }
	if e := asError(w.Open("alice", argv)); e == nil || e.Code != "window_open_failed" || e.Params["detail"] != "boom" {
		t.Fatalf("a failed opener: %v", e)
	}
	// the opener failed: its script is removed (Terminal.app would have deleted it after running it)
	if left, _ := filepath.Glob(filepath.Join(tmp, "symphony-*.command")); len(left) != 1 {
		t.Fatalf("one script (the opened one) should be left, got %v", left)
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

// the bridge's own Claude Code session must not reach a page-started session, or they collide and cross-session
// messaging breaks; a CLAUDE_CONFIG_DIR (the login) and other vars are kept.
func TestChildEnvDropsClaudeSession(t *testing.T) {
	for _, kv := range []string{"CLAUDE_CODE_MESSAGING_SOCKET=/tmp/x.sock", "CLAUDE_CODE_SESSION_ID=abc", "CLAUDECODE=1", "CLAUDE_PID=42"} {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/home/me/.claude")
	t.Setenv("ANTHROPIC_API_KEY", "sk-test")
	env := childEnv(map[string]string{"SYMPHONY_TOKEN": "t"})
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); leakedToChild(k) {
			t.Fatalf("the bridge's Claude Code session leaked to the child: %s", kv)
		}
	}
	for _, want := range []string{"SYMPHONY_TOKEN=t", "CLAUDE_CONFIG_DIR=/home/me/.claude", "ANTHROPIC_API_KEY=sk-test"} {
		if !slices.Contains(env, want) {
			t.Fatalf("env should keep %s: %v", want, env)
		}
	}
}

// TestChildEnvSessionVars: a session gets its own token, launch, task, bridge and office context, never the bridge's (a
// bridge the mod started has the starting session's), and each name once, regardless of case on Windows, where a program may read the
// first of two; SYMPHONY_DATA stays.
func TestChildEnvSessionVars(t *testing.T) {
	environ := []string{"PATH=/bin", "SYMPHONY_TOKEN=starter", "SYMPHONY_TASK=T9", "SYMPHONY_LAUNCH=Lstarter",
		"SYMPHONY_BRIDGE=http://x", "SYMPHONY_CONTEXT=1", "SYMPHONY_DATA=/d", "SYMPHONY_LANG=fr", "symphony_token=lower", "TERM=xterm"}
	extra := map[string]string{"SYMPHONY_TOKEN": "new", "SYMPHONY_LAUNCH": "L1", "SYMPHONY_LANG": "ja", "SYMPHONY_BRIDGE": "http://y"}
	count := func(env []string, goos, name string) (n int, value string) {
		for _, kv := range env {
			if k, v, _ := strings.Cut(kv, "="); envName(goos, k) == envName(goos, name) {
				n, value = n+1, v
			}
		}
		return n, value
	}
	for _, goos := range []string{"linux", "windows"} {
		env := childEnvFor(goos, environ, extra)
		for name, want := range map[string]string{"SYMPHONY_TOKEN": "new", "SYMPHONY_LAUNCH": "L1", "SYMPHONY_LANG": "ja",
			"SYMPHONY_BRIDGE": "http://y", "SYMPHONY_DATA": "/d", "PATH": "/bin", "TERM": "xterm"} {
			if n, v := count(env, goos, name); n != 1 || v != want {
				t.Errorf("%s: %s %d times (%q), want once: %q\n%q", goos, name, n, v, want, env)
			}
		}
		for _, name := range []string{"SYMPHONY_TASK", "SYMPHONY_CONTEXT"} {
			if n, _ := count(env, goos, name); n != 0 {
				t.Errorf("%s: the bridge's %s reached the session: %q", goos, name, env)
			}
		}
	}
	// on Linux another case is another variable
	if !slices.Contains(childEnvFor("linux", environ, extra), "symphony_token=lower") {
		t.Error("linux: symphony_token isn't SYMPHONY_TOKEN")
	}
	if slices.Contains(childEnvFor("windows", environ, extra), "symphony_token=lower") {
		t.Error("windows: symphony_token is SYMPHONY_TOKEN")
	}
	// the copy of the bridge started in the background keeps all but the session's
	got := stripSessionEnv("linux", environ)
	if want := []string{"PATH=/bin", "SYMPHONY_DATA=/d", "SYMPHONY_LANG=fr", "symphony_token=lower", "TERM=xterm"}; !slices.Equal(got, want) {
		t.Errorf("stripSessionEnv: %q, want %q", got, want)
	}
}

func TestPTYAttachCommand(t *testing.T) {
	if got := NewPTYTerminal().Attach("abc"); got != "symphony attach abc" {
		t.Fatalf("attach command: %q", got)
	}
}
