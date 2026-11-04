package bridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/feynmanzcys-bytes/symphony/internal/i18n"
)

// TestFaultRedact: what goes into the error log keeps no secret: the page key and the tokens (runs of 48 hex digits
// or more), a URL's user and password, a bearer token; the home folder reads ~, but not a longer name that starts like
// it; a git commit's 40 digits stay.
func TestFaultRedact(t *testing.T) {
	l := faultLog{home: "/home/ann"}
	token := strings.Repeat("ab12", 12) // 48
	for in, want := range map[string]string{
		"open http://127.0.0.1:7788/?key=" + testKey:                 "open http://127.0.0.1:7788/?key=‹redacted›",
		"token " + token + " refused":                                "token ‹redacted› refused",
		"proxy https://bob:s3cret@proxy.corp:8080/ failed":           "proxy https://‹redacted›@proxy.corp:8080/ failed",
		"Authorization: Bearer abc.def":                              "Authorization: Bearer ‹redacted›",
		"/home/ann/work/app: not a repository":                       "~/work/app: not a repository",
		"cd /home/ann && ls /home/anna/x /home/ann.bak /home/ann":    "cd ~ && ls /home/anna/x /home/ann.bak ~",
		"HEAD is now at 0123456789abcdef0123456789abcdef01234567 x":  "HEAD is now at 0123456789abcdef0123456789abcdef01234567 x",
		"ssh://git@github.com/feynmanzcys-bytes/symphony.git denied": "ssh://‹redacted›@github.com/feynmanzcys-bytes/symphony.git denied",
	} {
		if got := l.redact(in); got != want {
			t.Errorf("redact(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

// TestFaultsKeptOnceAndCounted: the same thing again counts up and moves to the end; the log keeps the newest
// keepFaults; the snapshot says how many and when the newest happened; a text too long is cut, saying how much.
func TestFaultsKeptOnceAndCounted(t *testing.T) {
	h := NewHub()
	clock := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return clock }
	h.fault(Fault{Source: "bridge", Code: "save_failed", Text: "can't write accounts.json: disk full"})
	clock = clock.Add(time.Minute)
	h.fault(Fault{Source: "task", Code: "task_git_failed", Text: "git worktree add: fatal", Task: "T1"})
	clock = clock.Add(time.Minute)
	h.fault(Fault{Source: "bridge", Code: "save_failed", Text: "can't write accounts.json: disk full"})
	list := h.Faults()
	if len(list) != 2 || list[1].Code != "save_failed" || list[1].Count != 2 || !list[1].Last.Equal(clock) || !list[1].Time.Equal(clock.Add(-2*time.Minute)) || list[0].Task != "T1" {
		t.Fatalf("the same fault again should count and move to the end: %+v", list)
	}
	if s := h.Snapshot().Faults; s.Count != 2 || s.Last == nil || !s.Last.Equal(clock) {
		t.Fatalf("the snapshot's summary: %+v", s)
	}
	h.fault(Fault{Source: "page", Code: "js", Text: "   "})
	if len(h.Faults()) != 2 {
		t.Fatal("an empty text should be dropped")
	}
	for i := 0; i < keepFaults+5; i++ {
		h.fault(Fault{Source: "page", Code: "js", Text: fmt.Sprintf("boom %d", i)})
	}
	if list := h.Faults(); len(list) != keepFaults || list[len(list)-1].Text != fmt.Sprintf("boom %d", keepFaults+4) {
		t.Fatalf("the log should keep the newest %d: %d, last %+v", keepFaults, len(list), list[len(list)-1])
	}
	h.fault(Fault{Source: "task", Text: strings.Repeat("x", faultTextMax+100)})
	if last := h.Faults()[keepFaults-1]; !strings.HasSuffix(last.Text, "[100 more bytes]") || len(last.Text) > faultTextMax+40 {
		t.Fatalf("a long text should be cut, saying how much: %q", last.Text[len(last.Text)-40:])
	}
	h.ClearFaults()
	if len(h.Faults()) != 0 || h.Snapshot().Faults.Count != 0 {
		t.Fatal("cleared, the log should be empty")
	}
}

// TestFaultsPersist: the error log is in errors.json and comes back with the bridge; one that doesn't parse is set
// aside (the log says so, and that is kept too).
func TestFaultsPersist(t *testing.T) {
	dir := t.TempDir()
	h := NewHub()
	if err := h.Open(dir); err != nil {
		t.Fatal(err)
	}
	h.fault(Fault{Source: "launch", Code: "launch_process_exited", Text: "claude exited: exit status 1", Name: "bob"})
	if err := h.Flush(); err != nil {
		t.Fatal(err)
	}
	again := NewHub()
	if err := again.Open(dir); err != nil {
		t.Fatal(err)
	}
	if list := again.Faults(); len(list) != 1 || list[0].Name != "bob" || list[0].Count != 1 {
		t.Fatalf("the error log should come back: %+v", list)
	}
	if err := os.WriteFile(filepath.Join(dir, "errors.json"), []byte("{half"), 0o600); err != nil {
		t.Fatal(err)
	}
	var w warnings
	third := NewHub()
	third.Warn = w.add
	if err := third.Open(dir); err != nil {
		t.Fatalf("a damaged errors.json shouldn't stop the bridge: %v", err)
	}
	if err := third.Flush(); err != nil { // (the error log, written again in the background)
		t.Fatal(err)
	}
	list := third.Faults()
	if lines := w.all(); len(list) != 1 || list[0].Code != "data_set_aside" || !strings.Contains(list[0].Text, "errors.json") || len(lines) != 1 {
		t.Fatalf("a damaged errors.json should be set aside and that kept: %+v %q", list, lines)
	}
	if aside, _ := filepath.Glob(filepath.Join(dir, "errors.json.corrupt-*")); len(aside) != 1 {
		t.Fatalf("set aside: %v", aside)
	}
}

// TestWarnKeepsFault: a warning goes to the bridge's log in its language and into the error log in English.
func TestWarnKeepsFault(t *testing.T) {
	was := i18n.Lang()
	i18n.SetLang("ja")
	defer i18n.SetLang(was)
	h := NewHub()
	var lines []string
	h.Warn = func(line string) { lines = append(lines, line) }
	h.warnWrite("/data/accounts.json", fmt.Errorf("disk full"))
	want := i18n.TLang("en", "log.save_failed", map[string]string{"file": "/data/accounts.json", "reason": "disk full"})
	if len(lines) != 1 || lines[0] != i18n.T("log.save_failed", "file", "/data/accounts.json", "reason", "disk full") || lines[0] == want {
		t.Fatalf("the log line should be in the bridge's language: %q", lines)
	}
	if list := h.Faults(); len(list) != 1 || list[0].Source != "bridge" || list[0].Code != "save_failed" || list[0].Text != want {
		t.Fatalf("the error log should keep it in English: %+v", list)
	}
}

// TestFaultsFromSessionsAndLaunches: a session's API error, a session that exited badly and a launch that failed go
// into the error log with the employee's name.
func TestFaultsFromSessionsAndLaunches(t *testing.T) {
	srv, base := newBridge(t)
	mod, _ := as(t, srv, base, "bob")
	mod.ok("POST", "/api/event", map[string]any{"session": "s1", "kind": "session.start", "cwd": "/w", "model": "claude-sonnet-5-5", "engine": map[string]any{"version": "2.1.316"}, "mod": "1.2.2"})
	mod.ok("POST", "/api/event", map[string]any{"session": "s1", "kind": "turn.error", "text": "API Error: 529 overloaded", "details": "try again later"})
	h := srv.Hub
	h.mu.Lock()
	h.exitedLocked(h.sessions["s1"], fmt.Errorf("exit status 2"), "boom\n")
	l := &Launch{ID: "L1", Name: "carol", Dir: "/w/carol", Status: LaunchStarting}
	h.launches[l.ID] = l
	h.failLaunchLocked(l, newError("launch_process_exited", "claude exited before it reported in: exit status 1"))
	h.mu.Unlock()
	got := map[string]Fault{}
	for _, f := range h.Faults() {
		got[f.Code] = f
	}
	if f := got["turn_error"]; f.Source != "session" || f.Name != "bob" || f.Text != "API Error: 529 overloaded try again later" || !strings.Contains(f.Detail, "Claude Code 2.1.316") {
		t.Errorf("a session's API error: %+v", f)
	}
	if f := got["exited"]; f.Source != "session" || f.Name != "bob" || !strings.Contains(f.Text, "exit status 2") {
		t.Errorf("a bad exit: %+v", f)
	}
	if f := got["launch_process_exited"]; f.Source != "launch" || f.Name != "carol" || !strings.Contains(f.Detail, "/w/carol") {
		t.Errorf("a failed launch: %+v", f)
	}
}

// TestFaultWriter: a page's call that fails is kept as the error log keeps them: every 5xx, a failed start of a task
// or a session but for a form left incomplete or a desk in use; not a 4xx elsewhere, not a success.
func TestFaultWriter(t *testing.T) {
	h := NewHub()
	answer := func(method, path string, status int, body string) {
		rec := httptest.NewRecorder()
		fw := &faultWriter{ResponseWriter: rec, r: httptest.NewRequest(method, path, nil)}
		fw.Header().Set("content-type", "application/json")
		fw.WriteHeader(status)
		_, _ = fw.Write([]byte(body))
		fw.keep(h)
		if rec.Code != status || rec.Body.String() != body {
			t.Fatalf("the answer should pass through: %d %q", rec.Code, rec.Body.String())
		}
	}
	answer("POST", "/api/tasks", 500, `{"error":"git worktree add failed: fatal: not a git repository","code":"task_git_failed","params":{"dir":"/w/app","git":"fatal"}}`)
	answer("POST", "/api/tasks", 400, `{"error":"the folder is the home folder","code":"task_dir_home","params":{"dir":"/home/x"}}`)
	answer("POST", "/api/tasks", 400, `{"error":"the task needs a description","code":"task_empty_title"}`)
	answer("POST", "/api/launch", 409, `{"error":"busy","code":"account_busy"}`)
	answer("POST", "/api/launch", 501, `{"error":"this bridge can't start sessions","code":"launch_disabled"}`)
	answer("POST", "/api/say/s1", 409, `{"error":"the session is not online","code":"session_offline"}`)
	answer("GET", "/api/chat/s1", 500, `{"error":"can't encode the answer"}`)
	answer("POST", "/api/tasks", 200, `{"id":"T1"}`)
	got := map[string]Fault{}
	for _, f := range h.Faults() {
		got[f.Code] = f
	}
	if len(got) != 4 {
		t.Fatalf("want the 5xx and the failed starts but a form's: %+v", h.Faults())
	}
	if f := got["task_git_failed"]; f.Source != "task" || !strings.Contains(f.Text, "not a git repository") || f.Detail != "POST /api/tasks → 500\ndir: /w/app\ngit: fatal" {
		t.Errorf("a task's failed start: %+v", f)
	}
	if f := got["task_dir_home"]; f.Source != "task" || !strings.HasPrefix(f.Detail, "POST /api/tasks → 400") {
		t.Errorf("a task's start refused for its folder: %+v", f)
	}
	if f := got["launch_disabled"]; f.Source != "launch" {
		t.Errorf("a launch's failed start: %+v", f)
	}
	if f := got[""]; f.Source != "api" || f.Text != "can't encode the answer" || f.Detail != "GET /api/chat/s1 → 500" {
		t.Errorf("a 5xx elsewhere: %+v", f)
	}
}

// TestFaultsAPI: the pages read the error log (GET /api/errors), report their own errors into it (POST, at most
// pageFaultsPerMinute a minute) and empty it (DELETE), all with the page key; GET /api/diagnostics says what a report
// needs besides, the bridge's log lines redacted. A failed call is kept by the bridge itself, once.
func TestFaultsAPI(t *testing.T) {
	srv, base := newBridge(t)
	srv.Version = "v9.9.9"
	srv.LogTail = NewLogTail(3)
	fmt.Fprintf(srv.LogTail, "one\ntwo\nlistening on http://127.0.0.1:7788/?key=%s\nfour\nfive", testKey)
	page := client{t: t, base: base}
	if st := (client{t: t, base: base, modOnly: true}).status("GET", "/api/errors", nil); st != http.StatusUnauthorized {
		t.Fatalf("the error log needs the page key: %d", st)
	}
	page.ok("POST", "/api/errors", map[string]any{"kind": "js", "text": "TypeError: x is undefined", "detail": "messenger.js:12:3", "theme": "messenger", "page": "/t/messenger/?lang=en"})
	if st, code := page.failure("POST", "/api/errors", map[string]any{"kind": "whatever", "text": "x"}); st != 400 || code != "fault_bad_request" {
		t.Fatalf("an unknown kind: %d %s", st, code)
	}
	if st, _ := page.failure("POST", "/api/tasks", map[string]any{}); st != http.StatusNotImplemented {
		t.Fatalf("no launcher: %d", st)
	}
	out := page.ok("GET", "/api/errors", nil)
	raw, _ := json.Marshal(out["errors"])
	var list []Fault
	_ = json.Unmarshal(raw, &list)
	if len(list) != 2 || list[0].Source != "page" || list[0].Code != "js" || list[0].Detail != "messenger.js:12:3\npage: messenger /t/messenger/?lang=en" ||
		list[1].Source != "task" || list[1].Code != "launch_disabled" {
		t.Fatalf("the page's report and the failed call: %+v", list)
	}
	for i := 1; i < pageFaultsPerMinute; i++ { // (one went already)
		page.ok("POST", "/api/errors", map[string]any{"kind": "rejection", "text": fmt.Sprint("nope ", i)})
	}
	if st, code := page.failure("POST", "/api/errors", map[string]any{"kind": "js", "text": "one too many"}); st != http.StatusTooManyRequests || code != "fault_too_many" {
		t.Fatalf("past %d a minute: %d %s", pageFaultsPerMinute, st, code)
	}
	diag := page.ok("GET", "/api/diagnostics", nil)
	lines, _ := json.Marshal(diag["log"])
	if diag["version"] != "v9.9.9" || diag["protocol"] != float64(Protocol) || string(lines) != `["two","listening on http://127.0.0.1:7788/?key=‹redacted›","four"]` {
		t.Fatalf("diagnostics: %v %s", diag, lines)
	}
	if errs, _ := diag["errors"].([]any); len(errs) != pageFaultsPerMinute+1 {
		t.Fatalf("diagnostics should carry the error log: %d", len(errs))
	}
	page.ok("DELETE", "/api/errors", nil)
	if out := page.ok("GET", "/api/errors", nil); len(out["errors"].([]any)) != 0 {
		t.Fatalf("emptied: %v", out)
	}
	if st := (client{t: t, base: base, header: map[string]string{"Origin": "http://evil.example"}}).status("DELETE", "/api/errors", nil); st != http.StatusForbidden {
		t.Fatalf("another site may not empty it: %d", st)
	}
}

// TestFaultPushesInOrder: the pages hear of the error log's entries in the order they were kept (one goroutine sends
// them), so a summary never goes back: its count only grows and its newest time never gets older.
func TestFaultPushesInOrder(t *testing.T) {
	h := NewHub()
	ch, stop := h.Subscribe()
	defer stop()
	<-ch // the first snapshot
	clock := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	h.mu.Lock()
	h.now = func() time.Time { return clock }
	h.mu.Unlock()
	const n = 60
	for i := 0; i < n; i++ {
		h.mu.Lock() // (as most of the bridge's callers hold it)
		clock = clock.Add(time.Millisecond)
		h.fault(Fault{Source: "bridge", Code: "x", Text: fmt.Sprint("fault ", i)})
		h.mu.Unlock()
	}
	var counts []int
	var lasts []time.Time
	for len(counts) < n {
		select {
		case p := <-ch:
			m, ok := p.Value.(map[string]any)
			if !ok || m["type"] != "fault" {
				continue
			}
			sum := m["faults"].(FaultSummary)
			counts = append(counts, sum.Count)
			lasts = append(lasts, *sum.Last)
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d pushes came", len(counts), n)
		}
	}
	for i := 1; i < n; i++ {
		if counts[i] < counts[i-1] || lasts[i].Before(lasts[i-1]) {
			t.Fatalf("push %d went back: count %d after %d, last %v after %v", i, counts[i], counts[i-1], lasts[i], lasts[i-1])
		}
	}
}
