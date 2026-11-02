package bridge

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProc stands in for a process on a pseudo-terminal: the test writes its output and reads its input.
type fakeProc struct {
	out    *io.PipeReader
	outW   *io.PipeWriter
	mu     sync.Mutex
	input  []byte
	size   [2]int
	exited chan struct{}
	once   sync.Once
	err    error
}

func newFakeProc() *fakeProc {
	r, w := io.Pipe()
	return &fakeProc{out: r, outW: w, exited: make(chan struct{})}
}

func (p *fakeProc) Read(b []byte) (int, error) { return p.out.Read(b) }
func (p *fakeProc) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.input = append(p.input, b...)
	return len(b), nil
}
func (p *fakeProc) Resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.size = [2]int{cols, rows}
	return nil
}
func (p *fakeProc) Wait() error  { <-p.exited; return p.err }
func (p *fakeProc) Close() error { return p.out.Close() }
func (p *fakeProc) Kill() error  { p.exit(errors.New("killed")); return nil }
func (p *fakeProc) exit(err error) {
	p.once.Do(func() { p.err = err; p.outW.Close(); close(p.exited) })
}
func (p *fakeProc) typed() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return string(p.input)
}

type start struct {
	id, name, dir string
	env, argv     []string
}

// fakeStarts records the sessions started on (fake) pseudo-terminals.
type fakeStarts struct {
	mu        sync.Mutex
	starts    []start
	procs     map[string]*fakeProc
	screen    string // what the process prints first
	dead      bool   // the process exits at once
	failStart bool
}

func (f *fakeStarts) start(id, name, dir string, env []string, argv []string) (*Term, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failStart {
		return nil, errors.New("fork/exec: no such file")
	}
	f.starts = append(f.starts, start{id, name, dir, env, argv})
	p := newFakeProc()
	if f.procs == nil {
		f.procs = map[string]*fakeProc{}
	}
	f.procs[id] = p
	t := hostTerm(id, name, p)
	if f.screen != "" {
		go p.outW.Write([]byte(f.screen))
	}
	if f.dead {
		go func() { time.Sleep(10 * time.Millisecond); p.exit(errors.New("exit status 1")) }()
	}
	return t, nil
}

func (f *fakeStarts) last() *start {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.starts) == 0 {
		return nil
	}
	return &f.starts[len(f.starts)-1]
}

func (f *fakeStarts) proc(id string) *fakeProc {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.procs[id]
}

func fakeTerminal(f *fakeStarts) *ptyTerminal {
	return &ptyTerminal{Terms: &Terms{}, start: f.start, program: "symphony"}
}

func newLaunchBridge(t *testing.T) (*Server, string, *fakeStarts) {
	t.Helper()
	srv := NewServer(NewHub(), testWeb)
	srv.Key = testKey
	f := &fakeStarts{}
	srv.SetLauncher(NewLauncher("claude", "/mods/sb", "http://127.0.0.1:7788", fakeTerminal(f)))
	srv.Launcher.OnlineTimeout = 3 * time.Second
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	return srv, ts.URL, f
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

// envOf collects NAME=value pairs.
func envMap(env []string) map[string]string {
	m := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		m[k] = v
	}
	return m
}

// TestLaunchAttachHint: the program's attach command (with its address and data folder) is what the launch, the session
// and a session that reports in after a restart carry, not the backend's bare one.
func TestLaunchAttachHint(t *testing.T) {
	srv, base, _ := newLaunchBridge(t)
	srv.Launcher.AttachHint = func(id string) string { return "symphony attach -addr 127.0.0.1:7799 " + id }
	web := client{t: t, base: base}
	res := web.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})
	id := res["id"].(string)
	want := "symphony attach -addr 127.0.0.1:7799 " + id
	if res["attach"] != want {
		t.Fatalf("the launch's attach: %v", res["attach"])
	}
	acc := srv.Hub.Account(srv.Hub.Snapshot().Accounts[0].ID)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: id})
	if s := session(t, srv, "s1"); s.Attach != want {
		t.Fatalf("the session's attach: %q", s.Attach)
	}
	_, b := as(t, srv, base, "b")
	modB := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + b.Token}, modOnly: true}
	modB.ok("POST", "/api/event", Event{Session: "s2", Kind: "heartbeat", Launch: "gone"}) // no launch record: after a restart
	if s := session(t, srv, "s2"); s.Attach != "symphony attach -addr 127.0.0.1:7799 gone" {
		t.Fatalf("after a restart: %q", s.Attach)
	}
}

