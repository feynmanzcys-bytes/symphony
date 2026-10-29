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

// looker 假装只装了 bins 里这些程序。
func looker(bins ...string) LookPath {
	return func(file string) (string, error) {
		if slices.Contains(bins, file) {
			return "/usr/bin/" + file, nil
		}
		return "", exec.ErrNotFound
	}
}

func TestPickTerminal(t *testing.T) {
	cases := []struct {
		goos, kind string
		bins       []string
		want       string // 空表示应报错
	}{
		{"linux", "auto", []string{"tmux"}, "tmux"},
		{"linux", "auto", []string{"gnome-terminal"}, "window:gnome-terminal"},
		{"linux", "auto", []string{"xterm", "konsole"}, "window:konsole"},
		{"linux", "auto", nil, ""},
		{"darwin", "auto", []string{"tmux"}, "window:open"}, // Mac 上默认弹 Terminal.app，想用 tmux 得明说
		{"darwin", "tmux", []string{"tmux"}, "tmux"},
		{"darwin", "tmux", nil, ""},
		{"windows", "auto", nil, "console"},
		{"windows", "tmux", nil, ""},
		{"linux", "screen", []string{"tmux"}, ""},
	}
	for _, c := range cases {
		term, err := PickTerminal(c.goos, c.kind, looker(c.bins...))
		got := ""
		if err == nil {
			got = term.Name()
		}
		if got != c.want {
			t.Errorf("%s/%s/%v: 想要 %q，得到 %q（%v）", c.goos, c.kind, c.bins, c.want, got, err)
		}
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
		`rm -f -- "$0"`, // 脚本里有 token，先删掉自己
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
		t.Fatalf("打开终端的命令不对: %v", opened)
	}
	script := opened[3]
	t.Cleanup(func() { _ = os.Remove(script) })
	fi, err := os.Stat(script)
	if err != nil || fi.Mode().Perm() != 0o700 || !strings.HasSuffix(script, ".command") {
		t.Fatalf("脚本应是只给自己的可执行 .command 文件: %v %v", fi, err)
	}
	// 打不开终端时，脚本（里面有 token）要删掉
	bad := &scriptTerminal{opener: []string{"nope"}, spawn: func(argv []string) error { opened = argv; return errors.New("boom") }}
	if err := bad.Start(l, "/w", nil, []string{"claude"}); err == nil {
		t.Fatal("应报错")
	}
	if _, err := os.Stat(opened[len(opened)-1]); !os.IsNotExist(err) {
		t.Fatal("失败时应删掉脚本")
	}
}

func TestConsoleCmd(t *testing.T) {
	cmd := consoleCmd(`C:\work\项目`, map[string]string{"SYMPHONY_TOKEN": "t"}, []string{`C:\bin\claude.exe`, "--name", "前端 小王"})
	if cmd.Dir != `C:\work\项目` || !slices.Equal(cmd.Args, []string{`C:\bin\claude.exe`, "--name", "前端 小王"}) {
		t.Fatalf("应直接起 claude（不经过 cmd.exe）: %v %v", cmd.Dir, cmd.Args)
	}
	if !slices.Contains(cmd.Env, "SYMPHONY_TOKEN=t") || len(cmd.Env) <= 1 {
		t.Fatal("环境变量要在继承的基础上加上")
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
	if st := hub.launchStatus(l.ID); st != LaunchFailed {
		t.Fatalf("进程先退出了应标失败: %s", st)
	}
}

func TestWindowWatchTimesOut(t *testing.T) {
	hub := NewHub()
	l := &Launch{ID: "w1", Status: LaunchStarting}
	hub.addLaunch(l)
	(&scriptTerminal{opener: []string{"open"}}).Watch(hub, l, 600*time.Millisecond)
	if st := hub.launchStatus(l.ID); st != LaunchFailed {
		t.Fatalf("超时没上线应标失败: %s", st)
	}
}
