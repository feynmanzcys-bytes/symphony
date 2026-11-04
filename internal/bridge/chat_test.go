package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	// a mod that acknowledges (1.2.0 on): an item stays in the inbox until a poll names its id or a later one, so a
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
	clock := time.Now()
	srv.Hub.mu.Lock()
	srv.Hub.now = func() time.Time { return clock }
	srv.Hub.mu.Unlock()
	id := a.ok("POST", "/api/ask", AskRequest{Session: "sa", Kind: "permission", Tool: "Bash", Input: map[string]any{"command": "rm -rf build"}})["id"].(string)
	if items := srv.Hub.Chat("sa"); len(items) != 1 || items[0].Kind != ChatAsk || items[0].Status != "pending" || items[0].Summary != "rm -rf build" || items[0].Ask != id {
		t.Fatalf("a pending ask should be in the chat: %+v", items)
	}
	srv.Hub.mu.Lock()
	clock = clock.Add(42 * time.Second) // the person took a while
	srv.Hub.mu.Unlock()
	client{t: t, base: base}.ok("POST", "/api/answer/"+id, map[string]any{"decision": "deny", "reason": "not now"})
	if items := srv.Hub.Chat("sa"); items[0].Status != "deny" || items[0].Text != "not now" || !items[0].Done || items[0].Ms != 42000 {
		t.Fatalf("the settled ask, with how long it waited: %+v", items[0])
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
	// a console that doesn't show the ❯: the input box's content still goes, from its top border on
	noPrompt := "ledger.json 里还留着上次的演示记录\n✻ Worked for 4m 20s · done 18:50\n" + strings.Repeat("─", 100) + " win-g ─\n清空 ledger.json 里的演示记录\n" + strings.Repeat("─", 110) + "\n⏵⏵ auto mode on"
	if got := tellingLines(noPrompt, 4, 400); got != "ledger.json 里还留着上次的演示记录\n✻ Worked for 4m 20s · done 18:50" {
		t.Fatalf("the input box without ❯: %q", got)
	}
	// a screen without an input box (a crash before the prompt drew): a single rule and what follows it go, the
	// lines before stay
	if got := tellingLines("a\nb\n────\nc", 4, 400); got != "a\nb" {
		t.Fatalf("a rule near the end: %q", got)
	}
	if got := tellingLines("error: unknown option --foo\nexit", 4, 400); got != "error: unknown option --foo\nexit" {
		t.Fatalf("no box at all: %q", got)
	}
	if got := tellingLines("error: unknown option --foo\nexit", 4, 400); got != "error: unknown option --foo\nexit" {
		t.Fatalf("plain lines: %q", got)
	}
}

// A page changes a running session's model and thinking effort: /model and /effort go to its inbox as command items,
// each only when asked for and one of the launch dialog's choices; an offline session takes nothing.
func TestSetModel(t *testing.T) {
	srv, base := newBridge(t)
	srv.WaitTimeout = 300 * time.Millisecond
	mod, _ := as(t, srv, base, "a")
	page := client{t: t, base: base}
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "session.start", Cwd: "/w"})

	if st, code := page.failure("POST", "/api/sessions/nope/model", map[string]any{"model": "opus"}); st != http.StatusConflict || code != "session_offline" {
		t.Fatalf("unknown session: %d %s", st, code)
	}
	if st, code := page.failure("POST", "/api/sessions/s1/model", map[string]any{"model": "gpt"}); st != http.StatusBadRequest || code != "launch_bad_option" {
		t.Fatalf("a model that isn't a choice: %d %s", st, code)
	}
	if st, code := page.failure("POST", "/api/sessions/s1/model", map[string]any{"effort": ""}); st != http.StatusBadRequest || code != "launch_bad_option" {
		t.Fatalf("the effort can't be unset: %d %s", st, code)
	}
	if st, code := page.failure("POST", "/api/sessions/s1/model", map[string]any{}); st != http.StatusBadRequest || code != "model_empty" {
		t.Fatalf("nothing to change: %d %s", st, code)
	}
	page.ok("POST", "/api/sessions/s1/model", map[string]any{"model": "opus", "effort": "max"})
	page.ok("POST", "/api/sessions/s1/model", map[string]any{"model": ""}) // Claude Code's own: /model default
	res := mod.ok("GET", "/api/inbox?session=s1", nil)
	items := res["items"].([]any)
	want := [][2]string{{"model", "opus"}, {"effort", "max"}, {"model", "default"}}
	if len(items) != len(want) {
		t.Fatalf("the inbox should hold the three commands: %v", items)
	}
	for i, w := range want {
		it := items[i].(map[string]any)
		if it["kind"] != "command" || it["command"] != w[0] || it["args"] != w[1] {
			t.Fatalf("item %d: %v, want /%s %s", i, it, w[0], w[1])
		}
	}
	// a command the engine wouldn't run is a notice in the chat
	mod.ok("POST", "/api/event", Event{Session: "s1", Kind: "command.failed", Command: "model", Args: "opus", Text: "unknown model"})
	chat := srv.Hub.Chat("s1")
	if last := chat[len(chat)-1]; last.Kind != ChatNotice || last.Text != "commandFailed" || last.Summary != "/model opus: unknown model" {
		t.Fatalf("the failure should be a notice: %+v", last)
	}
}

