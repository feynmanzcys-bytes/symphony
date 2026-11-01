//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/feynmanzcys-bytes/symphony/internal/bridge"
	"github.com/feynmanzcys-bytes/symphony/web"
)

// ---------- bridge ----------

type testBridge struct {
	URL      string
	Key      string // the page key: browsers get it as a cookie (newBrowser), raw page calls as a header (pageReq)
	Data     string // the data folder (accounts.json with the tokens, tasks.json)
	Hub      *bridge.Hub
	Launcher *bridge.Launcher
	// anon counts requests to the mod endpoints without a token: a session without SYMPHONY_TOKEN must send none
	anon atomic.Int64

	mu     sync.Mutex
	events []bridge.Event
}

// startBridge starts a bridge in this process (a random port, a temporary data folder, starting sessions from the
// page turned on) and records every event it pushes, for the assertions. With an empty mod, sessions use the mod
// installed in Claude Code; otherwise they get mod with --plugin-dir.
func startBridge(t *testing.T, mod string) *testBridge {
	t.Helper()
	hub := bridge.NewHub()
	data := t.TempDir()
	if err := hub.Open(data); err != nil {
		t.Fatal(err)
	}
	srv := bridge.NewServer(hub, web.FS)
	b := &testBridge{Hub: hub, Data: data, Key: srv.Key}
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isModAPI(r.URL.Path) && r.Header.Get("Authorization") == "" {
			b.anon.Add(1)
		}
		srv.ServeHTTP(w, r)
	}))
	b.URL = "http://" + ts.Listener.Addr().String()
	pageKeys.Store(b.URL, b.Key)
	t.Cleanup(func() { pageKeys.Delete(b.URL) })
	// sessions started from the page run on pseudo-terminals inside the bridge, as in the real thing
	b.Launcher = bridge.NewLauncher("claude", mod, b.URL, bridge.NewPTYTerminal())
	srv.SetLauncher(b.Launcher)
	ts.Start()
	t.Cleanup(ts.Close)
	t.Cleanup(b.Launcher.Terms.KillAll)
	ch, stop := hub.Subscribe()
	t.Cleanup(stop)
	go func() {
		for msg := range ch {
			if m, ok := msg.Value.(map[string]any); ok && m["type"] == "event" {
				b.mu.Lock()
				b.events = append(b.events, m["event"].(bridge.Event))
				b.mu.Unlock()
			}
		}
	}()
	return b
}

// post sends a request the way the mod does, with an account's token: made-up activity for tests without sessions.
func (b *testBridge) post(t *testing.T, token, path string, body any) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", b.URL+path, bytes.NewReader(data))
	req.Header.Set("content-type", "application/json")
	req.Header.Set("authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("POST %s: %s", path, res.Status)
	}
}

// pageKeys are the page keys of the bridges this test runs, by URL: newBrowser signs its browser in to each, and pageReq
// shows the key on raw calls to the page's endpoints.
var pageKeys sync.Map

// pageReq is a request to one of the page's endpoints of the bridge at base (its URL), with the bridge's page key.
func pageReq(method, base, path string, body io.Reader) *http.Request {
	req, err := http.NewRequest(method, base+path, body)
	if err != nil {
		panic(err)
	}
	if key, ok := pageKeys.Load(base); ok {
		req.Header.Set("X-Symphony-Key", key.(string))
	}
	return req
}

func isModAPI(path string) bool {
	return path == "/api/event" || path == "/api/ask" || strings.HasPrefix(path, "/api/wait/") || strings.HasPrefix(path, "/api/cancel/")
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

// online returns the account's online session, or nil.
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

// pending says whether the ask still waits for an answer.
func (b *testBridge) pending(id string) bool {
	for _, a := range b.Hub.Snapshot().Asks {
		if a.ID == id {
			return true
		}
	}
	return false
}

// ---------- waiting ----------

// eventually polls cond until it holds or the time is up, and says which.
func eventually(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(200 * time.Millisecond)
	}
	return true
}

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
	t.Fatalf("still waiting after %s: %s\n%s", timeout, what, strings.Join(extra, "\n----\n"))
}

// ---------- Claude Code in tmux ----------

type pane struct {
	t    *testing.T
	name string
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// startClaude starts an interactive Claude Code with the mod in tmux and waits for its prompt.
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
		t.Fatalf("tmux failed to start: %v %s", err, out)
	}
	p := &pane{t: t, name: name}
	t.Cleanup(p.kill)

	// a new folder first asks whether to trust it: pick "trust"
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		screen := p.capture()
		switch {
		case strings.Contains(screen, "trust this folder"):
			p.key("Down")
			p.key("Enter")
			time.Sleep(time.Second)
		case strings.Contains(screen, "for shortcuts"), strings.Contains(screen, "shift+tab to cycle"): // the prompt's footer, by permission mode
			return p
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s never showed its prompt:\n%s", name, p.capture())
	return nil
}

