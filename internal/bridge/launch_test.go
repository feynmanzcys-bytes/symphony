package bridge

import (
	"context"
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

// fakeTmux records tmux calls; capture-pane returns a preset screen.
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
	t.Fatalf("timed out waiting for %s", what)
}

func flagValue(argv []string, flag string) string {
	if i := slices.Index(argv, flag); i >= 0 && i+1 < len(argv) {
		return argv[i+1]
	}
	return ""
}

// tmuxEnv collects the -e NAME=value pairs of a tmux new-session call.
func tmuxEnv(argv []string) map[string]string {
	env := map[string]string{}
	for i := 0; i+1 < len(argv) && argv[i] != "--"; i++ {
		if argv[i] == "-e" {
			k, v, _ := strings.Cut(argv[i+1], "=")
			env[k] = v
		}
	}
	return env
}

func TestLaunchNewAccount(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	web := client{t: t, base: base}
	dir := t.TempDir()

	res := web.ok("POST", "/api/launch", LaunchRequest{Name: "前端小王", Dir: dir, Model: "sonnet", Effort: "high", PermissionMode: "manual", Resume: "continue"})
	id := res["id"].(string)
	if res["attach"] != "tmux attach -t sym-"+id || res["status"] != LaunchStarting {
		t.Fatalf("wrong response: %v", res)
	}
	accs := srv.Hub.Snapshot().Accounts
	if len(accs) != 1 || accs[0].Name != "前端小王" || accs[0].Seat != 0 {
		t.Fatalf("want a new account in seat 0: %+v", accs)
	}
	acc := srv.Hub.Account(accs[0].ID)

	argv := tmux.last("new-session")
	if flagValue(argv, "-c") != dir || flagValue(argv, "-s") != "sym-"+id {
		t.Fatalf("wrong folder or tmux session name: %v", argv)
	}
	cmd := argv[slices.Index(argv, "--")+1:]
	want := []string{"claude", "--plugin-dir", "/mods/sb", "--name", "前端小王", "--model", "sonnet", "--effort", "high", "--permission-mode", "manual", "--continue"}
	if !slices.Equal(cmd, want) {
		t.Fatalf("wrong claude arguments:\n got %v\nwant %v", cmd, want)
	}
	for _, env := range []string{"SYMPHONY_BRIDGE=http://127.0.0.1:7788", "SYMPHONY_LAUNCH=" + id, "SYMPHONY_TOKEN=" + acc.Token} {
		if !slices.Contains(argv, env) {
			t.Fatalf("missing environment variable %s: %v", env, argv)
		}
	}
	if cmdText := res["command"].(string); !strings.Contains(cmdText, "--name 前端小王") || strings.Contains(cmdText, acc.Token) {
		t.Fatalf("wrong command for people to read (needs the arguments, must hide the token): %v", cmdText)
	}
	if acc.Last == nil || acc.Last.Dir != dir || acc.Last.Model != "sonnet" || acc.Last.Effort != "high" {
		t.Fatalf("the account should remember these launch options: %+v", acc.Last)
	}

	// the session comes online with the token and the launch id, which ties it to its tmux;
	// until it reports its own model and effort, it shows the ones picked at launch
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: dir, Launch: id})
	s := session(t, srv, "s1")
	if s.Launch != id || s.Attach != "tmux attach -t sym-"+id || s.Model != "sonnet" || s.Effort != "high" {
		t.Fatalf("the session wasn't matched to its launch: %+v", s)
	}
	waitFor(t, "the launch to come online", func() bool { return srv.Hub.launchStatus(id) == LaunchOnline })

	// an online account can't be launched again
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Account: acc.ID, Dir: dir}); st != http.StatusConflict || code != "account_busy" {
		t.Fatalf("launching an online account should get 409 account_busy, got %d %q", st, code)
	}
	// a new account can't take a name in use
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Name: "前端小王", Dir: dir}); st != http.StatusConflict || code != "account_name_taken" {
		t.Fatalf("a duplicate name should get 409 account_name_taken, got %d %q", st, code)
	}
}

func TestLaunchExistingAccountKeepsSeatAndToken(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	web := client{t: t, base: base}
	_, _ = srv.Hub.CreateAccount("占位") // takes seat 0
	acc, _ := srv.Hub.CreateAccount("后端老李")
	web.ok("POST", "/api/launch", LaunchRequest{Account: acc.ID, Dir: t.TempDir()})
	argv := tmux.last("new-session")
	if !slices.Contains(argv, "SYMPHONY_TOKEN="+acc.Token) || flagValue(argv, "--name") != "后端老李" {
		t.Fatalf("the launch should reuse this account's token and name: %v", argv)
	}
	if v := view(t, srv, acc.ID); v.Seat != 1 || len(srv.Hub.Snapshot().Accounts) != 2 {
		t.Fatalf("no new account, and the seat stays: %+v", v)
	}
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Account: "nope", Dir: t.TempDir()}); st != http.StatusNotFound || code != "account_unknown" {
		t.Fatalf("an unknown account should get 404 account_unknown, got %d %q", st, code)
	}
}

