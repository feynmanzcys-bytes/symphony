package bridge

import (
	"net/http"
	"testing"
	"time"
)

// TestChatStreamAndStop: the model's pieces become blocks (thinking without words included), a stop closes them with
// how long the thinking took, and the log comes back in order.
func TestChatStreamAndStop(t *testing.T) {
	srv, base := newBridge(t)
	c, _ := as(t, srv, base, "a")
	now := time.Now()
	srv.Hub.now = func() time.Time { return now }
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.user", Text: "  what is 17*23?  ", Origin: "composer"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.delta", TurnID: "t1", Step: 0, Blocks: []ChatDelta{{I: 0, Kind: "thinking", Text: ""}, {I: 1, Kind: "text", Text: "17*23 "}}})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.delta", TurnID: "t1", Step: 0, Blocks: []ChatDelta{{I: 1, Kind: "text", Text: "= 391"}, {I: 2, Kind: "bogus", Text: "x"}}})
	now = now.Add(6 * time.Second)
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.stop", TurnID: "t1", Step: 0})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.start", Tool: "Bash", ToolUseID: "tu1", Summary: "ls"})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "tool.end", Tool: "Bash", ToolUseID: "tu1", IsError: true})
	c.ok("POST", "/api/event", Event{Session: "s1", Kind: "turn.complete", TurnID: "t1", IsAborted: true})

	items := srv.Hub.Chat("s1")
	kinds := ""
	for _, it := range items {
		kinds += it.Kind + " "
	}
	if kinds != "user thinking text tool notice " {
		t.Fatalf("items: %s", kinds)
	}
	if u := items[0]; u.Text != "what is 17*23?" || u.Origin != "composer" || !u.Done {
		t.Fatalf("user item: %+v", u)
	}
	if th := items[1]; th.Text != "" || !th.Done || th.Secs != 6 {
		t.Fatalf("a thinking block without words should still be there, done after 6s: %+v", th)
	}
	if tx := items[2]; tx.Text != "17*23 = 391" || !tx.Done {
		t.Fatalf("text block: %+v", tx)
	}
	if tool := items[3]; tool.Tool != "Bash" || tool.Summary != "ls" || tool.Status != "error" || !tool.Done {
		t.Fatalf("tool item: %+v", tool)
	}
	if n := items[4]; n.Text != "aborted" {
		t.Fatalf("notice: %+v", n)
	}
	for i := 1; i < len(items); i++ {
		if items[i].Seq <= items[i-1].Seq {
			t.Fatalf("items should be in order: %+v", items)
		}
	}
	// the page's endpoint gives the same
	if res := c.ok("GET", "/api/chat/s1", nil); len(res["items"].([]any)) != 5 {
		t.Fatalf("/api/chat: %v", res)
	}
	// a session the hub doesn't know has an empty log, not null
	if res := c.ok("GET", "/api/chat/nope", nil); res["items"] == nil {
		t.Fatalf("/api/chat of an unknown session: %v", res)
	}
}

// TestSayInboxAbort: the page's text reaches the mod's inbox and the chat shows it queued until the mod sent it;
// a stop goes the same way; an offline session takes nothing.
func TestSayInboxAbort(t *testing.T) {
	srv, base := newBridge(t)
	srv.WaitTimeout = 300 * time.Millisecond
	mod, _ := as(t, srv, base, "a")
	page := client{t: t, base: base}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})

	if st, code := page.failure("POST", "/api/say/s1", map[string]any{"text": "   "}); st != http.StatusBadRequest || code != "say_empty" {
		t.Fatalf("empty text: %d %s", st, code)
	}
	if st, code := page.failure("POST", "/api/say/nope", map[string]any{"text": "hi"}); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("unknown session: %d %s", st, code)
	}
	it := page.ok("POST", "/api/say/s1", map[string]any{"text": "run the build"})
	if it["kind"] != "user" || it["status"] != "queued" || it["origin"] != "page" {
		t.Fatalf("say should answer the queued item: %v", it)
	}
	page.ok("POST", "/api/abort/s1", nil)
	// the mod drains the inbox, in order; a second poll waits and comes back empty
	res := mod.ok("GET", "/api/inbox?session=s1", nil)
	items := res["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["kind"] != "say" || items[0].(map[string]any)["text"] != "run the build" || items[1].(map[string]any)["kind"] != "abort" {
		t.Fatalf("inbox: %v", items)
	}
	start := time.Now()
	if res := mod.ok("GET", "/api/inbox?session=s1", nil); len(res["items"].([]any)) != 0 || time.Since(start) < 250*time.Millisecond {
		t.Fatalf("an empty inbox should wait, then answer empty: %v after %v", res, time.Since(start))
	}
	// another account can't read this session's inbox
	other, _ := as(t, srv, base, "b")
	if st, code := other.failure("GET", "/api/inbox?session=s1", nil); st != http.StatusForbidden || code != "ask_wrong_account" {
		t.Fatalf("another account's poll: %d %s", st, code)
	}
	// the mod reports the prompt as submitted: the queued item becomes sent, no duplicate
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "chat.sent", Text: "run the build"})
	log := srv.Hub.Chat("s1")
	if len(log) != 1 || log[0].Status != "sent" || !log[0].Done {
		t.Fatalf("the queued item should be marked sent: %+v", log)
	}
	// a session that ended takes nothing more
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.end"})
	if st, code := page.failure("POST", "/api/say/s1", map[string]any{"text": "hi"}); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("an ended session: %d %s", st, code)
	}
}

