package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// offDuty is a session with a pending ask, and what puts it off duty.
type offDuty struct {
	h    *Hub
	acc  *Account
	sid  string
	put  func()
	kept bool   // its record and chat are still there afterwards (a session replaced by a new one is forgotten)
	why  string // its EndReason then
}

// clockOf lets a test move the hub's clock (under the lock: the hub reads it there).
func clockOf(h *Hub) func(d time.Duration) {
	now := time.Now()
	h.mu.Lock()
	h.now = func() time.Time { return now }
	h.mu.Unlock()
	return func(d time.Duration) {
		h.mu.Lock()
		now = now.Add(d)
		h.mu.Unlock()
	}
}

// taskOffDuty is bob's session in a running task (alice keeps it running), for the paths that end a member's session.
func taskOffDuty(t *testing.T, put func(e *taskEnv, task *Task)) offDuty {
	e := newTaskEnv(t, "")
	plain := e.folder(t, "plain")
	bob, bobAcc := as(t, e.srv, e.base, "bob")
	alice, aliceAcc := as(t, e.srv, e.base, "alice")
	e.srv.Hub.setLast(bobAcc.ID, LaunchRequest{Dir: plain})
	e.srv.Hub.setLast(aliceAcc.ID, LaunchRequest{Dir: plain})
	task := asTask(t, e.page.ok("POST", "/api/tasks", TaskRequest{Title: "t", Leader: true,
		Members: []TaskMemberRequest{{Account: bobAcc.ID}, {Account: aliceAcc.ID}}}))
	bob.ok("POST", "/api/event", Event{Session: "b", Kind: "session.start", Launch: task.Members[0].Launch, Task: task.ID})
	alice.ok("POST", "/api/event", Event{Session: "a", Kind: "session.start", Launch: task.Members[1].Launch, Task: task.ID})
	return offDuty{h: e.srv.Hub, acc: bobAcc, sid: "b", put: func() { put(e, task) }}
}