func TestLaunchFailedStartDropsNewAccount(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	tmux.failStart = true
	st, body := client{t: t, base: base}.do("POST", "/api/launch", LaunchRequest{Name: "小美", Dir: t.TempDir()})
	params, _ := body["params"].(map[string]any)
	if detail, _ := params["detail"].(string); st != http.StatusInternalServerError || body["code"] != "tmux_start_failed" || !strings.Contains(detail, "duplicate session") {
		t.Fatalf("a tmux that won't start should get 500 tmux_start_failed with its output, got %d %v", st, body)
	}
	if n := len(srv.Hub.Snapshot().Accounts); n != 0 {
		t.Fatalf("the new account should be dropped when the start fails, found %d", n)
	}
}

func TestLaunchAcceptsTrustPrompt(t *testing.T) {
	_, base, tmux := newLaunchBridge(t)
	tmux.screen = "Do you trust this folder?\n ❯ No, exit\n   Yes, I trust this folder"
	client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})
	waitFor(t, "the trust prompt to be answered", func() bool { return tmux.count("send-keys") >= 2 })
	time.Sleep(700 * time.Millisecond)
	if n := tmux.count("send-keys"); n != 2 {
		t.Fatalf("the trust prompt should be answered once (Down + Enter), got %d send-keys", n)
	}
}

func TestLaunchFailsWhenProcessDies(t *testing.T) {
	srv, base, tmux := newLaunchBridge(t)
	tmux.dead = true
	c := client{t: t, base: base}
	id := c.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
	waitFor(t, "the launch to be marked failed", func() bool { return srv.Hub.launchStatus(id) == LaunchFailed })
	// the page shows why in its own language, from errorCode
	var launch map[string]any
	for _, l := range c.ok("GET", "/api/state", nil)["launches"].([]any) {
		if l, _ := l.(map[string]any); l["id"] == id {
			launch = l
		}
	}
	if launch["errorCode"] != "launch_process_exited" {
		t.Fatalf("the failed launch should carry errorCode launch_process_exited: %v", launch)
	}
}

func TestLaunchValidation(t *testing.T) {
	_, base, tmux := newLaunchBridge(t)
	c := client{t: t, base: base}
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	_ = os.WriteFile(file, nil, 0o644)
	missing := filepath.Join(dir, "nope")
	for name, tc := range map[string]struct {
		req  LaunchRequest
		code string
	}{
		"new account without a name": {LaunchRequest{Dir: dir}, "account_bad_name"},
		"name with a newline":        {LaunchRequest{Name: "a\nb", Dir: dir}, "account_bad_name"},
		"relative folder":            {LaunchRequest{Name: "a", Dir: "relative/dir"}, "launch_dir_not_absolute"},
		"missing folder":             {LaunchRequest{Name: "a", Dir: missing}, "launch_dir_missing"},
		"a file, not a folder":       {LaunchRequest{Name: "a", Dir: file}, "launch_dir_missing"},
		"unknown model":              {LaunchRequest{Name: "a", Dir: dir, Model: "gpt"}, "launch_bad_option"},
		"unknown permission mode":    {LaunchRequest{Name: "a", Dir: dir, PermissionMode: "yolo"}, "launch_bad_option"},
	} {
		if st, code := c.failure("POST", "/api/launch", tc.req); st != http.StatusBadRequest || code != tc.code {
			t.Errorf("%s: want 400 %s, got %d %q", name, tc.code, st, code)
		}
	}
	// the page puts the folder into its message
	_, body := c.do("POST", "/api/launch", LaunchRequest{Name: "a", Dir: missing})
	if params, _ := body["params"].(map[string]any); params["dir"] != missing {
		t.Errorf("a missing folder should come back in params.dir: %v", body)
	}
	if tmux.last("new-session") != nil {
		t.Fatal("nothing should start with bad options")
	}
}

