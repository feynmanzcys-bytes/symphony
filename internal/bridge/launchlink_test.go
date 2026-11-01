package bridge

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

// hangupProc is a fakeProc that takes a hangup and goes on running (claude still starting up), so a Kill waits its
// grace before it kills.
type hangupProc struct{ *fakeProc }

func (p hangupProc) Hangup() error { return nil }

// TestContinueRelinksLaunch: "Start work" with "Continue" runs claude --continue, which goes on under the session id
// the employee had; the bridge still keeps that session's record, with the launch it ran in before. The new launch is
// the session's now: it comes online, and its process isn't killed at the timeout as one that never reported in.
func TestContinueRelinksLaunch(t *testing.T) {
	srv, base, f := newLaunchBridge(t)
	srv.Launcher.OnlineTimeout = 400 * time.Millisecond
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	dir := t.TempDir()
	l1 := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: dir})["id"].(string)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: l1})
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end", Reason: "prompt_input_exit", Launch: l1})
	f.proc(l1).exit(nil)
	waitFor(t, "the first launch to end", func() bool { return srv.Hub.launchStatus(l1) == LaunchEnded })

	l2 := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: dir, Resume: "continue"})["id"].(string)
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: l2})
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.start", Launch: l2})
	if st, s := srv.Hub.launchStatus(l2), session(t, srv, "s1"); st != LaunchOnline || s.Launch != l2 || s.State == StateEnded {
		t.Fatalf("the session goes on in the new launch: launch %s, session launch %s, state %s", st, s.Launch, s.State)
	}
	time.Sleep(900 * time.Millisecond) // past the timeout
	if !srv.Launcher.Terms.Live(l2) || srv.Hub.launchStatus(l2) != LaunchOnline || session(t, srv, "s1").State == StateEnded {
		t.Fatalf("a session at work isn't killed: live %v, launch %s", srv.Launcher.Terms.Live(l2), srv.Hub.launchStatus(l2))
	}
	f.proc(l2).exit(errors.New("exit status 2"))
	waitFor(t, "its exit to be noticed", func() bool { return session(t, srv, "s1").State == StateEnded })
}

// TestLateReportAfterExit: a report that was on its way when a page-started session's process exited doesn't bring the
// session back online (which would hold the desk until the heartbeat check, 45 s): it gets 410 session_gone.
func TestLateReportAfterExit(t *testing.T) {
	srv, base, f := newLaunchBridge(t)
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: id})
	f.proc(id).exit(errors.New("signal: killed"))
	waitFor(t, "the session to go off duty", func() bool { return session(t, srv, "s1").State == StateEnded })
	if st, code := mod.failure("POST", "/api/event", Event{Session: "s1", Kind: "session.usage", Launch: id}); st != http.StatusGone || code != "session_gone" {
		t.Fatalf("a late report: want 410 session_gone, got %d %q", st, code)
	}
	if s := session(t, srv, "s1"); s.State != StateEnded {
		t.Fatalf("the session came back: %s", s.State)
	}
	web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})
}

// TestHandStartedContinueAfterExit: the same session id goes on in a terminal by hand (claude --continue with the
// account's token, no launch) after its page-started process exited: it is back online at once.
func TestHandStartedContinueAfterExit(t *testing.T) {
	srv, base, f := newLaunchBridge(t)
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: id})
	f.proc(id).exit(errors.New("signal: killed"))
	waitFor(t, "the session to go off duty", func() bool { return session(t, srv, "s1").State == StateEnded })
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start"})
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat"})
	if s := session(t, srv, "s1"); s.State == StateEnded {
		t.Fatal("the session started by hand should be back online")
	}
}