// TestOffDutyPaths: however a session goes off duty, it goes the same way: its pending ask is withdrawn (the waiting
// mod gets "cancelled"), the activity log has one end line for it, and its chat says it is off duty.
func TestOffDutyPaths(t *testing.T) {
	cases := []struct {
		name string
		make func(t *testing.T) offDuty
	}{
		{"session.end", func(t *testing.T) offDuty {
			srv, base := newBridge(t)
			c, acc := as(t, srv, base, "ann")
			c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
			return offDuty{h: srv.Hub, acc: acc, sid: "s1", kept: true, why: "logout", put: func() {
				c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end", Reason: "logout"})
			}}
		}},
		{"lost (Reap)", func(t *testing.T) offDuty {
			srv, base := newBridge(t)
			move := clockOf(srv.Hub)
			c, acc := as(t, srv, base, "ann")
			c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
			return offDuty{h: srv.Hub, acc: acc, sid: "s1", kept: true, why: "lost", put: func() {
				move(StaleAfter + time.Second)
				srv.Hub.Reap()
			}}
		}},
		{"replaced in the same process (/clear)", func(t *testing.T) offDuty {
			srv, base := newBridge(t)
			c, acc := as(t, srv, base, "ann")
			c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: "L1"})
			return offDuty{h: srv.Hub, acc: acc, sid: "s1", put: func() {
				c.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.start", Launch: "L1"})
			}}
		}},
		{"replaced after being lost", func(t *testing.T) offDuty {
			srv, base := newBridge(t)
			move := clockOf(srv.Hub)
			c, acc := as(t, srv, base, "ann")
			c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
			return offDuty{h: srv.Hub, acc: acc, sid: "s1", put: func() {
				move(StaleAfter + time.Second) // no Reap yet: the new session finds the old one lost
				c.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.start"})
			}}
		}},
		{"a member's process exited", func(t *testing.T) offDuty {
			o := taskOffDuty(t, func(e *taskEnv, task *Task) {
				e.starts.proc(task.Members[0].Launch).exit(errors.New("signal: killed"))
				waitFor(t, "bob's session to go off duty", func() bool { return session(t, e.srv, "b").State == StateEnded })
			})
			o.kept, o.why = true, "exited"
			return o
		}},
		{"its task ended", func(t *testing.T) offDuty {
			return taskOffDuty(t, func(e *taskEnv, task *Task) {
				if err := e.srv.Hub.EndTask(task.ID); err != nil {
					t.Fatal(err)
				}
				e.srv.Hub.Flush() // the ended task's sessions are forgotten
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.make(t)
			ask, err := o.h.CreateAsk(AskRequest{Session: o.sid, Kind: "permission", Tool: "Bash"}, o.acc)
			if err != nil {
				t.Fatal(err)
			}
			o.put()

			if st, _, _ := o.h.Wait(ask, o.acc.ID, 50*time.Millisecond, nil); st != AskCancelled {
				t.Fatalf("the waiting mod should see its ask cancelled, got %q", st)
			}
			for _, a := range o.h.Snapshot().Asks {
				if a.Session == o.sid {
					t.Fatalf("an ask of a session off duty is still pending: %+v", a)
				}
			}
			ends := 0
			for _, a := range o.h.Activity() {
				if a.Kind == "end" && a.Session == o.sid {
					ends++
				}
			}
			if ends != 1 {
				t.Fatalf("want one end line in the activity log, got %d", ends)
			}
			if !o.kept {
				return
			}
			if s := sessionOf(t, o.h, o.sid); s.State != StateEnded || s.EndReason != o.why {
				t.Fatalf("want the session off duty with the reason %q: %+v", o.why, s)
			}
			notices, cancelled := 0, false
			for _, it := range o.h.Chat(o.sid) {
				if it.Kind == ChatNotice && it.Text == "ended" {
					notices++
				}
				if it.Kind == ChatAsk && it.Status == "cancelled" {
					cancelled = true
				}
			}
			if notices != 1 || !cancelled {
				t.Fatalf("the chat should say off duty once (%d) and show the ask cancelled (%v)", notices, cancelled)
			}
		})
	}
}

// TestOffDutyOnce: a session put off duty again is left as it is, and one back online has no end reason.
func TestOffDutyOnce(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "ann")
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	h := srv.Hub
	h.mu.Lock()
	h.offDutyLocked(h.sessions["s1"], "logout")
	h.offDutyLocked(h.sessions["s1"], "lost")
	h.mu.Unlock()
	ends := 0
	for _, a := range h.Activity() {
		if a.Kind == "end" {
			ends++
		}
	}
	if s := session(t, srv, "s1"); ends != 1 || len(h.Chat("s1")) != 1 || s.EndReason != "logout" {
		t.Fatalf("the second time should change nothing: %d end lines, chat %d, reason %q", ends, len(h.Chat("s1")), s.EndReason)
	}
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	if s := session(t, srv, "s1"); s.State == StateEnded || s.EndReason != "" {
		t.Fatalf("back online, the session has no end reason: %+v", s)
	}
}

