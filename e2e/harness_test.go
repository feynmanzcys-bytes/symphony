//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
	"github.com/feynmanzcys-bytes/symphony/web"
)

// ---------- bridge ----------

type testBridge struct {
	URL string
	Hub *bridge.Hub

	mu     sync.Mutex
	events []bridge.Event
}

// startBridge 起一个进程内 bridge（随机端口、临时数据目录、能从页面启动会话），
// 并记下推送出来的每条事件，方便断言。
func startBridge(t *testing.T, mod string) *testBridge {
	t.Helper()
	hub := bridge.NewHub()
	if err := hub.Open(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	srv := bridge.NewServer(hub, web.FS)
	ts := httptest.NewUnstartedServer(srv)
	url := "http://" + ts.Listener.Addr().String()
	// e2e 要用 tmux 盯终端、替人点“信任”，所以不管什么平台都固定用 tmux
	srv.SetLauncher(bridge.NewLauncher("claude", mod, url, bridge.NewTmux()))
	ts.Start()
	t.Cleanup(ts.Close)
	b := &testBridge{URL: url, Hub: hub}
	ch, stop := hub.Subscribe()
	t.Cleanup(stop)
	go func() {
		for msg := range ch {
			if m, ok := msg.(map[string]any); ok && m["type"] == "event" {
				b.mu.Lock()
				b.events = append(b.events, m["event"].(bridge.Event))
				b.mu.Unlock()
			}
		}
	}()
	return b
}

func (b *testBridge) hasEvent(match func(bridge.Event) bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ev := range b.events {
		if match(ev) {
			return true
		}
	}
	return false
}

// online 返回账号当前在线的会话，不在线返回 nil。
func (b *testBridge) online(name string) *bridge.Session {
	snap := b.Hub.Snapshot()
	for _, a := range snap.Accounts {
		if a.Name != name || a.Session == "" {
			continue
		}
		for _, s := range snap.Sessions {
			if s.ID == a.Session {
				return s
			}
		}
	}
	return nil
}

func (b *testBridge) account(name string) *bridge.AccountView {
	for _, a := range b.Hub.Snapshot().Accounts {
		if a.Name == name {
			return &a
		}
	}
	return nil
}

func (b *testBridge) session(id string) *bridge.Session {
	for _, s := range b.Hub.Snapshot().Sessions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// ---------- 等待 ----------

func waitUntil(t *testing.T, timeout time.Duration, what string, cond func() bool, dump ...func() string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	var extra []string
	for _, d := range dump {
		extra = append(extra, d())
	}
	t.Fatalf("等了 %s 还没等到：%s\n%s", timeout, what, strings.Join(extra, "\n----\n"))
}

// ---------- tmux 里的 Claude Code ----------

type pane struct {
	t    *testing.T
	name string
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// startClaude 在 tmux 里起一个带 mod 的交互式 Claude Code，等它到输入框为止。
func startClaude(t *testing.T, name, dir string, env map[string]string, args ...string) *pane {
	t.Helper()
	cmd := []string{"env"}
	for k, v := range env {
		cmd = append(cmd, k+"="+v)
	}
	cmd = append(cmd, "claude")
	cmd = append(cmd, args...)
	quoted := make([]string, len(cmd))
	for i, c := range cmd {
		quoted[i] = shellQuote(c)
	}
	out, err := exec.Command("tmux", "new-session", "-d", "-s", name, "-x", "200", "-y", "50", "-c", dir, strings.Join(quoted, " ")).CombinedOutput()
	if err != nil {
		t.Fatalf("tmux 起不来: %v %s", err, out)
	}
	p := &pane{t: t, name: name}
	t.Cleanup(p.kill)

	// 新目录会先问信不信任，选“信任”
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		screen := p.capture()
		switch {
		case strings.Contains(screen, "trust this folder"):
			p.key("Down")
			p.key("Enter")
			time.Sleep(time.Second)
		case strings.Contains(screen, "for shortcuts"):
			return p
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s 没等到输入框:\n%s", name, p.capture())
	return nil
}

func (p *pane) capture() string {
	out, _ := exec.Command("tmux", "capture-pane", "-p", "-t", p.name, "-S", "-300").Output()
	return string(out)
}

func (p *pane) key(k string) {
	_ = exec.Command("tmux", "send-keys", "-t", p.name, k).Run()
}

// say 在输入框里打一段话并回车。
func (p *pane) say(text string) {
	_ = exec.Command("tmux", "send-keys", "-t", p.name, "-l", text).Run()
	time.Sleep(300 * time.Millisecond)
	p.key("Enter")
}

func (p *pane) kill() { _ = exec.Command("tmux", "kill-session", "-t", p.name).Run() }

func (p *pane) dump() string { return fmt.Sprintf("[%s 屏幕]\n%s", p.name, p.capture()) }

// ---------- 浏览器 ----------

func newBrowser(t *testing.T) context.Context {
	t.Helper()
	path, err := exec.LookPath("chromium")
	if err != nil {
		t.Skip("没有 chromium")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(path),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("enable-unsafe-swiftshader", true),
		chromedp.NoSandbox,
		chromedp.WindowSize(1400, 860),
	)
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancel := chromedp.NewContext(alloc)
	t.Cleanup(func() { cancel(); cancelAlloc() })
	if err := chromedp.Do(ctx); err != nil {
		t.Fatalf("chromium 起不来: %v", err)
	}
	return ctx
}

// tab 在同一个浏览器里开新标签页。
func tab(t *testing.T, browser context.Context, url string) context.Context {
	t.Helper()
	ctx, cancel := chromedp.NewContext(browser)
	t.Cleanup(cancel)
	if err := chromedp.Do(ctx, chromedp.Navigate(url)); err != nil {
		t.Fatalf("打开 %s 失败: %v", url, err)
	}
	return ctx
}

// do 依次执行浏览器操作，超时或出错就让测试失败。
func do(t *testing.T, ctx context.Context, timeout time.Duration, steps ...chromedp.Action[chromedp.Void]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := chromedp.Do(ctx, steps...); err != nil {
		t.Fatalf("浏览器操作失败: %v", err)
	}
}

// get 执行一个有返回值的浏览器操作。
func get[T any](t *testing.T, ctx context.Context, timeout time.Duration, action chromedp.Action[T]) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	v, err := chromedp.Run(ctx, action)
	if err != nil {
		t.Fatalf("浏览器操作失败: %v", err)
	}
	return v
}

func screenshot(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	buf := get(t, ctx, 20*time.Second, chromedp.CaptureScreenshot())
	dir := filepath.Join("artifacts")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(path)
	t.Logf("截图: %s", abs)
}
