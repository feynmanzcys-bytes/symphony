package bridge

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
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

func TestPickTerminal(t *testing.T) {
	cases := []struct {
		goos, kind string
		bins       []string
		want, code string // the backend's name, or the error's code when there is none
	}{
		{"linux", "auto", []string{"tmux"}, "tmux", ""},
		{"linux", "auto", []string{"gnome-terminal"}, "window:gnome-terminal", ""},
		{"linux", "auto", []string{"xterm", "konsole"}, "window:konsole", ""},
		{"linux", "auto", nil, "", "terminal_none"},
		{"darwin", "auto", []string{"tmux"}, "window:open", ""}, // on macOS the default is Terminal.app; tmux must be asked for
		{"darwin", "tmux", []string{"tmux"}, "tmux", ""},
		{"darwin", "tmux", nil, "", "terminal_no_tmux"},
		{"windows", "auto", nil, "console", ""},
		{"windows", "tmux", nil, "", "terminal_no_tmux"},
		{"linux", "screen", []string{"tmux"}, "", "terminal_unknown"},
	}
	for _, c := range cases {
		term, err := PickTerminal(c.goos, c.kind, looker(c.bins...))
		got, code := "", ""
		if err == nil {
			got = term.Name()
		} else if e := asError(err); e != nil {
			code = e.Code
		}
		if got != c.want || code != c.code {
			t.Errorf("%s/%s/%v: want %q %q, got %q %q (%v)", c.goos, c.kind, c.bins, c.want, c.code, got, code, err)
		}
	}
	// the startup log names the unknown type
	_, err := PickTerminal("linux", "screen", looker())
	if e := asError(err); e == nil || e.Params["kind"] != "screen" {
		t.Errorf("an unknown type should come back in params.kind: %v", err)
	}
}

func TestTmuxArgs(t *testing.T) {
	args := tmuxArgs("abc", "/w/项目", map[string]string{"B": "2", "A": "1"}, []string{"claude", "--name", "前端 小王"})
	want := []string{"new-session", "-d", "-s", "sym-abc", "-x", "200", "-y", "50", "-c", "/w/项目", "-e", "A=1", "-e", "B=2", "--", "claude", "--name", "前端 小王"}
	if !slices.Equal(args, want) {
		t.Fatalf("\n got %v\nwant %v", args, want)
	}
}

func TestScriptBody(t *testing.T) {
	body := scriptBody("/w/it's 项目", map[string]string{"SYMPHONY_TOKEN": "t0k"}, []string{"/usr/bin/claude", "--name", "前端 小王"})
	lines := strings.Split(strings.TrimSpace(body), "\n")
	want := []string{
		"#!/bin/sh",
		`rm -f -- "$0"`, // the script holds the token, so it deletes itself first
		`cd '/w/it'\''s 项目' || exit 1`,
		`exec env SYMPHONY_TOKEN=t0k /usr/bin/claude --name '前端 小王'`,
	}
	if !slices.Equal(lines, want) {
		t.Fatalf("\n got %q\nwant %q", lines, want)
	}
}

func TestScriptTerminalStart(t *testing.T) {
	var opened []string
	term := &scriptTerminal{opener: []string{"open", "-a", "Terminal"}, spawn: func(argv []string) error { opened = argv; return nil }}
	l := &Launch{ID: "x"}
	if err := term.Start(l, "/w", map[string]string{"SYMPHONY_TOKEN": "t"}, []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	if len(opened) != 4 || opened[0] != "open" || opened[2] != "Terminal" {
		t.Fatalf("wrong command to open the terminal: %v", opened)
	}
	script := opened[3]
	t.Cleanup(func() { _ = os.Remove(script) })
	fi, err := os.Stat(script)
	if err != nil || fi.Mode().Perm() != 0o700 || !strings.HasSuffix(script, ".command") {
		t.Fatalf("the script should be an executable .command file only its owner can use: %v %v", fi, err)
	}
	// when the terminal won't open, the script (it holds the token) is deleted
	bad := &scriptTerminal{opener: []string{"nope"}, spawn: func(argv []string) error { opened = argv; return errors.New("boom") }}
	err = bad.Start(l, "/w", nil, []string{"claude"})
	if e := asError(err); e == nil || e.Code != "window_open_failed" || e.Params["opener"] != "nope" || e.Params["detail"] != "boom" {
		t.Fatalf("want window_open_failed naming the opener and the cause, got %v (%+v)", err, e)
	}
	if _, err := os.Stat(opened[len(opened)-1]); !os.IsNotExist(err) {
		t.Fatal("the script should be deleted on failure")
	}
}

// TestAttach checks where each backend sends people to find a session's terminal; the page turns
// "window" and "console" into a sentence in its own language.
func TestAttach(t *testing.T) {
	for _, tc := range []struct {
		term Terminal
		want string
	}{
		{&tmuxTerminal{}, "tmux attach -t sym-x"},
		{&scriptTerminal{opener: []string{"open"}}, "window"},
		{&consoleTerminal{}, "console"},
	} {
		if got := tc.term.Attach("x"); got != tc.want {
			t.Errorf("%s: want %q, got %q", tc.term.Name(), tc.want, got)
		}
	}
}

func TestConsoleCmd(t *testing.T) {
	cmd := consoleCmd(`C:\work\项目`, map[string]string{"SYMPHONY_TOKEN": "t"}, []string{`C:\bin\claude.exe`, "--name", "前端 小王"})
	if cmd.Dir != `C:\work\项目` || !slices.Equal(cmd.Args, []string{`C:\bin\claude.exe`, "--name", "前端 小王"}) {
		t.Fatalf("claude should start directly (no cmd.exe): %v %v", cmd.Dir, cmd.Args)
	}
	if !slices.Contains(cmd.Env, "SYMPHONY_TOKEN=t") || len(cmd.Env) <= 1 {
		t.Fatal("the extra variables should be added to the inherited environment")
	}
}

func TestConsoleExitBeforeOnline(t *testing.T) {
	hub := NewHub()
	exited := make(chan error, 1)
	term := &consoleTerminal{start: func(*exec.Cmd) error { return nil }}
	l := &Launch{ID: "c1", Status: LaunchStarting}
	hub.addLaunch(l)
	if err := term.Start(l, "/w", nil, []string{"claude"}); err != nil {
		t.Fatal(err)
	}
	term.exits[l.ID] = exited
	exited <- errors.New("exit status 1")
	term.Watch(hub, l, 5*time.Second)
	if st := hub.launchStatus(l.ID); st != LaunchFailed || l.ErrorCode != "launch_exited_early" || l.ErrorParams["detail"] != "exit status 1" {
		t.Fatalf("a process that exits first should fail the launch with launch_exited_early: %s %s %v", st, l.ErrorCode, l.ErrorParams)
	}
}

func TestWindowWatchTimesOut(t *testing.T) {
	hub := NewHub()
	l := &Launch{ID: "w1", Status: LaunchStarting}
	hub.addLaunch(l)
	(&scriptTerminal{opener: []string{"open"}}).Watch(hub, l, 600*time.Millisecond)
	if st := hub.launchStatus(l.ID); st != LaunchFailed || l.ErrorCode != "launch_timeout_window" || l.ErrorParams["timeout"] != "600ms" {
		t.Fatalf("a launch not online in time should fail with launch_timeout_window: %s %s %v", st, l.ErrorCode, l.ErrorParams)
	}
}