func TestLaunchPassesLanguage(t *testing.T) {
	_, base, tmux := newLaunchBridge(t)
	c := client{t: t, base: base}
	for _, tc := range []struct{ lang, want string }{
		{"zh-CN", "zh-CN"},
		{"pt", "pt-BR"}, // a bare language maps to the variant the bridge supports
		{"xx", ""},      // unsupported: left out
		{"", ""},
	} {
		c.ok("POST", "/api/launch", LaunchRequest{Name: "lang " + tc.lang, Dir: t.TempDir(), Lang: tc.lang})
		got, set := tmuxEnv(tmux.last("new-session"))["SYMPHONY_LANG"]
		if got != tc.want || set != (tc.want != "") {
			t.Errorf("Lang %q: want SYMPHONY_LANG %q, got %q (set: %v)", tc.lang, tc.want, got, set)
		}
	}
}

func TestLaunchDisabled(t *testing.T) {
	_, base := newBridge(t)
	c := client{t: t, base: base}
	if opts := c.ok("GET", "/api/launch/options", nil); opts["enabled"] != false || opts["mod"] != nil {
		t.Fatalf("without a Launcher, page launches should be off and mod null: %v", opts)
	}
	if st, code := c.failure("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()}); st != http.StatusNotImplemented || code != "launch_disabled" {
		t.Fatalf("want 501 launch_disabled, got %d %q", st, code)
	}
}

// fakePlugins stands in for `claude plugin list --json`.
type fakePlugins struct {
	mu    sync.Mutex
	out   string
	err   error
	calls int
}

func (f *fakePlugins) list(context.Context) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return []byte(f.out), f.err
}

func (f *fakePlugins) set(out string) { f.mu.Lock(); f.out = out; f.mu.Unlock() }

const installedMod = `[{"id":"cowork@synced","version":"1.0.0","scope":"synced","enabled":true},
	{"id":"symphony-bridge@symphony","version":"0.2.0","scope":"user","enabled":true}]`

func TestLaunchUsesInstalledMod(t *testing.T) {
	srv := NewServer(NewHub(), testWeb)
	tmux := &fakeTmux{}
	plugins := &fakePlugins{out: `[]`}
	srv.SetLauncher(&Launcher{Claude: "claude", Market: "/src/symphony", BridgeURL: "http://127.0.0.1:7788",
		Term: &tmuxTerminal{run: tmux.run}, OnlineTimeout: 3 * time.Second, ListPlugins: plugins.list})
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	c := client{t: t, base: ts.URL}
	dir := t.TempDir()
	install := "claude plugin install symphony-bridge --marketplace /src/symphony"

	// not installed: page launches are off and the page says how to install the mod;
	// a launch anyway is refused, with no account and no session
	opts := c.ok("GET", "/api/launch/options", nil)
	mod, _ := opts["mod"].(map[string]any)
	if opts["enabled"] != false || mod["ok"] != false || mod["code"] != "mod_not_installed" || mod["fix"] != install {
		t.Fatalf("without the mod, page launches should be off, with the install command: %v", opts)
	}
	if st, body := c.do("POST", "/api/launch", LaunchRequest{Name: "a", Dir: dir}); st != http.StatusPreconditionFailed ||
		body["code"] != "mod_not_installed" || body["fix"] != install {
		t.Fatalf("a launch without the mod should get 412 mod_not_installed with the fix, got %d %v", st, body)
	}
	if len(srv.Hub.Snapshot().Accounts) != 0 || tmux.last("new-session") != nil {
		t.Fatal("without the mod, no account and no session should be created")
	}

	// once installed, no bridge restart is needed; sessions use the installed mod, without --plugin-dir
	plugins.set(installedMod)
	opts = c.ok("GET", "/api/launch/options", nil)
	if mod, _ := opts["mod"].(map[string]any); opts["enabled"] != true || mod["ok"] != true || mod["code"] != nil ||
		mod["id"] != "symphony-bridge@symphony" || mod["version"] != "0.2.0" {
		t.Fatalf("once installed, page launches should be on: %v", opts)
	}
	res := c.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: dir})
	argv := tmux.last("new-session")
	if cmd, want := argv[slices.Index(argv, "--")+1:], []string{"claude", "--name", "a"}; !slices.Equal(cmd, want) {
		t.Fatalf("wrong claude arguments:\n got %v\nwant %v", cmd, want)
	}
	if strings.Contains(res["command"].(string), "--plugin-dir") {
		t.Fatalf("with the installed mod there should be no --plugin-dir: %v", res["command"])
	}
}

