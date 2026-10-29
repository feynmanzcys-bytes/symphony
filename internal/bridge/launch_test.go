package bridge

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeTmux 记下 tmux 调用，capture-pane 返回预设的屏幕。
type fakeTmux struct {
	mu        sync.Mutex
	calls     [][]string
	screen    string
	dead      bool
	failStart bool
}

func (f *fakeTmux) run(name string, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string{name}, args...))
	switch args[0] {
	case "capture-pane":
		if f.dead {
			return nil, os.ErrNotExist
		}
		return []byte(f.screen), nil
	case "new-session":
		if f.failStart {
			return []byte("duplicate session"), errors.New("exit 1")
		}
	}
	return nil, nil
}

func (f *fakeTmux) last(sub string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if c := f.calls[i]; len(c) > 1 && c[1] == sub {
			return c
		}
	}
	return nil
}

func (f *fakeTmux) count(sub string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if len(c) > 1 && c[1] == sub {
			n++
		}
	}
	return n
}

func newLaunchBridge(t *testing.T) (*Server, string, *fakeTmux) {
	t.Helper()
	srv := NewServer(NewHub(), testWeb)
	tmux := &fakeTmux{}
	srv.SetLauncher(&Launcher{Claude: "claude", ModDir: "/mods/sb", BridgeURL: "http://127.0.0.1:7788",
		Term: &tmuxTerminal{run: tmux.run}, OnlineTimeout: 3 * time.Second})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts.URL, tmux
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("没等到：%s", what)
}

func flagValue(argv []string, flag string) string {
	if i := slices.Index(argv, flag); i >= 0 && i+1 < len(argv) {
		return argv[i+1]
	}
	return ""
}

func TestLaunchNewAccount(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	web := client{t: t, base: base}
	dir := t.TempDir()

	res := web.ok("POST", "/api/launch", LaunchRequest{Name: "前端小王", Dir: dir, Model: "sonnet", Effort: "high", PermissionMode: "manual", Resume: "continue"})
	id := res["id"].(string)
	if res["attach"] != "tmux attach -t sym-"+id || res["status"] != LaunchStarting {
		t.Fatalf("返回不对: %v", res)
	}
	accs := srv.Hub.Snapshot().Accounts
	if len(accs) != 1 || accs[0].Name != "前端小王" || accs[0].Seat != 0 {
		t.Fatalf("应新建账号并坐 0 号座位: %+v", accs)
	}
	acc := srv.Hub.Account(accs[0].ID)

	argv := tmux.last("new-session")
	if flagValue(argv, "-c") != dir || flagValue(argv, "-s") != "sym-"+id {
		t.Fatalf("目录或会话名不对: %v", argv)
	}
	cmd := argv[slices.Index(argv, "--")+1:]
	want := []string{"claude", "--plugin-dir", "/mods/sb", "--name", "前端小王", "--model", "sonnet", "--effort", "high", "--permission-mode", "manual", "--continue"}
	if !slices.Equal(cmd, want) {
		t.Fatalf("claude 参数不对:\n got %v\nwant %v", cmd, want)
	}
	for _, env := range []string{"SYMPHONY_BRIDGE=http://127.0.0.1:7788", "SYMPHONY_LAUNCH=" + id, "SYMPHONY_TOKEN=" + acc.Token} {
		if !slices.Contains(argv, env) {
			t.Fatalf("少了环境变量 %s: %v", env, argv)
		}
	}
	if cmdText := res["command"].(string); !strings.Contains(cmdText, "--name 前端小王") || strings.Contains(cmdText, acc.Token) {
		t.Fatalf("给人看的命令不对（要有参数、不能露 token）: %v", cmdText)
	}
	if acc.Last == nil || acc.Last.Dir != dir || acc.Last.Model != "sonnet" || acc.Last.Effort != "high" {
		t.Fatalf("账号应记住这次的启动参数: %+v", acc.Last)
	}

	// 会话凭 token 带着启动编号上线，就和 tmux 对上；先用启动时选的模型和强度
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: dir, Launch: id})
	s := session(t, srv, "s1")
	if s.Launch != id || s.Attach != "tmux attach -t sym-"+id || s.Model != "sonnet" || s.Effort != "high" {
		t.Fatalf("会话没和启动对上: %+v", s)
	}
	waitFor(t, "启动状态变成 online", func() bool { return srv.Hub.launchStatus(id) == LaunchOnline })

	// 在线的账号不能再启动
	if st := web.status("POST", "/api/launch", LaunchRequest{Account: acc.ID, Dir: dir}); st != http.StatusConflict {
		t.Fatalf("在线账号再启动应 409，得到 %d", st)
	}
	// 重名不能新建
	if st := web.status("POST", "/api/launch", LaunchRequest{Name: "前端小王", Dir: dir}); st != http.StatusConflict {
		t.Fatalf("重名应 409，得到 %d", st)
	}
}

func TestLaunchExistingAccountKeepsSeatAndToken(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	web := client{t: t, base: base}
	_, _ = srv.Hub.CreateAccount("占位")
	acc, _ := srv.Hub.CreateAccount("后端老李")
	web.ok("POST", "/api/launch", LaunchRequest{Account: acc.ID, Dir: t.TempDir()})
	argv := tmux.last("new-session")
	if !slices.Contains(argv, "SYMPHONY_TOKEN="+acc.Token) || flagValue(argv, "--name") != "后端老李" {
		t.Fatalf("应沿用这个账号的 token 和名字: %v", argv)
	}
	if v := view(t, srv, acc.ID); v.Seat != 1 || len(srv.Hub.Snapshot().Accounts) != 2 {
		t.Fatalf("不该新建账号，座位不变: %+v", v)
	}
	if st := web.status("POST", "/api/launch", LaunchRequest{Account: "nope", Dir: t.TempDir()}); st != http.StatusNotFound {
		t.Fatalf("不存在的账号应 404，得到 %d", st)
	}
}