// TestLaunchExitWhileOnline: a desk session started from the page whose process dies goes off duty at once, saying how
// in its chat; one that said goodbye first gets no second word, and its launch is over either way.
func TestLaunchExitWhileOnline(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	web := client{t: t, base: base}
	online := func(name, sid string) (string, client) {
		id := web.ok("POST", "/api/launch", LaunchRequest{Name: name, Dir: t.TempDir()})["id"].(string)
		acc := srv.Hub.Account(accountNamed(t, srv, name))
		mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + acc.Token}, modOnly: true}
		mod.ok("POST", "/api/event", Event{Session: sid, Kind: "session.start", Launch: id})
		mod.ok("POST", "/api/event", Event{Session: sid, Kind: "turn.start"})
		return id, mod
	}
	notices := func(sid string) (codes []string, details []string) {
		for _, it := range srv.Hub.Chat(sid) {
			if it.Kind == ChatNotice {
				codes, details = append(codes, it.Text), append(details, it.Summary)
			}
		}
		return
	}
	ends := func(sid string) (n int) {
		for _, a := range srv.Hub.Activity() {
			if a.Kind == "end" && a.Session == sid {
				n++
			}
		}
		return
	}

	// a crash: off duty now (not after StaleAfter), with the exit status and the last lines of the screen
	crash, mod := online("ann", "s1")
	ask := mod.ok("POST", "/api/ask", AskRequest{Session: "s1", Kind: "permission", Tool: "Bash"})["id"].(string)
	p := starts.proc(crash)
	if _, err := p.outW.Write([]byte("FATAL ERROR: Reached heap limit\r\n")); err != nil {
		t.Fatal(err)
	}
	p.exit(errors.New("exit status 3"))
	waitFor(t, "the session to go off duty", func() bool { return session(t, srv, "s1").State == StateEnded })
	codes, details := notices("s1")
	if s := session(t, srv, "s1"); s.EndReason != "exited" || strings.Join(codes, " ") != "exited ended" ||
		!strings.Contains(details[0], "exit status 3") || !strings.Contains(details[0], "FATAL ERROR: Reached heap limit") {
		t.Fatalf("want the reason, an exited notice with the exit status and the screen, then off duty: %q %q %+v", codes, details, s)
	}
	if st, _, _ := srv.Hub.Wait(ask, "", 50*time.Millisecond, nil); st != AskCancelled || ends("s1") != 1 {
		t.Fatalf("the ask should be withdrawn (%q) and the activity log have one end line (%d)", st, ends("s1"))
	}
	waitFor(t, "the launch to be over", func() bool { return srv.Hub.launchStatus(crash) == LaunchEnded })

	// /exit: the goodbye comes first, then the process exits
	bye, mod := online("bob", "s2")
	mod.ok("POST", "/api/event", Event{Session: "s2", Kind: "session.end", Reason: "prompt_input_exit"})
	starts.proc(bye).exit(nil)
	waitFor(t, "the launch to be over", func() bool { return srv.Hub.launchStatus(bye) == LaunchEnded })
	if codes, _ := notices("s2"); strings.Join(codes, " ") != "ended" || ends("s2") != 1 || session(t, srv, "s2").EndReason != "prompt_input_exit" {
		t.Fatalf("a session that said goodbye gets nothing more: %q, %d end lines, %+v", codes, ends("s2"), session(t, srv, "s2"))
	}

	// a clean exit without a goodbye: off duty, no error
	quiet, _ := online("cid", "s3")
	starts.proc(quiet).exit(nil)
	waitFor(t, "the session to go off duty", func() bool { return session(t, srv, "s3").State == StateEnded })
	if codes, _ := notices("s3"); strings.Join(codes, " ") != "ended" || session(t, srv, "s3").EndReason != "exited" {
		t.Fatalf("a clean exit is no error: %q %+v", codes, session(t, srv, "s3"))
	}
}