func TestModStatus(t *testing.T) {
	ctx := context.Background()
	github := "claude plugin install symphony-bridge --marketplace feynmanzcys-bytes/symphony"
	for _, tc := range []struct {
		name, out string
		err       error
		ok        bool
		code, fix string
		params    map[string]string // expected among the status's params
	}{
		{"claude can't run", "", errors.New("boom"), false, "mod_check_failed", "", map[string]string{"detail": "boom"}},
		{"output isn't JSON", "oops", nil, false, "mod_check_failed", "", nil},
		{"not installed", `[{"id":"other@x","scope":"user","enabled":true}]`, nil, false, "mod_not_installed", github, nil},
		{"installed but disabled", `[{"id":"symphony-bridge@symphony","scope":"user","enabled":false}]`, nil, false, "mod_disabled", "claude plugin enable symphony-bridge@symphony", nil},
		{"installed for one project only", `[{"id":"symphony-bridge@symphony","scope":"project","enabled":true}]`, nil, false, "mod_project_scope", github, map[string]string{"scope": "project"}},
		{"installed (any marketplace counts)", `[{"id":"symphony-bridge@fork","version":"0.3.0","scope":"user","enabled":true}]`, nil, true, "", "", nil},
	} {
		m := (&Launcher{ListPlugins: (&fakePlugins{out: tc.out, err: tc.err}).list}).Mod(ctx)
		// a problem keeps an English text for logs and API callers
		if m.OK != tc.ok || m.Code != tc.code || m.Fix != tc.fix || (m.Error == "") != tc.ok {
			t.Errorf("%s: %+v", tc.name, m)
		}
		for k, v := range tc.params {
			if m.Params[k] != v {
				t.Errorf("%s: params[%s] = %q, want %q", tc.name, k, m.Params[k], v)
			}
		}
	}

	// a local repository path with spaces is quoted
	l := &Launcher{Market: "/My Code/symphony", ListPlugins: (&fakePlugins{out: `[]`}).list}
	if fix := l.Mod(ctx).Fix; fix != `claude plugin install symphony-bridge --marketplace "/My Code/symphony"` {
		t.Fatalf("wrong install command: %s", fix)
	}

	// development mode (-mod) doesn't check the install
	f := &fakePlugins{err: errors.New("should not be called")}
	if m := (&Launcher{ModDir: "/mods/sb", ListPlugins: f.list}).Mod(ctx); !m.OK || !m.Dev || f.calls != 0 {
		t.Fatalf("development mode should be ready without a check: %+v (checked %d times)", m, f.calls)
	}

	// a bad answer is checked again every time (install, then reload the page); a good one is kept for a while
	f = &fakePlugins{out: `[]`}
	l = &Launcher{ListPlugins: f.list}
	l.Mod(ctx)
	l.Mod(ctx)
	f.set(installedMod)
	if !l.Mod(ctx).OK {
		t.Fatal("the install should be noticed right away")
	}
	l.Mod(ctx)
	if f.calls != 3 {
		t.Fatalf("want 3 checks, got %d", f.calls)
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
		t.Fatalf("wrong folder list: %v", res)
	}
	if roots, _ := res["roots"].([]any); len(roots) == 0 {
		t.Fatalf("want the top levels (/ or drive letters): %v", res["roots"])
	}
	empty := c.ok("GET", "/api/dirs?path="+filepath.Join(root, "a"), nil)
	if d, ok := empty["dirs"].([]any); !ok || len(d) != 0 {
		t.Fatalf("with no subfolders the list should be empty: %v", empty["dirs"])
	}
	if st, code := c.failure("GET", "/api/dirs?path=relative", nil); st != http.StatusBadRequest || code != "dirs_not_absolute" {
		t.Fatalf("a relative path should get 400 dirs_not_absolute, got %d %q", st, code)
	}
}

func TestCrossSiteRequestsRefused(t *testing.T) {
	srv, base := newBridge(t)
	mod, _ := as(t, srv, base, "a")
	id := mod.ok("POST", "/api/ask", AskRequest{Session: "s", Kind: "permission"})["id"].(string)

	evil := client{t: t, base: base, header: map[string]string{"Origin": "https://evil.example"}}
	for path, body := range map[string]any{"/api/answer/" + id: map[string]any{"decision": "allow"}, "/api/launch": LaunchRequest{Name: "x", Dir: "/"}, "/api/accounts": map[string]string{"name": "x"}} {
		if st := evil.status("POST", path, body); st != http.StatusForbidden {
			t.Fatalf("%s from another site should get 403, got %d", path, st)
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
		t.Fatalf("a Host that isn't this machine should get 403, got %d", resp.StatusCode)
	}
	// the page's own requests go through
	client{t: t, base: base, header: map[string]string{"Origin": base}}.ok("POST", "/api/answer/"+id, map[string]any{"decision": "allow"})
}