func TestLaunchNewAccount(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	web := client{t: t, base: base}
	dir := t.TempDir()

	res := web.ok("POST", "/api/launch", LaunchRequest{Name: "前端小王", Dir: dir, Model: "sonnet", Effort: "high", PermissionMode: "manual", Resume: "continue"})
	id := res["id"].(string)
	if res["attach"] != "symphony attach "+id || res["status"] != LaunchStarting {
		t.Fatalf("wrong response: %v", res)
	}
	accs := srv.Hub.Snapshot().Accounts
	if len(accs) != 1 || accs[0].Name != "前端小王" || accs[0].Seat != 0 {
		t.Fatalf("want a new account in seat 0: %+v", accs)
	}
	acc := srv.Hub.Account(accs[0].ID)

	st := starts.last()
	if st == nil || st.dir != dir || st.id != id || st.name != "前端小王" {
		t.Fatalf("wrong folder, id or name: %+v", st)
	}
	want := []string{"claude", "--plugin-dir", "/mods/sb", "--name", "前端小王", "--model", "sonnet", "--effort", "high", "--permission-mode", "manual", "--continue"}
	if !slices.Equal(st.argv, want) {
		t.Fatalf("wrong claude arguments:\n got %v\nwant %v", st.argv, want)
	}
	for _, env := range []string{"SYMPHONY_BRIDGE=http://127.0.0.1:7788", "SYMPHONY_LAUNCH=" + id, "SYMPHONY_TOKEN=" + acc.Token} {
		if !slices.Contains(st.env, env) {
			t.Fatalf("missing environment variable %s: %v", env, st.env)
		}
	}
	if !srv.Launcher.Terms.Live(id) {
		t.Fatal("the launch should have a live terminal")
	}
	if cmdText := res["command"].(string); !strings.Contains(cmdText, "--name 前端小王") || strings.Contains(cmdText, acc.Token) {
		t.Fatalf("wrong command for people to read (needs the arguments, must hide the token): %v", cmdText)
	}
	if acc.Last == nil || acc.Last.Dir != dir || acc.Last.Model != "sonnet" || acc.Last.Effort != "high" {
		t.Fatalf("the account should remember these launch options: %+v", acc.Last)
	}

	// the session comes online with the token and the launch id, which ties it to its terminal;
	// until it reports its own model and effort, it shows the ones picked at launch
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: dir, Launch: id})
	s := session(t, srv, "s1")
	if s.Launch != id || s.Attach != "symphony attach "+id || s.Model != "sonnet" || s.Effort != "high" {
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
	srv, base, starts := newLaunchBridge(t)
	web := client{t: t, base: base}
	_, _ = srv.Hub.CreateAccount("占位") // takes seat 0
	acc, _ := srv.Hub.CreateAccount("后端老李")
	web.ok("POST", "/api/launch", LaunchRequest{Account: acc.ID, Dir: t.TempDir()})
	st := starts.last()
	if !slices.Contains(st.env, "SYMPHONY_TOKEN="+acc.Token) || flagValue(st.argv, "--name") != "后端老李" {
		t.Fatalf("the launch should reuse this account's token and name: %+v", st)
	}
	if v := view(t, srv, acc.ID); v.Seat != 1 || len(srv.Hub.Snapshot().Accounts) != 2 {
		t.Fatalf("no new account, and the seat stays: %+v", v)
	}
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Account: "nope", Dir: t.TempDir()}); st != http.StatusNotFound || code != "account_unknown" {
		t.Fatalf("an unknown account should get 404 account_unknown, got %d %q", st, code)
	}
}