// TestFailedThenOnlineFollowed: a launch that times out has its process hung up on; the session reports in during the
// kill's grace, so the launch is online after all. Its process is followed to its end all the same: when it is gone,
// the session goes off duty at once and the desk is free.
func TestFailedThenOnlineFollowed(t *testing.T) {
	old := killGrace
	killGrace = 400 * time.Millisecond
	t.Cleanup(func() { killGrace = old })
	srv, base, f := newLaunchBridge(t)
	srv.Launcher.OnlineTimeout = 300 * time.Millisecond
	term := srv.Launcher.Term.(*ptyTerminal)
	term.start = func(id, name, dir string, env []string, argv []string) (*Term, error) {
		p := newFakeProc()
		f.mu.Lock()
		if f.procs == nil {
			f.procs = map[string]*fakeProc{}
		}
		f.procs[id] = p
		f.mu.Unlock()
		return hostTerm(id, name, hangupProc{p}), nil
	}
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	waitFor(t, "the launch to fail", func() bool { return srv.Hub.launchStatus(id) == LaunchFailed })
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: id})
	if st := srv.Hub.launchStatus(id); st != LaunchOnline {
		t.Fatalf("the session reported in: want the launch online, got %s", st)
	}
	waitFor(t, "the process to be gone", func() bool { return !srv.Launcher.Terms.Live(id) })
	waitFor(t, "the session to go off duty", func() bool { return session(t, srv, "s1").State == StateEnded })
	waitFor(t, "the launch to end", func() bool { return srv.Hub.launchStatus(id) == LaunchEnded })
	web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})
}

// TestReportBeforeAddLaunch: the session reports in before the launch is in the hub (the launch handler records the
// account's last choices in between); once it is, the launch is online, and nobody kills the session at the timeout.
func TestReportBeforeAddLaunch(t *testing.T) {
	srv, base, _ := newLaunchBridge(t)
	ann, _ := srv.Hub.CreateAccount("ann")
	l := &Launch{ID: "L9", Name: "ann", Account: ann.ID, Status: LaunchStarting, Time: time.Now()}
	if err := srv.Launcher.Start(l, LaunchRequest{Account: ann.ID, Name: "ann", Dir: t.TempDir()}, map[string]string{}); err != nil {
		t.Fatal(err)
	}
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: "L9"})
	srv.Hub.addLaunch(l)
	if st := srv.Hub.launchStatus("L9"); st != LaunchOnline {
		t.Fatalf("the session that reported in first: want the launch online, got %s", st)
	}
	done := make(chan struct{})
	go func() { srv.Launcher.Term.Watch(srv.Hub, l, 300*time.Millisecond); close(done) }()
	time.Sleep(700 * time.Millisecond)
	if !srv.Launcher.Terms.Live("L9") || session(t, srv, "s1").State == StateEnded {
		t.Fatal("the session at work was killed at the timeout")
	}
	srv.Launcher.Terms.Get("L9").Kill()
	<-done
}

// TestTimeoutSparesAttachedTerminal: a launch not online in time is failed, but its process stays while someone uses
// its terminal (a login, a first-run prompt); when the session reports in, the launch comes online, and its end is
// followed as any other.
func TestTimeoutSparesAttachedTerminal(t *testing.T) {
	srv, base, f := newLaunchBridge(t)
	srv.Launcher.OnlineTimeout = 300 * time.Millisecond
	f.screen = "Choose the text style"
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	_, _, detach := srv.Launcher.Terms.Get(id).Attach(80, 24) // symphony attach
	defer detach()
	waitFor(t, "the launch to fail", func() bool { return srv.Hub.launchStatus(id) == LaunchFailed })
	time.Sleep(200 * time.Millisecond)
	if !srv.Launcher.Terms.Live(id) {
		t.Fatal("the process someone is using in its terminal was killed")
	}
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: id})
	if st := srv.Hub.launchStatus(id); st != LaunchOnline {
		t.Fatalf("want the launch online once its session reported in, got %s", st)
	}
	f.proc(id).exit(nil)
	waitFor(t, "the session to go off duty", func() bool { return session(t, srv, "s1").State == StateEnded })
}