// An image sent into a conversation: uploaded first (kept in the session's folder, .symphony/inbox, served back by its
// id, png/jpeg/gif/webp only, 10 MiB at most), then named in a say: the session gets the file's path after the text,
// the chat shows the text with the image, and the mod's echo of the sent text settles the bubble.
func TestSayImages(t *testing.T) {
	srv, page, hub, _ := staffBridge(t)
	srv.WaitTimeout = 300 * time.Millisecond
	acc, _ := hub.CreateAccount("ann")
	other, _ := hub.CreateAccount("bob")
	cwd := t.TempDir()
	mod := client{t: t, base: page.base, header: map[string]string{"authorization": "Bearer " + acc.Token}, modOnly: true}
	mod.ok("POST", "/api/event", map[string]any{"session": "s1", "kind": "session.start", "cwd": cwd})
	modB := client{t: t, base: page.base, header: map[string]string{"authorization": "Bearer " + other.Token}, modOnly: true}
	modB.ok("POST", "/api/event", map[string]any{"session": "s2", "kind": "session.start", "cwd": t.TempDir()})

	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 64)...)
	upload := func(session string, body []byte, typ string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", page.base+"/api/files/"+session, bytes.NewReader(body))
		req.Header.Set("Content-Type", typ)
		req.Header.Set("X-Symphony-Name", "shot.png")
		req.Header.Set(keyHeader, testKey)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}
	if st, out := upload("s1", []byte("hello, not an image"), "text/plain"); st != http.StatusBadRequest || out["code"] != "file_not_image" {
		t.Fatalf("text isn't an image: %d %v", st, out)
	}
	if st, out := upload("s1", append(png, make([]byte, fileMax)...), "image/png"); st != http.StatusRequestEntityTooLarge || out["code"] != "file_too_big" {
		t.Fatalf("over the cap: %d %v", st, out)
	}
	if st, out := upload("nope", png, "image/png"); st != http.StatusConflict || out["code"] != "session_offline" {
		t.Fatalf("an unknown session: %d %v", st, out)
	}
	st, saved := upload("s1", png, "image/png")
	id, _ := saved["id"].(string)
	if st != 200 || !strings.HasSuffix(id, ".png") || saved["url"] != "/api/files/s1/"+id || saved["name"] != "shot.png" {
		t.Fatalf("the upload should answer the file's id and url: %d %v", st, saved)
	}
	if _, err := os.Stat(filepath.Join(cwd, ".symphony", "inbox", id)); err != nil {
		t.Fatalf("the file should be in the session's folder: %v", err)
	}
	// served back to the page, as an image; not under another session, nor by a name that isn't an id
	resp, err := pageGet(page.base + "/api/files/s1/" + id)
	if err != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("serving the file: %v %v", err, resp)
	}
	resp.Body.Close()
	for _, path := range []string{"/api/files/s2/" + id, "/api/files/s1/../accounts.json", "/api/files/s1/x.png"} {
		if resp, err := pageGet(page.base + path); err == nil && resp.StatusCode == 200 {
			t.Fatalf("%s should not be served", path)
		}
	}
	// the say: the chat item shows the text with the image, the session gets the path
	if st, code := page.failure("POST", "/api/say/s1", map[string]any{"text": "look", "images": []string{"nope.png"}}); st != http.StatusNotFound || code != "file_unknown" {
		t.Fatalf("a file that isn't there: %d %s", st, code)
	}
	it := page.ok("POST", "/api/say/s1", map[string]any{"text": "What is in this screenshot?", "images": []string{id}})
	if it["text"] != "What is in this screenshot?" || it["status"] != "queued" || fmt.Sprint(it["images"]) != "["+saved["url"].(string)+"]" {
		t.Fatalf("the queued item should carry the text and the image: %v", it)
	}
	items := mod.ok("GET", "/api/inbox?session=s1", nil)["items"].([]any)
	sent, _ := items[0].(map[string]any)["text"].(string)
	want := filepath.Join(cwd, ".symphony", "inbox", id)
	if len(items) != 1 || !strings.HasPrefix(sent, "What is in this screenshot?") || !strings.Contains(sent, "Attached image 1 (read it with the Read tool): "+want) {
		t.Fatalf("the session should get the text and the path: %v", items)
	}
	mod.ok("POST", "/api/event", map[string]any{"session": "s1", "kind": "chat.sent", "text": sent})
	chat := hub.Chat("s1")
	if last := chat[len(chat)-1]; last.Status != "sent" || last.Text != "What is in this screenshot?" || len(last.Images) != 1 {
		t.Fatalf("the mod's echo of the sent text should settle the bubble: %+v", last)
	}
	// an image alone is a say too
	page.ok("POST", "/api/say/s1", map[string]any{"text": "", "images": []string{id}})
	if st, code := page.failure("POST", "/api/say/s1", map[string]any{"text": " ", "images": []string{}}); st != http.StatusBadRequest || code != "say_empty" {
		t.Fatalf("nothing at all: %d %s", st, code)
	}
}