func TestLaunchFailedStartDropsNewAccount(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	starts.failStart = true
	st, body := client{t: t, base: base}.do("POST", "/api/launch", LaunchRequest{Name: "小美", Dir: t.TempDir()})
	params, _ := body["params"].(map[string]any)
	if detail, _ := params["detail"].(string); st != http.StatusInternalServerError || body["code"] != "pty_start_failed" || !strings.Contains(detail, "no such file") || params["program"] != "claude" {
		t.Fatalf("a claude that won't start should get 500 pty_start_failed with the reason, got %d %v", st, body)
	}
	if n := len(srv.Hub.Snapshot().Accounts); n != 0 {
		t.Fatalf("the new account should be dropped when the start fails, found %d", n)
	}
}

// trustPrompt is how claude asks whether to trust a new folder, the cursor on "No, exit"; trustYes and trustNo are the
// two lines drawn again when the cursor moves.
const (
	trustPrompt = "\x1b[1mDo you \x1b[0mtrust this folder?\r\n\x1b[2G❯\x1b[4GNo,\x1b[8Gexit\r\n\x1b[4GYes,\x1b[9GI\x1b[11Gtrust\x1b[17Gthis\x1b[22Gfolder\r\n"
	trustYes    = "\x1b[3A\r \x1b[4GNo, exit\r\n\x1b[2G❯\x1b[4GYes, I trust this folder\r\n"
	trustNo     = "\x1b[3A\r\x1b[2G❯\x1b[4GNo, exit\r\n \x1b[4GYes, I trust this folder\r\n"
)

// redrawOnDown draws what draw says after each Down the process gets (the nth one, from 1), as claude moves its cursor.
func redrawOnDown(t *testing.T, p *fakeProc, draw func(n int) string) {
	go func() {
		seen := 0
		for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			n := strings.Count(p.typed(), "\x1b[B")
			for ; seen < n; seen++ {
				if _, err := p.outW.Write([]byte(draw(seen + 1))); err != nil {
					return
				}
			}
			if strings.Contains(p.typed(), "\r") {
				return
			}
		}
	}()
}

func TestLaunchAcceptsTrustPrompt(t *testing.T) {
	_, base, starts := newLaunchBridge(t)
	starts.screen = trustPrompt
	id := client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
	redrawOnDown(t, starts.proc(id), func(int) string { return trustYes })
	waitFor(t, "the trust prompt to be answered", func() bool { return strings.HasSuffix(starts.proc(id).typed(), "\r") })
	time.Sleep(700 * time.Millisecond)
	if typed := starts.proc(id).typed(); typed != "\x1b[B\r" {
		t.Fatalf("the trust prompt should be answered once (Down, then Enter on Yes), got %q", typed)
	}
}

// TestLaunchTrustWaitsForTheCursor: claude may miss the first key after it drew the question, or draw it again with the
// cursor back on "No, exit"; Enter comes only once the screen shows the cursor on "Yes, I trust this folder".
func TestLaunchTrustWaitsForTheCursor(t *testing.T) {
	for name, draw := range map[string]func(n int) string{
		"the first Down missed": func(n int) string {
			if n == 1 {
				return ""
			}
			return trustYes
		},
		"the cursor back on No": func(n int) string {
			if n == 1 {
				return trustYes + trustNo
			}
			return trustYes
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, base, starts := newLaunchBridge(t)
			starts.screen = trustPrompt
			id := client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
			redrawOnDown(t, starts.proc(id), draw)
			waitFor(t, "the trust prompt to be answered", func() bool { return strings.HasSuffix(starts.proc(id).typed(), "\r") })
			time.Sleep(700 * time.Millisecond)
			if typed := starts.proc(id).typed(); typed != "\x1b[B\x1b[B\r" {
				t.Fatalf("want Down, Down, then Enter once the cursor is on Yes, got %q", typed)
			}
		})
	}
}

