package bridge

import (
	"net/http"
	"testing"
	"time"
)

func inboxCommands(t *testing.T, mod client, session string) []string {
	t.Helper()
	var got []string
	for _, v := range mod.ok("GET", "/api/inbox?session="+session, nil)["items"].([]any) {
		it := v.(map[string]any)
		if it["kind"] != "command" {
			t.Fatalf("not a command item: %v", it)
		}
		got = append(got, it["command"].(string))
	}
	return got
}

// TestClearSession: the page starts a session's conversation over: /clear goes to its inbox as a command item, for a
// session whose mod runs it (1.9.0 on); an older mod's is refused, as is one off duty.
func TestClearSession(t *testing.T) {
	srv, base := newBridge(t)
	srv.WaitTimeout = 300 * time.Millisecond
	mod, _ := as(t, srv, base, "a")
	page := client{t: t, base: base}
	if st, code := page.failure("POST", "/api/sessions/nope/clear", nil); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("unknown session: %d %s", st, code)
	}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w", Mod: "1.8.0"})
	for _, path := range []string{"/api/sessions/s1/clear", "/api/sessions/s1/end"} {
		if st, code := page.failure("POST", path, nil); st != http.StatusConflict || code != "mod_outdated" {
			t.Fatalf("%s with an older mod: %d %s", path, st, code)
		}
	}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat", Mod: "1.9.0"})
	page.ok("POST", "/api/sessions/s1/clear", nil)
	// a session started by hand has no process the bridge could end: /exit goes to its inbox, and it stays on duty
	// until it says goodbye
	page.ok("POST", "/api/sessions/s1/end", nil)
	if got := inboxCommands(t, mod, "s1"); len(got) != 2 || got[0] != "clear" || got[1] != "exit" {
		t.Fatalf("the inbox should hold /clear and /exit: %v", got)
	}
	if s := session(t, srv, "s1"); s.State == StateEnded {
		t.Fatal("a hand-started session is off duty only once it says so")
	}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end", Reason: "prompt_input_exit"})
	if st, code := page.failure("POST", "/api/sessions/s1/end", nil); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("off duty already: %d %s", st, code)
	}
}

func TestModAtLeast(t *testing.T) {
	for v, want := range map[string]bool{"1.9.0": true, "1.9.1": true, "1.10.0": true, "2.0.0": true, "1.9.0-rc.1": true,
		"1.8.9": false, "0.99.0": false, "": false, "1.9": false, "x.y.z": false} {
		if got := modAtLeast(v, pageCommandsMod); got != want {
			t.Errorf("modAtLeast(%q) = %v, want %v", v, got, want)
		}
	}
}

// TestEndSession: the page sends a session it started off duty: off duty at once (reason dismissed, no exit notice,
// nothing in the error log), its process killed, its launch over, a report on its way refused, and the desk free
// again once the process is gone. Its mod's version doesn't matter: the bridge ends the process itself.
func TestEndSession(t *testing.T) {
	srv, base, _ := newLaunchBridge(t)
	web := client{t: t, base: base}
	ann, _ := srv.Hub.CreateAccount("ann")
	id := web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})["id"].(string)
	mod := client{t: t, base: base, header: map[string]string{"authorization": "Bearer " + ann.Token}, modOnly: true}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Launch: id})
	ask := mod.ok("POST", "/api/ask", map[string]any{"session": "s1", "kind": "permission", "tool": "Bash", "input": map[string]any{"command": "ls"}})["id"].(string)

	web.ok("POST", "/api/sessions/s1/end", nil)
	s := session(t, srv, "s1")
	if s.State != StateEnded || s.EndReason != "dismissed" {
		t.Fatalf("off duty at once: state %s, reason %q", s.State, s.EndReason)
	}
	if st := srv.Hub.launchStatus(id); st != LaunchEnded {
		t.Fatalf("its launch is over: %s", st)
	}
	for _, a := range srv.Hub.Snapshot().Asks {
		if a.ID == ask {
			t.Fatalf("its pending ask is withdrawn: %+v", a)
		}
	}
	// the process is being ended (the fake one takes the hangup and stays until killed): what it still reports
	// doesn't bring the session back
	if st, code := mod.failure("POST", "/api/event", Event{Session: "s1", Kind: "heartbeat", Launch: id}); st != http.StatusGone || code != "session_gone" {
		t.Fatalf("a report while it dies: want 410 session_gone, got %d %q", st, code)
	}
	waitFor(t, "the process to go", func() bool { return !srv.Launcher.Terms.Live(id) })
	if s := session(t, srv, "s1"); s.State != StateEnded || s.EndReason != "dismissed" {
		t.Fatalf("the exit changes nothing: state %s, reason %q", s.State, s.EndReason)
	}
	for _, it := range srv.Hub.Chat("s1") {
		if it.Kind == ChatNotice && it.Text == "exited" {
			t.Fatalf("no exit notice for a session sent off duty: %+v", it)
		}
	}
	if n := len(srv.Hub.Faults()); n != 0 {
		t.Fatalf("nothing in the error log: %d", n)
	}
	web.ok("POST", "/api/launch", LaunchRequest{Account: ann.ID, Dir: t.TempDir()})
}