// TestChatFollowsAsksAndMessages: asks and messages between sessions get items too, and an account's new session
// starts a fresh log.
func TestChatFollowsAsksAndMessages(t *testing.T) {
	srv, base := newBridge(t)
	a, _ := as(t, srv, base, "alice")
	bb, _ := as(t, srv, base, "bob")
	a.ok("POST", "/api/event", Event{Session: "sa", Kind: "session.start"})
	bb.ok("POST", "/api/event", Event{Session: "sb", Kind: "session.start"})
	id := a.ok("POST", "/api/ask", AskRequest{Session: "sa", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "rm -rf build"}})["id"].(string)
	if items := srv.Hub.Chat("sa"); len(items) != 1 || items[0].Kind != ChatAsk || items[0].Status != "pending" || items[0].Summary != "rm -rf build" || items[0].Ask != id {
		t.Fatalf("a pending ask should be in the chat: %+v", items)
	}
	client{t: t, base: base}.ok("POST", "/api/answer/"+id, map[string]any{"decision": "deny", "reason": "not now"})
	if items := srv.Hub.Chat("sa"); items[0].Status != "deny" || items[0].Text != "not now" || !items[0].Done {
		t.Fatalf("the settled ask: %+v", items[0])
	}
	a.ok("POST", "/api/event", Event{Session: "sa", Kind: "message.send", To: "bob", Text: "hello bob", Delivered: true})
	bb.ok("POST", "/api/event", Event{Session: "sb", Kind: "message.receive", Origin: map[string]any{"kind": "peer"},
		Text: "<cross-session-message from=\"x\" from-name=\"alice\">\nhello bob\n</cross-session-message>"})
	if items := srv.Hub.Chat("sa"); len(items) != 2 || items[1].Kind != ChatMessage || items[1].From != "alice" || items[1].To != "bob" {
		t.Fatalf("alice's chat should show the message she sent: %+v", items)
	}
	if items := srv.Hub.Chat("sb"); len(items) != 1 || items[0].Kind != ChatMessage || items[0].From != "alice" || items[0].Text != "hello bob" {
		t.Fatalf("bob's chat should show the message he got: %+v", items)
	}
	// bob's prompt from the peer isn't a user item on top of the message
	bb.ok("POST", "/api/event", Event{Session: "sb", Kind: "chat.user", Text: "hello bob", Origin: "peer"})
	if items := srv.Hub.Chat("sb"); len(items) != 1 {
		t.Fatalf("a peer's prompt should not double the message: %+v", items)
	}
	// alice's next session (she ended and came back) starts with an empty log; the old one is gone
	a.ok("POST", "/api/event", Event{Session: "sa", Kind: "session.end"})
	a.ok("POST", "/api/event", Event{Session: "sa2", Kind: "session.start"})
	if items := srv.Hub.Chat("sa2"); len(items) != 0 {
		t.Fatalf("a new session starts fresh: %+v", items)
	}
	if items := srv.Hub.Chat("sa"); len(items) != 0 {
		t.Fatalf("the old session's log should be dropped with it: %+v", items)
	}
}