// TestLaunchTrustUnsure: a screen that doesn't say where the cursor is (another glyph, or a console that draws only
// the cells that changed): the answer waits a moment after the question showed, presses Down, then Enter.
func TestLaunchTrustUnsure(t *testing.T) {
	for name, c := range map[string]struct{ prompt, redraw string }{
		"another glyph":                 {"Do you trust this folder?\r\n > No, exit\r\n   Yes, I trust this folder\r\n", "\x1b[2A\r   No, exit\r\n > Yes, I trust this folder\r\n"},
		"a console that draws a change": {trustPrompt, "\x1b[5;2H \x1b[6;2H❯\x1b[8;1H"},
	} {
		t.Run(name, func(t *testing.T) {
			_, base, starts := newLaunchBridge(t)
			starts.screen = c.prompt
			began := time.Now()
			id := client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
			redrawOnDown(t, starts.proc(id), func(int) string { return c.redraw })
			waitFor(t, "the trust prompt to be answered", func() bool { return strings.HasSuffix(starts.proc(id).typed(), "\r") })
			time.Sleep(700 * time.Millisecond)
			if typed := starts.proc(id).typed(); typed != "\x1b[B\r" {
				t.Fatalf("want one Down, then Enter, got %q", typed)
			}
			if c.prompt != trustPrompt && time.Since(began) < trustSettle {
				t.Fatalf("the Down should wait %s after the question, the answer took %s", trustSettle, time.Since(began))
			}
		})
	}
}

// TestLaunchTrustConPTY: the trust prompt as the Windows pseudo console drew it (Claude Code 2.1.316, Windows 11
// 26200): placed with cursor positions, the gaps as moves to the right, and after a Down both lines drawn again.
func TestLaunchTrustConPTY(t *testing.T) {
	prompt := "\x1b[?9001h\x1b[?1004h\x1b[?25l\x1b[2J\x1b[m\x1b[H\x1b]0;claude\x07\x1b[?25h\x1b[?2004h\x1b[?2031h\x1b[?1004h\x1b[?25l\x1b[>0q\x1b[?u\x1b]7501;?\x07\x1b[93m\r\n" +
		strings.Repeat("─", 120) + "\x1b[1m\x1b[3;2HAccessing\x1b[1Cworkspace:\x1b[m\x1b[1m\x1b[5;2HC:\\Users\\gb\\AppData\\Local\\Temp\\work-\x1b[6;2Hraw-83933699\x1b[22m" +
		"\x1b[8;2HQuick\x1b[1Csafety\x1b[1Ccheck:\x1b[1CIs\x1b[1Cthis\x1b[1Ca\x1b[1Cproject\x1b[1Cyou\x1b[1Ccreated\x1b[1Cor\x1b[1Cone\x1b[1Cyou\x1b[1Ctrust?" +
		"\x1b[11;2HClaude\x1b[1CCode'll\x1b[1Cbe\x1b[1Cable\x1b[1Cto\x1b[1Cread,\x1b[1Cedit,\x1b[1Cand\x1b[1Cexecute\x1b[1Cfiles\x1b[1Chere.\x1b[37m\x1b[2m\x1b[13;2HSecurity\x1b[1Cguide" +
		"\x1b[94m\x1b[22m\x1b[15;2H❯\x1b[1CNo,\x1b[1Cexit\x1b[m\x1b[16;4HYes,\x1b[1CI\x1b[1Ctrust\x1b[1Cthis\x1b[1Cfolder\x1b[37m\x1b[2m\x1b[18;2HEnter\x1b[1Cto\x1b[1Cconfirm\x1b[1C·\x1b[1CEsc\x1b[1Cto\x1b[1Ccancel\x1b[m"
	afterDown := "\x1b[15;2H \x1b[1CNo, exit\x1b[94m\x1b[16;2H❯\x1b[1CYes, I trust this folder\x1b[m"
	if got := trustCursor(plainText([]byte(prompt))); got != "no" {
		t.Fatalf("the question as drawn first: the cursor on %q, want no", got)
	}
	if got := trustCursor(plainText([]byte(prompt + afterDown))); got != "yes" {
		t.Fatalf("after a Down: the cursor on %q, want yes", got)
	}
	_, base, starts := newLaunchBridge(t)
	starts.screen = prompt
	id := client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
	redrawOnDown(t, starts.proc(id), func(int) string { return afterDown })
	waitFor(t, "the trust prompt to be answered", func() bool { return strings.HasSuffix(starts.proc(id).typed(), "\r") })
	time.Sleep(700 * time.Millisecond)
	if typed := starts.proc(id).typed(); typed != "\x1b[B\r" {
		t.Fatalf("want Down, then Enter on Yes, got %q", typed)
	}
}