// sessionOf is the hub's session with this id.
func sessionOf(t *testing.T, h *Hub, id string) *Session {
	t.Helper()
	for _, s := range h.Snapshot().Sessions {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no session %s", id)
	return nil
}

// accountNamed is the id of the account with this name.
func accountNamed(t *testing.T, srv *Server, name string) string {
	t.Helper()
	for _, a := range srv.Hub.Snapshot().Accounts {
		if a.Name == name {
			return a.ID
		}
	}
	t.Fatalf("no account %s", name)
	return ""
}

// launchStatusCode posts a launch with the page key and answers the status and the error code.
func launchStatusCode(base string, req LaunchRequest) (int, string, error) {
	data, _ := json.Marshal(req)
	r, _ := http.NewRequest("POST", base+"/api/launch", bytes.NewReader(data))
	r.Header.Set("content-type", "application/json")
	r.Header.Set(keyHeader, testKey)
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	code, _ := out["code"].(string)
	return resp.StatusCode, code, nil
}

// TestLaunchOneAtATime: an account gets one start from the page at a time: not while one is on its way, not twice at
// once, not while the process of the last one still runs; a start that fails lets go of the desk.
func TestLaunchOneAtATime(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	web := client{t: t, base: base}
	count := func() int { starts.mu.Lock(); defer starts.mu.Unlock(); return len(starts.starts) }

	ann, _ := srv.Hub.CreateAccount("ann")
	first := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()}); st != http.StatusConflict || code != "account_busy" || count() != 1 {
		t.Fatalf("a second start while the first is on its way: want 409 account_busy and one process, got %d %q, %d", st, code, count())
	}

	// two at once: one gets through
	bob, _ := srv.Hub.CreateAccount("bob")
	var wg sync.WaitGroup
	results := make([]int, 2)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _, _ = launchStatusCode(base, LaunchRequest{Account: bob.ID, Dir: t.TempDir()})
		}()
	}
	wg.Wait()
	if (results[0] == 200) == (results[1] == 200) || results[0]+results[1] != 200+http.StatusConflict || count() != 2 {
		t.Fatalf("two starts at once: want one 200 and one 409, got %v, %d processes", results, count())
	}

	// a start that fails lets go of the desk
	cid, _ := srv.Hub.CreateAccount("cid")
	failStart := func(v bool) { starts.mu.Lock(); starts.failStart = v; starts.mu.Unlock() }
	failStart(true)
	if st, _ := web.failure("POST", "/api/launch", LaunchRequest{Account: cid.ID, Dir: t.TempDir()}); st != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", st)
	}
	failStart(false)
	web.ok("POST", "/api/launch", LaunchRequest{Account: cid.ID, Dir: t.TempDir()})

	// off duty while its process still runs (it may come back): no second one until it is gone
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: first})
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end"})
	if st, code := web.failure("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()}); st != http.StatusConflict || code != "launch_still_running" {
		t.Fatalf("a start while the last process still runs: want 409 launch_still_running, got %d %q", st, code)
	}
	starts.proc(first).exit(nil)
	waitFor(t, "the process to go", func() bool { return !srv.Launcher.Terms.Live(first) })
	web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})
}

// TestLaunchTimeoutKillsProcess: a desk session that never reports in is failed and its process killed (stuck at a
// prompt out of sight, it would hold the desk for nothing); the error keeps its screen, and the desk is free again.
func TestLaunchTimeoutKillsProcess(t *testing.T) {
	srv, base, starts := newLaunchBridge(t)
	srv.Launcher.OnlineTimeout = 300 * time.Millisecond
	starts.screen = "Choose the text style that looks best with your terminal\r\n"
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	waitFor(t, "the launch to fail", func() bool { return srv.Hub.launchStatus(id) == LaunchFailed })
	waitFor(t, "the process to be killed", func() bool { return !srv.Launcher.Terms.Live(id) })
	for _, l := range srv.Hub.Snapshot().Launches {
		if l.ID == id && (l.ErrorCode != "launch_timeout_pty" || !strings.Contains(l.ErrorParams["screen"], "text style")) {
			t.Fatalf("the failed launch should say it timed out, with the screen: %+v", l)
		}
	}
	web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})
}

// TestLaunchOnlineClearsError: a launch that comes online after all carries no error.
func TestLaunchOnlineClearsError(t *testing.T) {
	srv, base := newBridge(t)
	c, acc := as(t, srv, base, "ann")
	srv.Hub.addLaunch(&Launch{ID: "L1", Name: "ann", Account: acc.ID, Status: LaunchStarting})
	srv.Hub.failLaunch("L1", newError("launch_timeout_pty", "not online", "timeout", "90s"))
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: "L1"})
	for _, l := range srv.Hub.Snapshot().Launches {
		if l.ID == "L1" && (l.Status != LaunchOnline || l.Error != "" || l.ErrorCode != "" || l.ErrorParams != nil) {
			t.Fatalf("an online launch has no error: %+v", l)
		}
	}
}
