package bridge

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
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
	page := client{t: t, base: base}
	if res := page.ok("GET", "/api/chat/s1", nil); len(res["items"].([]any)) != 5 {
		t.Fatalf("/api/chat: %v", res)
	}
	// a session the hub doesn't know has an empty log, not null
	if res := page.ok("GET", "/api/chat/nope", nil); res["items"] == nil {
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
	// a mod that acknowledges (1.1.2 on): an item stays in the inbox until a poll names its id or a later one, so a
	// poll cut off on its way back loses nothing; the ids count up per session
	page.ok("POST", "/api/say/s1", map[string]any{"text": "again"})
	page.ok("POST", "/api/say/s1", map[string]any{"text": "and again"})
	ids := func(res map[string]any) (out []float64) {
		for _, it := range res["items"].([]any) {
			out = append(out, it.(map[string]any)["id"].(float64))
		}
		return out
	}
	first := ids(mod.ok("GET", "/api/inbox?session=s1&ack=0", nil))
	if len(first) != 2 || first[1] != first[0]+1 {
		t.Fatalf("the first acknowledging poll should get both, with consecutive ids: %v", first)
	}
	// handed out a moment ago: a second poll out at the same time (two mods in one session) gets nothing; one that
	// comes once inboxRehand passed (the poll was cut off) gets them again
	start = time.Now()
	if res := mod.ok("GET", "/api/inbox?session=s1&ack=0", nil); len(res["items"].([]any)) != 0 || time.Since(start) < 250*time.Millisecond {
		t.Fatalf("a second poll right after should wait and get nothing: %v after %v", res, time.Since(start))
	}
	srv.Hub.now = func() time.Time { return time.Now().Add(inboxRehand + time.Second) }
	if got := ids(mod.ok("GET", "/api/inbox?session=s1&ack=0", nil)); !slices.Equal(got, first) {
		t.Fatalf("the poll was cut off: the next one should get them again: %v", got)
	}
	ack := func(id float64) string { return "/api/inbox?session=s1&ack=" + strconv.FormatInt(int64(id), 10) }
	srv.Hub.now = func() time.Time { return time.Now().Add(2 * (inboxRehand + time.Second)) }
	if got := ids(mod.ok("GET", ack(first[0]), nil)); !slices.Equal(got, first[1:]) {
		t.Fatalf("acknowledging the first should leave the second: %v", got)
	}
	start = time.Now()
	if res := mod.ok("GET", ack(first[1]), nil); len(res["items"].([]any)) != 0 || time.Since(start) < 250*time.Millisecond {
		t.Fatalf("acknowledging the second should leave nothing, and wait: %v after %v", res, time.Since(start))
	}
	// a bridge that restarted hands out larger ids than the one before it did (they start at the time), so the ack the
	// mod kept from the earlier bridge drops nothing
	srv2, base2 := newBridge(t)
	mod2, _ := as(t, srv2, base2, "a")
	mod2.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})
	(client{t: t, base: base2}).ok("POST", "/api/say/s1", map[string]any{"text": "after the restart"})
	if got := ids(mod2.ok("GET", ack(first[1]), nil)); len(got) != 1 || got[0] <= first[1] {
		t.Fatalf("a stale ack should drop nothing on a restarted bridge: %v (old ack %v)", got, first[1])
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

// TestTellingLines: the exit notice shows the screen's last lines that tell what the session did: not the input box's
// borders (the employee's name in one or not), not the input or status line, whole lines from the end within the limit.
func TestTellingLines(t *testing.T) {
	screen := "命令行本身还是没有单独的测试。\n✻ Churned for 2m 50s · done 18:16\n" + strings.Repeat("─", 112) + " win-e ─\n❯ 给命令行也加个测试\n" +
		strings.Repeat("─", 120) + "\n ⏵⏵ auto mode on (shift+tab to cycle)"
	if got := tellingLines(screen, 4, 400); got != "命令行本身还是没有单独的测试。\n✻ Churned for 2m 50s · done 18:16" {
		t.Fatalf("got %q", got)
	}
	if got := tellingLines(screen, 1, 400); got != "✻ Churned for 2m 50s · done 18:16" {
		t.Fatalf("the last one: %q", got)
	}
	if got := tellingLines(screen, 4, 40); got != "✻ Churned for 2m 50s · done 18:16" {
		t.Fatalf("within 40 runes, whole lines: %q", got)
	}
	if got := tellingLines("────\n\n❯ x\n⏵⏵ y", 4, 400); got != "" {
		t.Fatalf("borders, input and status only: %q", got)
	}
	if got := tellingLines("error: unknown option --foo\nexit", 4, 400); got != "error: unknown option --foo\nexit" {
		t.Fatalf("plain lines: %q", got)
	}
}