// TestLaunchTrustGivesUp: a prompt whose cursor never reaches "Yes" gets a few Downs and never Enter, which would take
// "No, exit"; the launch times out with its screen.
func TestLaunchTrustGivesUp(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	srv.Launcher.OnlineTimeout = 6 * time.Second
	starts.screen = trustPrompt
	id := client{t: t, base: base}.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: t.TempDir()})["id"].(string)
	waitFor(t, "the Downs", func() bool { return strings.Count(starts.proc(id).typed(), "\x1b[B") == trustDowns })
	waitFor(t, "the launch to time out", func() bool { return srv.Hub.launchStatus(id) == LaunchFailed })
	if typed := starts.proc(id).typed(); typed != strings.Repeat("\x1b[B", trustDowns) {
		t.Fatalf("want %d Downs and no Enter, got %q", trustDowns, typed)
	}
}

func TestLaunchFailsWhenProcessDies(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	starts.dead = true
	starts.screen = "claude: unknown option --nope\n"
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
	params, _ := launch["errorParams"].(map[string]any)
	if launch["errorCode"] != "launch_process_exited" || params["detail"] != "exit status 1" || !strings.Contains(params["screen"].(string), "unknown option") {
		t.Fatalf("the failed launch should carry errorCode launch_process_exited, the exit status and the screen: %v", launch)
	}
}

func TestLaunchValidation(t *testing.T) {
	_, base, starts := newLaunchBridge(t)
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
	if starts.last() != nil {
		t.Fatal("nothing should start with bad options")
	}
}