func (p *pane) capture() string {
	out, _ := exec.Command("tmux", "capture-pane", "-p", "-t", p.name, "-S", "-300").Output()
	return string(out)
}

func (p *pane) key(k string) {
	_ = exec.Command("tmux", "send-keys", "-t", p.name, k).Run()
}

// say types text into the prompt and presses Enter.
func (p *pane) say(text string) {
	_ = exec.Command("tmux", "send-keys", "-t", p.name, "-l", text).Run()
	time.Sleep(300 * time.Millisecond)
	p.key("Enter")
}

func (p *pane) kill() { _ = exec.Command("tmux", "kill-session", "-t", p.name).Run() }

func (p *pane) dump() string { return fmt.Sprintf("[%s screen]\n%s", p.name, p.capture()) }

// ---------- browser ----------

// browserPath finds the browser for chromedp: SYMPHONY_E2E_BROWSER when set, otherwise Chromium or Google Chrome
// (GitHub's Ubuntu runners have Chrome).
func browserPath() (string, bool) {
	names := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}
	if b := os.Getenv("SYMPHONY_E2E_BROWSER"); b != "" {
		names = []string{b}
	}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}

// requireBrowser skips a test when there's no browser, except on CI, where that's a broken setup.
func requireBrowser(t *testing.T) string {
	t.Helper()
	path, ok := browserPath()
	switch {
	case ok:
		return path
	case os.Getenv("CI") != "":
		t.Fatal("no browser on CI: install Chromium or Chrome, or set SYMPHONY_E2E_BROWSER")
	default:
		t.Skip("no browser found: install Chromium or Chrome, or set SYMPHONY_E2E_BROWSER")
	}
	return ""
}

func newBrowser(t *testing.T) context.Context {
	t.Helper()
	path := requireBrowser(t)
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
		t.Fatalf("the browser failed to start: %v", err)
	}
	// signed in to every bridge the test runs (start the bridge first), the way symphony opens the office: the address
	// with ?key= sets the cookie and takes the key out of the address; the cookie is the browser's, for every tab
	pageKeys.Range(func(base, key any) bool {
		do(t, ctx, 30*time.Second, chromedp.Navigate(base.(string)+"/api/themes?key="+key.(string)))
		if page := get(t, ctx, 10*time.Second, chromedp.Evaluate[string](`location.href + ' ' + document.body.innerText`)); strings.Contains(page, "key=") || !strings.Contains(page, `"themes"`) {
			t.Fatalf("signing the browser in to %s failed: %s", base, page)
		}
		return true
	})
	do(t, ctx, 30*time.Second, chromedp.Navigate("about:blank"))
	return ctx
}

// tab opens a new tab in the same browser.
func tab(t *testing.T, browser context.Context, url string) context.Context {
	t.Helper()
	ctx, cancel := openTab(t, browser, url)
	t.Cleanup(cancel)
	return ctx
}

// openTab opens url in a new tab; cancel closes it. Every page keeps an event stream open and the
// browser allows six connections per host, so a test that opens many pages closes them as it goes.
func openTab(t *testing.T, browser context.Context, url string) (context.Context, context.CancelFunc) {
	t.Helper()
	ctx, cancel := chromedp.NewContext(browser)
	if err := chromedp.Do(ctx); err != nil { // creates the tab
		cancel()
		t.Fatalf("opening a tab failed: %v", err)
	}
	t.Cleanup(cancel)
	do(t, ctx, 30*time.Second, chromedp.Navigate(url))
	return ctx, cancel
}

// clickLabel clicks an employee's name tag from inside the page: tags move with their employees and may pass in
// front of each other, so a mouse click at a tag's position could land on another one.
func clickLabel(t *testing.T, ctx context.Context, account string) {
	t.Helper()
	ok := get(t, ctx, 10*time.Second, chromedp.Evaluate[bool](fmt.Sprintf(
		`(l => l ? (l.click(), true) : false)(document.querySelector('.emp-label[data-account=%q]'))`, account)))
	if !ok {
		t.Fatalf("no name tag for account %s", account)
	}
}

// do runs browser actions in order and fails the test on an error or a timeout.
func do(t *testing.T, ctx context.Context, timeout time.Duration, steps ...chromedp.Action[chromedp.Void]) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := chromedp.Do(ctx, steps...); err != nil {
		t.Fatalf("browser action failed: %v", err)
	}
}

// get runs a browser action that returns a value.
func get[T any](t *testing.T, ctx context.Context, timeout time.Duration, action chromedp.Action[T]) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	v, err := chromedp.Run(ctx, action)
	if err != nil {
		t.Fatalf("browser action failed: %v", err)
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
	t.Logf("screenshot: %s", abs)
}
