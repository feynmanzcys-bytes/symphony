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

// Terminal 是“开一个能交互的终端跑 claude”的办法，各平台不一样：
//
//	tmux     Linux（装了 tmux 时默认）、macOS 可选：后台起，bridge 能替人点“信任此目录”
//	console  Windows：直接在新的控制台窗口里起 claude
//	script   macOS 用 Terminal.app，Linux 没有 tmux 时用图形终端：写个一次性脚本让终端去跑
type Terminal interface {
	Name() string
	// Start 在 dir 里开一个终端跑 argv，env 是额外的环境变量。
	Start(l *Launch, dir string, env map[string]string, argv []string) error
	// Watch 盯着刚开的终端，直到会话上线、进程退出或超时。
	Watch(hub *Hub, l *Launch, timeout time.Duration)
	// Attach 告诉人去哪儿找这个会话的终端。
	Attach(launchID string) string
	// TrustHint 为 true 时，新目录第一次启动要人在弹出的窗口里自己点“信任此目录”。
	TrustHint() bool
}

// LookPath 找可执行文件，测试里换成假的。
type LookPath func(file string) (string, error)

// PickTerminal 按平台和 -terminal 参数挑一个终端后端：auto | tmux | window。
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
			return nil, errors.New("找不到 tmux")
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
			return nil, errors.New("没有 tmux，也找不到图形终端（x-terminal-emulator / gnome-terminal / konsole / xterm）")
		}
	}
	return nil, fmt.Errorf("不认识的终端类型 %q（auto | tmux | window）", kind)
}

func sortedKeys(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// waitOnline 等会话带着启动编号上线；exited 关闭表示进程已经退出（拿不到就传 nil）。
func waitOnline(hub *Hub, l *Launch, timeout time.Duration, exited <-chan error, hint string) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if hub.launchStatus(l.ID) != LaunchStarting {
			return
		}
		select {
		case err := <-exited:
			hub.failLaunch(l.ID, fmt.Sprintf("claude 还没上线就退出了（%v），看看参数对不对", err))
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
	hub.failLaunch(l.ID, "等了 "+timeout.String()+" 没上线。"+hint)
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

// tmuxArgs 拼 tmux new-session 的参数：命令直接跟在 -- 后面，不经过 shell。
func tmuxArgs(launchID, dir string, env map[string]string, argv []string) []string {
	args := []string{"new-session", "-d", "-s", tmuxName(launchID), "-x", "200", "-y", "50", "-c", dir}
	for _, k := range sortedKeys(env) {
		args = append(args, "-e", k+"="+env[k])
	}
	return append(append(args, "--"), argv...)
}

func (t *tmuxTerminal) Start(l *Launch, dir string, env map[string]string, argv []string) error {
	if out, err := t.run("tmux", tmuxArgs(l.ID, dir, env, argv)...); err != nil {
		return fmt.Errorf("tmux 起不来：%v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Watch：新目录先问“信不信任”就替用户点信任（目录是用户在页面上亲手选的）。
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
			hub.failLaunch(l.ID, "claude 进程退出了，看看参数对不对")
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
	hub.failLaunch(l.ID, "等了 "+timeout.String()+" 没上线，终端最后几行：\n"+lastLines(string(out), 8))
}

// ---------- 一次性脚本 + 图形终端（macOS Terminal.app、Linux 图形终端） ----------

type scriptTerminal struct {
	opener []string // 打开终端的命令，脚本路径接在最后
	// spawn 跑 opener，测试里换成假的
	spawn func(argv []string) error
}

func (t *scriptTerminal) Name() string    { return "window:" + t.opener[0] }
func (t *scriptTerminal) TrustHint() bool { return true }
func (t *scriptTerminal) Attach(string) string {
	return "弹出的终端窗口（标题是员工名字）"
}
func (t *scriptTerminal) Watch(hub *Hub, l *Launch, timeout time.Duration) {
	waitOnline(hub, l, timeout, nil, "看看弹出的终端窗口：第一次在这个目录启动要先点“信任此目录”。")
}

// scriptBody 是交给终端跑的脚本：先删掉自己（里面有 token），再进目录、带上环境变量换成 claude。
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
		return fmt.Errorf("打不开终端窗口（%s）：%w", t.opener[0], err)
	}
	return nil
}

// ---------- Windows 控制台 ----------

type consoleTerminal struct {
	// start 起进程，测试里换成假的
	start func(cmd *exec.Cmd) error
	exits map[string]chan error
}

func (t *consoleTerminal) Name() string    { return "console" }
func (t *consoleTerminal) TrustHint() bool { return true }
func (t *consoleTerminal) Attach(string) string {
	return "弹出的控制台窗口（标题是员工名字）"
}

// consoleCmd 直接起 claude（不经过 cmd.exe，名字里的中文和空格都不用转义），在新控制台窗口里跑。
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
		return fmt.Errorf("起不来 %s：%w", filepath.Base(argv[0]), err)
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
	waitOnline(hub, l, timeout, t.exits[l.ID], "看看弹出的控制台窗口：第一次在这个目录启动要先点“信任此目录”。")
}