func TestLaunchPassesLanguage(t *testing.T) {
	_, base, starts := newLaunchBridge(t)
	c := client{t: t, base: base}
	for _, tc := range []struct{ lang, want string }{
		{"zh-CN", "zh-CN"},
		{"pt", "pt-BR"}, // a bare language maps to the variant the bridge supports
		{"xx", ""},      // unsupported: left out
		{"", ""},
	} {
		c.ok("POST", "/api/launch", LaunchRequest{Name: "lang " + tc.lang, Dir: t.TempDir(), Lang: tc.lang})
		got, set := envMap(starts.last().env)["SYMPHONY_LANG"]
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
	srv.Key = testKey
	starts := &fakeStarts{}
	plugins := &fakePlugins{out: `[]`}
	now := time.Now()
	srv.SetLauncher(&Launcher{Claude: "claude", Market: "/src/symphony", BridgeURL: "http://127.0.0.1:7788",
		Term: fakeTerminal(starts), OnlineTimeout: 3 * time.Second, ListPlugins: plugins.list,
		now: func() time.Time { return now }})
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
	if len(srv.Hub.Snapshot().Accounts) != 0 || starts.last() != nil {
		t.Fatal("without the mod, no account and no session should be created")
	}

	// once installed, no bridge restart is needed (the bad answer is kept only briefly); sessions use the installed
	// mod, without --plugin-dir
	plugins.set(installedMod)
	now = now.Add(modBadTTL)
	opts = c.ok("GET", "/api/launch/options", nil)
	if mod, _ := opts["mod"].(map[string]any); opts["enabled"] != true || mod["ok"] != true || mod["code"] != nil ||
		mod["id"] != "symphony-bridge@symphony" || mod["version"] != "0.2.0" {
		t.Fatalf("once installed, page launches should be on: %v", opts)
	}
	res := c.ok("POST", "/api/launch", LaunchRequest{Name: "a", Dir: dir})
	if cmd, want := starts.last().argv, []string{"claude", "--name", "a"}; !slices.Equal(cmd, want) {
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
		// read from a folder marketplace: sessions load the folder's version, not the one recorded at install
		{"installed from a folder that moved on", `[{"id":"symphony-bridge@symphony","version":"0.2.0","folderVersion":"0.3.0","readFromFolder":"/src/symphony/mods/symphony-bridge","scope":"user","enabled":true}]`, nil, true, "", "", nil},
	} {
		m := (&Launcher{ListPlugins: (&fakePlugins{out: tc.out, err: tc.err}).list}).Mod(ctx)
		if tc.ok && m.Version != "0.3.0" {
			t.Errorf("%s: version %q, want 0.3.0", tc.name, m.Version)
		}
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

	// a bad answer is kept briefly (opening the page doesn't run claude every time), then checked again (install,
	// then reload the page); a good one is kept for a minute
	now := time.Now()
	f = &fakePlugins{out: `[]`}
	l = &Launcher{ListPlugins: f.list, now: func() time.Time { return now }}
	l.Mod(ctx)
	l.Mod(ctx)
	if f.calls != 1 {
		t.Fatalf("a bad answer should be kept for a moment, got %d checks", f.calls)
	}
	f.set(installedMod)
	now = now.Add(modBadTTL)
	if !l.Mod(ctx).OK {
		t.Fatal("the install should be noticed once the bad answer has aged")
	}
	now = now.Add(modGoodTTL - time.Second)
	l.Mod(ctx)
	now = now.Add(2 * time.Second)
	l.Mod(ctx)
	if f.calls != 3 {
		t.Fatalf("want 3 checks, got %d", f.calls)
	}
}

// TestModCheckOutlivesCaller: the page that set off the check may go away meanwhile (a reload, another theme); the
// check runs on, and neither that page nor the next is told the mod can't be checked.
func TestModCheckOutlivesCaller(t *testing.T) {
	var calls atomic.Int32
	l := &Launcher{ListPlugins: func(ctx context.Context) ([]byte, error) {
		calls.Add(1)
		select {
		case <-ctx.Done():
			return nil, errors.New("signal: killed")
		case <-time.After(300 * time.Millisecond):
			return []byte(installedMod), nil
		}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	if m := l.Mod(ctx); !m.OK {
		t.Fatalf("the first page's check: %+v", m)
	}
	if m := l.Mod(context.Background()); !m.OK || calls.Load() != 1 {
		t.Fatalf("the next page's: %+v (%d checks)", m, calls.Load())
	}
}

// TestModMarket: a folder is taken for the mod's marketplace (the install hint's local path) only when its
// marketplace offers the mod; another plugin marketplace isn't.
func TestModMarket(t *testing.T) {
	market := func(content string) string {
		dir := t.TempDir()
		if content != "" {
			if err := os.MkdirAll(filepath.Join(dir, ".claude-plugin"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	for _, tc := range []struct {
		content string
		want    bool
	}{
		{`{"name":"symphony","plugins":[{"name":"symphony-bridge","source":"./mods/symphony-bridge"}]}`, true},
		{`{"name":"other","plugins":[{"name":"something-else","source":"./x"}]}`, false},
		{`not json`, false},
		{"", false},
	} {
		if got := ModMarket(market(tc.content)); got != tc.want {
			t.Errorf("%q: %v, want %v", tc.content, got, tc.want)
		}
	}
	// this repository
	if !ModMarket(filepath.Join("..", "..")) {
		t.Error("the repository's own marketplace should count")
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