func TestLaunchFailedStartDropsNewAccount(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	tmux.failStart = true
	if st := (client{t: t, base: base}).status("POST", "/api/launch", LaunchRequest{Name: "小美", Dir: t.TempDir()}); st != http.StatusInternalServerError {
		t.Fatalf("tmux 起不来应 500，得到 %d", st)
	}
	if n := len(srv.Hub.Snapshot().Accounts); n != 0 {
		t.Fatalf("没起来的新账号应撤掉，现在 %d 个", n)
	}
}

func TestLaunchAcceptsTrustPrompt(t *testing.T) {
	_, base, tmux := newLaunchBridge(t)
	tmux.screen = "Do you trust this folder?\n ❯ No, exit\n   Yes, I trust this folder"
	client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})
	waitFor(t, "替用户点了信任", func() bool { return tmux.count("send-keys") >= 2 })
	time.Sleep(700 * time.Millisecond)
	if n := tmux.count("send-keys"); n != 2 {
		t.Fatalf("信任框应只点一次（Down + Enter），实际 send-keys %d 次", n)
	}
}

func TestLaunchFailsWhenProcessDies(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	tmux.dead = true
	id := client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
	waitFor(t, "启动标成失败", func() bool { return srv.Hub.launchStatus(id) == LaunchFailed })
}

func TestLaunchValidation(t *testing.T) {
	_, base, tmux := newLaunchBridge(t)
	c := client{t: t, base: base}
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	_ = os.WriteFile(file, nil, 0o644)
	for name, req := range map[string]LaunchRequest{
		"新账号没名字": {Dir: dir},
		"名字带换行":  {Name: "a\nb", Dir: dir},
		"相对路径":   {Name: "a", Dir: "relative/dir"},
		"目录不存在":  {Name: "a", Dir: filepath.Join(dir, "nope")},
		"是文件":    {Name: "a", Dir: file},
		"模型不认识":  {Name: "a", Dir: dir, Model: "gpt"},
		"权限不认识":  {Name: "a", Dir: dir, PermissionMode: "yolo"},
	} {
		if st := c.status("POST", "/api/launch", req); st != http.StatusBadRequest {
			t.Errorf("%s 应 400，得到 %d", name, st)
		}
	}
	if tmux.last("new-session") != nil {
		t.Fatal("参数不对时不应启动")
	}
}

func TestLaunchDisabled(t *testing.T) {
	_, base := newBridge(t)
	c := client{t: t, base: base}
	if c.ok("GET", "/api/launch/options", nil)["enabled"] != false {
		t.Fatal("没配 Launcher 时应显示未开启")
	}
	if st := c.status("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()}); st != http.StatusNotImplemented {
		t.Fatalf("应 501，得到 %d", st)
	}
}

func TestDirs(t *testing.T) {
	_, base := newBridge(t)
	c := client{t: t, base: base}
	root := t.TempDir()
	for _, d := range []string{"b", "a", ".hidden"} {
		_ = os.Mkdir(filepath.Join(root, d), 0o755)
	}
	_ = os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o644)
	res := c.ok("GET", "/api/dirs?path="+root, nil)
	dirs := res["dirs"].([]any)
	first, _ := dirs[0].(map[string]any)
	if len(dirs) != 2 || first["name"] != "a" || first["path"] != filepath.Join(root, "a") || res["parent"] != filepath.Dir(root) {
		t.Fatalf("目录列表不对: %v", res)
	}
	if roots, _ := res["roots"].([]any); len(roots) == 0 {
		t.Fatalf("应给出最上层（/ 或盘符）: %v", res["roots"])
	}
	empty := c.ok("GET", "/api/dirs?path="+filepath.Join(root, "a"), nil)
	if d, ok := empty["dirs"].([]any); !ok || len(d) != 0 {
		t.Fatalf("没有子目录时应返回空列表: %v", empty["dirs"])
	}
	if st := c.status("GET", "/api/dirs?path=relative", nil); st != http.StatusBadRequest {
		t.Fatalf("相对路径应 400，得到 %d", st)
	}
}

func TestCrossSiteRequestsRefused(t *testing.T) {
	srv, base := newBridge(t)
	mod, _ := as(t, srv, base, "a")
	id := mod.ok("POST", "/api/ask", AskRequest{Session: "s", Kind: "permission"})["id"].(string)

	evil := client{t: t, base: base, header: map[string]string{"Origin": "https://evil.example"}}
	for path, body := range map[string]any{"/api/answer/" + id: map[string]any{"decision": "allow"}, "/api/launch": LaunchRequest{Name: "x", Dir: "/"}, "/api/accounts": map[string]string{"name": "x"}} {
		if st := evil.status("POST", path, body); st != http.StatusForbidden {
			t.Fatalf("别的网站发来的 %s 应 403，得到 %d", path, st)
		}
	}
	req, _ := http.NewRequest("GET", base+"/api/state", nil)
	req.Host = "evil.example"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Host 不是本机应 403，得到 %d", resp.StatusCode)
	}
	// 本站页面自己发的照常
	client{t: t, base: base, header: map[string]string{"Origin": base}}.ok("POST", "/api/answer/"+id, map[string]any{"decision": "allow"})
}
